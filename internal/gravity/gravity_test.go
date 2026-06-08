package gravity

import (
	"image/color"
	"testing"
)

// testField builds a deterministic field with no particles and an allocated
// trail buffer, with physics constants set explicitly so tests don't depend on
// the package defaults.
func testField(w, h int) *field {
	return &field{
		w: w, h: h,
		trail:       make([]color.RGBA, w*h),
		decay:       0.5,
		restitution: 0.5,
		gain:        100,
		dt:          0.1,
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestGravityAccelerates(t *testing.T) {
	f := testField(8, 8)
	f.parts = []particle{{x: 4, y: 1, r: 255, g: 255, b: 255}}

	f.step(0, 1) // 1g downward (+y)
	vy1, y1 := f.parts[0].vy, f.parts[0].y
	if vy1 <= 0 {
		t.Fatalf("expected downward velocity after gravity, got vy=%v", vy1)
	}

	f.step(0, 1)
	if f.parts[0].vy <= vy1 {
		t.Errorf("expected velocity to keep increasing: %v then %v", vy1, f.parts[0].vy)
	}
	if f.parts[0].y <= y1 {
		t.Errorf("expected y to keep increasing: %v then %v", y1, f.parts[0].y)
	}
}

func TestWallBounceDamps(t *testing.T) {
	f := testField(8, 8)
	// Just above the floor, moving down fast, no gravity (level board).
	f.parts = []particle{{x: 4, y: 7.0, vy: 5, r: 255}} // vx=0, level board: a pure vertical bounce

	f.step(0, 0)
	p := f.parts[0]
	if p.vy >= 0 {
		t.Errorf("expected velocity reversed after bounce, got vy=%v", p.vy)
	}
	if abs32(p.vy) >= 5 {
		t.Errorf("expected speed to drop after damped bounce, got |vy|=%v", abs32(p.vy))
	}
	if p.y < 0 || p.y > float32(f.h-1) {
		t.Errorf("particle left bounds: y=%v", p.y)
	}
}

func TestTrailFadesToBlack(t *testing.T) {
	f := testField(4, 4)
	f.trail[5] = color.RGBA{100, 100, 100, 255}

	f.step(0, 0) // no particles: pixel only fades
	if f.trail[5].R >= 100 {
		t.Fatalf("expected trail to fade, got R=%d", f.trail[5].R)
	}
	// 64 is well above the ~7 steps a decay of 0.5 needs to reach black.
	for i := 0; i < 64 && f.trail[5] != (color.RGBA{}); i++ {
		f.step(0, 0)
	}
	if f.trail[5] != (color.RGBA{}) {
		t.Errorf("expected trail to reach black, got %+v", f.trail[5])
	}
}

func TestLevelBoardNoDrift(t *testing.T) {
	f := testField(8, 8)
	f.parts = []particle{{x: 4, y: 4, r: 255}}
	for i := 0; i < 10; i++ {
		f.step(0, 0)
	}
	if f.parts[0].vy != 0 || f.parts[0].vx != 0 {
		t.Errorf("expected no drift on level board, got v=(%v,%v)", f.parts[0].vx, f.parts[0].vy)
	}
}

func TestAdjustCountClamps(t *testing.T) {
	f := testField(4, 4) // capacity 16
	f.adjustCount(10)
	if len(f.parts) != 10 {
		t.Fatalf("expected 10 particles, got %d", len(f.parts))
	}
	f.adjustCount(1000) // clamp to w*h
	if len(f.parts) != 16 {
		t.Fatalf("expected clamp to 16, got %d", len(f.parts))
	}
	f.adjustCount(-1000) // clamp to 0
	if len(f.parts) != 0 {
		t.Fatalf("expected clamp to 0, got %d", len(f.parts))
	}
}
