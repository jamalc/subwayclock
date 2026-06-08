//go:build !tinygo

package hub75

// fakeReg records accumulated Set() calls, mirroring the volatile.Register32
// API used by the real portGroup on SAMD51.
type fakeReg struct{ v uint32 }

func (r *fakeReg) Set(bits uint32) { r.v |= bits }

// portGroup is the fake GPIO port group used on host (non-TinyGo) builds.
type portGroup struct {
	OUTSET fakeReg
	OUTCLR fakeReg
}

// State returns the net logical pin state accumulated across all Set() calls.
// NOTE: fakeReg.Set accumulates with |=, so once a bit is set in OUTCLR it
// stays set even if OUTSET is written later. Call reset() between probes when
// asserting final GPIO state after a clockRow or latchRow sequence.
func (p *portGroup) State() uint32 { return p.OUTSET.v &^ p.OUTCLR.v }

func (p *portGroup) reset() { *p = portGroup{} }

// fakePort is the singleton returned by platformPortGroup.
var fakePort portGroup

// --- Board primitives -------------------------------------------------------

func platformPortGroupForPin(p Pin) uint8 { return uint8(p) / 32 }
func platformPortBitForPin(p Pin) uint8   { return uint8(p) % 32 }

// platformPortGroup always returns the same singleton. All test pins must be in
// group 0 (pin values 0–31) to avoid aliased port state.
func platformPortGroup(group uint8) *portGroup { return &fakePort }

func platformPauseTimer()  {}
func platformResumeTimer() {}

// platformConfigureOutputs is a no-op on host: there is no real GPIO to configure.
func platformConfigureOutputs(pins []Pin) {}

// --- Controllable timer -----------------------------------------------------
//
// platformStartTimer stores the callback instead of starting a real timer.
// Tests drive the ISR by calling onTimerTick() directly.

var (
	timerCallback func()
	lastDuration  uint16
)

func platformStartTimer(initial uint16, callback func()) error {
	timerCallback = callback
	lastDuration = initial
	return nil
}

func platformSetNextDuration(d uint16) { lastDuration = d }

// --- Test helpers -----------------------------------------------------------

// resetForTesting resets all package-level state so tests don't bleed into
// each other. Only available on host (non-TinyGo) builds.
func resetForTesting() {
	activeDevice = nil
	fakePort.reset()
	timerCallback = nil
	lastDuration = 0
}
