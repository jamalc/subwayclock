//go:build tinygo

package main

import (
	"machine"

	"github.com/jamalc/subwayclock/internal/input"
)

// buttons reads the Matrix Portal M4's on-board UP/DOWN buttons as an
// input.Source. The buttons are wired active-low (pressed = pin low), so they
// use the internal pull-up. Poll is edge-triggered: it reports a button only
// on the press (high→low) transition, so holding one doesn't repeat.
type buttons struct {
	up, down                 machine.Pin
	prevUpDown, prevDownDown bool // true = currently pressed
}

// newButtons configures the button pins and returns the adapter. Call once at
// startup.
func newButtons() *buttons {
	b := &buttons{up: machine.BUTTON_UP, down: machine.BUTTON_DOWN}
	b.up.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	b.down.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	return b
}

// Poll implements input.Source. It returns the button pressed since the last
// call, if any, updating internal state for edge detection.
func (b *buttons) Poll() input.Button {
	upDown := !b.up.Get()     // active low
	downDown := !b.down.Get() // active low

	var out input.Button
	switch {
	case upDown && !b.prevUpDown:
		out = input.Up
	case downDown && !b.prevDownDown:
		out = input.Down
	default:
		out = input.None
	}

	b.prevUpDown = upDown
	b.prevDownDown = downDown
	return out
}
