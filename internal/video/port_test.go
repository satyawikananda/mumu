package video

import (
	"math"
	"testing"
)

func TestFit(t *testing.T) {
	tests := []struct {
		name   string
		area   Rect
		aspect float64
		want   Rect
	}{
		{
			name:   "wide area letterboxes on the sides",
			area:   Rect{X: 10, Y: 20, W: 400, H: 100},
			aspect: 16.0 / 9.0,
			want:   Rect{X: 10 + (400-177.77778)/2, Y: 20, W: 177.77778, H: 100},
		},
		{
			name:   "tall area letterboxes top and bottom",
			area:   Rect{W: 320, H: 400},
			aspect: 16.0 / 9.0,
			want:   Rect{X: 0, Y: (400 - 180) / 2, W: 320, H: 180},
		},
		{
			name:   "exact aspect matches the area",
			area:   Rect{X: 5, Y: 6, W: 320, H: 180},
			aspect: 16.0 / 9.0,
			want:   Rect{X: 5, Y: 6, W: 320, H: 180},
		},
		{
			name:   "square video in a wide area",
			area:   Rect{W: 200, H: 100},
			aspect: 1,
			want:   Rect{X: 50, Y: 0, W: 100, H: 100},
		},
		{
			name:   "zero area has no frame",
			area:   Rect{W: 0, H: 100},
			aspect: 16.0 / 9.0,
			want:   Rect{},
		},
		{
			name:   "invalid aspect has no frame",
			area:   Rect{W: 100, H: 100},
			aspect: math.NaN(),
			want:   Rect{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Fit(test.area, test.aspect)
			if !rectNearlyEqual(got, test.want) {
				t.Fatalf("Fit(%+v, %v) = %+v, want %+v", test.area, test.aspect, got, test.want)
			}
		})
	}
}

func TestFitKeepsFrameInsideArea(t *testing.T) {
	area := Rect{X: 3, Y: 4, W: 641, H: 359}

	for _, aspect := range []float64{4.0 / 3.0, 16.0 / 9.0, 21.0 / 9.0, 1, 0.5} {
		frame := Fit(area, aspect)
		if frame.W > area.W || frame.H > area.H {
			t.Fatalf("aspect %v produced %+v outside %+v", aspect, frame, area)
		}
		if frame.X < area.X || frame.Y < area.Y {
			t.Fatalf("aspect %v produced a negative margin: %+v", aspect, frame)
		}
	}
}

func TestPixelSize(t *testing.T) {
	tests := []struct {
		name  string
		frame Rect
		scale float32
		wantW int
		wantH int
	}{
		{name: "retina doubles the drawable", frame: Rect{W: 640, H: 360}, scale: 2, wantW: 1280, wantH: 720},
		{name: "non-retina keeps DIPs", frame: Rect{W: 640, H: 360}, scale: 1, wantW: 640, wantH: 360},
		{name: "fractional frame rounds", frame: Rect{W: 100.4, H: 50.6}, scale: 2, wantW: 201, wantH: 101},
		{name: "unknown scale falls back to 1", frame: Rect{W: 8, H: 4}, scale: 0, wantW: 8, wantH: 4},
		{name: "zero width has no drawable", frame: Rect{W: 0, H: 360}, scale: 2, wantW: 0, wantH: 0},
		{name: "negative height has no drawable", frame: Rect{W: 640, H: -1}, scale: 2, wantW: 0, wantH: 0},
		{name: "sub-pixel frame keeps one pixel", frame: Rect{W: 0.2, H: 0.2}, scale: 1, wantW: 1, wantH: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height := PixelSize(test.frame, test.scale)
			if width != test.wantW || height != test.wantH {
				t.Fatalf("PixelSize(%+v, %v) = %dx%d, want %dx%d", test.frame, test.scale, width, height, test.wantW, test.wantH)
			}
		})
	}
}

func TestOriginY(t *testing.T) {
	frame := Rect{X: 0, Y: 40, W: 320, H: 180}

	if got := originY(frame, 400, true); got != 40 {
		t.Fatalf("flipped origin = %v, want 40", got)
	}
	// AppKit's default content view is not flipped: the origin counts upwards
	// from the bottom edge.
	if got := originY(frame, 400, false); got != 180 {
		t.Fatalf("unflipped origin = %v, want 180", got)
	}
}

func rectNearlyEqual(a, b Rect) bool {
	return nearlyEqual(a.X, b.X) && nearlyEqual(a.Y, b.Y) &&
		nearlyEqual(a.W, b.W) && nearlyEqual(a.H, b.H)
}

const epsilon = 0.001

func nearlyEqual(a, b float32) bool {
	diff := a - b

	return diff < epsilon && diff > -epsilon
}
