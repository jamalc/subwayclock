package simulator_test

import (
	"image/color"
	"sync"
	"testing"

	"github.com/jamalc/subwayclock/internal/simulator"
)

func TestSize(t *testing.T) {
	d := simulator.New(64, 32)
	w, h := d.Size()
	if w != 64 || h != 32 {
		t.Fatalf("Size() = (%d, %d), want (64, 32)", w, h)
	}
}

func TestWidthHeight(t *testing.T) {
	d := simulator.New(64, 32)
	if d.Width() != 64 {
		t.Errorf("Width() = %d, want 64", d.Width())
	}
	if d.Height() != 32 {
		t.Errorf("Height() = %d, want 32", d.Height())
	}
}

func TestSetPixelAndCopyPixels(t *testing.T) {
	d := simulator.New(4, 4)
	red := color.RGBA{R: 255, A: 255}
	d.SetPixel(1, 2, red)
	pixels := d.CopyPixels()
	if got := pixels[2*4+1]; got != red {
		t.Errorf("pixel at (1,2) = %v, want %v", got, red)
	}
}

func TestSetPixelOutOfBounds(t *testing.T) {
	d := simulator.New(4, 4)
	red := color.RGBA{R: 255, A: 255}
	d.SetPixel(-1, 0, red)
	d.SetPixel(4, 0, red)
	d.SetPixel(0, -1, red)
	d.SetPixel(0, 4, red)
	for _, p := range d.CopyPixels() {
		if p != (color.RGBA{}) {
			t.Errorf("out-of-bounds SetPixel modified a pixel: %v", p)
		}
	}
}

func TestClear(t *testing.T) {
	d := simulator.New(4, 4)
	d.SetPixel(0, 0, color.RGBA{R: 255, A: 255})
	d.Clear()
	for _, p := range d.CopyPixels() {
		if p != (color.RGBA{}) {
			t.Errorf("Clear left a non-zero pixel: %v", p)
		}
	}
}

func TestCopyPixelsIsACopy(t *testing.T) {
	d := simulator.New(4, 4)
	pixels := d.CopyPixels()
	pixels[0] = color.RGBA{R: 255, A: 255}
	fresh := d.CopyPixels()
	if fresh[0] != (color.RGBA{}) {
		t.Error("CopyPixels returned a reference, not a copy")
	}
}

func TestDisplay(t *testing.T) {
	d := simulator.New(4, 4)
	if err := d.Display(); err != nil {
		t.Errorf("Display() = %v, want nil", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	d := simulator.New(4, 4)
	var wg sync.WaitGroup
	red := color.RGBA{R: 255, A: 255}
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); d.SetPixel(0, 0, red) }()
		go func() { defer wg.Done(); _ = d.CopyPixels() }()
	}
	wg.Wait()
}
