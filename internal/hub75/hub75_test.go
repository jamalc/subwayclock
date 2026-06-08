//go:build !tinygo

package hub75

import (
	"image/color"
	"math"
	"runtime"
	"testing"
)

// testPins returns a Pins struct using pins 0–13, all in port group 0
// (values < 32), so New() won't panic with ErrMixedPortGroups.
func testPins() Pins {
	return Pins{
		R1: Pin(0), G1: Pin(1), B1: Pin(2),
		R2: Pin(3), G2: Pin(4), B2: Pin(5),
		Address: []Pin{6, 7, 8, 9, 10},
		Clock:   Pin(11),
		Latch:   Pin(12),
		OE:      Pin(13),
	}
}

// setup creates and configures a device. Calls t.Cleanup to reset
// package state after the test.
func setup(t *testing.T, cfg Config) *Device {
	t.Helper()
	resetForTesting()
	t.Cleanup(resetForTesting)
	d := New(testPins())
	if err := d.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return d
}

// rgba is shorthand for constructing a color.RGBA.
func rgba(r, g, b, a uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: a} }

// bufIdx returns the flat frame-buffer index for a given (plane, row, x)
// under the smallCfg configuration (rows=2, width=8).
func bufIdx(plane, row, x int) int { return plane*2*8 + row*8 + x }

// smallCfg is a compact configuration used for pixel-buffer inspection.
var smallCfg = Config{Width: 8, Height: 4, BitDepth: 3}

// tickCfg is a minimal configuration for driving the state machine by hand.
// rows=2, bitDepth=2 → full frame = 4 ticks.
var tickCfg = Config{Width: 4, Height: 4, BitDepth: 2}

// ============================================================
// Pure helpers
// ============================================================

func TestBuildGammaTable_Endpoints(t *testing.T) {
	for _, depth := range []int{1, 2, 3, 4, 5, 6} {
		table := buildGammaTable(depth)
		maxIdx := len(table) - 1
		if table[0] != 0 {
			t.Errorf("depth=%d: table[0]=%d, want 0", depth, table[0])
		}
		if int(table[maxIdx]) != maxIdx {
			t.Errorf("depth=%d: table[%d]=%d, want %d", depth, maxIdx, table[maxIdx], maxIdx)
		}
	}
}

func TestBuildGammaTable_Monotonic(t *testing.T) {
	for _, depth := range []int{1, 2, 3, 4, 5, 6} {
		table := buildGammaTable(depth)
		for i := 1; i < len(table); i++ {
			if table[i] < table[i-1] {
				t.Errorf("depth=%d: not monotonic at i=%d: table[%d]=%d > table[%d]=%d",
					depth, i, i-1, table[i-1], i, table[i])
			}
		}
	}
}

func TestBuildGammaTable_DarkerThanLinear(t *testing.T) {
	// Gamma curve bends downward: midpoint output should be less than
	// midpoint input.
	table := buildGammaTable(6)
	mid := len(table) / 2
	if int(table[mid]) >= mid {
		t.Errorf("gamma not darker than linear: table[%d]=%d, want < %d", mid, table[mid], mid)
	}
}

func TestBuildPlaneOnTimes_MinFloor(t *testing.T) {
	for _, depth := range []int{1, 2, 3, 4, 5, 6} {
		times := buildPlaneOnTimes(depth)
		for p, v := range times {
			if v < planeMinTicks {
				t.Errorf("depth=%d plane=%d: on-time %d < floor %d", depth, p, v, planeMinTicks)
			}
		}
	}
}

func TestBuildPlaneOnTimes_DoublingRatio(t *testing.T) {
	// The 2× ratio only holds between planes that aren't floored. Plane p-1 is
	// unfloored when its natural value planeBaseTicks<<(p-1) >= planeMinTicks;
	// once it is, plane p (larger) is too, so the doubling must hold.
	times := buildPlaneOnTimes(6)
	for p := 1; p < len(times); p++ {
		if planeBaseTicks<<(p-1) >= planeMinTicks {
			if times[p] != times[p-1]*2 {
				t.Errorf("plane %d (%d) != 2 × plane %d (%d)", p, times[p], p-1, times[p-1])
			}
		}
	}
}

func TestSqrtApprox(t *testing.T) {
	cases := []struct{ in, want float32 }{
		{0, 0},
		{1, 1},
		{0.25, 0.5},
		{0.49, 0.7}, // 0.09 converges poorly in 3 Newton iters; 0.49 converges to <0.1%
	}
	for _, c := range cases {
		got := sqrtApprox(c.in)
		diff := got - c.want
		if diff < 0 {
			diff = -diff
		}
		tol := float32(math.Abs(float64(c.want)) * 0.001)
		if tol < 0.0001 {
			tol = 0.0001
		}
		if diff > tol {
			t.Errorf("sqrtApprox(%v)=%v, want ~%v (tol %v)", c.in, got, c.want, tol)
		}
	}
}

func TestConfigure_WidthBoundaries(t *testing.T) {
	valid := []int{4, 8, 16, 32, 64, 128, 256}
	for _, w := range valid {
		resetForTesting()
		d := New(testPins())
		if err := d.Configure(Config{Width: w, Height: 16}); err != nil {
			t.Errorf("Width=%d rejected: %v", w, err)
		}
		resetForTesting()
	}
	invalid := []int{0, 2, 3, 5, 512}
	for _, w := range invalid {
		resetForTesting()
		d := New(testPins())
		if err := d.Configure(Config{Width: w, Height: 16}); err != ErrInvalidConfig {
			t.Errorf("Width=%d: got %v, want ErrInvalidConfig", w, err)
		}
		resetForTesting()
	}
}

func TestConfigure_HeightBoundaries(t *testing.T) {
	valid := []int{4, 8, 16, 32, 64}
	for _, h := range valid {
		resetForTesting()
		d := New(testPins())
		if err := d.Configure(Config{Width: 16, Height: h}); err != nil {
			t.Errorf("Height=%d rejected: %v", h, err)
		}
		resetForTesting()
	}
	invalid := []int{0, 2, 3, 5, 128}
	for _, h := range invalid {
		resetForTesting()
		d := New(testPins())
		if err := d.Configure(Config{Width: 16, Height: h}); err != ErrInvalidConfig {
			t.Errorf("Height=%d: got %v, want ErrInvalidConfig", h, err)
		}
		resetForTesting()
	}
}

// ============================================================
// New()
// ============================================================

func TestNew_BitMasks(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	pins := testPins()
	d := New(pins)

	check := func(name string, got uint32, wantPin Pin) {
		t.Helper()
		want := uint32(1) << platformPortBitForPin(wantPin)
		if got != want {
			t.Errorf("%s: got %032b, want %032b", name, got, want)
		}
	}
	check("r1Bit", d.r1Bit, pins.R1)
	check("g1Bit", d.g1Bit, pins.G1)
	check("b1Bit", d.b1Bit, pins.B1)
	check("r2Bit", d.r2Bit, pins.R2)
	check("g2Bit", d.g2Bit, pins.G2)
	check("b2Bit", d.b2Bit, pins.B2)
	check("clkBit", d.clkBit, pins.Clock)
	check("latBit", d.latBit, pins.Latch)
	check("oeBit", d.oeBit, pins.OE)

	wantRGB := d.r1Bit | d.g1Bit | d.b1Bit | d.r2Bit | d.g2Bit | d.b2Bit
	if d.rgbDataMask != wantRGB {
		t.Errorf("rgbDataMask: got %032b, want %032b", d.rgbDataMask, wantRGB)
	}

	var wantAddr uint32
	for _, p := range pins.Address {
		wantAddr |= 1 << platformPortBitForPin(p)
	}
	if d.addrMask != wantAddr {
		t.Errorf("addrMask: got %032b, want %032b", d.addrMask, wantAddr)
	}
}

func TestNew_PanicMixedPortGroups(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	pins := testPins()
	pins.R1 = Pin(32) // group 1 (32/32=1) vs pins 0-13 in group 0

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic with ErrMixedPortGroups, got none")
		}
	}()
	New(pins)
}

func TestNew_PanicTooFewAddressPins(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	pins := testPins()
	pins.Address = pins.Address[:2] // 2 pins, minimum is 3

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic with ErrInvalidConfig, got none")
		}
	}()
	New(pins)
}

func TestNew_PanicTooManyAddressPins(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	pins := testPins()
	pins.Address = append(pins.Address, Pin(14)) // 6 pins, maximum is 5

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic with ErrInvalidConfig, got none")
		}
	}()
	New(pins)
}

// ============================================================
// Configure()
// ============================================================

func TestConfigure_AlreadyInitializedSameDevice(t *testing.T) {
	d := setup(t, Config{Width: 64, Height: 32})
	if err := d.Configure(Config{Width: 64, Height: 32}); err != ErrAlreadyInitialized {
		t.Errorf("got %v, want ErrAlreadyInitialized", err)
	}
}

func TestConfigure_ActiveDeviceBlocksSecondDevice(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	d1 := New(testPins())
	if err := d1.Configure(Config{Width: 64, Height: 32}); err != nil {
		t.Fatalf("first Configure: %v", err)
	}
	d2 := New(testPins())
	if err := d2.Configure(Config{Width: 64, Height: 32}); err != ErrAlreadyInitialized {
		t.Errorf("got %v, want ErrAlreadyInitialized", err)
	}
}

func TestConfigure_InvalidWidth(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	d := New(testPins())
	if err := d.Configure(Config{Width: 100, Height: 32}); err != ErrInvalidConfig {
		t.Errorf("got %v, want ErrInvalidConfig", err)
	}
}

func TestConfigure_InvalidHeight(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	d := New(testPins())
	if err := d.Configure(Config{Width: 64, Height: 48}); err != ErrInvalidConfig {
		t.Errorf("got %v, want ErrInvalidConfig", err)
	}
}

func TestConfigure_HeightExceedsAddressPins(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	pins := testPins()
	pins.Address = pins.Address[:4]
	d := New(pins)
	if err := d.Configure(Config{Width: 64, Height: 64}); err != ErrInvalidConfig {
		t.Errorf("got %v, want ErrInvalidConfig", err)
	}
}

func TestConfigure_InvalidBitDepth(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	d := New(testPins())
	if err := d.Configure(Config{Width: 64, Height: 32, BitDepth: 7}); err != ErrInvalidConfig {
		t.Errorf("got %v, want ErrInvalidConfig", err)
	}
}

func TestConfigure_DefaultBitDepth(t *testing.T) {
	d := setup(t, Config{Width: 64, Height: 32, BitDepth: 0})
	if d.bitDepth != 6 {
		t.Errorf("bitDepth=%d, want 6 (default)", d.bitDepth)
	}
	if d.maxLevel != 63 {
		t.Errorf("maxLevel=%d, want 63", d.maxLevel)
	}
}

func TestConfigure_SingleBufferSize(t *testing.T) {
	// Width=8, Height=4, BitDepth=3 → rows=2, bufSize = 3*2*8 = 48
	d := setup(t, Config{Width: 8, Height: 4, BitDepth: 3})
	if len(d.frames[0]) != 48 {
		t.Errorf("frames[0] len=%d, want 48", len(d.frames[0]))
	}
	if d.frames[1] != nil {
		t.Error("frames[1] should be nil in single-buffer mode")
	}
}

func TestConfigure_DoubleBufferSize(t *testing.T) {
	d := setup(t, Config{Width: 8, Height: 4, BitDepth: 3, DoubleBuffer: true})
	if len(d.frames[0]) != 48 {
		t.Errorf("frames[0] len=%d, want 48", len(d.frames[0]))
	}
	if len(d.frames[1]) != 48 {
		t.Errorf("frames[1] len=%d, want 48", len(d.frames[1]))
	}
}

func TestConfigure_SetsActiveDevice(t *testing.T) {
	d := setup(t, Config{Width: 64, Height: 32})
	if !d.configured {
		t.Error("d.configured should be true after Configure")
	}
	if activeDevice != d {
		t.Error("activeDevice should point to d after Configure")
	}
}

// ============================================================
// SetPixel / setRGB
// ============================================================

func TestSetPixel_OutOfBounds(t *testing.T) {
	d := setup(t, smallCfg)
	before := make([]uint32, len(d.frames[0]))
	copy(before, d.frames[0])

	d.SetPixel(-1, 0, rgba(255, 0, 0, 255))
	d.SetPixel(8, 0, rgba(255, 0, 0, 255))
	d.SetPixel(0, -1, rgba(255, 0, 0, 255))
	d.SetPixel(0, 4, rgba(255, 0, 0, 255))

	for i, v := range d.frames[0] {
		if v != before[i] {
			t.Errorf("out-of-bounds write modified buffer at index %d", i)
		}
	}
}

func TestSetPixel_UpperHalfUsesR1Bits(t *testing.T) {
	d := setup(t, smallCfg)
	// R=255 → setRGB receives 255>>5=7 → gammaTable[7]=7=0b111 → all 3 planes set.
	d.SetPixel(0, 0, rgba(255, 0, 0, 255))
	for p := 0; p < 3; p++ {
		idx := bufIdx(p, 0, 0)
		if d.frames[0][idx]&d.r1Bit == 0 {
			t.Errorf("plane %d: r1Bit not set for upper-half pixel", p)
		}
		if d.frames[0][idx]&d.r2Bit != 0 {
			t.Errorf("plane %d: r2Bit should not be set for upper-half pixel", p)
		}
	}
}

func TestSetPixel_LowerHalfUsesR2Bits(t *testing.T) {
	d := setup(t, smallCfg)
	// rows=2, so y=2 is in the lower half; row index = y - rows = 0.
	d.SetPixel(0, 2, rgba(255, 0, 0, 255))
	for p := 0; p < 3; p++ {
		idx := bufIdx(p, 0, 0)
		if d.frames[0][idx]&d.r2Bit == 0 {
			t.Errorf("plane %d: r2Bit not set for lower-half pixel", p)
		}
		if d.frames[0][idx]&d.r1Bit != 0 {
			t.Errorf("plane %d: r1Bit should not be set for lower-half pixel", p)
		}
	}
}

func TestSetPixel_BitplaneDecomposition(t *testing.T) {
	d := setup(t, smallCfg)
	// SetPixel shifts R right by (8-bitDepth)=5, so raw=192 → setRGB receives
	// 192>>5=6.
	// buildGammaTable(3)[6] = 5 = 0b101.
	// Expected: planes 0 and 2 have r1Bit set; plane 1 does not.
	g := buildGammaTable(3)
	expected := g[6] // recompute so the test stays valid if gamma math changes
	d.SetPixel(0, 0, rgba(192, 0, 0, 255))
	for p := 0; p < 3; p++ {
		idx := bufIdx(p, 0, 0)
		bitSet := d.frames[0][idx]&d.r1Bit != 0
		wantBitSet := expected&(1<<p) != 0
		if bitSet != wantBitSet {
			t.Errorf("plane %d: r1Bit set=%v, want=%v (gammaTable[6]=%d=%b)",
				p, bitSet, wantBitSet, expected, expected)
		}
	}
}

func TestSetPixel_GammaApplied(t *testing.T) {
	d := setup(t, smallCfg)
	g := buildGammaTable(3)
	// Find an input i where gammaTable[i] != i, proving gamma is applied
	// rather than identity.
	var inputIdx uint8
	found := false
	for i, v := range g {
		if uint8(i) != v && i > 0 {
			inputIdx = uint8(i)
			found = true
			break
		}
	}
	if !found {
		t.Skip("gamma table is identity at bitDepth=3; cannot test gamma application")
	}
	raw := inputIdx << (8 - 3) // reverse the >>5 shift in SetPixel
	d.SetPixel(0, 0, rgba(raw, 0, 0, 255))
	expectedGamma := g[inputIdx]
	for p := 0; p < 3; p++ {
		idx := bufIdx(p, 0, 0)
		bitSet := d.frames[0][idx]&d.r1Bit != 0
		wantBitSet := expectedGamma&(1<<p) != 0
		if bitSet != wantBitSet {
			t.Errorf("plane %d: gamma not applied — r1Bit set=%v, want=%v (input=%d gamma=%d)",
				p, bitSet, wantBitSet, inputIdx, expectedGamma)
		}
	}
}

func TestSetPixel_DoubleBufferWritesToBack(t *testing.T) {
	d := setup(t, Config{Width: 8, Height: 4, BitDepth: 3, DoubleBuffer: true})
	frontBefore := make([]uint32, len(d.frames[d.frontIndex]))
	copy(frontBefore, d.frames[d.frontIndex])

	d.SetPixel(0, 0, rgba(255, 255, 255, 255))

	for i, v := range d.frames[d.frontIndex] {
		if v != frontBefore[i] {
			t.Errorf("front buffer modified at index %d after SetPixel", i)
		}
	}
	allZero := true
	for _, v := range d.frames[d.backIndex] {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("back buffer unchanged after SetPixel with non-black color")
	}
}

// ============================================================
// addressBits()
// ============================================================

func TestAddressBits(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	d := New(testPins())
	// testPins() puts address lines at pin values 6,7,8,9,10 →
	// bits A=1<<6, B=1<<7, C=1<<8, D=1<<9, E=1<<10.
	A := uint32(1 << 6)
	B := uint32(1 << 7)
	C := uint32(1 << 8)
	D := uint32(1 << 9)
	E := uint32(1 << 10)

	cases := []struct {
		row  int
		want uint32
	}{
		{0, 0},
		{1, A},
		{2, B},
		{3, A | B},
		{4, C},
		{5, A | C},
		{7, A | B | C},
		{15, A | B | C | D},
		{31, A | B | C | D | E},
	}
	for _, tc := range cases {
		got := d.addressBits(tc.row)
		if got != tc.want {
			t.Errorf("addressBits(%d) = %032b, want %032b", tc.row, got, tc.want)
		}
	}
}

// ============================================================
// clockRow() / latchRow()
// ============================================================

func TestClockRow_KeepsOELow(t *testing.T) {
	d := setup(t, tickCfg) // width 4
	d.port.reset()         // clear the fake of Configure's writes

	row := []uint32{d.r1Bit, d.g1Bit, 0, d.b1Bit}
	d.clockRow(row)

	if d.port.OUTSET.v&d.oeBit != 0 {
		t.Error("clockRow raised OE; the panel would blank during the shift")
	}
	if d.port.OUTSET.v&d.clkBit == 0 || d.port.OUTCLR.v&d.clkBit == 0 {
		t.Error("clockRow did not pulse the clock")
	}
	if d.port.OUTSET.v&(d.r1Bit|d.g1Bit|d.b1Bit) == 0 {
		t.Error("clockRow did not present row data")
	}
}

func TestLatchRow_BlanksThenLatches(t *testing.T) {
	d := setup(t, tickCfg) // rows=2, valid rows 0..1
	d.port.reset()

	d.latchRow(1) // address row 1 → A bit set, assertable

	if d.port.OUTSET.v&d.oeBit == 0 {
		t.Error("latchRow never raised OE (no blank around the latch)")
	}
	if d.port.OUTCLR.v&d.oeBit == 0 {
		t.Error("latchRow left OE high (panel would stay blanked)")
	}
	if d.port.OUTSET.v&d.latBit == 0 || d.port.OUTCLR.v&d.latBit == 0 {
		t.Error("latchRow did not pulse the latch")
	}
	if d.port.OUTSET.v&d.addressBits(1) == 0 {
		t.Error("latchRow did not set the address lines for the row")
	}
}

// ============================================================
// onTimerTick() state machine
// ============================================================

// After Configure primes the pipeline, prev points at (0,0) (what tick 1 will
// display) and the clock cursor has advanced past it to row 1. Configure has
// already clocked plane 0, row 0 into the shift registers, so tick 1 can
// immediately latch it.
func TestConfigure_PrimesPipeline(t *testing.T) {
	d := setup(t, tickCfg) // rows=2, bitDepth=2
	if d.prevPlane != 0 || d.prevRow != 0 {
		t.Errorf("prev=(%d,%d), want (0,0)", d.prevPlane, d.prevRow)
	}
	if d.currentPlane != 0 || d.currentRow != 1 {
		t.Errorf("clock cursor=(%d,%d), want (0,1)", d.currentPlane, d.currentRow)
	}
}

// onTimerTick displays the previously-clocked row. Across a 4-tick frame the
// display cursor walks (plane,row): (0,0),(0,1),(1,0),(1,1), then wraps. The
// cursor is read BEFORE each tick because it reflects what that tick latches.
func TestOnTimerTick_DisplaySequence(t *testing.T) {
	d := setup(t, tickCfg) // rows=2, bitDepth=2 → 4-tick frame
	type pr struct{ plane, row int }
	want := []pr{{0, 0}, {0, 1}, {1, 0}, {1, 1}, {0, 0}}
	for i, w := range want {
		if d.prevPlane != w.plane || d.prevRow != w.row {
			t.Errorf("before tick %d: display cursor (%d,%d), want (%d,%d)",
				i+1, d.prevPlane, d.prevRow, w.plane, w.row)
		}
		onTimerTick()
	}
}

// The timer duration set each tick is the on-time of the plane being displayed
// (prev), so it lags the clock cursor by one tick.
func TestOnTimerTick_DisplayedPlaneDuration(t *testing.T) {
	d := setup(t, tickCfg)
	wantPlane := []int{0, 0, 1, 1} // displayed plane per tick across the frame
	for i, wp := range wantPlane {
		onTimerTick()
		if lastDuration != d.planeOnTimes[wp] {
			t.Errorf("tick %d: lastDuration=%d, want planeOnTimes[%d]=%d",
				i+1, lastDuration, wp, d.planeOnTimes[wp])
		}
	}
}

func TestOnTimerTick_SwapNotMidFrame(t *testing.T) {
	d := setup(t, Config{Width: 4, Height: 4, BitDepth: 2, DoubleBuffer: true})
	origFront := d.frontIndex
	d.swapWanted.Store(true)
	// 3 of the 4 ticks in the frame — the clock cursor reaches (0,0) only on
	// tick 4, so no swap yet.
	for i := 0; i < 3; i++ {
		onTimerTick()
	}
	if d.frontIndex != origFront {
		t.Error("swap happened before frame boundary")
	}
}

func TestOnTimerTick_SwapAtFrameBoundary(t *testing.T) {
	d := setup(t, Config{Width: 4, Height: 4, BitDepth: 2, DoubleBuffer: true})
	origFront := d.frontIndex
	d.swapWanted.Store(true)
	// 4 ticks complete the frame; on tick 4 the clock cursor is (0,0) and the swap fires.
	for i := 0; i < 4; i++ {
		onTimerTick()
	}
	if d.frontIndex == origFront {
		t.Error("swap did not happen at frame boundary")
	}
	if d.swapWanted.Load() {
		t.Error("swapWanted not cleared after swap")
	}
}

// The swap must happen BEFORE the row is read, so the row clocked on the swap
// tick comes from the new front buffer. Mark the back buffer's (plane 0, row 0,
// x 0) cell with a recognizable bit (r1Bit, distinct from clock/latch/OE/address
// bits) while the front buffer stays all-zero; after the swap tick that bit must
// appear on the data lines.
func TestOnTimerTick_SwapClocksFromNewBuffer(t *testing.T) {
	d := setup(t, Config{Width: 4, Height: 4, BitDepth: 2, DoubleBuffer: true})
	d.frames[d.backIndex][0] = d.r1Bit
	d.swapWanted.Store(true)

	// 3 ticks: no swap yet (clock cursor reaches (0,0) only on tick 4). Reset the
	// fake so we observe only the swap tick's clocking.
	for i := 0; i < 3; i++ {
		onTimerTick()
	}
	d.port.reset()
	onTimerTick() // tick 4: swaps, then clocks the new front buffer's (0,0)

	if d.port.OUTSET.v&d.r1Bit == 0 {
		t.Error("swap tick did not clock the marker from the new front buffer; swap-before-read is broken")
	}
}

// ============================================================
// Clear(), Pause(), Resume()
// ============================================================

func TestClear_ZerosBackBuffer(t *testing.T) {
	d := setup(t, Config{Width: 8, Height: 4, BitDepth: 3, DoubleBuffer: true})
	// Write something non-zero to the back buffer.
	d.SetPixel(0, 0, rgba(255, 255, 255, 255))
	// Verify it's non-zero before Clear.
	nonZero := false
	for _, v := range d.frames[d.backIndex] {
		if v != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatal("precondition failed: back buffer is all-zero before Clear()")
	}
	d.Clear()
	for i, v := range d.frames[d.backIndex] {
		if v != 0 {
			t.Errorf("back buffer not zeroed at index %d after Clear()", i)
		}
	}
}

func TestPauseResume_NoopBeforeConfigure(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	d := New(testPins())
	// Must not panic.
	d.Pause()
	d.Resume()
}

// ============================================================
// Display()
// ============================================================

func TestDisplay_SingleBufferNoop(t *testing.T) {
	d := setup(t, Config{Width: 4, Height: 4, BitDepth: 2, DoubleBuffer: false})
	if err := d.Display(); err != nil {
		t.Errorf("Display() in single-buffer mode returned %v, want nil", err)
	}
}

func TestDisplay_DoubleBufferUnblocksAfterSwap(t *testing.T) {
	d := setup(t, Config{Width: 4, Height: 4, BitDepth: 2, DoubleBuffer: true})
	origFront := d.frontIndex

	done := make(chan struct{})
	go func() {
		d.Display()
		close(done)
	}()

	// Drive 4 ticks (rows=2 × bitDepth=2) to reach the frame boundary where
	// the swap happens and swapWanted is cleared, unblocking Display().
	for i := 0; i < 4; i++ {
		onTimerTick()
		runtime.Gosched()
	}

	// Spin-yield up to 1000 times to let the Display() goroutine observe the
	// cleared swapWanted and close done. A non-blocking select here would race
	// on single-threaded schedulers (GOMAXPROCS=1).
	for i := 0; i < 1000; i++ {
		select {
		case <-done:
			if d.frontIndex == origFront {
				t.Error("front/back buffers not swapped after Display()")
			}
			return
		default:
			runtime.Gosched()
		}
	}
	t.Fatal("Display() did not unblock after frame boundary")
}
