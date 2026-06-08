// Package simulator provides a software display that stands in for the
// physical HUB75 panel, so the subway clock UI can run on a host machine
// (rendered by Ebiten) instead of on the embedded controller.
package simulator

import (
	"image/color"
	"sync"

	"tinygo.org/x/drivers"
)

// Display is a software implementation of drivers.Displayer backed by a
// flat pixel buffer. Safe for concurrent use: the Ebiten draw loop and the
// render goroutine access it from different goroutines.
type Display struct {
	mu     sync.RWMutex
	pixels []color.RGBA
	// width and height are immutable after construction. SetPixel reads them
	// without holding a lock, which is safe because they are set once in New
	// and never mutated afterward.
	width  int
	height int
}

// New allocates a Display of the given dimensions.
func New(width, height int) *Display {
	if width <= 0 || height <= 0 {
		panic("simulator.New: dimensions must be positive")
	}
	return &Display{
		pixels: make([]color.RGBA, width*height),
		width:  width,
		height: height,
	}
}

// SetPixel implements drivers.Displayer. Out-of-bounds coordinates are
// silently ignored.
func (d *Display) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || int(x) >= d.width || y < 0 || int(y) >= d.height {
		return
	}
	d.mu.Lock()
	d.pixels[int(y)*d.width+int(x)] = c
	d.mu.Unlock()
}

// Size implements drivers.Displayer.
func (d *Display) Size() (int16, int16) {
	return int16(d.width), int16(d.height)
}

// Width returns the display width in pixels.
func (d *Display) Width() int { return d.width }

// Height returns the display height in pixels.
func (d *Display) Height() int { return d.height }

// Clear zeros all pixels.
func (d *Display) Clear() {
	d.mu.Lock()
	clear(d.pixels)
	d.mu.Unlock()
}

// Display is a no-op: in single-buffer mode, SetPixel writes are immediately
// visible.
func (d *Display) Display() error { return nil }

// CopyPixels returns a snapshot of the current pixel buffer, safe to read
// without holding any lock. Row-major order: index = y*width + x.
func (d *Display) CopyPixels() []color.RGBA {
	d.mu.RLock()
	defer d.mu.RUnlock()
	cp := make([]color.RGBA, len(d.pixels))
	copy(cp, d.pixels)
	return cp
}

// Compile-time assertion.
var _ drivers.Displayer = (*Display)(nil)
