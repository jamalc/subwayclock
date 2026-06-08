# hub75

A TinyGo driver for HUB75 RGB LED matrix panels. It implements
[`drivers.Displayer`](https://pkg.go.dev/tinygo.org/x/drivers#Displayer), so it
works with any TinyGo display library that targets that interface (`tinydraw`,
`tinyfont`, etc.).

Currently targets **SAMD51** (developed on the Adafruit Matrix Portal M4). The
driver core is chip-independent; per-chip primitives live in `platform_*.go` and
per-product pin maps in `board_*.go`, both behind build tags, so porting to
another MCU or board is additive (see [Porting](#porting-to-a-new-chip)).

See the package doc comment in [`hub75.go`](hub75.go) (or pkg.go.dev) for the
full API and a runnable usage example. This README covers the *why* — design,
constraints, and the decisions a contributor or reuser needs to know.

## Quick start

```go
panel := hub75.New(hub75.Pins{ /* R1,G1,B1,R2,G2,B2, Address[], Clock,Latch,OE */ })
if err := panel.Configure(hub75.Config{Width: 64, Height: 32, DoubleBuffer: true}); err != nil {
    panic(err)
}
for {
    panel.Clear()
    drawFrame(panel)        // SetPixel via tinydraw/tinyfont
    panel.Display()         // atomic swap; blocks until shown
}
```

All signal pins **must be on the same GPIO port group** (see
[Constraints](#constraints)). `New` panics with `ErrMixedPortGroups` otherwise.

## How it works

- **Grayscale via bit-angle modulation (BCM).** Each color channel is split into
  bitplanes; plane *p* is displayed for a duration weighted 2^p. Gamma is applied
  (a cheap sqrt-based curve) and folded into the precomputed bitplanes.
- **Work is front-loaded into `SetPixel`.** Gamma lookup, bitplane decomposition,
  and packing each pixel's bits into GPIO-port-register positions all happen at
  draw time and land in the per-`Device` `frames` buffers. This is the right
  trade when refresh rate ≫ draw rate, which is the typical embedded case.
- **Refresh is a timer-ISR + software shift loop.** A hardware timer fires
  `bitDepth × (Height/2)` times per frame; each tick latches the
  previously-clocked row (`latchRow`) and sets that displayed plane's on-time,
  then clocks the next row into the shift registers while the panel stays lit
  (`clockRow`). Because the pixel values are precomputed, the ISR does **no
  computation** — but it still performs O(width) port writes per tick to clock
  the data out.
- **Optional double buffering.** `Display()` requests an atomic front/back swap
  that the ISR performs at the start of a frame, then blocks until it completes.
- **One active Device.** The design claims a single suitable timer, so only one
  `Device` can be configured at a time; a second `Configure` returns
  `ErrAlreadyInitialized`. `Pause`/`Resume` stop and restart refresh without
  losing buffer contents.

There is a minimum per-plane on-time floor (~200 timer ticks ≈ 27 µs at 7.5 MHz
on the M4). It ensures the next row finishes clocking within the displayed
plane's on-time window so the latch has valid data on the following tick;
the cost is reduced color resolution at the dim end (the lowest planes get
truncated up to the floor). This is a deliberate trade against a visible
"row 0 brighter" artifact on chained panels.

## Constraints

**All signal pins share one GPIO port group.** The shift loop's speed comes from
writing every signal (6 RGB bits + clock + latch + OE + address) with a single
atomic port store. That requires them to live in one 32-bit port group. This is
not an idiosyncrasy — it's intrinsic to this class of driver (Adafruit
Protomatter has the same requirement) and it's what the
[parallel-chains non-goal](#non-goals) follows from.

**Panel geometry.** `Width` is the total daisy-chain width (power of 2, 4..256);
`Height` is one panel's height (power of 2, 4..64, and ≤ 2^(addressPins+1)).
`BitDepth` is 1..6 (default 6).

## Refresh: timer-ISR shift loop

The current refresh path bit-bangs the data out from a timer ISR. This is
**deliberate**: it's simple, self-contained, has no external dependencies beyond
the timer and PORT registers, and is cheap enough for this driver's intended
workloads. The shift loop is the dominant continuous CPU cost, but front-loading
removed the *computation* from the ISR, so what remains is just I/O.

## Non-goals

- **Parallel chains** (multiple simultaneous RGB data buses, as on Raspberry Pi
  HUB75 hats). The single-port-group atomic write — the basis of the shift loop's
  performance — requires all signals on one 32-bit port, and the supported boards
  don't expose enough same-port GPIO for additional chains. To drive more pixels,
  lengthen the daisy-chain (trading refresh rate) or use a coordinate-remap layer
  for [serpentine grids](#roadmap). Parallel output is a Pi/FPGA-class feature.
- **Non-standard scan types** (1/4, 1/8 "ABC"-style outdoor panels with nonlinear
  internal pixel order). The driver assumes standard 1/(Height÷2) indoor scan.
  These panels need bespoke per-panel remaps and are out of scope.

## Roadmap

Not yet implemented, roughly in order of priority:

- **Serpentine / grid chaining.** A single horizontal row of panels already works
  via `Width`. Anything taller needs a coordinate-remap layer: keep the core
  `Device` as one linear electrical chain and add a thin `Displayer` wrapper that
  translates logical (x, y) to the physical chain coordinate, flipping alternate
  tile rows. This keeps the timing-critical core untouched and is independently
  testable.
- **Runtime brightness control.** The machinery exists — scaling `planeOnTimes`
  (or inserting blanking) gives a brightness knob without touching the
  decomposition path.
- **More chip/board ports.** The `platform_*.go` / `board_*.go` contract is in place; ports are additive.
- **`Close`/`Deinit`** to release the timer and clear the active-device singleton
  (useful for reuse and tests; current code only offers `Pause`/`Resume`).
- **Configurable gamma** (currently a fixed sqrt-based curve).

## Porting to a new chip

Two tiers, split by build tag:

- **MCU (chip):** add a `platform_<chip>.go` implementing the platform contract
  documented at the top of [`hub75.go`](hub75.go): the `portGroup` type, the
  pin/port primitives (`platformPortGroupForPin`, `platformPortBitForPin`,
  `platformPortGroup`, `platformConfigureOutputs`), the short delay
  (`platformShortDelay`), and the timer primitives (`platformStartTimer`,
  `platformSetNextDuration`, `platformPauseTimer`, `platformResumeTimer`).
- **Board (product):** add a `board_<product>.go` exposing a `Pins` constructor
  (like `MatrixPortalM4`) that maps the product's HUB75 connector to driver pins.

The driver core in `hub75.go` is otherwise chip-independent. `platform_host.go`
provides a machine-free fake port + controllable timer for host (`!tinygo`)
builds, which is what lets the driver suite run under plain `go test`.

## References

- [Adafruit Protomatter](https://github.com/adafruit/Adafruit_Protomatter) — the
  C reference for this exact job (the engine behind CircuitPython's
  `rgbmatrix.RGBMatrix` and the Arduino library on the Matrix Portal). Same
  single-port and BCM design.
- [tinygo.org/x/drivers](https://pkg.go.dev/tinygo.org/x/drivers) — the
  `Displayer` interface this driver implements.
