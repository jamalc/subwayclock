package server

import "testing"

func TestParseStopParam(t *testing.T) {
	sr := parseStopParam("A27N:Q,!R,!N")
	if sr.StopID != "A27N" {
		t.Errorf("stopID: got %q", sr.StopID)
	}
	if len(sr.Pins) != 1 || !sr.Pins["Q"] {
		t.Errorf("pins: got %v, want {Q}", sr.Pins)
	}
	if len(sr.Mutes) != 2 || !sr.Mutes["R"] || !sr.Mutes["N"] {
		t.Errorf("mutes: got %v, want {R,N}", sr.Mutes)
	}

	// Tokens are upper-cased and mute wins when a route is both.
	both := parseStopParam("r30n:q,!q")
	if len(both.Pins) != 0 || !both.Mutes["Q"] {
		t.Errorf("mute should win: pins=%v mutes=%v", both.Pins, both.Mutes)
	}

	// No colon means no tokens (every arriving route shows).
	bare := parseStopParam("635N")
	if len(bare.Pins) != 0 || len(bare.Mutes) != 0 {
		t.Errorf("bare stop should have no tokens; got pins=%v mutes=%v", bare.Pins, bare.Mutes)
	}
}
