package conway

import "testing"

// set marks the given (x,y) cells alive on a fresh w×h game.
func gameWith(w, h int, cells [][2]int) *game {
	g := newGame(w, h)
	for _, c := range cells {
		g.grid[c[1]*w+c[0]] = true
		g.age[c[1]*w+c[0]] = 1
	}
	return g
}

func alive(g *game) [][2]int {
	var out [][2]int
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			if g.grid[y*g.w+x] {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

func sameCells(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[[2]int]bool{}
	for _, c := range a {
		seen[c] = true
	}
	for _, c := range b {
		if !seen[c] {
			return false
		}
	}
	return true
}

// A blinker (3 in a row) oscillates with period 2.
func TestStepBlinker(t *testing.T) {
	g := gameWith(5, 5, [][2]int{{1, 2}, {2, 2}, {3, 2}})
	g.step()
	want := [][2]int{{2, 1}, {2, 2}, {2, 3}}
	if got := alive(g); !sameCells(got, want) {
		t.Fatalf("after 1 step: got %v, want %v", got, want)
	}
	g.step()
	want = [][2]int{{1, 2}, {2, 2}, {3, 2}}
	if got := alive(g); !sameCells(got, want) {
		t.Fatalf("after 2 steps: got %v, want %v", got, want)
	}
}

// A 2×2 block is a still life: unchanged after a step.
func TestStepBlock(t *testing.T) {
	cells := [][2]int{{1, 1}, {2, 1}, {1, 2}, {2, 2}}
	g := gameWith(5, 5, cells)
	g.step()
	if got := alive(g); !sameCells(got, cells) {
		t.Fatalf("block changed: got %v, want %v", got, cells)
	}
}

// Neighbor counting wraps toroidally: a cell at a corner sees the
// opposite corner/edges as neighbors.
func TestNeighborsWrap(t *testing.T) {
	g := gameWith(3, 3, [][2]int{{2, 2}, {0, 0}})
	// (0,0) neighbors include (2,2) via wrap.
	if n := g.neighbors(0, 0); n != 1 {
		t.Fatalf("neighbors(0,0) = %d, want 1", n)
	}
}

// step ages surviving cells and resets dead ones to 0.
func TestStepAging(t *testing.T) {
	g := gameWith(5, 5, [][2]int{{1, 1}, {2, 1}, {1, 2}, {2, 2}})
	g.step() // block survives
	if g.age[1*g.w+1] != 2 {
		t.Fatalf("survivor age = %d, want 2", g.age[1*g.w+1])
	}
	g2 := gameWith(5, 5, [][2]int{{0, 0}}) // lone cell dies
	g2.step()
	if g2.age[0] != 0 {
		t.Fatalf("dead cell age = %d, want 0", g2.age[0])
	}
}
