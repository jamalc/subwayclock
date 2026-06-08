// Package gravity runs an accelerometer-driven particle field on a HUB75-style
// display. It exists to stress the hub75 driver's double-buffer swap and
// refresh timing: it redraws every pixel and requests a buffer swap every
// frame, the worst case for a driver that front-loads work into SetPixel.
//
// The simulation is portable — it drives any Display and reads gravity through
// a Tilt source — so the same logic runs on the on-device hub75.Device and in
// the host Ebiten simulator.
package gravity

import (
	"image/color"
	"math/rand"
	"time"

	"github.com/jamalc/subwayclock/internal/config"
)

// Display is the subset of a panel the simulation drives. Both hub75.Device
// and simulator.Display satisfy it (the same set conway uses).
type Display interface {
	SetPixel(x, y int16, c color.RGBA)
	Display() error
	Clear()
}

// Tilt is the gravity source in screen space: +x is right, +y is down,
// magnitude in units of g (1.0 ≈ one gravity). A level board reads ~(0, 0).
type Tilt interface {
	Vector() (x, y float32)
}

// Control is the live stress-tuning input. StressStep reports +1 to raise the
// active particle count, -1 to lower it, and 0 otherwise.
type Control interface {
	StressStep() int
}

// Tunable constants. dt is a fixed simulation step (decoupled from real frame
// rate so physics stays stable regardless of refresh speed).
const (
	frameDT               = 1.0 / 60.0
	gravityGain           = 30.0 // px/s² per g
	restitution           = 0.6  // velocity retained across a wall bounce
	trailDecay            = 0.82 // per-frame trail brightness multiplier
	defaultDensityDivisor = 8    // initial particle count = w*h / this
	stressStepSize        = 16   // particles added/removed per Control step
)

// particle is one moving point with a fixed color.
type particle struct {
	x, y, vx, vy float32
	r, g, b      uint8
}

// field holds the particle set and the owned trail framebuffer. Indexing is
// y*w + x. Physics constants are fields (not raw consts) so tests can set them.
type field struct {
	w, h        int
	parts       []particle
	trail       []color.RGBA
	decay       float32
	restitution float32
	gain        float32
	dt          float32
}

// newField allocates a field sized w×h with the default particle count.
func newField(w, h int) *field {
	f := &field{
		w: w, h: h,
		trail:       make([]color.RGBA, w*h),
		decay:       trailDecay,
		restitution: restitution,
		gain:        gravityGain,
		dt:          frameDT,
	}
	f.adjustCount(w * h / defaultDensityDivisor)
	return f
}

// spawn returns a new particle at a random position with a random bright color.
func (f *field) spawn() particle {
	return particle{
		x: rand.Float32() * float32(f.w),
		y: rand.Float32() * float32(f.h),
		r: uint8(40 + rand.Intn(216)),
		g: uint8(40 + rand.Intn(216)),
		b: uint8(40 + rand.Intn(216)),
	}
}

// adjustCount grows or shrinks the active particle count by delta, clamped to
// [0, w*h].
func (f *field) adjustCount(delta int) {
	target := len(f.parts) + delta
	if target < 0 {
		target = 0
	}
	if max := f.w * f.h; target > max {
		target = max
	}
	for len(f.parts) < target {
		f.parts = append(f.parts, f.spawn())
	}
	f.parts = f.parts[:target]
}

// step advances the simulation one frame: integrate particles under the given
// gravity vector (units of g), bounce off walls with damping, fade the trail
// buffer, then stamp particles onto it.
func (f *field) step(gx, gy float32) {
	ax := gx * f.gain
	ay := gy * f.gain

	for i := range f.parts {
		p := &f.parts[i]
		p.vx += ax * f.dt
		p.vy += ay * f.dt
		p.x += p.vx * f.dt
		p.y += p.vy * f.dt

		if p.x < 0 {
			p.x, p.vx = 0, -p.vx*f.restitution
		} else if p.x > float32(f.w-1) {
			p.x, p.vx = float32(f.w-1), -p.vx*f.restitution
		}
		if p.y < 0 {
			p.y, p.vy = 0, -p.vy*f.restitution
		} else if p.y > float32(f.h-1) {
			p.y, p.vy = float32(f.h-1), -p.vy*f.restitution
		}
	}

	for i := range f.trail {
		c := &f.trail[i]
		c.R = uint8(float32(c.R) * f.decay)
		c.G = uint8(float32(c.G) * f.decay)
		c.B = uint8(float32(c.B) * f.decay)
		if c.R == 0 && c.G == 0 && c.B == 0 {
			c.A = 0
		}
	}

	for i := range f.parts {
		p := &f.parts[i]
		xi := clamp(int(p.x+0.5), 0, f.w-1)
		yi := clamp(int(p.y+0.5), 0, f.h-1)
		f.trail[yi*f.w+xi] = color.RGBA{p.r, p.g, p.b, 255}
	}
}

// renderTo blits every trail pixel to the display.
func (f *field) renderTo(d Display) {
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			d.SetPixel(int16(x), int16(y), f.trail[y*f.w+x])
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Run animates the particle field forever. It is intentionally thin glue over
// the tested field type, so it is verified by build, not unit tests.
func Run(cfg config.Config, d Display, t Tilt, c Control) {
	f := newField(cfg.Width, cfg.Height)
	d.Clear()
	runLoop(f, d, t, c)
}

// runLoop drives one field forever: apply any stress step, read gravity, step
// physics, blit, swap. There is no artificial pacing — on device, the
// double-buffered Display() blocks until the ISR completes a swap, which paces
// the loop to the panel's refresh; in the simulator Display() returns
// immediately and the loop runs free. Effective frames/sec is printed ~1×/sec
// (USB serial on device, stderr in the simulator) as the stress readout.
func runLoop(f *field, d Display, t Tilt, c Control) {
	frames := 0
	last := time.Now()
	for {
		if s := c.StressStep(); s != 0 {
			f.adjustCount(s * stressStepSize)
		}
		gx, gy := t.Vector()
		f.step(gx, gy)
		f.renderTo(d)
		d.Display()

		frames++
		if now := time.Now(); now.Sub(last) >= time.Second {
			println("gravity:", len(f.parts), "particles,", frames, "fps")
			frames = 0
			last = now
		}
	}
}
