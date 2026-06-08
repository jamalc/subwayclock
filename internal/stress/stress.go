// Package stress drives worst-case test patterns at a HUB75 panel to
// verify shift-register data timing and signal integrity.
//
// The patterns target several distinct failure mechanisms:
//
//   - Data setup time: the RGB lines must be stable before each clock edge
//     (see clockRow in the hub75 package). Stressed by
//     pixel-to-pixel transitions along the shift direction (x). Sparse apps
//     like conway and clock barely exercise it; the stripe/checker/noise
//     patterns flip the data lines on every clock instead.
//   - Simultaneous switching: the shift loop pre-clears the six RGB lines to 0
//     each pixel, so a solid-white field rises all six together on every clock
//     — the worst ground-bounce case (the "full white" pattern).
//   - Bit-plane (BCM) timing: dim values exercise only the low-order planes,
//     which the full-brightness patterns never touch ("dim green stripes").
//   - Row addressing: adjacent scan rows differing maximally stresses address
//     settling and OE blanking ("horizontal stripes", "half split").
//   - Visual color correctness: SMPTE-order bars make a wrong channel obvious
//     to the eye ("color bars").
package stress

import (
	"image/color"
	"time"

	"github.com/jamalc/subwayclock/internal/input"
)

var (
	black   = color.RGBA{0, 0, 0, 255}
	white   = color.RGBA{255, 255, 255, 255}
	red     = color.RGBA{255, 0, 0, 255}
	green   = color.RGBA{0, 255, 0, 255}
	blue    = color.RGBA{0, 0, 255, 255}
	yellow  = color.RGBA{255, 255, 0, 255}
	cyan    = color.RGBA{0, 255, 255, 255}
	magenta = color.RGBA{255, 0, 255, 255}
)

// dimLevel is the 8-bit green value for the dim stripe. It looks high for
// "dim", but the driver right-shifts it into the panel's native depth and then
// applies a steep ratio²·√ratio gamma curve: at the default 6-bit depth every
// 8-bit input below 40 collapses to native 0 (a blank panel). 72 is the lowest
// round value that survives to a low-but-visible native level (3 of 63, lighting
// only bit-planes 0 and 1 — the shortest, most timing-sensitive OE windows).
const dimLevel = 72

// Display is the subset of a panel these patterns drive. The on-device
// hub75.Device satisfies it.
type Display interface {
	SetPixel(x, y int16, c color.RGBA)
	Display() error
	Clear()
}

// Pattern is a named full-screen test pattern. At reports the color of pixel
// (x, y) on the given animation frame; static patterns ignore frame.
type Pattern struct {
	Name string
	At   func(x, y, frame int) color.RGBA
}

// verticalStripe returns on for even columns and black for odd columns,
// producing 1-pixel-wide vertical stripes. Because the panel shifts pixels out
// along x, this toggles the RGB data lines on every clock — the maximum
// setup-time pressure.
func verticalStripe(on color.RGBA, x int) color.RGBA {
	if x%2 == 0 {
		return on
	}
	return black
}

// horizontalStripe returns on for even rows and black for odd rows, making
// adjacent scan rows differ to stress row-address settling and OE blanking.
func horizontalStripe(on color.RGBA, y int) color.RGBA {
	if y%2 == 0 {
		return on
	}
	return black
}

// checker returns on when x+y is even and black otherwise, toggling the data
// lines on both axes.
func checker(on color.RGBA, x, y int) color.RGBA {
	if (x+y)%2 == 0 {
		return on
	}
	return black
}

// scrollStripe is verticalStripe with its phase advanced by frame, so every
// column position experiences both transitions over time — no column gets a
// permanently "lucky" stable sample.
func scrollStripe(on color.RGBA, x, frame int) color.RGBA {
	return verticalStripe(on, x+frame)
}

// whiteMagenta holds R and B high on every column and toggles only green
// (white vs magenta), isolating the green line as the lone variable on an
// otherwise fully loaded, switching bus.
func whiteMagenta(x int) color.RGBA {
	if x%2 == 0 {
		return white
	}
	return magenta
}

// dimGreenStripe is a green stripe at dimLevel, so corruption shows up in the
// low-order bit-planes that full-brightness patterns never exercise.
func dimGreenStripe(x int) color.RGBA {
	return verticalStripe(color.RGBA{0, dimLevel, 0, 255}, x)
}

// halfSplit lights the top sub-panel (y < h/2) white and leaves the bottom
// black, driving the r1g1b1 group hard while r2g2b2 stays idle.
func halfSplit(y, h int) color.RGBA {
	if y < h/2 {
		return white
	}
	return black
}

// noise is a deterministic per-pixel hash whose channels are each fully on or
// off. Re-evaluated every frame it acts as a fuzzer, hitting aggressor
// combinations the structured patterns never produce — without carrying any
// RNG state across the portable/device boundary.
func noise(x, y, frame int) color.RGBA {
	h := uint32(x)*73856093 ^ uint32(y)*19349663 ^ uint32(frame)*83492791
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	ch := func(bit uint) uint8 {
		if h&(1<<bit) != 0 {
			return 255
		}
		return 0
	}
	return color.RGBA{ch(0), ch(1), ch(2), 255}
}

// scaleColor dims c uniformly to the given level (0 = off, 255 = full),
// preserving its hue. Each channel scales independently, so zero channels stay
// zero.
func scaleColor(c color.RGBA, level uint8) color.RGBA {
	s := func(v uint8) uint8 { return uint8(uint16(v) * uint16(level) / 255) }
	return color.RGBA{s(c.R), s(c.G), s(c.B), 255}
}

// ramp scales base from off (at x=0) to full intensity (at x=w-1) across the
// panel width, sweeping every brightness level. This exercises the full set of
// BCM bit-planes and exposes gamma banding — coverage the all-on and low-plane
// patterns miss. Only base's non-zero channels light, so a colored base stays
// that color.
//
// The dim end sits below the panel's representable floor and reads as black
// (the steep gamma curve crushes the lowest inputs to native 0); how many
// columns that covers depends on the configured bit depth. The ramp stays
// linear in 8-bit space so it makes no bit-depth assumption.
func ramp(base color.RGBA, x, w int) color.RGBA {
	if w <= 1 {
		return base
	}
	return scaleColor(base, uint8(uint16(x)*255/uint16(w-1)))
}

// rgbRamps stacks a red, green, and blue ramp as three equal horizontal bands,
// so each channel's full intensity range — gamma, banding, plane timing — can
// be compared side by side on one screen.
func rgbRamps(x, y, w, h int) color.RGBA {
	switch y * 3 / h {
	case 0:
		return ramp(red, x, w)
	case 1:
		return ramp(green, x, w)
	default:
		return ramp(blue, x, w)
	}
}

// wheel maps a position around the hue circle (0..255) to a fully saturated
// color using integer math: red at 0, green at 85, blue at 170. Exactly one
// channel is off at every position.
func wheel(pos uint8) color.RGBA {
	switch {
	case pos < 85:
		return color.RGBA{255 - pos*3, pos * 3, 0, 255}
	case pos < 170:
		pos -= 85
		return color.RGBA{0, 255 - pos*3, pos * 3, 255}
	default:
		pos -= 170
		return color.RGBA{pos * 3, 0, 255 - pos*3, 255}
	}
}

// rainbow is a 2D color-space sweep: hue runs across x and brightness down y
// (full at the top, fading to black at the bottom edge). It covers far more of
// the color space than any single gradient and is the best at surfacing channel
// mixing and crosstalk.
func rainbow(x, y, w, h int) color.RGBA {
	base := wheel(uint8(x * 256 / w))
	if h <= 1 {
		return base
	}
	return scaleColor(base, uint8(255-uint16(y)*255/uint16(h-1)))
}

// barColors is the SMPTE-order color bar sequence, left to right by descending
// luminance.
var barColors = []color.RGBA{white, yellow, cyan, green, magenta, red, blue}

// colorBars splits a w-wide panel into equal vertical bands of barColors. A
// wrong channel is instantly visible because every bar's color is known.
func colorBars(x, w int) color.RGBA {
	bw := w / len(barColors)
	if bw < 1 {
		bw = 1
	}
	i := x / bw
	if i >= len(barColors) {
		i = len(barColors) - 1
	}
	return barColors[i]
}

// buildPatterns returns the full pattern sequence for a w×h panel, ordered from
// most diagnostic (isolated channels and transitions) toward visual references
// and baselines. Patterns that depend on panel size or animation frame capture
// w, h here.
func buildPatterns(w, h int) []Pattern {
	return []Pattern{
		{"rainbow", func(x, y, f int) color.RGBA { return rainbow(x, y, w, h) }},
		{"color bars", func(x, y, f int) color.RGBA { return colorBars(x, w) }},
		{"noise", func(x, y, f int) color.RGBA { return noise(x, y, f) }},
		{"red stripes", func(x, y, f int) color.RGBA { return verticalStripe(red, x) }},
		{"green stripes", func(x, y, f int) color.RGBA { return verticalStripe(green, x) }},
		{"blue stripes", func(x, y, f int) color.RGBA { return verticalStripe(blue, x) }},
		{"dim green stripes", func(x, y, f int) color.RGBA { return dimGreenStripe(x) }},
		{"green gradient", func(x, y, f int) color.RGBA { return ramp(green, x, w) }},
		{"rgb ramps", func(x, y, f int) color.RGBA { return rgbRamps(x, y, w, h) }},
		{"green loaded", func(x, y, f int) color.RGBA { return whiteMagenta(x) }},
		{"white stripes", func(x, y, f int) color.RGBA { return verticalStripe(white, x) }},
		{"scroll stripes", func(x, y, f int) color.RGBA { return scrollStripe(white, x, f) }},
		{"horizontal stripes", func(x, y, f int) color.RGBA { return horizontalStripe(white, y) }},
		{"white checker", func(x, y, f int) color.RGBA { return checker(white, x, y) }},
		{"full white", func(x, y, f int) color.RGBA { return white }},
		{"half split", func(x, y, f int) color.RGBA { return halfSplit(y, h) }},
	}
}

// frameInterval paces animated patterns (scroll, noise). Static patterns
// redraw identically each tick, which is harmless.
const frameInterval = 50 * time.Millisecond

// step maps a navigation button to a sequencer direction: Up advances to the
// next pattern, Down goes back to the previous one, and None holds.
func step(b input.Button) int {
	switch b {
	case input.Up:
		return +1
	case input.Down:
		return -1
	default:
		return 0
	}
}

// wrapIndex reduces i into [0, n) with wrap-around in both directions, so
// stepping past either end lands on the opposite end.
func wrapIndex(i, n int) int {
	i %= n
	if i < 0 {
		i += n
	}
	return i
}

// sequencer decides which pattern is on screen. Each frame it advances one
// step toward the auto-advance deadline (holdFrames), unless a manual step
// moves it immediately — which also resets the deadline so the chosen pattern
// gets a full hold window.
type sequencer struct {
	n          int
	holdFrames int
	idx        int
	elapsed    int
}

func newSequencer(n, holdFrames int) *sequencer {
	if holdFrames < 1 {
		holdFrames = 1
	}
	return &sequencer{n: n, holdFrames: holdFrames}
}

// tick advances one frame. step is manual navigation (+1/-1/0). It returns the
// pattern index to display and whether the pattern just changed, so the caller
// can reset the animation frame counter.
func (s *sequencer) tick(step int) (idx int, changed bool) {
	s.elapsed++
	switch {
	case step != 0:
		s.idx = wrapIndex(s.idx+step, s.n)
		s.elapsed = 0
		return s.idx, true
	case s.elapsed >= s.holdFrames:
		s.idx = wrapIndex(s.idx+1, s.n)
		s.elapsed = 0
		return s.idx, true
	}
	return s.idx, false
}

// Run draws each pattern across the whole w×h display, animating it for hold
// before auto-advancing, and cycles forever. in (if non-nil) is polled each
// frame so the user can step through patterns manually: Up = next pattern,
// Down = previous. Watch for per-pixel color corruption (classically the green
// channel dropping or smearing) — that's a timing margin failing.
func Run(d Display, w, h int, hold time.Duration, in input.Source) {
	patterns := buildPatterns(w, h)
	seq := newSequencer(len(patterns), int(hold/frameInterval))
	frame := 0
	for {
		p := patterns[seq.idx]
		d.Clear()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				d.SetPixel(int16(x), int16(y), p.At(x, y, frame))
			}
		}
		d.Display()
		time.Sleep(frameInterval)

		s := 0
		if in != nil {
			s = step(in.Poll())
		}
		if _, changed := seq.tick(s); changed {
			frame = 0
		} else {
			frame++
		}
	}
}
