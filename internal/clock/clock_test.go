package clock

import (
	"testing"

	"github.com/jamalc/subwayclock/internal/input"
)

func TestNavigate(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
		button input.Button
		n      int
		slots  int
		want   int
	}{
		{"down advances a page", 0, input.Down, 5, 2, 2},
		{"down from last page wraps to start", 4, input.Down, 5, 2, 0},
		{"down wraps cleanly when evenly divided", 2, input.Down, 4, 2, 0},
		{"up goes back a page", 2, input.Up, 5, 2, 0},
		{"up from first page wraps to short last page", 0, input.Up, 5, 2, 4},
		{"up from first page wraps to last full page", 0, input.Up, 4, 2, 2},
		{"single slot steps one group", 0, input.Down, 3, 1, 1},
		{"single slot up wraps to end", 0, input.Up, 3, 1, 2},
		{"none leaves cursor unchanged", 2, input.None, 5, 2, 2},
		{"no groups stays at zero", 0, input.Down, 0, 2, 0},
		{"zero slots stays at zero", 2, input.Down, 5, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := navigate(tt.cursor, tt.button, tt.n, tt.slots); got != tt.want {
				t.Errorf("navigate(%d, %v, %d, %d) = %d, want %d", tt.cursor, tt.button, tt.n, tt.slots, got, tt.want)
			}
		})
	}
}
