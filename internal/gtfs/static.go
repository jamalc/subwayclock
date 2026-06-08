// Package gtfs loads and queries the static GTFS feeds (routes, stops,
// headsigns, and a stop→routes index) from the MTA zip files. Server-side,
// native only (used by the serve binary).
package gtfs

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

// Route is a subway route: its ID, short and long names, and colors.
type Route struct {
	LongName  string
	ID        string
	ShortName string
	Color     string
	TextColor string
}

// StaticData holds the parsed static GTFS data needed to serve arrivals:
// routes, stop names, headsigns, and a stop→routes index. It is safe for
// concurrent use by the HTTP handlers and the realtime poller.
type StaticData struct {
	mu sync.RWMutex

	Routes     map[string]Route
	Stops      map[string]string
	StopRoutes map[string][]string // base stop_id -> sorted route IDs

	// Headsigns keyed by normalized trip_id. Supplemented feed takes precedence.
	suppHeadsigns    map[string]string
	regularHeadsigns map[string]string
}

// NewStaticData returns a StaticData with empty maps. Call Load to populate it
// from the GTFS zip files.
func NewStaticData() *StaticData {
	return &StaticData{
		Routes:           make(map[string]Route),
		Stops:            make(map[string]string),
		StopRoutes:       make(map[string][]string),
		suppHeadsigns:    make(map[string]string),
		regularHeadsigns: make(map[string]string),
	}
}

// Load reads both zip files from disk and parses only what's needed.
// The zip bytes and raw CSV rows are not retained after Load returns.
func (s *StaticData) Load(regularPath, supplementedPath string) error {
	regularZip, err := openZip(regularPath)
	if err != nil {
		return fmt.Errorf("regular feed: %w", err)
	}

	routes, err := parseRoutes(regularZip)
	if err != nil {
		return fmt.Errorf("regular/routes: %w", err)
	}
	stops, err := parseStops(regularZip)
	if err != nil {
		return fmt.Errorf("regular/stops: %w", err)
	}

	// trips.txt gives us headsigns and the trip->route mapping for the
	// stop->routes index.
	regularHeadsigns, tripRoutes, err := parseTrips(regularZip)
	if err != nil {
		return fmt.Errorf("regular/trips: %w", err)
	}

	// stop_times.txt is streamed only to build the stop->routes index.
	stopRoutes, err := parseStopRoutes(regularZip, tripRoutes)
	if err != nil {
		return fmt.Errorf("regular/stop_times: %w", err)
	}

	suppZip, err := openZip(supplementedPath)
	if err != nil {
		return fmt.Errorf("supplemented feed: %w", err)
	}
	suppHeadsigns, _, err := parseTrips(suppZip)
	if err != nil {
		return fmt.Errorf("supplemented/trips: %w", err)
	}

	s.mu.Lock()
	s.Routes = routes
	s.Stops = stops
	s.StopRoutes = stopRoutes
	s.regularHeadsigns = regularHeadsigns
	s.suppHeadsigns = suppHeadsigns
	s.mu.Unlock()
	return nil
}

// GetRoute returns the Route for a given route ID, or false if not found.
func (s *StaticData) GetRoute(id string) (Route, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.Routes[id]
	return r, ok
}

// GetStopName returns the stop name for a stop ID. If the exact ID is missing,
// it retries with any trailing N/S direction suffix stripped (so "123N" falls
// back to the "123" parent station); failing that, it returns id unchanged.
func (s *StaticData) GetStopName(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if name, ok := s.Stops[id]; ok {
		return name
	}
	if len(id) > 1 {
		if name, ok := s.Stops[id[:len(id)-1]]; ok {
			return name
		}
	}
	return id
}

// GetHeadsign returns the headsign for a trip ID, such as "96 St" or
// "Far Rockaway". The supplemented feed takes precedence; falls back to the
// regular feed.
func (s *StaticData) GetHeadsign(tripID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := normalizeTripID(tripID)
	if h, ok := s.suppHeadsigns[key]; ok {
		return h
	}
	return s.regularHeadsigns[key]
}

// normalizeTripID strips the service/schedule prefix from a static GTFS trip ID
// so it matches the shorter form used in the realtime feed.
//
//	"AFA23GEN-1037-Weekday-00_072150_A..N55R" -> "072150_A..N55R"
//	"072150_A..N55R" -> "072150_A..N55R" (already normalized, unchanged)
func normalizeTripID(id string) string {
	parts := strings.Split(id, "_")
	for i, p := range parts {
		if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
			return strings.Join(parts[i:], "_")
		}
	}
	return id
}

// ── parsers ───────────────────────────────────────────────────────────────────

// parseRoutes reads routes.txt and returns a map of route_id -> Route.
func parseRoutes(zr *zip.Reader) (map[string]Route, error) {
	rows, err := readAllCSV(zr, "routes.txt")
	if err != nil {
		return nil, err
	}
	m := make(map[string]Route, len(rows))
	for _, r := range rows {
		m[r["route_id"]] = Route{
			ID:        r["route_id"],
			ShortName: r["route_short_name"],
			LongName:  r["route_long_name"],
			Color:     r["route_color"],
			TextColor: r["route_text_color"],
		}
	}
	return m, nil
}

// parseStops reads stops.txt and returns a map of stop_id -> stop_name.
func parseStops(zr *zip.Reader) (map[string]string, error) {
	rows, err := readAllCSV(zr, "stops.txt")
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r["stop_id"]] = r["stop_name"]
	}
	return m, nil
}

// parseTrips reads trips.txt and returns:
//   - headsigns: normalized trip_id -> trip_headsign
//   - tripRoutes: trip_id -> route_id (used to build stop->routes index)
func parseTrips(zr *zip.Reader) (headsigns map[string]string, tripRoutes map[string]string, err error) {
	rows, err := readAllCSV(zr, "trips.txt")
	if err != nil {
		return nil, nil, err
	}
	headsigns = make(map[string]string, len(rows))
	tripRoutes = make(map[string]string, len(rows))
	for _, r := range rows {
		key := normalizeTripID(r["trip_id"])
		headsigns[key] = r["trip_headsign"]
		tripRoutes[r["trip_id"]] = r["route_id"]
	}
	return headsigns, tripRoutes, nil
}

// parseStopRoutes streams stop_times.txt to build a base stop_id -> sorted
// route IDs index, using tripRoutes to resolve trip_id -> route_id.
func parseStopRoutes(zr *zip.Reader, tripRoutes map[string]string) (map[string][]string, error) {
	f, err := fileFromZip(zr, "stop_times.txt")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return nil, err
	}

	tripCol, stopCol := -1, -1
	for i, h := range headers {
		switch strings.TrimSpace(h) {
		case "trip_id":
			tripCol = i
		case "stop_id":
			stopCol = i
		}
	}
	if tripCol < 0 || stopCol < 0 {
		return nil, fmt.Errorf("stop_times.txt missing trip_id or stop_id column")
	}

	routeStops := make(map[string]map[string]bool)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		routeID, ok := tripRoutes[rec[tripCol]]
		if !ok {
			continue
		}

		// Strip N/S suffix to get the base stop ID.
		stopID := rec[stopCol]
		baseID := stopID
		if len(baseID) > 0 {
			last := baseID[len(baseID)-1]
			if last == 'N' || last == 'S' {
				baseID = baseID[:len(baseID)-1]
			}
		}

		if routeStops[baseID] == nil {
			routeStops[baseID] = make(map[string]bool)
		}
		routeStops[baseID][strings.ToUpper(routeID)] = true
	}

	stopRoutes := make(map[string][]string, len(routeStops))
	for stopID, routeSet := range routeStops {
		routes := make([]string, 0, len(routeSet))
		for r := range routeSet {
			routes = append(routes, r)
		}
		sort.Strings(routes)
		stopRoutes[stopID] = routes
	}
	return stopRoutes, nil
}

// ── zip helpers ───────────────────────────────────────────────────────────────

// openZip reads a zip file into memory and returns a zip.Reader over it.
func openZip(path string) (*zip.Reader, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return zip.NewReader(bytes.NewReader(b), int64(len(b)))
}

// fileFromZip opens the named file within the zip, or returns an error if it
// is not present.
func fileFromZip(zr *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("%q not found in zip", name)
}

// readAllCSV loads an entire CSV from the zip into memory.
// Fine for small files (routes.txt, stops.txt, trips.txt).
// Use streaming for stop_times.txt.
func readAllCSV(zr *zip.Reader, name string) ([]map[string]string, error) {
	f, err := fileFromZip(zr, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return nil, err
	}
	for i, h := range headers {
		headers[i] = strings.TrimSpace(h)
	}

	var rows []map[string]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row := make(map[string]string, len(headers))
		for i, h := range headers {
			row[h] = rec[i]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// ── stop search ───────────────────────────────────────────────────────────────

// StopResult is a stop returned by SearchStops: its ID, name, and routes.
type StopResult struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Routes []string `json:"routes"`
}

// SearchStops returns stops whose name contains q (case-insensitive).
// Only parent stations are returned (IDs without an N/S suffix).
func (s *StaticData) SearchStops(q string) []StopResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q = strings.ToLower(q)
	var results []StopResult
	for id, name := range s.Stops {
		if len(id) > 0 {
			last := id[len(id)-1]
			if last == 'N' || last == 'S' {
				continue
			}
		}
		if strings.Contains(strings.ToLower(name), q) {
			results = append(results, StopResult{
				ID:     id,
				Name:   name,
				Routes: s.StopRoutes[id],
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results
}
