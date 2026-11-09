package ui

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestThemeTokens(t *testing.T) {
	dark := themeFor(ThemeDark, false)
	if !dark.Dark || dark.Background != ui.Hex("#151517") || dark.Surface != ui.Hex("#262629") {
		t.Fatalf("dark theme colors are wrong: %+v", dark)
	}
	if dark.Accent != ui.Hex("#D93636") || dark.AccentText != ui.Hex("#FFFFFF") {
		t.Fatalf("dark accent is not the accessible action red")
	}
	if dark.Focus != ui.Hex("#FF4D4D") || dark.Radius != 8 || dark.Spacing != 4 || dark.FontSize != 13 {
		t.Fatalf("dark metrics are wrong: %+v", dark)
	}

	light := themeFor(ThemeLight, true)
	if light.Dark || light.Background != ui.Hex("#F7F7F8") || light.Surface != ui.Hex("#FFFFFF") {
		t.Fatalf("light theme colors are wrong: %+v", light)
	}
	if light.Accent != ui.Hex("#D93636") || light.Focus != ui.Hex("#D93636") {
		t.Fatalf("light accent or focus is wrong: %+v", light)
	}
	if light.Danger != ui.Hex("#C22B2B") || light.Warning != ui.Hex("#A16207") || light.Success != ui.Hex("#157F5F") {
		t.Fatalf("light state colors are wrong: %+v", light)
	}
}

func TestThemeForSystem(t *testing.T) {
	if got := themeFor(ThemeSystem, true); !got.Dark {
		t.Fatal("ThemeSystem with a dark desktop should be dark")
	}
	if got := themeFor(ThemeSystem, false); got.Dark {
		t.Fatal("ThemeSystem with a light desktop should be light")
	}
	if got := themeFor(ThemeLight, true); got.Dark {
		t.Fatal("ThemeLight should stay light on a dark desktop")
	}
}

func TestPaletteContrast(t *testing.T) {
	dark, light := paletteFor(true), paletteFor(false)
	darkTheme, lightTheme := themeFor(ThemeDark, true), themeFor(ThemeLight, false)

	tests := []struct {
		name string
		fg   ui.Color
		bg   ui.Color
		min  float64
	}{
		{"dark text", darkTheme.Text, dark.canvas, 4.5},
		{"dark secondary", dark.secondary, dark.canvas, 4.5},
		{"dark muted", dark.muted, dark.canvas, 4.5},
		{"dark sidebar text", darkTheme.Text, dark.sidebar, 4.5},
		{"dark sidebar muted", dark.muted, dark.sidebar, 4.5},
		{"dark action text", darkTheme.AccentText, darkTheme.Accent, 4.5},
		{"dark danger", darkTheme.Danger, dark.canvas, 4.5},
		{"dark warning", darkTheme.Warning, dark.canvas, 4.5},
		{"dark success", darkTheme.Success, dark.canvas, 4.5},
		{"dark focus", darkTheme.Focus, dark.canvas, 3},
		{"light text", lightTheme.Text, light.canvas, 4.5},
		{"light secondary", light.secondary, light.canvas, 4.5},
		{"light muted", light.muted, light.canvas, 4.5},
		{"light sidebar text", lightTheme.Text, light.sidebar, 4.5},
		{"light sidebar muted", light.muted, light.sidebar, 4.5},
		{"light action text", lightTheme.AccentText, lightTheme.Accent, 4.5},
		{"light danger", lightTheme.Danger, light.canvas, 4.5},
		{"light warning", lightTheme.Warning, light.canvas, 4.5},
		{"light success", lightTheme.Success, light.canvas, 4.5},
		{"light focus", lightTheme.Focus, light.canvas, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := contrast(tc.fg, tc.bg); got < tc.min {
				t.Errorf("contrast %.2f, want at least %.1f", got, tc.min)
			}
		})
	}
}

func contrast(a, b ui.Color) float64 {
	high, low := luminance(a), luminance(b)
	if high < low {
		high, low = low, high
	}

	return (high + 0.05) / (low + 0.05)
}

func luminance(c ui.Color) float64 {
	channel := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}

		return math.Pow((s+0.055)/1.055, 2.4)
	}

	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}
