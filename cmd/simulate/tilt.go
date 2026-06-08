package main

import (
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// tiltPad adapts the keyboard to gravity.Tilt + gravity.Control for the
// simulator. Arrow keys set the gravity direction (held = tilted); '='/'+'
// and '-' step the particle count. Ebiten input must be read from the Update
// loop, so capture() (called there) snapshots held direction into atomics and
// pushes count steps onto a buffered channel for the Run goroutine to read.
type tiltPad struct {
	dir   atomic.Int64 // packed direction: high 32 bits = gx, low 32 bits = gy
	steps chan int
}

func newTiltPad() *tiltPad {
	return &tiltPad{steps: make(chan int, 8)}
}

// capture reads key state. Call once per Ebiten Update tick.
func (p *tiltPad) capture() {
	var x, y int32
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		x--
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		x++
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		y--
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		y++
	}
	p.dir.Store(int64(x)<<32 | int64(uint32(y)))

	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		p.push(+1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		p.push(-1)
	}
}

func (p *tiltPad) push(n int) {
	select {
	case p.steps <- n:
	default: // buffer full: drop
	}
}

// Vector implements gravity.Tilt. Arrow keys yield a perfectly axis-aligned
// vector (e.g. down = (0, 1)); held steadily, the particle field flattens onto
// the wall it's pulled toward, since the model has no inter-particle forces and
// nothing to move particles along that wall. This is expected — it's also what
// the real panel does under a steady tilt; the device only looks lively because
// a hand never holds a tilt perfectly still. Don't "fix" it with tilt jitter:
// measured, a symmetric wobble (even 0.5g) never lifts particles off the wall,
// so it animates the line but can't stop the collapse. Only a per-particle
// thermal kick in gravity.step or true inter-particle forces would.
func (p *tiltPad) Vector() (x, y float32) {
	v := p.dir.Load()
	return float32(int32(v >> 32)), float32(int32(v))
}

// StressStep implements gravity.Control.
func (p *tiltPad) StressStep() int {
	select {
	case n := <-p.steps:
		return n
	default:
		return 0
	}
}
