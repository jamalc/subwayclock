package stress

import (
	"image/color"
	"math"
	"testing"

	"github.com/jamalc/subwayclock/internal/input"
)

// nativeLevel models the hub75 driver's SetPixel pipeline: it right-shifts an
// 8-bit channel into the configured bit depth, then applies the same
// ratio²·√ratio gamma curve (see hub75.buildGammaTable). Dim values must
// survive this to light anything — at 6-bit depth the curve maps every 8-bit
// input below 40 to native 0, i.e. a blank panel.
func nativeLevel(v8 uint8, bitDepth int) uint8 {
	size := 1 << bitDepth
	maxVal := float64(size - 1)
	ratio := float64(int(v8)>>(8-bitDepth)) / maxVal
	out := int(ratio*ratio*math.Sqrt(ratio)*maxVal + 0.5)
	if out > size-1 {
		out = size - 1
	}
	return uint8(out)
}

func TestVerticalStripeTogglesEveryColumn(t *testing.T) {
	on := color.RGBA{0, 255, 0, 255}
	if got := verticalStripe(on, 0); got != on {
		t.Errorf("column 0: got %v, want %v (on)", got, on)
	}
	if got := verticalStripe(on, 1); got != black {
		t.Errorf("column 1: got %v, want %v (black)", got, black)
	}
	if got := verticalStripe(on, 2); got != on {
		t.Errorf("column 2: got %v, want %v (on)", got, on)
	}
}

func TestCheckerTogglesOnBothAxes(t *testing.T) {
	on := color.RGBA{255, 255, 255, 255}
	cases := []struct {
		x, y int
		want color.RGBA
	}{
		{0, 0, on},
		{1, 0, black},
		{0, 1, black},
		{1, 1, on},
	}
	for _, c := range cases {
		if got := checker(on, c.x, c.y); got != c.want {
			t.Errorf("checker(%d,%d): got %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestHorizontalStripeTogglesEveryRow(t *testing.T) {
	on := color.RGBA{255, 255, 255, 255}
	if got := horizontalStripe(on, 0); got != on {
		t.Errorf("row 0: got %v, want on", got)
	}
	if got := horizontalStripe(on, 1); got != black {
		t.Errorf("row 1: got %v, want black", got)
	}
}

// scrollStripe is verticalStripe with the phase advanced by frame, so a given
// column flips parity from one frame to the next.
func TestScrollStripeShiftsPhaseWithFrame(t *testing.T) {
	on := color.RGBA{255, 255, 255, 255}
	if got := scrollStripe(on, 0, 0); got != on {
		t.Errorf("col 0 frame 0: got %v, want on", got)
	}
	if got := scrollStripe(on, 0, 1); got != black {
		t.Errorf("col 0 frame 1: got %v, want black (phase shifted)", got)
	}
}

// whiteMagenta holds R and B high and toggles only green, so green is the lone
// variable on an otherwise fully loaded bus.
func TestWhiteMagentaTogglesOnlyGreen(t *testing.T) {
	even := whiteMagenta(0)
	odd := whiteMagenta(1)
	if even != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("even column: got %v, want white", even)
	}
	if odd != (color.RGBA{255, 0, 255, 255}) {
		t.Errorf("odd column: got %v, want magenta", odd)
	}
	if even.R != odd.R || even.B != odd.B {
		t.Errorf("R/B must stay loaded across columns: even=%v odd=%v", even, odd)
	}
}

// dimGreenStripe must be dim green only — and, crucially, dim enough to sit in
// the low bit-planes yet bright enough to survive the driver's shift+gamma to a
// nonzero native level. A value that gamma-crushes to native 0 leaves the panel
// blank, which is the bug this guards against.
func TestDimGreenStripeStaysVisibleInLowPlanes(t *testing.T) {
	c := dimGreenStripe(0)
	if c.R != 0 || c.B != 0 {
		t.Errorf("dim green leaked into R/B: %v", c)
	}
	if dimGreenStripe(1) != black {
		t.Errorf("odd column should be black, got %v", dimGreenStripe(1))
	}
	n := nativeLevel(c.G, 6) // cmd runs at the default 6-bit depth
	if n == 0 {
		t.Fatalf("dim green %d maps to native level 0 at 6-bit depth: blank panel", c.G)
	}
	if n > 7 {
		t.Errorf("dim green native level %d not in the low planes (want 1..7)", n)
	}
}

func TestHalfSplitTopBright(t *testing.T) {
	h := 4
	for y := 0; y < 2; y++ {
		if halfSplit(y, h) == black {
			t.Errorf("top half row %d should be lit", y)
		}
	}
	for y := 2; y < 4; y++ {
		if halfSplit(y, h) != black {
			t.Errorf("bottom half row %d should be black", y)
		}
	}
}

// noise is a deterministic per-pixel hash whose channels are full-on or
// full-off, so it can be a fuzzer without carrying RNG state.
func TestNoiseIsDeterministicBinaryAndVaries(t *testing.T) {
	if noise(3, 4, 5) != noise(3, 4, 5) {
		t.Error("noise must be deterministic for the same inputs")
	}
	seen := map[color.RGBA]bool{}
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			c := noise(x, y, 0)
			for _, ch := range []uint8{c.R, c.G, c.B} {
				if ch != 0 && ch != 255 {
					t.Fatalf("noise channel not binary at (%d,%d): %v", x, y, c)
				}
			}
			seen[c] = true
		}
	}
	if len(seen) < 2 {
		t.Error("noise produced only one color across 64 pixels")
	}
}

func TestColorBarsAreStandardOrder(t *testing.T) {
	want := []color.RGBA{
		{255, 255, 255, 255}, // white
		{255, 255, 0, 255},   // yellow
		{0, 255, 255, 255},   // cyan
		{0, 255, 0, 255},     // green
		{255, 0, 255, 255},   // magenta
		{255, 0, 0, 255},     // red
		{0, 0, 255, 255},     // blue
	}
	// One bar per column when width == bar count.
	for x, w := range want {
		if got := colorBars(x, len(want)); got != w {
			t.Errorf("bar %d: got %v, want %v", x, got, w)
		}
	}
	// Wider panel: last column is the final bar.
	if got := colorBars(63, 64); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("rightmost bar on 64-wide: got %v, want blue", got)
	}
}

// The green-channel stripe is the prime suspect for the setup-time bug, so it
// must isolate green: only the G component is ever non-zero.
func TestGreenStripePatternIsolatesGreenChannel(t *testing.T) {
	var p Pattern
	for _, pat := range buildPatterns(8, 8) {
		if pat.Name == "green stripes" {
			p = pat
		}
	}
	if p.At == nil {
		t.Fatal("no \"green stripes\" pattern found")
	}
	for x := 0; x < 8; x++ {
		c := p.At(x, 0, 0)
		if c.R != 0 || c.B != 0 {
			t.Errorf("green stripes at x=%d leaked into R/B: %v", x, c)
		}
	}
}

func TestRampSpansFullRange(t *testing.T) {
	if got := ramp(white, 0, 256); got != black {
		t.Errorf("x=0: got %v, want black (level 0)", got)
	}
	if got := ramp(white, 255, 256); got != white {
		t.Errorf("x=max: got %v, want white (full level)", got)
	}
	if got := ramp(white, 128, 256); got.R != 128 {
		t.Errorf("x=128: got R=%d, want 128 (mid level)", got.R)
	}
	if ramp(white, 50, 256).R >= ramp(white, 200, 256).R {
		t.Error("ramp must increase with x")
	}
}

// A colored base ramps only its own channels, so the gradient stays that color.
func TestRampKeepsBaseColor(t *testing.T) {
	for x := 0; x < 256; x++ {
		c := ramp(green, x, 256)
		if c.R != 0 || c.B != 0 {
			t.Fatalf("green ramp leaked into R/B at x=%d: %v", x, c)
		}
	}
	if got := ramp(green, 255, 256); got != green {
		t.Errorf("green ramp at full: got %v, want green", got)
	}
}

// wheel walks the full hue circle with integer math: red at 0, green at 85,
// blue at 170, and exactly one channel off at every step (full saturation).
func TestWheelHitsPrimariesAndStaysSaturated(t *testing.T) {
	if wheel(0) != red {
		t.Errorf("wheel(0) = %v, want red", wheel(0))
	}
	if wheel(85) != green {
		t.Errorf("wheel(85) = %v, want green", wheel(85))
	}
	if wheel(170) != blue {
		t.Errorf("wheel(170) = %v, want blue", wheel(170))
	}
	for pos := 0; pos < 256; pos++ {
		c := wheel(uint8(pos))
		if c.R != 0 && c.G != 0 && c.B != 0 {
			t.Fatalf("wheel(%d) = %v is not fully saturated (no channel off)", pos, c)
		}
	}
}

// rgbRamps stacks a red, green, and blue ramp as three horizontal bands so each
// channel's full intensity range can be compared side by side.
func TestRgbRampsStacksPerChannelBands(t *testing.T) {
	w, h := 256, 3
	if got := rgbRamps(255, 0, w, h); got != red {
		t.Errorf("top band full: got %v, want red", got)
	}
	if got := rgbRamps(255, 1, w, h); got != green {
		t.Errorf("middle band full: got %v, want green", got)
	}
	if got := rgbRamps(255, 2, w, h); got != blue {
		t.Errorf("bottom band full: got %v, want blue", got)
	}
	if got := rgbRamps(0, 0, w, h); got != black {
		t.Errorf("left edge: got %v, want black (level 0)", got)
	}
}

// rainbow is a 2D field: hue across x, brightness down y (bright at top, off at
// the bottom edge).
func TestRainbowIsHueByBrightness(t *testing.T) {
	w, h := 256, 32
	if got := rainbow(0, 0, w, h); got != red {
		t.Errorf("top-left (hue 0, full bright): got %v, want red", got)
	}
	if got := rainbow(10, h-1, w, h); got != black {
		t.Errorf("bottom edge should fade to black: got %v", got)
	}
	if rainbow(0, 0, w, h) == rainbow(w/2, 0, w, h) {
		t.Error("hue must change across x")
	}
}

func TestWrapIndex(t *testing.T) {
	cases := []struct{ i, n, want int }{
		{0, 3, 0},
		{1, 3, 1},
		{3, 3, 0},  // off the top wraps to start
		{-1, 3, 2}, // off the bottom wraps to end
		{-3, 3, 0},
		{4, 3, 1},
	}
	for _, c := range cases {
		if got := wrapIndex(c.i, c.n); got != c.want {
			t.Errorf("wrapIndex(%d, %d) = %d, want %d", c.i, c.n, got, c.want)
		}
	}
}

func TestSequencerAutoAdvancesAfterHold(t *testing.T) {
	s := newSequencer(3, 2) // 3 patterns, advance every 2 frames

	if idx, changed := s.tick(0); idx != 0 || changed {
		t.Errorf("frame 1: idx=%d changed=%v, want 0,false", idx, changed)
	}
	if idx, changed := s.tick(0); idx != 1 || !changed {
		t.Errorf("frame 2: idx=%d changed=%v, want 1,true (auto-advance)", idx, changed)
	}
}

func TestSequencerManualStepOverridesHold(t *testing.T) {
	s := newSequencer(3, 100) // hold long enough that auto-advance won't fire

	if idx, changed := s.tick(+1); idx != 1 || !changed {
		t.Errorf("up: idx=%d changed=%v, want 1,true", idx, changed)
	}
	if idx, changed := s.tick(-1); idx != 0 || !changed {
		t.Errorf("down: idx=%d changed=%v, want 0,true", idx, changed)
	}
	if idx, changed := s.tick(-1); idx != 2 || !changed {
		t.Errorf("down wraps: idx=%d changed=%v, want 2,true", idx, changed)
	}
}

// A manual step must reset the auto-advance timer, so the held pattern gets a
// full hold window after the user navigates.
func TestSequencerManualStepResetsHoldTimer(t *testing.T) {
	s := newSequencer(3, 2)
	s.tick(0)  // elapsed = 1
	s.tick(+1) // manual jump to idx 1, elapsed reset to 0
	if idx, changed := s.tick(0); idx != 1 || changed {
		t.Errorf("frame after manual step: idx=%d changed=%v, want 1,false", idx, changed)
	}
	if idx, changed := s.tick(0); idx != 2 || !changed {
		t.Errorf("auto-advance should fire 2 frames after step: idx=%d changed=%v, want 2,true", idx, changed)
	}
}

func TestStepMapsButtonToDirection(t *testing.T) {
	tests := []struct {
		name string
		b    input.Button
		want int
	}{
		{"up advances", input.Up, +1},
		{"down goes back", input.Down, -1},
		{"none holds", input.None, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := step(tt.b); got != tt.want {
				t.Errorf("step(%v) = %d, want %d", tt.b, got, tt.want)
			}
		})
	}
}
