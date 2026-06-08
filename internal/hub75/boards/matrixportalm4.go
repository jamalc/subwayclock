//go:build matrixportal_m4

// Board file for the Adafruit Matrix Portal M4. Maps the product's HUB75
// connector to driver pins. The MCU primitives it runs on live in
// platform_samd51.go (the M4 is a SAMD51 board).
package boards

import (
	"machine"

	"github.com/jamalc/subwayclock/internal/hub75"
)

// MatrixPortalM4Pins returns the HUB75 connector pin assignments
// for the Adafruit Matrix Portal M4.
//
// All 14 pins are on GPIO port group B, which is what makes the
// driver's bulk-write shift loop efficient.
func MatrixPortalM4Pins() hub75.Pins {
	return hub75.Pins{
		R1: hub75.Pin(machine.HUB75_R1),
		G1: hub75.Pin(machine.HUB75_G1),
		B1: hub75.Pin(machine.HUB75_B1),
		R2: hub75.Pin(machine.HUB75_R2),
		G2: hub75.Pin(machine.HUB75_G2),
		B2: hub75.Pin(machine.HUB75_B2),
		Address: []hub75.Pin{
			hub75.Pin(machine.HUB75_ADDR_A),
			hub75.Pin(machine.HUB75_ADDR_B),
			hub75.Pin(machine.HUB75_ADDR_C),
			hub75.Pin(machine.HUB75_ADDR_D),
			hub75.Pin(machine.HUB75_ADDR_E),
		},
		Clock: hub75.Pin(machine.HUB75_CLK),
		Latch: hub75.Pin(machine.HUB75_LAT),
		OE:    hub75.Pin(machine.HUB75_OE),
	}
}
