# Subway Clock

Real-time NYC subway arrivals on a 64×32 HUB75 LED matrix.

A Go backend merges the MTA's static GTFS data with its GTFS-realtime feeds and
serves per-stop arrivals over a small HTTP API. A display app polls that API and
animates the arrivals on the panel — either as TinyGo firmware on an Adafruit
Matrix Portal M4, or in a host simulator rendered with [Ebiten](https://ebitengine.org/).

```
 device / simulator  ──HTTP──▶  cmd/serve  ──HTTPS──▶  MTA GTFS feeds
```

## Components

| Path             | What it is                                                        | Runs on        |
|------------------|-------------------------------------------------------------------|----------------|
| `cmd/serve`      | Arrivals HTTP API (`/arrivals`, `/stops`, `/stops/search`, `/health`) | native (server) |
| `cmd/fetch`      | Downloads the MTA static GTFS zip files                           | native         |
| `cmd/simulate`   | Host simulator (Ebiten); runs the clock, conway, or gravity       | native         |
| `cmd/clock`      | The subway clock, as device firmware                              | TinyGo (SAMD51)|
| `cmd/conway`     | Conway's Game of Life on the panel, as device firmware            | TinyGo (SAMD51)|
| `cmd/gravity`    | Accelerometer-driven particle field that stress-tests the driver  | TinyGo (SAMD51)|
| `cmd/stress`     | A set of patterns to stress test the hub75 driver                 | TinyGo (SAMD51)|

The portable display logic (`internal/clock`, `internal/conway`, `internal/gravity`)
is shared by the device and the simulator through a small `Display` interface that
both `hub75.Device` (the TinyGo HUB75 driver) and `simulator.Display` satisfy.

## Configuration

Each binary reads a config file next to it; the real files hold secrets and are
gitignored. Copy the example and edit it:

```sh
cp cmd/serve/config.example.yaml    cmd/serve/config.yaml
cp cmd/simulate/config.example.txt  cmd/simulate/config.txt
cp cmd/clock/config.example.txt     cmd/clock/config.txt
```

The clock and simulate configs take a `stops:` line of space-separated stop IDs.
Each stop may add an optional route filter after `:` — a comma-separated list of
tokens:

- a bare route (e.g. `Q`) is a **pin**: always shown, with **"No service"** when
  it isn't arriving;
- `!route` (e.g. `!R`) is a **mute**: never shown, even when arriving.

With no tokens, every arriving route is shown. For example, `R30N:Q,!R` always
shows Q (No service if it's down), hides R, and shows everything else live.

## Quick start

### Backend

```sh
cp cmd/serve/config.example.yaml cmd/serve/config.yaml   # add your MTA API key
go run ./cmd/fetch                                       # download GTFS zips
go run ./cmd/serve                                       # listens on :8080
```

Get an MTA API key at <https://api.mta.info/>.

### Simulator

```sh
cp cmd/simulate/config.example.txt cmd/simulate/config.txt   # set host + stops
go run ./cmd/simulate            # the clock (default)
go run ./cmd/simulate conway     # Conway's Game of Life
go run ./cmd/simulate gravity    # particle-field stress toy (arrow keys tilt, +/- particles)
```

### Device (Adafruit Matrix Portal M4 + 64×32 HUB75 panel)

```sh
cp cmd/clock/config.example.txt cmd/clock/config.txt     # WiFi + host + stops
make clock     # tinygo flash ./cmd/clock
make conway    # tinygo flash ./cmd/conway
make gravity   # tinygo flash ./cmd/gravity (tilt the board to steer; Up/Down change particle count)
make stress    # tinygo flash ./cmd/stress (Up/Down change test pattern)
```

The device has no filesystem, so `config.txt` is embedded into the firmware at
build time — reflash after changing it.

## Requirements

- Go 1.26+
- [TinyGo](https://tinygo.org/) (for the device firmware only)
- An MTA API key
- Hardware: Adafruit Matrix Portal M4 and a 64×32 HUB75 LED panel
