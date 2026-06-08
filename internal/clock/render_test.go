package clock

import "testing"

func TestEtaText(t *testing.T) {
	if got := etaText(nil, 100); got != "No service" {
		t.Errorf("empty ETAs: got %q, want %q", got, "No service")
	}
	if got := etaText([]string{}, 100); got != "No service" {
		t.Errorf("zero-len ETAs: got %q, want %q", got, "No service")
	}
	if got := etaText([]string{"3m"}, 100); got != "3m" {
		t.Errorf("with ETAs: got %q, want %q", got, "3m")
	}
}
