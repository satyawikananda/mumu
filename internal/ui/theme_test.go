package ui

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestThemeTokens(t *testing.T) {
	dark := themeFor(ThemeDark, false)
	if !dark.Dark || dark.Background != ui.Hex("#0A0A0C") || dark.Surface != ui.Hex("#17171C") {
		t.Fatalf("dark theme colors are wrong: %+v", dark)
	}
	if dark.Accent != ui.Hex("#D93636") || dark.AccentText != ui.Hex("#FFFFFF") {
		t.Fatalf("dark accent is not the accessible action red")
	}
	if dark.Focus != ui.Hex("#FFFFFF") || dark.FocusRingWidth != 2 || dark.FocusRingOffset != -2 {
		t.Fatalf("dark focus ring is wrong: %+v", dark)
	}
	if dark.Radius != 10 || dark.Spacing != 4 || dark.FontSize != 14 {
		t.Fatalf("dark metrics are wrong: %+v", dark)
	}

	light := themeFor(ThemeLight, true)
	if light.Dark || light.Background != ui.Hex("#F7F7F8") || light.Surface != ui.Hex("#FFFFFF") {
		t.Fatalf("light theme colors are wrong: %+v", light)
	}
	if light.Accent != ui.Hex("#C22B2B") || light.Focus != ui.Hex("#18181B") {
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
		{"dark muted on surface", dark.muted, dark.surface, 4.5},
		{"dark muted on surface2", dark.muted, dark.surface2, 4.5},
		{"dark brand on canvas", dark.brand, dark.canvas, 4.5},
		{"dark action text", darkTheme.AccentText, darkTheme.Accent, 4.5},
		{"dark danger", darkTheme.Danger, dark.canvas, 4.5},
		{"dark danger on surface", darkTheme.Danger, dark.surface, 4.5},
		{"dark warning", darkTheme.Warning, dark.canvas, 4.5},
		{"dark warning on surface", darkTheme.Warning, dark.surface, 4.5},
		{"dark success", darkTheme.Success, dark.canvas, 4.5},
		{"dark focus", darkTheme.Focus, dark.canvas, 3},
		{"light text", lightTheme.Text, light.canvas, 4.5},
		{"light secondary", light.secondary, light.canvas, 4.5},
		{"light muted", light.muted, light.canvas, 4.5},
		{"light muted on surface", light.muted, light.surface, 4.5},
		{"light muted on surface2", light.muted, light.surface2, 4.5},
		{"light action text", lightTheme.AccentText, lightTheme.Accent, 4.5},
		{"light danger", lightTheme.Danger, light.canvas, 4.5},
		{"light danger on surface", lightTheme.Danger, light.surface, 4.5},
		{"light warning", lightTheme.Warning, light.canvas, 4.5},
		{"light warning on surface", lightTheme.Warning, light.surface, 4.5},
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

func TestLayoutForReferenceSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  float32
		height float32
		want   Layout
	}{
		{
			name:   "wide",
			width:  1470,
			height: 956,
			want: Layout{
				Breakpoint: BreakpointWide, Padding: 64, HeroTitle: 76, RailCards: 6,
				DiscoverColumns: 5, TileHeight: 206, SettingsRail: 286, HintCount: 4,
				CategoryColumn: true,
			},
		},
		{
			name:   "medium",
			width:  1280,
			height: 720,
			want: Layout{
				Breakpoint: BreakpointMedium, Padding: 48, HeroTitle: 60, RailCards: 5,
				DiscoverColumns: 4, TileHeight: 214, SettingsRail: 260, HintCount: 4,
				CategoryColumn: true,
			},
		},
		{
			name:   "compact",
			width:  1024,
			height: 640,
			want: Layout{
				Breakpoint: BreakpointCompact, Padding: 28, HeroTitle: 48, RailCards: 4,
				DiscoverColumns: 4, TileHeight: 186, SettingsRail: 220, HintCount: 3,
				CategoryColumn: false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := layoutFor(tc.width, tc.height); got != tc.want {
				t.Fatalf("layoutFor(%v,%v) = %+v, want %+v", tc.width, tc.height, got, tc.want)
			}
		})
	}
}

func TestLayoutUsesHeight(t *testing.T) {
	if got := layoutFor(1470, 720); got.Breakpoint != BreakpointMedium {
		t.Fatalf("1470x720 breakpoint = %v, want medium", got.Breakpoint)
	}
	if got := layoutFor(1280, 640); got.Breakpoint != BreakpointCompact {
		t.Fatalf("1280x640 breakpoint = %v, want compact", got.Breakpoint)
	}
	if got := layoutFor(-1, -1); got.Breakpoint != BreakpointCompact {
		t.Fatalf("negative size breakpoint = %v, want compact", got.Breakpoint)
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

func TestThemeForContextFollowsTextScale(t *testing.T) {
	var theme *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		theme = themeForContext(ThemeDark, c)
		ui.Text(c, "x")
	}, 120, 80)
	tt.SetPreferences(ui.Preferences{TextScale: 2, HighContrast: true})
	tt.Frame()

	if theme == nil || theme.FontSize != 28 {
		t.Fatalf("font size = %v, want 28 after 2x text scale", theme)
	}
	if theme.Focus != ui.Hex("#FFFFFF") {
		t.Fatalf("focus = %v, want the app ring after preferences", theme.Focus)
	}
}
