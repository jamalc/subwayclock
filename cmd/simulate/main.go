// Command simulate is a desktop application to simulate the clock display and
// test the clock, conway, gravity, and stress apps.
//
// It uses the Ebiten game library to create a window and render the display
// output from the clock and conway packages. It also captures keyboard input
// to simulate button presses for the clock app.
//
// Usage:
//
//	go run ./cmd/simulate [-config path/to/config.txt] (clock|conway|gravity|stress)
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/jamalc/subwayclock/internal/clock"
	"github.com/jamalc/subwayclock/internal/config"
	"github.com/jamalc/subwayclock/internal/conway"
	"github.com/jamalc/subwayclock/internal/gravity"
	"github.com/jamalc/subwayclock/internal/simulator"
	"github.com/jamalc/subwayclock/internal/stress"
)

var (
	configPath = flag.String("config", "cmd/simulate/config.txt", "path to config file")
)

// holdPerPattern matches the device auto-advance interval in cmd/stress: each
// stress pattern stays on screen this long before advancing when the arrow
// keys aren't touched.
const holdPerPattern = 10 * time.Second

// inputCapturer is anything that reads Ebiten input once per Update tick.
type inputCapturer interface {
	capture()
}

// game implements the Ebiten Game interface, holding the display, keyboard
// input, and render scale for the running app.
type game struct {
	title string
	disp  *simulator.Display
	scale int
	img   *ebiten.Image
	pix   []byte // reused each frame: RGBA bytes for WritePixels
	input inputCapturer
}

// Update polls the keyboard once per tick. It implements ebiten.Game.
func (g *game) Update() error {
	if g.input != nil {
		g.input.capture()
	}
	return nil
}

// Draw copies the simulator's pixel buffer into an Ebiten image and blits it
// to the screen, scaled by g.scale.
func (g *game) Draw(screen *ebiten.Image) {
	pixels := g.disp.CopyPixels()
	for i, c := range pixels {
		g.pix[i*4] = c.R
		g.pix[i*4+1] = c.G
		g.pix[i*4+2] = c.B
		g.pix[i*4+3] = 0xff
	}
	g.img.WritePixels(g.pix)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(g.scale), float64(g.scale))
	screen.DrawImage(g.img, op)
}

// Layout reports the logical screen size: the display dimensions scaled by
// g.scale.
func (g *game) Layout(_, _ int) (int, int) {
	return g.disp.Width() * g.scale, g.disp.Height() * g.scale
}

func main() {
	scale := flag.Int("scale", 8, "pixel scale factor (default 8 → 512×256 for a 64×32 panel)")
	flag.Parse()

	if *scale < 0 {
		log.Fatalf("--scale must be >= 0, got %d", *scale)
	}

	app := flag.Arg(0)
	if app == "" {
		app = "clock"
	}
	if app != "clock" && app != "conway" && app != "gravity" && app != "stress" {
		log.Fatalf("unknown app %q (valid: clock, conway, gravity, stress)", app)
	}

	cfg := config.Config{}
	if err := cfg.Load(*configPath); err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		log.Fatal("config must specify width and height")
	}

	display := simulator.New(cfg.Width, cfg.Height)

	g := &game{
		disp:  display,
		scale: *scale,
		img:   ebiten.NewImage(cfg.Width, cfg.Height),
		pix:   make([]byte, cfg.Width*cfg.Height*4),
	}

	switch app {
	case "clock":
		kb := newKeyboard()
		g.input = kb
		g.title = "Subway Clock"
		go clock.Run(cfg, display, kb)
	case "conway":
		g.title = "Conway's Game of Life"
		go conway.Run(cfg, display)
	case "gravity":
		pad := newTiltPad()
		g.input = pad
		g.title = "Gravity"
		go gravity.Run(cfg, display, pad, pad)
	case "stress":
		kb := newKeyboard()
		g.input = kb
		g.title = "HUB75 Stress"
		go stress.Run(display, cfg.Width, cfg.Height, holdPerPattern, kb)
	}

	ebiten.SetWindowSize(cfg.Width**scale, cfg.Height**scale)
	ebiten.SetWindowTitle(g.title + fmt.Sprintf(" — %s (%dx%d @ %dx)", app, cfg.Width, cfg.Height, *scale))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
