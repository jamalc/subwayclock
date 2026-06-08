// Package server exposes the HTTP API (/arrivals, /stops, /stops/search,
// /health) over the arrivals engine and static GTFS data. Server-side, native
// only (used by the serve binary).
package server

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jamalc/subwayclock/internal/arrivals"
	"github.com/jamalc/subwayclock/internal/gtfs"
)

//go:embed stops.html
var stopsHTML []byte

// Handler serves the HTTP API over an arrivals engine and the static GTFS data.
type Handler struct {
	engine *arrivals.Engine
	static *gtfs.StaticData
}

// NewHandler returns a Handler backed by the given engine and static data.
func NewHandler(engine *arrivals.Engine, static *gtfs.StaticData) *Handler {
	return &Handler{engine: engine, static: static}
}

// RegisterRoutes registers the API routes on the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /arrivals", h.handleArrivals)
	mux.HandleFunc("GET /stops", h.handleStops)
	mux.HandleFunc("GET /stops/search", h.handleStopSearch)
	mux.HandleFunc("GET /health", h.handleHealth)
}

// handleArrivals serves GET /arrivals
//
// Query params:
//
//	stop=<stopID>[:<token>[,<token>...]]   (repeatable)
//	split=headsign                          (optional) split groups by headsign
//
// A stop's tokens tune which routes show. A bare route is a pin (always shown,
// with "No service" when not arriving); "!route" is a mute (never shown). With
// no tokens, every arriving route is shown.
//
// Examples:
//
//	?stop=A27N                  — every route arriving at Franklin
//	?stop=A27N:C                — all arrivals, plus "No service" for C when down
//	?stop=A27N:C,!A             — pin C (No service if absent), hide A, show rest
//	?stop=A27N&split=headsign   — separate group per headsign
func (h *Handler) handleArrivals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	rawStops := q["stop"]
	if len(rawStops) == 0 {
		http.Error(w, `{"error":"at least one stop= param required"}`, http.StatusBadRequest)
		return
	}

	stops := make([]arrivals.StopRequest, len(rawStops))
	for i, raw := range rawStops {
		stops[i] = parseStopParam(raw)
	}

	splitByHeadsign := q.Get("split") == "headsign"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.engine.GetArrivals(stops, splitByHeadsign))
}

// handleStops serves GET /stops — the embedded HTML stop lookup page.
func (h *Handler) handleStops(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(stopsHTML)
}

// handleStopSearch serves GET /stops/search?q=franklin
func (h *Handler) handleStopSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.static.SearchStops(q))
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// parseStopParam parses "A27N:Q,!R" into a StopRequest. The part after ":" is
// a comma-separated list of route tokens: a bare route is a pin (always
// shown), "!route" is a mute (never shown). A route given as both is treated
// as muted. The token portion is optional; "635N" with no colon shows every
// arriving route.
func parseStopParam(raw string) arrivals.StopRequest {
	parts := strings.SplitN(raw, ":", 2)
	sr := arrivals.StopRequest{
		StopID: parts[0],
		Pins:   make(map[string]bool),
		Mutes:  make(map[string]bool),
	}
	if len(parts) == 2 {
		for _, tok := range strings.Split(parts[1], ",") {
			if tok = strings.TrimSpace(tok); tok == "" {
				continue
			}
			if strings.HasPrefix(tok, "!") {
				if r := strings.ToUpper(strings.TrimSpace(tok[1:])); r != "" {
					sr.Mutes[r] = true
				}
			} else {
				sr.Pins[strings.ToUpper(tok)] = true
			}
		}
		// Mute wins when a route is listed as both.
		for r := range sr.Mutes {
			delete(sr.Pins, r)
		}
	}
	return sr
}
