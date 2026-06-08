// Package conway runs Conway's Game of Life on a HUB75-style display.
//
// The simulation is portable: it drives any Display (the on-device
// hub75.Device or the host simulator.Display) and sizes its grid from the
// configured dimensions, so the same logic runs on hardware and in the
// Ebiten simulator.
package conway

import (
	"image/color"
	"math/rand"
	"time"

	"github.com/jamalc/subwayclock/internal/config"
)

// Display is the subset of a panel the simulation drives. Both
// hub75.Device and simulator.Display satisfy it.
type Display interface {
	SetPixel(x, y int16, c color.RGBA)
	Display() error
	Clear()
}

// game holds the simulation state for a w×h toroidal grid. The grid,
// next, and age buffers are flat slices indexed as y*w + x.
type game struct {
	w, h int
	grid []bool
	next []bool
	age  []uint8
}

// newGame creates a new game with the given dimensions and allocates buffers.
func newGame(w, h int) *game {
	return &game{
		w:    w,
		h:    h,
		grid: make([]bool, w*h),
		next: make([]bool, w*h),
		age:  make([]uint8, w*h),
	}
}

// neighbors counts the living neighbors of (x, y) using toroidal
// wrap-around. The (x + dx + w) % w trick handles negative indices.
func (g *game) neighbors(x, y int) int {
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := (x + dx + g.w) % g.w
			ny := (y + dy + g.h) % g.h
			if g.grid[ny*g.w+nx] {
				count++
			}
		}
	}
	return count
}

// step advances the simulation by one generation, then ages cells:
// survivors age up (saturating at 255), dead cells reset to 0.
func (g *game) step() {
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			n := g.neighbors(x, y)
			alive := g.grid[y*g.w+x]
			g.next[y*g.w+x] = (alive && (n == 2 || n == 3)) || (!alive && n == 3)
		}
	}
	g.grid, g.next = g.next, g.grid
	for i, alive := range g.grid {
		if alive {
			if g.age[i] < 255 {
				g.age[i]++
			}
		} else {
			g.age[i] = 0
		}
	}
}

// ageColor maps a cell's age to its display color. Young cells are
// bright white, then fade through yellow / orange / red / dim red as
// they age. Dead cells are off.
func ageColor(a uint8) color.RGBA {
	switch {
	case a == 0:
		return color.RGBA{0, 0, 0, 255}
	case a == 1:
		return color.RGBA{255, 255, 255, 255} // newborn: bright white
	case a < 4:
		return color.RGBA{255, 255, 80, 255} // young: yellow
	case a < 8:
		return color.RGBA{255, 160, 0, 255} // teenage: orange
	case a < 16:
		return color.RGBA{200, 40, 0, 255} // mature: red
	case a < 32:
		return color.RGBA{120, 20, 0, 255} // old: dim red
	default:
		return color.RGBA{60, 8, 0, 255} // ancient: very dim red
	}
}

// render draws the current grid to the display, coloring each cell by age via
// ageColor.
func (g *game) render(d Display) {
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			d.SetPixel(int16(x), int16(y), ageColor(g.age[y*g.w+x]))
		}
	}
}

// seed populates the grid with random life at the given density (0-100).
func (g *game) seed(density int) {
	for i := range g.grid {
		if rand.Intn(100) < density {
			g.grid[i] = true
			g.age[i] = 1
		} else {
			g.grid[i] = false
			g.age[i] = 0
		}
	}
}

// sprinkle adds a few random cells without erasing what's there.
func (g *game) sprinkle(count int) {
	for i := 0; i < count; i++ {
		idx := rand.Intn(g.w * g.h)
		g.grid[idx] = true
		if g.age[idx] == 0 {
			g.age[idx] = 1
		}
	}
}

// countAlive returns the number of living cells in the grid.
func (g *game) countAlive() int {
	c := 0
	for _, a := range g.grid {
		if a {
			c++
		}
	}
	return c
}

// abs returns the absolute value of n. Used for boredom detection in Run.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Run sizes a grid from cfg, seeds it, then animates Conway's Game of Life
// forever at ~8 generations per second. It sprinkles new cells when the
// population stagnates and reseeds if it crashes.
func Run(cfg config.Config, d Display) {
	g := newGame(cfg.Width, cfg.Height)

	d.Clear()
	g.seed(35)
	g.render(d)
	d.Display()

	stagnantGens := 0
	lastAliveCount := 0

	for {
		g.step()
		g.render(d)
		d.Display()

		// Boredom detection: if alive count hasn't changed much, sprinkle.
		alive := g.countAlive()
		if abs(alive-lastAliveCount) < 10 {
			stagnantGens++
		} else {
			stagnantGens = 0
		}
		lastAliveCount = alive

		if stagnantGens > 30 {
			g.sprinkle(40)
			stagnantGens = 0
		}

		// If population crashed, reseed.
		if alive < 20 {
			g.seed(35)
		}

		// Pace: ~8 generations per second.
		time.Sleep(time.Millisecond * 125)
	}
}
