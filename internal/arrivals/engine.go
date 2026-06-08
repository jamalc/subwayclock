// Package arrivals is the serve backend's engine.
//
// It combines static GTFS data with the realtime feeds into per-stop arrival
// groups.
package arrivals

import (
	"fmt"
	"sort"
	"strings"
	"time"

	gtfsrt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"

	"github.com/jamalc/subwayclock/internal/api"
	"github.com/jamalc/subwayclock/internal/gtfs"
	"github.com/jamalc/subwayclock/internal/realtime"
)

// maxETAs is the maximum number of ETAs to show per route/headsign group.
const maxETAs = 4

// StopRequest is a single stop with optional per-stop route pins and mutes,
// parsed from query params like "A27N:Q,!R". Bare routes are pins (always
// shown, with "No service" when not arriving); "!route" entries are mutes
// (never shown, even when arriving). A route listed as both is treated as
// muted.
type StopRequest struct {
	StopID string
	Pins   map[string]bool // always shown; "No service" when not arriving
	Mutes  map[string]bool // never shown, even when arriving
}

// entitySource supplies realtime feed entities. *realtime.Poller satisfies it;
// tests provide a fake.
type entitySource interface {
	AllEntities() []*gtfsrt.FeedEntity
}

// Engine combines static and realtime data into API-friendly arrival groups.
type Engine struct {
	static *gtfs.StaticData
	poller entitySource
}

// NewEngine returns an Engine backed by the given static data and poller.
func NewEngine(static *gtfs.StaticData, poller *realtime.Poller) *Engine {
	return &Engine{static: static, poller: poller}
}

// tripETA pairs an ETA in seconds with the headsign of that specific trip.
type tripETA struct {
	eta      int
	headsign string
}

// groupKey identifies a group of arrivals. In default mode it's just the
// route; with split=headsign it includes the headsign too.
type groupKey struct {
	Route    string
	Headsign string // empty in default (route-only) mode
}

// GetArrivals returns one StopArrivals per requested stop, each stop's Groups
// sorted by route.
//
// Realtime-first: by default every arriving route at a requested stop is shown.
// A stop's optional filter tunes this — a pinned route is always shown, with a
// "No service" row when it isn't arriving; a muted route is never shown, even
// when arriving. If splitByHeadsign is true, arriving routes split into one row
// per headsign.
func (e *Engine) GetArrivals(stops []StopRequest, splitByHeadsign bool) []api.StopArrivals {
	now := time.Now().Unix()

	stopIndex := make(map[string]StopRequest, len(stops))
	for _, s := range stops {
		stopIndex[s.StopID] = s
	}

	// stopID -> groupKey -> []tripETA, excluding muted routes.
	index := make(map[string]map[groupKey][]tripETA)
	for _, entity := range e.poller.AllEntities() {
		tu := entity.TripUpdate
		if tu == nil {
			continue
		}
		routeID := strings.ToUpper(tu.Trip.GetRouteId())
		tripID := tu.Trip.GetTripId()

		for _, stu := range tu.StopTimeUpdate {
			stopID := stu.GetStopId()
			sr, wanted := stopIndex[stopID]
			if !wanted {
				continue
			}
			if sr.Mutes[routeID] {
				continue
			}

			var eta int64
			if stu.Arrival != nil && stu.Arrival.Time != nil {
				eta = *stu.Arrival.Time - now
			} else if stu.Departure != nil && stu.Departure.Time != nil {
				eta = *stu.Departure.Time - now
			} else {
				continue
			}
			if eta < 0 || eta > 3600 {
				continue
			}

			headsign := e.static.GetHeadsign(tripID)
			key := groupKey{Route: routeID}
			if splitByHeadsign {
				key.Headsign = headsign
			}
			if index[stopID] == nil {
				index[stopID] = make(map[groupKey][]tripETA)
			}
			index[stopID][key] = append(index[stopID][key], tripETA{int(eta), headsign})
		}
	}

	result := make([]api.StopArrivals, 0, len(stops))
	for _, sr := range stops {
		result = append(result, api.StopArrivals{
			StopID:   sr.StopID,
			StopName: e.static.GetStopName(sr.StopID),
			Groups:   buildGroups(index[sr.StopID], sr.StopID, sr.Pins, e.static),
		})
	}
	return result
}

// buildGroups turns the per-stop arrival index into display rows, then appends
// a "No service" row (empty ETAs) for every pinned route with no arrivals.
// pins is the stop's set of always-show routes.
func buildGroups(raw map[groupKey][]tripETA, stopID string, pins map[string]bool, static *gtfs.StaticData) []api.ArrivalGroup {
	directionFallback := "Northbound"
	if len(stopID) > 0 && stopID[len(stopID)-1] == 'S' {
		directionFallback = "Southbound"
	}

	var groups []api.ArrivalGroup
	arriving := make(map[string]bool)

	for key, etas := range raw {
		arriving[key.Route] = true
		sort.Slice(etas, func(i, j int) bool { return etas[i].eta < etas[j].eta })

		headsign := key.Headsign // non-empty only in split mode
		if headsign == "" {
			for _, t := range etas {
				if t.headsign != "" {
					headsign = t.headsign
					break
				}
			}
		}
		if headsign == "" {
			headsign = directionFallback
		}

		if len(etas) > maxETAs {
			etas = etas[:maxETAs]
		}
		formatted := make([]string, len(etas))
		for i, t := range etas {
			formatted[i] = fmtETA(t.eta)
		}

		route, _ := static.GetRoute(key.Route)
		groups = append(groups, api.ArrivalGroup{
			Route:     key.Route,
			Color:     route.Color,
			RouteName: route.LongName,
			Headsign:  headsign,
			ETAs:      formatted,
		})
	}

	// "No service" rows for pinned routes with nothing arriving.
	absent := make([]string, 0, len(pins))
	for r := range pins {
		if !arriving[r] {
			absent = append(absent, r)
		}
	}
	sort.Strings(absent)
	for _, r := range absent {
		route, _ := static.GetRoute(r)
		groups = append(groups, api.ArrivalGroup{
			Route:     r,
			Color:     route.Color,
			RouteName: route.LongName,
			Headsign:  directionFallback,
			ETAs:      nil, // empty = No service
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Route != groups[j].Route {
			return groups[i].Route < groups[j].Route
		}
		return groups[i].Headsign < groups[j].Headsign
	})
	return groups
}

// fmtETA formats an ETA in seconds as a display string such as "5m" or "Now".
// It rounds to the nearest minute, with anything under 30s counting as "Now".
func fmtETA(seconds int) string {
	mins := (seconds + 30) / 60
	if mins == 0 {
		return "Now"
	}
	return fmt.Sprintf("%dm", mins)
}
