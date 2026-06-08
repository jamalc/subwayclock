// Package hub75 drives a HUB75 RGB LED matrix from a TinyGo
// microcontroller.
//
// It implements drivers.Displayer, so it works with any TinyGo
// display library that targets that interface (tinydraw, tinyfont,
// etc.).
//
// Hardware requirements:
//   - A 32-bit MCU with a GPIO port that supports atomic bulk
//     SET and CLEAR operations on at least 16 bits at once
//     (currently SAMD51; ports to other chips are straightforward).
//   - A 16-bit hardware timer with an interrupt the driver can claim.
//   - All HUB75 control pins wired to a single GPIO port group.
//
// Usage:
//
//	panel := hub75.New(hub75.Pins{
//	    R1: machine.HUB75_R1, G1: machine.HUB75_G1, B1: machine.HUB75_B1,
//	    R2: machine.HUB75_R2, G2: machine.HUB75_G2, B2: machine.HUB75_B2,
//	    Address: []machine.Pin{
//	        machine.HUB75_ADDR_A, machine.HUB75_ADDR_B,
//	        machine.HUB75_ADDR_C, machine.HUB75_ADDR_D,
//	        machine.HUB75_ADDR_E,  // omit for ≤32-tall panels
//	    },
//	    Clock: machine.HUB75_CLK, Latch: machine.HUB75_LAT,
//	    OE:    machine.HUB75_OE,
//	})
//	if err := panel.Configure(hub75.Config{Width: 64, Height: 32}); err != nil { ... }
//	panel.SetPixel(0, 0, color.RGBA{255, 0, 0, 255})
//	select{}
//
// For animation, enable double buffering:
//
//	panel := hub75.New(pins)
//	panel.Configure(hub75.Config{Width: 64, Height: 32, DoubleBuffer: true})
//	for {
//	    panel.Clear()
//	    drawFrame(panel)
//	    panel.Display() // blocks until atomically shown
//	}
package hub75

import (
	"errors"
	"image/color"
	"runtime"
	"sync/atomic"

	"tinygo.org/x/drivers"
)

// --- Platform contract -----------------------------------------------------
//
// The platform file (platform_*.go) must provide these names. The
// driver code (this file) uses them but does not define them. The MCU
// (chip) and the board (product pin map) are separate concerns: the
// platform_*.go file holds the per-chip register/timer primitives,
// while a boards/*.go file holds a product's HUB75 pin assignments.
//
// Types:
//   type portGroup ... // chip-specific GPIO port group struct type
//   (the driver defines type Pin uint8; platform files convert to/from
//    their chip's native pin type)
//
// NOTE: a generic (non-SAMD51) TinyGo target selects no platform file
// and won't link until a platform_*.go for it is added. SAMD51
// (platform_samd51.go) is the only supported chip today; host (!tinygo)
// builds use platform_host.go.
//
// Pin and port primitives:
//   func platformPortGroupForPin(p Pin) uint8
//       returns the port group index a pin belongs to
//   func platformPortBitForPin(p Pin) uint8
//       returns the bit position within the port group (0..31)
//   func platformPortGroup(group uint8) *portGroup
//       returns a pointer to the port group struct. The driver writes GPIO
//       only through this struct's OUTSET and OUTCLR fields, each of which
//       must expose a Set(bits uint32) method that atomically sets (OUTSET)
//       or clears (OUTCLR) the given bits. No other port-group registers
//       are used.
//   func platformConfigureOutputs(pins []Pin)
//       configures every given pin as a push-pull output
//
// Timer:
//   func platformStartTimer(initial uint16, callback func()) error
//       starts a hardware timer that fires the callback every
//       `initial` ticks. Subsequent durations are set via
//       platformSetNextDuration from inside the callback.
//   func platformSetNextDuration(d uint16)
//       called from the timer ISR; sets the duration until the
//       next callback.
//   func platformPauseTimer()
//       stops timer, preserves state
//   func platformResumeTimer()
//       restarts after Pause

// --- Public types ---------------------------------------------------------

// Pin identifies a board GPIO pin. It mirrors the underlying type of
// TinyGo's machine.Pin; board files convert between the two. Keeping a
// package-local type is what lets the core compile without importing
// machine (so it builds and tests on host Go).
type Pin uint8

// Pins describes how the HUB75 signals connect to the board's GPIO
// pins. All pins must be on the same GPIO port group; New panics
// with ErrMixedPortGroups otherwise.
//
// Address is a slice of address pins (typically 3-5) in order A, B,
// C, D, E. Its length determines the maximum panel height:
//   - 3 pins (A, B, C):        up to 16-tall panels
//   - 4 pins (A, B, C, D):     up to 32-tall panels
//   - 5 pins (A, B, C, D, E):  up to 64-tall panels
//
// Configure validates that the requested Height fits.
type Pins struct {
	R1, G1, B1       Pin
	R2, G2, B2       Pin
	Address          []Pin
	Clock, Latch, OE Pin
}

// Config configures a Device. Width and Height are required.
type Config struct {
	// Width is the total chain width in pixels (power of 2, 4..256).
	Width int

	// Height is one panel's pixel height (power of 2, 4..64).
	Height int

	// BitDepth is bits per color channel, 1..6. Default 6.
	BitDepth int

	// DoubleBuffer enables double-buffered drawing. Doubles frame
	// buffer memory.
	DoubleBuffer bool
}

// Errors used in panics (static programmer errors) and returns
// (runtime hardware failures).
var (
	// Panics from New and Configure:
	ErrAlreadyInitialized = errors.New("hub75: device already configured")
	ErrInvalidConfig      = errors.New("hub75: invalid config (Width/Height must be power-of-2, Width 4..256, Height 4..64 ≤ 2^(addrPins+1), BitDepth 1..6, Address 3-5 pins)")
	ErrMixedPortGroups    = errors.New("hub75: all pins must be on the same GPIO port group")
)

// Device drives one HUB75 panel chain.
//
// Only one Device can be active at a time. This is a consequence of
// the timer-driven refresh architecture: there is one suitable timer
// on the MCU, and its ISR can only meaningfully refresh one panel
// chain. Attempting to Configure a second Device while another is
// active panics with ErrAlreadyInitialized.
type Device struct {
	pins Pins
	port *portGroup

	// Pre-computed bit positions within the port group, set in New.
	r1Bit, g1Bit, b1Bit uint32
	r2Bit, g2Bit, b2Bit uint32
	addrBits            []uint32 // one bit per address pin, A through E
	clkBit, latBit      uint32
	oeBit               uint32
	rgbDataMask         uint32
	addrMask            uint32

	width    int
	height   int
	rows     int
	bitDepth int
	maxLevel uint8

	// frames holds the pre-decomposed bitplane buffers. Each buffer
	// is indexed as plane*rows*width + row*width + col, with each
	// uint32 value pre-positioned with bits in the GPIO port-register
	// positions for that pixel. Gamma is already applied.
	//
	// ARCHITECTURE: This design front-loads work in SetPixel (gamma
	// lookup + bitplane decomposition + bit packing — ~50 ops per
	// pixel) to make the refresh ISR essentially free (just slice
	// the buffer and write to the port register).
	//
	// The tradeoff is right when refresh rate >> draw rate, which
	// is the typical embedded case. For workloads where draw rate
	// approaches refresh rate (full-frame 60fps video, real-time
	// pixel-by-pixel updates), a different architecture (storing
	// raw RGBA and decomposing during refresh, like ardnew/drivers
	// PR #213's rgb75 driver) might be faster overall. We
	// deliberately chose the embedded-typical case.
	frames [2][]uint32

	doubleBuffer bool
	frontIndex   int
	backIndex    int

	// swapWanted is set by user code (Display) and cleared by the
	// ISR. Atomic because both non-interrupt and interrupt code touch
	// it. (atomic, not volatile, so the core stays host-buildable —
	// runtime/volatile is TinyGo-only.)
	swapWanted atomic.Bool

	// gammaTable maps internal-bit-depth values to gamma-corrected
	// equivalents. Applied in setRGB before bitplane decomposition;
	// the decomposed bits in `frames` already reflect gamma.
	gammaTable   []uint8
	planeOnTimes []uint16

	// The clock cursor (current*) is the (plane,row) being clocked this tick;
	// the display cursor (prev*) is what's latched/displayed this tick — one tick
	// behind. See onTimerTick.
	currentPlane int
	currentRow   int
	prevPlane    int
	prevRow      int

	configured bool
}

// activeDevice is the singleton currently-running Device.
var activeDevice *Device

// --- Device lifecycle ------------------------------------------------------

// New constructs a Device with the given pin assignments. It is a
// pure constructor: it validates inputs and computes internal masks
// but does NOT touch hardware. Call Configure to configure the GPIO
// pins, claim the timer, and start refresh.
//
// PANICS with ErrMixedPortGroups if the pins don't all share a single
// GPIO port group. PANICS with ErrInvalidConfig if Address has fewer
// than 3 or more than 5 pins.
//
// These are static programmer errors — the pins are determined by
// source code, not runtime — so a clean halt is more useful than an
// error return that callers would propagate to a halt anyway.
func New(pins Pins) *Device {
	if len(pins.Address) < 3 || len(pins.Address) > 5 {
		panic(ErrInvalidConfig)
	}

	// All pins must share a port group.
	allPins := allPinsFrom(pins)
	group := platformPortGroupForPin(allPins[0])
	for _, p := range allPins[1:] {
		if platformPortGroupForPin(p) != group {
			panic(ErrMixedPortGroups)
		}
	}

	d := &Device{
		pins: pins,
		port: platformPortGroup(group),
	}

	// Compute bit masks. Pure computation, no hardware access.
	d.r1Bit = 1 << platformPortBitForPin(pins.R1)
	d.g1Bit = 1 << platformPortBitForPin(pins.G1)
	d.b1Bit = 1 << platformPortBitForPin(pins.B1)
	d.r2Bit = 1 << platformPortBitForPin(pins.R2)
	d.g2Bit = 1 << platformPortBitForPin(pins.G2)
	d.b2Bit = 1 << platformPortBitForPin(pins.B2)

	d.addrBits = make([]uint32, len(pins.Address))
	for i, p := range pins.Address {
		d.addrBits[i] = 1 << platformPortBitForPin(p)
	}

	d.clkBit = 1 << platformPortBitForPin(pins.Clock)
	d.latBit = 1 << platformPortBitForPin(pins.Latch)
	d.oeBit = 1 << platformPortBitForPin(pins.OE)

	d.rgbDataMask = d.r1Bit | d.g1Bit | d.b1Bit | d.r2Bit | d.g2Bit | d.b2Bit
	for _, b := range d.addrBits {
		d.addrMask |= b
	}

	return d
}

// allPinsFrom returns a flat slice of all pins in a Pins struct, used
// for validation and bulk configuration.
func allPinsFrom(pins Pins) []Pin {
	all := []Pin{
		pins.R1, pins.G1, pins.B1,
		pins.R2, pins.G2, pins.B2,
		pins.Clock, pins.Latch, pins.OE,
	}
	return append(all, pins.Address...)
}

// Configure validates the config, allocates frame buffers, configures
// the GPIO pins as outputs, blanks the panel, claims the hardware
// timer, and starts refresh.
//
// RETURNS with ErrAlreadyInitialized if another Device is already
// active, or ErrInvalidConfig for bad values. These are static
// programmer errors.
//
// RETURNS an error for runtime hardware failures (currently only
// ErrTimerInit, which means the hardware timer didn't respond as
// expected).
//
// After Configure returns successfully, the panel is actively
// refreshing and SetPixel calls take effect immediately (single
// buffer) or on the next Display() call (double buffer).
func (d *Device) Configure(cfg Config) error {
	if d.configured {
		return ErrAlreadyInitialized
	}
	if activeDevice != nil {
		return ErrAlreadyInitialized
	}

	if cfg.BitDepth == 0 {
		cfg.BitDepth = 6
	}
	if !isPow2(cfg.Width) || cfg.Width < 4 || cfg.Width > 256 ||
		!isPow2(cfg.Height) || cfg.Height < 4 || cfg.Height > 64 ||
		cfg.BitDepth < 1 || cfg.BitDepth > 6 {
		return ErrInvalidConfig
	}

	// Height must fit in the address pins provided to New.
	maxHeight := 1 << (len(d.pins.Address) + 1)
	if cfg.Height > maxHeight {
		return ErrInvalidConfig
	}

	rows := cfg.Height / 2
	bufSize := cfg.BitDepth * rows * cfg.Width

	d.width = cfg.Width
	d.height = cfg.Height
	d.rows = rows
	d.bitDepth = cfg.BitDepth
	d.maxLevel = uint8((1 << cfg.BitDepth) - 1)
	d.doubleBuffer = cfg.DoubleBuffer

	d.frames[0] = make([]uint32, bufSize)
	if cfg.DoubleBuffer {
		d.frames[1] = make([]uint32, bufSize)
		d.backIndex = 1
	}
	d.gammaTable = buildGammaTable(cfg.BitDepth)
	d.planeOnTimes = buildPlaneOnTimes(cfg.BitDepth)

	// Configure GPIO pins as outputs.
	platformConfigureOutputs(allPinsFrom(d.pins))

	// Blank the panel: OE high to disable output, everything else low
	// for a clean known state. Done after pin configuration so the
	// panel doesn't briefly display whatever the shift registers held
	// before we took control.
	d.port.OUTSET.Set(d.oeBit)
	d.port.OUTCLR.Set(d.rgbDataMask | d.clkBit | d.latBit | d.addrMask)

	// Prime the pipeline: clock (plane 0, row 0) so the first tick latches valid
	// data, set the display cursor to it, and point the clock cursor at the next
	// row. (rows >= 2 always, since Height >= 4.)
	d.clockRow(d.frames[d.frontIndex][0:d.width])
	d.prevPlane, d.prevRow = 0, 0
	d.currentPlane, d.currentRow = 0, 1

	activeDevice = d

	if err := platformStartTimer(d.planeOnTimes[0], onTimerTick); err != nil {
		activeDevice = nil
		return err
	}

	d.configured = true
	return nil
}

// isPow2 reports whether n is a positive power of two.
func isPow2(n int) bool { return n > 0 && n&(n-1) == 0 }

// --- Public accessors ----------------------------------------------------

// Width returns the panel chain width in pixels.
func (d *Device) Width() int { return d.width }

// Height returns the panel height in pixels.
func (d *Device) Height() int { return d.height }

// MaxLevel returns the maximum value for a single internal color
// channel. Mainly useful for debugging.
func (d *Device) MaxLevel() uint8 { return d.maxLevel }

// Clear turns every pixel off in the back buffer.
func (d *Device) Clear() {
	if !d.configured {
		return
	}
	buf := d.frames[d.backIndex]
	for i := range buf {
		buf[i] = 0
	}
}

// Pause stops refreshing the panel. The current frame buffer is
// retained — when Resume is called, the panel resumes showing the
// same content (or whatever has been drawn into the buffer since).
// SetPixel calls during Pause still write to the buffer; they just
// don't become visible until Resume.
//
// Useful for:
//   - Power management (pause before deep sleep)
//   - Time-critical code that can't tolerate ISR interruptions
//     (e.g., precise bit-banged protocols, USB enumeration)
//   - Clean shutdown before reconfiguring hardware
//
// No-op if Configure has not been called.
func (d *Device) Pause() {
	if !d.configured {
		return
	}
	platformPauseTimer()
}

// Resume restarts refresh after a Pause. No-op if Configure has not
// been called.
func (d *Device) Resume() {
	if !d.configured {
		return
	}
	platformResumeTimer()
}

// --- drivers.Displayer ---------------------------------------------------

// Size returns the panel chain dimensions in pixels.
func (d *Device) Size() (x, y int16) {
	return int16(d.width), int16(d.height)
}

// SetPixel sets the pixel at (x, y). Part of drivers.Displayer.
// Color components are 8-bit; they are quantized to the driver's
// internal bit depth. In double-buffer mode, the change isn't
// visible until Display() is called.
func (d *Device) SetPixel(x, y int16, c color.RGBA) {
	if !d.configured {
		return
	}
	shift := uint8(8 - d.bitDepth)
	d.setRGB(int(x), int(y), c.R>>shift, c.G>>shift, c.B>>shift)
}

// Display, in double-buffer mode, requests an atomic frame swap and
// blocks until the swap completes. In single-buffer mode, no-op.
func (d *Device) Display() error {
	if !d.doubleBuffer {
		return nil
	}
	d.swapWanted.Store(true)
	for d.swapWanted.Load() {
		runtime.Gosched()
	}
	return nil
}

// Compile-time assertion that *Device implements drivers.Displayer.
var _ drivers.Displayer = (*Device)(nil)

// --- Internal: pixel writing --------------------------------------------

// setRGB sets a pixel in the back buffer using the driver's native
// bit-depth representation. Each of r, g, b must be in 0..d.maxLevel.
// Out-of-bounds coordinates are silently ignored.
func (d *Device) setRGB(x, y int, r, g, b uint8) {
	if x < 0 || x >= d.width || y < 0 || y >= d.height {
		return
	}

	r = d.gammaTable[r&d.maxLevel]
	g = d.gammaTable[g&d.maxLevel]
	b = d.gammaTable[b&d.maxLevel]

	var rBit, gBit, bBit uint32
	row := y
	if y < d.rows {
		rBit, gBit, bBit = d.r1Bit, d.g1Bit, d.b1Bit
	} else {
		row = y - d.rows
		rBit, gBit, bBit = d.r2Bit, d.g2Bit, d.b2Bit
	}

	colorMask := rBit | gBit | bBit
	planeStride := d.rows * d.width
	baseIdx := row*d.width + x
	buf := d.frames[d.backIndex]

	for p := 0; p < d.bitDepth; p++ {
		idx := p*planeStride + baseIdx
		mask := uint8(1 << p)
		cell := buf[idx] &^ colorMask
		if r&mask != 0 {
			cell |= rBit
		}
		if g&mask != 0 {
			cell |= gBit
		}
		if b&mask != 0 {
			cell |= bBit
		}
		buf[idx] = cell
	}
}

// --- Internal: shift loop ------------------------------------------------

// clockRow shifts one row's data into the panel's shift registers. It does NOT
// touch OE: the panel keeps displaying the previously latched row while this
// runs (see onTimerTick), which is what keeps the panel lit during the shift.
//
// TIMING: the panel samples the RGB lines on the clock's rising edge, so the
// data must be stable before OUTSET(clkBit). On the SAMD51 at 120MHz the
// register writes themselves give enough setup time (verified with cmd/stress);
// a faster GPIO bus may need a short delay between the data write and the clock.
//
// PERF: don't try to speed this up by cutting register writes. Measured on the
// Matrix Portal M4: an OUTTGL XOR-delta variant at 2 writes per pixel was a
// wash — ~16µs/row either way. SAMD51 PORT writes are posted/cheap, so the
// cost here is per-pixel loop overhead (branch, counter, slice access, field
// loads), not the writes; the same reasoning rules out the IOBUS path (see
// platform_samd51.go). One lever that could move it is loop unrolling +
// bounds-check elimination (what Protomatter does), and even that is
// measure-first, not a sure thing. 8-bit color is out of reach regardless — it
// needs DMA, which Protomatter itself doesn't do on SAMD.
func (d *Device) clockRow(rowData []uint32) {
	for x := 0; x < len(rowData); x++ {
		d.port.OUTCLR.Set(d.rgbDataMask | d.clkBit)
		d.port.OUTSET.Set(rowData[x])
		d.port.OUTSET.Set(d.clkBit)
		d.port.OUTCLR.Set(d.clkBit)
	}
}

// latchRow makes the data currently in the shift registers visible on the row
// selected by addr. It blanks the panel (OE high) only for the address change
// and latch pulse — the classic anti-ghosting window — then re-enables output.
// This is the only place OE is raised.
func (d *Device) latchRow(addr int) {
	d.port.OUTSET.Set(d.oeBit)
	d.port.OUTCLR.Set(d.addrMask)
	d.port.OUTSET.Set(d.addressBits(addr))
	d.port.OUTSET.Set(d.latBit)
	d.port.OUTCLR.Set(d.latBit)
	d.port.OUTCLR.Set(d.oeBit)
}

func (d *Device) addressBits(r int) uint32 {
	var bits uint32
	for i, mask := range d.addrBits {
		if r&(1<<i) != 0 {
			bits |= mask
		}
	}
	return bits
}

// --- Internal: refresh state machine ------------------------------------

// onTimerTick is called from the timer ISR on every fire. It runs a one-tick
// pipeline: latch and display the row clocked during the previous tick, time
// that plane, then clock the next row's data behind the latched output so the
// panel stays lit during the shift. Configure primes the registers with
// (plane 0, row 0) before the first tick.
//
// CORRECTNESS: the duration set is for the *displayed* (prev) plane.
// CORRECTNESS: the buffer swap happens when the *clock* cursor reaches
// (plane 0, row 0), before the row about to be clocked is read, so it comes
// from the new front buffer.
func onTimerTick() {
	d := activeDevice
	if d == nil {
		return
	}

	// Show the row clocked during the previous tick, and time its plane.
	d.latchRow(d.prevRow)
	platformSetNextDuration(d.planeOnTimes[d.prevPlane])

	// At the start of a new frame, honor a pending swap before reading the row
	// we're about to clock.
	if d.currentPlane == 0 && d.currentRow == 0 && d.swapWanted.Load() {
		d.frontIndex, d.backIndex = d.backIndex, d.frontIndex
		d.swapWanted.Store(false)
	}

	// Clock the current row's data while the latched row is displayed.
	buf := d.frames[d.frontIndex]
	start := d.currentPlane*d.rows*d.width + d.currentRow*d.width
	d.clockRow(buf[start : start+d.width])

	// The row we just clocked becomes next tick's displayed row; advance the
	// clock cursor (row, then plane, wrapping at the frame boundary).
	d.prevPlane, d.prevRow = d.currentPlane, d.currentRow
	d.currentRow++
	if d.currentRow >= d.rows {
		d.currentRow = 0
		d.currentPlane++
		if d.currentPlane >= d.bitDepth {
			d.currentPlane = 0
		}
	}
}

// --- Helpers -------------------------------------------------------------

func buildGammaTable(bitDepth int) []uint8 {
	size := 1 << bitDepth
	maxVal := float32(size - 1)
	t := make([]uint8, size)
	for i := 0; i < size; i++ {
		ratio := float32(i) / maxVal
		v := ratio * ratio * sqrtApprox(ratio)
		out := int(v*maxVal + 0.5)
		if out > size-1 {
			out = size - 1
		}
		t[i] = uint8(out)
	}
	return t
}

func sqrtApprox(x float32) float32 {
	if x <= 0 {
		return 0
	}
	g := x
	g = (g + x/g) * 0.5
	g = (g + x/g) * 0.5
	g = (g + x/g) * 0.5
	return g
}

// Plane-timing parameters for binary-code modulation. These are the tuning
// knobs for the brightness/refresh/color tradeoff (see the pipelined-latch
// design doc) and are intended to be adjusted on-hardware with cmd/stress.
//
//   - planeBaseTicks is plane 0's on-time (the BCM unit); plane p is
//     planeBaseTicks<<p. Raising it toward the measured row-shift time gives
//     clean low-end weighting at the cost of refresh.
//   - planeMinTicks floors every plane: a plane's on-time must be ≥ the row
//     shift time, so the next row finishes clocking within the displayed
//     plane's window (the pipeline invariant).
const (
	planeBaseTicks = uint16(100)
	planeMinTicks  = uint16(100)
)

// buildPlaneOnTimes computes timer durations for each bit plane. Ratio is
// normally 1:2:4:...:2^(bitDepth-1) for proper BCM weighting, floored at
// planeMinTicks.
//
// CORRECTNESS: Each plane's on-time must be longer than the time to shift one
// full row of data into the panel; otherwise the next row can't finish clocking
// within the displayed plane's window. When the floor truncates lower planes,
// color resolution at the dim end is reduced.
func buildPlaneOnTimes(bitDepth int) []uint16 {
	t := make([]uint16, bitDepth)
	for p := 0; p < bitDepth; p++ {
		v := planeBaseTicks << p
		if v < planeMinTicks {
			v = planeMinTicks
		}
		t[p] = v
	}
	return t
}
