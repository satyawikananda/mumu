package logos

import (
	"image"
	"image/color"
	"math"
)

// artworkSample bounds the pixels inspected for one dominant-color pass.
const artworkSample = 32

// NeutralArtwork is the fallback tint for a missing, light or unsaturated
// logo. It keeps white tile text readable.
var NeutralArtwork = color.RGBA{R: 0x23, G: 0x23, B: 0x2B, A: 0xFF}

// DominantColor returns a bounded artwork tint for img. Transparent pixels are
// ignored, the average is sampled at most 32x32 points, and the result is
// darkened until white text passes 4.5:1.
func DominantColor(img image.Image) color.RGBA {
	if img == nil {
		return NeutralArtwork
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return NeutralArtwork
	}

	step := max(1, max(width, height)/artworkSample)
	var red, green, blue, alpha float64
	var count int
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, a := img.At(x, y).RGBA()
			if a < 0x2000 {
				continue
			}
			weight := float64(a) / 0xFFFF
			red += float64(r) * weight
			green += float64(g) * weight
			blue += float64(b) * weight
			alpha += weight
			count++
		}
	}
	if count == 0 || alpha == 0 {
		return NeutralArtwork
	}

	tint := color.RGBA{
		R: uint8(red / alpha / 257),
		G: uint8(green / alpha / 257),
		B: uint8(blue / alpha / 257),
		A: 0xFF,
	}

	return NormalizeArtwork(tint)
}

// NormalizeArtwork keeps a tint dark enough for white text. A nearly gray
// result falls back to the neutral artwork surface so the tile does not read
// as a fake channel color.
func NormalizeArtwork(tint color.RGBA) color.RGBA {
	tint.A = 0xFF
	high := max(tint.R, max(tint.G, tint.B))
	low := min(tint.R, min(tint.G, tint.B))
	if high-low < 18 {
		return NeutralArtwork
	}

	for range 12 {
		if luminance(tint) <= 0.15 {
			return tint
		}
		tint.R = scaleChannel(tint.R, 0.92)
		tint.G = scaleChannel(tint.G, 0.92)
		tint.B = scaleChannel(tint.B, 0.92)
	}

	return tint
}

// scale multiplies one 8-bit channel without wrapping.
func scaleChannel(value uint8, factor float64) uint8 {
	return uint8(min(255, float64(value)*factor))
}

// luminance returns WCAG relative luminance.
func luminance(c color.RGBA) float64 {
	channel := func(value uint8) float64 {
		s := float64(value) / 255
		if s <= 0.03928 {
			return s / 12.92
		}

		return math.Pow((s+0.055)/1.055, 2.4)
	}

	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}
