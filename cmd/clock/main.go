//go:build tinygo

// Command clock is the TinyGo firmware for the Matrix Portal M4.
//
// It connects to WiFi, fetches arrival data from the server, and renders it on
// the LED matrix. It also reads the UP/DOWN buttons to scroll through arrival
// groups when there are more than fit on the display. Specify the server
// address and other parameters in the embedded config.txt file.
//
// Usage:
//
//	go run ./cmd/clock [-config path/to/config.txt]
package main

import (
	_ "embed"
	"time"

	"github.com/jamalc/subwayclock/internal/clock"
	"github.com/jamalc/subwayclock/internal/config"
	"github.com/jamalc/subwayclock/internal/hub75"
	"github.com/jamalc/subwayclock/internal/hub75/boards"

	"tinygo.org/x/drivers/netlink"
	"tinygo.org/x/drivers/netlink/probe"
)

// configData is the config baked into the firmware at build time. The device
// has no filesystem, so config can't be read from disk at runtime — it must be
// embedded. See internal/config.Parse.
//
//go:embed config.txt
var configData []byte

// connect connects to WiFi. Blocks until a connection is established,
// retrying up to 5 times every 5 seconds on failure. Call once at startup.
func connect(ssid, pass string) error {
	link, _ := probe.Probe()
	attempts := 0
	for {
		err := link.NetConnect(&netlink.ConnectParams{
			Ssid:       ssid,
			Passphrase: pass,
		})
		if err == nil || err == netlink.ErrConnected {
			return nil
		}
		attempts++
		if attempts >= 5 {
			return err
		}
		time.Sleep(5 * time.Second)
	}
}

func main() {
	var cfg config.Config
	cfg.Parse(configData)

	panel := hub75.New(boards.MatrixPortalM4Pins())
	if err := panel.Configure(hub75.Config{
		Width:        cfg.Width,
		Height:       cfg.Height,
		DoubleBuffer: true,
	}); err != nil {
		panic(err)
	}

	clock.Info(panel, "Connecting to WiFi")
	if err := connect(cfg.SSID, cfg.Passphrase); err != nil {
		clock.Error(panel, "Failed to connect to WiFi: "+err.Error())
		return
	}
	clock.Info(panel, "Connected to WiFi")

	clock.Run(cfg, panel, newButtons())
}
