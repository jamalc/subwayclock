package clock

import (
	"image/color"
	"strings"

	"tinygo.org/x/tinydraw"
	"tinygo.org/x/tinyfont"

	"github.com/jamalc/subwayclock/internal/api"
	"github.com/jamalc/subwayclock/internal/fonts"
)

// slotHeight is the vertical space each arrival group occupies on the display.
const slotHeight = 16

// rowY returns the Picopixel baseline y-coordinate for the given row index.
// Each row is 8px tall; +5 positions the baseline within the row for Picopixel.
func rowY(row int) int16 { return int16(row*8 + 5) }

// trimEtas trims the ETA list to fit within maxWidth pixels, dropping trailing
// entries.
func trimEtas(etas []string, maxWidth int) string {
	etasStr := strings.Join(etas, " ")
	_, outboxWidth := tinyfont.LineWidth(&tinyfont.Picopixel, etasStr)
	for outboxWidth > uint32(maxWidth) && len(etas) > 0 {
		etas = etas[:len(etas)-1]
		etasStr = strings.Join(etas, " ")
		_, outboxWidth = tinyfont.LineWidth(&tinyfont.Picopixel, etasStr)
	}
	return etasStr
}

// etaText is the ETA line for a row: the trimmed ETA list, or "No service"
// when the route has no arrivals (empty ETAs).
func etaText(etas []string, maxWidth int) string {
	if len(etas) == 0 {
		return "No service"
	}
	return trimEtas(etas, maxWidth)
}

// group draws one arrival group at the given slot position.
// Callers must call display.Clear() before and display.Display() after.
func group(display Display, g api.FlatArrivalGroup, textPhase int, position int) {
	w, _ := display.Size()
	yoffset := int16(position * slotHeight)
	filledRoundedRectangle(display, 1, 2+yoffset, 12, 12, 4, routeColor(g.Route))
	tinyfont.WriteLine(display, &fonts.RouteSign, 5, 11+yoffset, g.Route, white)
	switch textPhase {
	case 0:
		tinyfont.WriteLine(display, &tinyfont.Picopixel, 14, rowY(0)+yoffset, g.StopName, grey)
	case 1:
		tinyfont.WriteLine(display, &tinyfont.Picopixel, 14, rowY(0)+yoffset, g.RouteName, grey)
	case 2:
		tinyfont.WriteLine(display, &tinyfont.Picopixel, 14, rowY(0)+yoffset, g.Headsign, grey)
	}
	tinyfont.WriteLine(display, &tinyfont.Picopixel, 14, rowY(1)+yoffset, etaText(g.ETAs, int(w)-15), gold)
}

// filledRoundedRectangle draws a filled rounded rectangle of width w and
// height h at (x, y), with corner radius r clamped to fit.
func filledRoundedRectangle(display Display, x int16, y int16, w int16, h int16, r int16, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	if r < 0 {
		r = 0
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}

	for cy := y + r; cy < y+h-r; cy++ {
		tinydraw.Line(display, x, cy, x+w-1, cy, c)
	}

	// For each corner row dy in [0, r), find the inset = r - u, where u is
	// the largest non-negative integer with u*u + v*v <= r*r and v = r - dy.
	// u is monotonically non-decreasing as dy grows, so the inner loop is
	// amortized O(r) across the whole outer loop.
	u := int16(0)
	for dy := int16(0); dy < r; dy++ {
		v := r - dy
		threshold := r*r - v*v
		for (u+1)*(u+1) <= threshold {
			u++
		}
		inset := r - u
		if 2*inset >= w {
			continue
		}
		tinydraw.Line(display, x+inset, y+dy, x+w-1-inset, y+dy, c)
		tinydraw.Line(display, x+inset, y+h-1-dy, x+w-1-inset, y+h-1-dy, c)
	}
}

// wrapString splits s into lines that each fit within maxWidth pixels when
// drawn with font.
func wrapString(s string, maxWidth uint32, font *tinyfont.Font) []string {
	var lines []string
	var currentLine string
	currentWidth := uint32(0)

	for _, r := range s {
		_, charWidth := tinyfont.LineWidth(font, string(r))
		if currentWidth+charWidth > maxWidth && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = ""
			currentWidth = 0
		}
		currentLine += string(r)
		currentWidth += charWidth
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// Info clears the display and draws an informational message.
func Info(display Display, msg string) {
	display.Clear()
	w, _ := display.Size()
	for i, s := range wrapString(msg, uint32(w), &tinyfont.Picopixel) {
		tinyfont.WriteLine(display, &tinyfont.Picopixel, 0, rowY(i), s, green)
	}
	display.Display()
	println(msg)
}

// Warning clears the display and draws a Warning message.
func Warning(display Display, msg string) {
	display.Clear()
	w, _ := display.Size()
	for i, s := range wrapString(msg, uint32(w), &tinyfont.Picopixel) {
		tinyfont.WriteLine(display, &tinyfont.Picopixel, 0, rowY(i), s, yellow)
	}
	display.Display()
	println(msg)
}

// Error clears the display and draws an error message.
func Error(display Display, msg string) {
	display.Clear()
	w, _ := display.Size()
	for i, s := range wrapString(msg, uint32(w), &tinyfont.Picopixel) {
		tinyfont.WriteLine(display, &tinyfont.Picopixel, 0, rowY(i), s, grey)
	}
	display.Display()
	println(msg)
}
