//go:build tinygo

package main

import "machine"

// buttons reads the Matrix Portal M4's Up and Down buttons and implements
// gravity.Control: Up = +1 (more particles), Down = -1 (fewer). Both are
// active-low with an internal pull-up; edge detection plus per-frame polling
// debounces the mechanical contacts.
type buttons struct {
	up, down         machine.Pin
	upPrev, downPrev bool
}

func newButtons() *buttons {
	machine.BUTTON_UP.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	machine.BUTTON_DOWN.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	return &buttons{up: machine.BUTTON_UP, down: machine.BUTTON_DOWN}
}

// StressStep reports +1 on an Up press edge, -1 on a Down press edge, 0
// otherwise. If both are pressed on the same frame, Down wins.
func (b *buttons) StressStep() int {
	up := !b.up.Get()
	down := !b.down.Get()
	step := 0
	if up && !b.upPrev {
		step = +1
	}
	if down && !b.downPrev {
		step = -1
	}
	b.upPrev, b.downPrev = up, down
	return step
}
