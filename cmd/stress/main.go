//go:build tinygo

// Command stress drives worst-case test patterns at the panel to verify
// HUB75 shift-register data timing (see internal/stress).
//
// Use it to check the panel's data-setup timing: flash this, let it run warm
// for a few minutes, and watch the per-channel stripe frames (green especially)
// for pixel corruption. Sparse apps like conway and clock won't surface the
// margin; this does.
//
// Each pattern holds for holdPerPattern before auto-advancing; the board's Up
// and Down buttons step forward/back through them manually.
//
// Usage:
//
//	make stress
package main

import (
	_ "embed"
	"machine"
	"time"

	"github.com/jamalc/subwayclock/internal/config"
	"github.com/jamalc/subwayclock/internal/hub75"
	"github.com/jamalc/subwayclock/internal/hub75/boards"
	"github.com/jamalc/subwayclock/internal/input"
	"github.com/jamalc/subwayclock/internal/stress"
)

// holdPerPattern is how long each pattern stays on screen before auto-advancing
// when the buttons aren't touched. Long enough to study a pattern warm.
const holdPerPattern = 10 * time.Second

// buttons reads the Matrix Portal M4's Up and Down buttons as an input.Source.
// Both are active-low with an internal pull-up (pressed connects to ground), so
// a pressed button reads as false.
type buttons struct {
	up, down         machine.Pin
	upPrev, downPrev bool
}

func newButtons() *buttons {
	machine.BUTTON_UP.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	machine.BUTTON_DOWN.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	return &buttons{up: machine.BUTTON_UP, down: machine.BUTTON_DOWN}
}

// Poll reports input.Up on an Up press edge, input.Down on a Down press edge,
// and input.None otherwise. Edge detection (press, not hold) plus per-frame
// polling debounces the mechanical contacts. If both are pressed on the same
// frame, Down wins.
func (b *buttons) Poll() input.Button {
	up := !b.up.Get()
	down := !b.down.Get()
	out := input.None
	if up && !b.upPrev {
		out = input.Up
	}
	if down && !b.downPrev {
		out = input.Down
	}
	b.upPrev, b.downPrev = up, down
	return out
}

// configData is the config baked into the firmware at build time. The device
// has no filesystem, so config can't be read from disk at runtime — it must be
// embedded. See internal/config.Parse.
//
//go:embed config.txt
var configData []byte

func main() {
	cfg := config.Config{}
	cfg.Parse(configData)
	if cfg.Width == 0 || cfg.Height == 0 {
		panic("config must specify width and height")
	}

	panel := hub75.New(boards.MatrixPortalM4Pins())
	if err := panel.Configure(hub75.Config{
		Width:        cfg.Width,
		Height:       cfg.Height,
		DoubleBuffer: true,
	}); err != nil {
		panic(err)
	}

	stress.Run(panel, cfg.Width, cfg.Height, holdPerPattern, newButtons())
}
