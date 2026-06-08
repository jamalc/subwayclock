// Package subwayclock is a NYC subway arrival-time display for a HUB75 LED
// matrix.
//
// The system has two halves:
//
//   - A backend service (cmd/serve) merges the MTA's static GTFS data with its
//     GTFS-realtime feeds and exposes per-stop arrivals over a small HTTP API.
//     cmd/fetch downloads the static GTFS zips it needs.
//   - A display app polls that API and animates arrivals on a 64x32 panel. It
//     runs as TinyGo firmware on an Adafruit Matrix Portal M4 (cmd/clock) or in
//     a host simulator rendered with Ebiten (cmd/simulate). cmd/conway (Game of
//     Life) and cmd/gravity (an accelerometer-driven particle field that
//     stress-tests the panel driver) are toys that drive the same panel.
//
// The portable display logic lives in internal/clock, internal/conway, and
// internal/gravity and is shared by the device and the simulator through a small
// Display interface that both hub75.Device (TinyGo) and simulator.Display (host)
// satisfy.
//
// See README.md for build and run instructions, and the cmd/*/config.example.*
// files for configuration.
package subwayclock
