package arrivals

import (
	"testing"
	"time"

	"github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"

	"github.com/jamalc/subwayclock/internal/api"
	gtfsstatic "github.com/jamalc/subwayclock/internal/gtfs"
)

// apiGroup keeps the assertions readable.
type apiGroup = api.ArrivalGroup

// fakeSource is a test entitySource.
type fakeSource struct{ entities []*gtfs.FeedEntity }

func (f fakeSource) AllEntities() []*gtfs.FeedEntity { return f.entities }

// arrival builds a FeedEntity for one route/trip arriving at stopID in `mins`.
func arrival(routeID, tripID, stopID string, mins int) *gtfs.FeedEntity {
	t := time.Now().Unix() + int64(mins*60)
	return &gtfs.FeedEntity{
		TripUpdate: &gtfs.TripUpdate{
			Trip: &gtfs.TripDescriptor{
				RouteId: proto.String(routeID),
				TripId:  proto.String(tripID),
			},
			StopTimeUpdate: []*gtfs.TripUpdate_StopTimeUpdate{{
				StopId:  proto.String(stopID),
				Arrival: &gtfs.TripUpdate_StopTimeEvent{Time: proto.Int64(t)},
			}},
		},
	}
}

// staticFixture returns static data with route metadata for A, C, Q, R.
func staticFixture() *gtfsstatic.StaticData {
	s := gtfsstatic.NewStaticData()
	s.Stops["A27"] = "Franklin Av"
	s.Routes["A"] = gtfsstatic.Route{ID: "A", LongName: "8 Avenue", Color: "0039A6"}
	s.Routes["C"] = gtfsstatic.Route{ID: "C", LongName: "8 Avenue Local", Color: "0039A6"}
	s.Routes["Q"] = gtfsstatic.Route{ID: "Q", LongName: "2 Av", Color: "FCCC0A"}
	s.Routes["R"] = gtfsstatic.Route{ID: "R", LongName: "4 Av Local", Color: "FCCC0A"}
	return s
}

// byRoute indexes one stop's Groups by route for assertions.
func byRoute(rows []apiGroup) map[string]apiGroup {
	m := make(map[string]apiGroup, len(rows))
	for _, r := range rows {
		m[r.Route] = r
	}
	return m
}

// req builds a StopRequest with the given pins and mutes (mute wins on overlap,
// mirroring parseStopParam).
func req(id string, pins, mutes []string) StopRequest {
	sr := StopRequest{StopID: id, Pins: map[string]bool{}, Mutes: map[string]bool{}}
	for _, p := range pins {
		sr.Pins[p] = true
	}
	for _, m := range mutes {
		sr.Mutes[m] = true
		delete(sr.Pins, m)
	}
	return sr
}

func newTestEngine(s *gtfsstatic.StaticData, ents ...*gtfs.FeedEntity) *Engine {
	return &Engine{static: s, poller: fakeSource{ents}}
}

func groups(e *Engine, sr StopRequest) map[string]apiGroup {
	return byRoute(e.GetArrivals([]StopRequest{sr}, false)[0].Groups)
}

func TestGetArrivals_NoFilterShowsAllArrivals(t *testing.T) {
	e := newTestEngine(staticFixture(), arrival("A", "t1", "A27N", 3), arrival("C", "t2", "A27N", 7))
	got := groups(e, req("A27N", nil, nil))
	if len(got) != 2 || len(got["A"].ETAs) == 0 || len(got["C"].ETAs) == 0 {
		t.Errorf("no filter should show all arriving routes; got %+v", got)
	}
}

func TestGetArrivals_PinNoServiceWhenAbsent(t *testing.T) {
	// Pin Q at A27; R is arriving but Q is not -> R arrival + Q "No service".
	e := newTestEngine(staticFixture(), arrival("R", "t1", "A27N", 4))
	got := groups(e, req("A27N", []string{"Q"}, nil))
	if r := got["R"]; len(r.ETAs) == 0 {
		t.Errorf("R should be a live arrival; got %+v", r)
	}
	if q, ok := got["Q"]; !ok || len(q.ETAs) != 0 {
		t.Errorf("Q should be a no-service row (empty ETAs); got %+v ok=%v", q, ok)
	}
}

func TestGetArrivals_PinArrivingNotDuplicated(t *testing.T) {
	// Pin Q and Q is arriving -> one live Q row, no extra no-service row.
	e := newTestEngine(staticFixture(), arrival("Q", "t1", "A27N", 2))
	rows := e.GetArrivals([]StopRequest{req("A27N", []string{"Q"}, nil)}, false)[0].Groups
	if len(rows) != 1 || rows[0].Route != "Q" || len(rows[0].ETAs) == 0 {
		t.Errorf("pinned arriving Q should be a single live row; got %+v", rows)
	}
}

func TestGetArrivals_MuteSuppressesArriving(t *testing.T) {
	// Mute R; both Q and R arriving -> only Q.
	e := newTestEngine(staticFixture(), arrival("Q", "t1", "A27N", 3), arrival("R", "t2", "A27N", 5))
	got := groups(e, req("A27N", nil, []string{"R"}))
	if _, ok := got["R"]; ok {
		t.Errorf("muted R must not appear")
	}
	if _, ok := got["Q"]; !ok {
		t.Errorf("Q should still show")
	}
}

func TestGetArrivals_MuteWinsOverPin(t *testing.T) {
	// Q is both pinned and muted -> mute wins: no Q row even though it's absent.
	e := newTestEngine(staticFixture(), arrival("R", "t1", "A27N", 4))
	got := groups(e, req("A27N", []string{"Q"}, []string{"Q"}))
	if _, ok := got["Q"]; ok {
		t.Errorf("Q is muted; should not appear as a no-service row")
	}
}
