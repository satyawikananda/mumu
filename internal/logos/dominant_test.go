package logos

import (
	"image"
	"image/color"
	"testing"
)

func TestDominantColorUsesArtworkBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{R: 20, G: 80, B: 220, A: 255})
		}
	}

	got := DominantColor(img)
	if got == NeutralArtwork {
		t.Fatal("blue artwork fell back to neutral")
	}
	if got.A != 255 {
		t.Fatalf("alpha = %d, want 255", got.A)
	}
	if whiteContrast(got) < 4.5 {
		t.Fatalf("white contrast %.2f, want at least 4.5", whiteContrast(got))
	}
}

func TestDominantColorRejectsGrayAndTransparent(t *testing.T) {
	white := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			white.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	if got := DominantColor(white); got != NeutralArtwork {
		t.Fatalf("white artwork = %+v, want neutral", got)
	}

	empty := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if got := DominantColor(empty); got != NeutralArtwork {
		t.Fatalf("transparent artwork = %+v, want neutral", got)
	}
	if got := DominantColor(nil); got != NeutralArtwork {
		t.Fatalf("nil artwork = %+v, want neutral", got)
	}
}

func TestNormalizeArtworkDarkensBrightTint(t *testing.T) {
	got := NormalizeArtwork(color.RGBA{R: 255, G: 220, B: 0, A: 255})
	if got == NeutralArtwork {
		t.Fatal("saturated yellow fell back to neutral")
	}
	if luminance(got) > 0.15 {
		t.Fatalf("luminance %.3f, want at most 0.15", luminance(got))
	}
	if whiteContrast(got) < 4.5 {
		t.Fatalf("white contrast %.2f, want at least 4.5", whiteContrast(got))
	}
}

func TestDominantColorIgnoresTransparentPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{R: 220, G: 30, B: 30, A: 0})
		}
	}
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 30, G: 120, B: 220, A: 255})
		}
	}

	got := DominantColor(img)
	if got.R > got.B {
		t.Fatalf("dominant color %+v kept transparent red", got)
	}
}

func whiteContrast(c color.RGBA) float64 {
	return 1.05 / (luminance(c) + 0.05)
}
