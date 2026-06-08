//go:build tinygo

// Command gravity is an accelerometer-driven particle-field stress test for the
// HUB75 panel. It redraws every pixel and swaps buffers every frame, exercising
// the driver's double-buffer and refresh-timing margin under continuous load.
// Tilt the board to steer gravity; the Up/Down buttons add/remove particles to
// dial the stress level. Effective FPS prints over USB serial.
//
// Usage:
//
//	make gravity
package main

import (
	_ "embed"

	"github.com/jamalc/subwayclock/internal/config"
	"github.com/jamalc/subwayclock/internal/gravity"
	"github.com/jamalc/subwayclock/internal/hub75"
	"github.com/jamalc/subwayclock/internal/hub75/boards"
)

// configData is the config baked into the firmware at build time. The device
// has no filesystem, so config must be embedded. See internal/config.Parse.
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

	gravity.Run(cfg, panel, newAccel(), newButtons())
}
