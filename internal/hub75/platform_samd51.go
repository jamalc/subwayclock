//go:build atsamd51

// Platform file for SAMD51 chips. Provides chip-level GPIO and timer
// primitives that the protocol code uses. The driver code in hub75.go
// is otherwise chip-independent.
//
// To port hub75 to a new chip family, add an equivalent platform_*.go
// file with the appropriate build tag and implement the platform*
// primitives listed in hub75.go's platform contract.
package hub75

import (
	"device/sam"
	"errors"
	"machine"
	"runtime/interrupt"
)

// --- Port group primitives ------------------------------------------------

// portGroup is an opaque (to the driver code) handle for a chip's
// GPIO port group. On SAMD51 it's a pointer into the PORT register
// array; on other chips it would be whatever they use.
type portGroup = sam.PORT_GROUP_Type

// platformPortGroupForPin returns the port group index a pin belongs to.
// On SAMD51, pins are encoded as uint8 where the high bits are the
// group and the low 5 bits are the bit position within the group.
func platformPortGroupForPin(p Pin) uint8 {
	return uint8(p) / 32
}

// platformPortBitForPin returns the bit position within the pin's port
// group (0..31).
func platformPortBitForPin(p Pin) uint8 {
	return uint8(p) % 32
}

// platformPortGroup returns a pointer to the port group struct for atomic
// bulk register access (OUTSET, OUTCLR, etc).
//
// NOTE: this uses the APB-mapped PORT. The single-cycle GPIO mirror (IOBUS at
// 0x60000000) that some SAMD drivers use for fast bit-banging is a Cortex-M0+
// feature (SAMD21) — the SAMD51's M4 core does not have it, and writing there
// hard-faults. Faster shifting on this chip means fewer writes per pixel or DMA,
// not a faster bus address.
func platformPortGroup(group uint8) *portGroup {
	return &sam.PORT.GROUP[group]
}

// platformConfigureOutputs configures every given pin as a push-pull
// output. This is the only place in the SAMD51 board that drives pin
// mode; the core calls it once from Configure.
func platformConfigureOutputs(pins []Pin) {
	for _, p := range pins {
		machine.Pin(p).Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
}

// --- Timer ----------------------------------------------------------------
//
// SAMD51 uses TC3 as a 16-bit timer in match-frequency mode. Other
// chips would use whatever their suitable timer is.

// ErrTimerInit is returned when a timer register sync-busy operation
// fails to complete in a reasonable number of cycles. This means the
// timer peripheral isn't responding; either the chip is broken, clocks
// aren't running, or there's a configuration conflict elsewhere.
var ErrTimerInit = errors.New("hub75: timer initialization failed (SYNCBUSY timeout)")

// syncBusyRetries bounds register-sync busy-wait loops. Normal
// completion takes tens of cycles; this gives ~3 orders of magnitude
// of headroom before declaring the peripheral broken.
const syncBusyRetries = 10000

// waitSyncBusy waits for the given SYNCBUSY bits to clear, up to
// syncBusyRetries iterations. Returns true on success, false on
// timeout. Per the NASA Power of 10 rules, all loops have fixed bounds.
func waitSyncBusy(bits uint32) bool {
	for i := 0; i < syncBusyRetries; i++ {
		if !sam.TC3_COUNT16.SYNCBUSY.HasBits(bits) {
			return true
		}
	}
	return false
}

var platformTimerCallback func()

func platformStartTimer(initial uint16, callback func()) error {
	platformTimerCallback = callback

	sam.GCLK.PCHCTRL[26].Set(sam.GCLK_PCHCTRL_CHEN | (0 << sam.GCLK_PCHCTRL_GEN_Pos))
	sam.MCLK.APBBMASK.SetBits(sam.MCLK_APBBMASK_TC3_)

	sam.TC3_COUNT16.CTRLA.Set(sam.TC_COUNT16_CTRLA_SWRST)
	if !waitSyncBusy(sam.TC_COUNT16_SYNCBUSY_SWRST) {
		return ErrTimerInit
	}

	sam.TC3_COUNT16.CTRLA.Set(
		(sam.TC_COUNT16_CTRLA_MODE_COUNT16 << sam.TC_COUNT16_CTRLA_MODE_Pos) |
			(sam.TC_COUNT16_CTRLA_PRESCALER_DIV16 << sam.TC_COUNT16_CTRLA_PRESCALER_Pos),
	)
	sam.TC3_COUNT16.WAVE.Set(sam.TC_COUNT16_WAVE_WAVEGEN_MFRQ)

	sam.TC3_COUNT16.CC[0].Set(initial)
	if !waitSyncBusy(sam.TC_COUNT16_SYNCBUSY_CC0) {
		return ErrTimerInit
	}

	sam.TC3_COUNT16.INTENSET.Set(sam.TC_COUNT16_INTENSET_MC0)

	// interrupt.New is a compile-time static registration in TinyGo; the
	// returned value does not need to be stored to keep the interrupt alive.
	timerInt := interrupt.New(sam.IRQ_TC3, timerISR)
	timerInt.Enable()

	sam.TC3_COUNT16.CTRLA.SetBits(sam.TC_COUNT16_CTRLA_ENABLE)
	if !waitSyncBusy(sam.TC_COUNT16_SYNCBUSY_ENABLE) {
		return ErrTimerInit
	}

	return nil
}

// platformSetNextDuration tells the timer when to fire next.
// Called from inside the timer callback.
//
// No SYNCBUSY wait here: this is called from the timer ISR, so blocking
// would cause the next tick to be missed. The peripheral picks up the
// new CC[0] value before the next compare event.
func platformSetNextDuration(d uint16) {
	sam.TC3_COUNT16.CC[0].Set(d)
}

// platformPauseTimer stops the TC3 timer without resetting its state.
// Counter value is preserved; when platformResumeTimer is called the
// counter continues from where it left off.
func platformPauseTimer() {
	sam.TC3_COUNT16.CTRLA.ClearBits(sam.TC_COUNT16_CTRLA_ENABLE)
	// Don't waitSyncBusy — caller may be in time-critical context.
	// The peripheral disable will complete in ~1us regardless.
}

// platformResumeTimer restarts the TC3 timer.
func platformResumeTimer() {
	sam.TC3_COUNT16.CTRLA.SetBits(sam.TC_COUNT16_CTRLA_ENABLE)
}

func timerISR(i interrupt.Interrupt) {
	sam.TC3_COUNT16.INTFLAG.Set(sam.TC_COUNT16_INTFLAG_MC0)
	if platformTimerCallback != nil {
		platformTimerCallback()
	}
}
