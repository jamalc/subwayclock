package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/jamalc/subwayclock/internal/input"
)

// keyboard adapts the arrow keys to an input.Source, standing in for the
// device's UP/DOWN buttons. Ebiten input must be read from the Update loop, so
// capture() runs there and hands events to Poll() (called from the Run
// goroutine) through a small buffered channel.
type keyboard struct {
	events chan input.Button
}

func newKeyboard() *keyboard {
	return &keyboard{events: make(chan input.Button, 8)}
}

// capture detects key presses. Call once per Ebiten Update tick.
func (k *keyboard) capture() {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		k.send(input.Up)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		k.send(input.Down)
	}
}

func (k *keyboard) send(b input.Button) {
	select {
	case k.events <- b:
	default: // buffer full: drop, the user is mashing faster than the loop ticks
	}
}

// Poll implements input.Source.
func (k *keyboard) Poll() input.Button {
	select {
	case b := <-k.events:
		return b
	default:
		return input.None
	}
}
