//go:build tinygo

// Command conway is a Conway's Game of Life animation for testing the display.
//
// Usage:
//
//	go run ./cmd/conway [-config path/to/config.txt]
package main

import (
	_ "embed"

	"github.com/jamalc/subwayclock/internal/config"
	"github.com/jamalc/subwayclock/internal/conway"
	"github.com/jamalc/subwayclock/internal/hub75"
	"github.com/jamalc/subwayclock/internal/hub75/boards"
)

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

	conway.Run(cfg, panel)
}
