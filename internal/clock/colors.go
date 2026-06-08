package clock

import "image/color"

// MTA line colors plus a few UI accents, used when drawing arrival groups and
// status messages. Internal to the clock package.
var (
	blue    = color.RGBA{0, 57, 166, 255}
	orange  = color.RGBA{255, 99, 25, 255}
	green   = color.RGBA{108, 190, 69, 255}
	brown   = color.RGBA{153, 102, 51, 255}
	grey    = color.RGBA{167, 169, 172, 255}
	yellow  = color.RGBA{252, 204, 10, 255}
	red     = color.RGBA{238, 53, 46, 255}
	dkGreen = color.RGBA{0, 147, 60, 255}
	purple  = color.RGBA{185, 51, 173, 255}

	white = color.RGBA{255, 255, 255, 255}
	gold  = color.RGBA{212, 175, 55, 255}
)

// routeColor returns the MTA line color for a route ID.
func routeColor(id string) color.RGBA {
	if len(id) == 0 {
		return grey
	}
	switch id[0] {
	case 'A', 'C', 'E':
		return blue
	case 'B', 'D', 'F', 'M':
		return orange
	case 'G':
		return green
	case 'J', 'Z':
		return brown
	case 'N', 'Q', 'R', 'W':
		return yellow
	case '1', '2', '3':
		return red
	case '4', '5', '6':
		return dkGreen
	case '7':
		return purple
	default:
		return grey
	}
}
