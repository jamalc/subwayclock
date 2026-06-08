//go:build tinygo

package main

import (
	"machine"

	"tinygo.org/x/drivers/lis3dh"
)

// i2cAddrs lists the LIS3DH I2C addresses to probe, board address first. The
// Matrix Portal M4's onboard LIS3DH is at 0x19 (SA0 high); 0x18 (the driver's
// default, SA0 low) is the breakout default and a sensible fallback.
var i2cAddrs = []uint16{0x19, 0x18}

// accel reads the Matrix Portal M4's onboard LIS3DH and implements
// gravity.Tilt. ReadAcceleration returns micro-g per axis (±1_000_000 = 1g);
// dividing by 1e6 yields g units. ok is false when no accelerometer was found;
// Vector then reports neutral gravity so the stress test still runs.
type accel struct {
	dev lis3dh.Device
	ok  bool
}

// newAccel configures the I2C bus and probes for a LIS3DH. It never panics: if
// no accelerometer responds (wrong board, bad solder joint, etc.) it returns an
// accel with ok=false rather than halting before the panel ever draws. This is
// deliberate — the firmware's purpose is the display stress test, not the
// accelerometer; a missing accel should degrade to neutral gravity, not a black
// screen.
func newAccel() *accel {
	machine.I2C0.Configure(machine.I2CConfig{})
	a := &accel{dev: lis3dh.New(machine.I2C0)}
	for _, addr := range i2cAddrs {
		if err := a.dev.Configure(lis3dh.Config{Address: addr}); err == nil && a.dev.Connected() {
			a.dev.SetRange(lis3dh.RANGE_2_G)
			a.ok = true
			println("gravity: LIS3DH found at I2C address", addr)
			return a
		}
		println("gravity: no LIS3DH at I2C address", addr)
	}
	println("gravity: no accelerometer found; running with neutral gravity")
	return a
}

// Vector maps the board's physical axes to screen space (+x right, +y down).
// On the Matrix Portal M4 the chip's axes align directly with screen space
// (verified on hardware: tilting the panel's physical bottom downward pulls
// particles down on screen). If tilt steers the wrong way on different
// hardware, flip the sign of the corresponding axis here.
func (a *accel) Vector() (x, y float32) {
	if !a.ok {
		return 0, 0
	}
	ax, ay, _, err := a.dev.ReadAcceleration()
	if err != nil {
		return 0, 0
	}
	return float32(ax) / 1e6, float32(ay) / 1e6
}
