// Package input defines the hardware-agnostic navigation port shared by the
// apps that take discrete Up/Down input (clock, stress). A keyboard maps to
// it just as well as a GPIO button: the platform adapter reports which
// control fired; the app decides what it means.
//
// This package imports nothing, so depending on it adds no weight to a device
// firmware build.
package input

// Button is a logical navigation control reported by a Source.
type Button int

const (
	None Button = iota
	Up
	Down
)

// Source is the port for user navigation input. It is polled once per loop
// tick and reports any control actuated since the last poll. Adapters live in
// the platform layer (device GPIO, simulator keyboard).
type Source interface {
	Poll() Button
}
