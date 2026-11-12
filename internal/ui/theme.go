package ui

import "github.com/egoist/mygo/ui"

// WindowBackground is the color behind the first frame.
func WindowBackground() string { return "#0A0A0C" }

// ThemeMode chooses the app's appearance.
type ThemeMode int

const (
	// ThemeDark is the charcoal-first default.
	ThemeDark ThemeMode = iota + 1
	// ThemeLight uses the light palette.
	ThemeLight
	// ThemeSystem follows the macOS appearance.
	ThemeSystem
)

// themeFor builds the MyGo theme for mode, using systemDark for ThemeSystem.
func themeFor(mode ThemeMode, systemDark bool) *ui.Theme {
	if mode == ThemeSystem {
		mode = ThemeDark
		if !systemDark {
			mode = ThemeLight
		}
	}

	dark := mode != ThemeLight
	p := paletteFor(dark)
	t := &ui.Theme{
		Dark:            dark,
		Background:      p.canvas,
		Surface:         p.surface,
		SurfaceHover:    p.surface2,
		SurfacePressed:  p.surface2,
		Border:          p.border,
		Text:            p.text,
		TextMuted:       p.muted,
		Accent:          p.brandFill,
		AccentHover:     p.brandHover,
		AccentPressed:   p.brandPress,
		AccentText:      p.brandText,
		Danger:          p.danger,
		Warning:         p.warning,
		Success:         p.success,
		Selection:       p.brand.Alpha(selectionAlpha(dark)),
		Focus:           p.focus,
		FocusRingWidth:  2,
		FocusRingOffset: -2,
		Inverse:         p.inverse,
		InverseText:     p.inverseInk,
		Scrollbar:       p.scrollbar,
		Radius:          radiusRow,
		Spacing:         4,
		FontSize:        baseFontSize,
		Font:            bodyFont(),
	}

	return t
}

// themeForContext applies the desktop's text scale and contrast to the app
// theme, then restores the app's brand accent and focus ring. The default
// theme follows the desktop's accent; mumu keeps its own.
func themeForContext(mode ThemeMode, c *ui.Context) *ui.Theme {
	t := themeFor(mode, c.Theme().Dark)
	t = t.WithPreferences(c.Preferences())
	p := paletteFor(t.Dark)
	t.Accent = p.brandFill
	t.AccentHover = p.brandHover
	t.AccentPressed = p.brandPress
	t.AccentText = p.brandText
	t.Focus = p.focus
	t.Selection = p.brand.Alpha(selectionAlpha(t.Dark))

	return t
}

// selectionAlpha keeps the brand selection readable on each appearance.
func selectionAlpha(dark bool) float32 {
	if dark {
		return 0.30
	}

	return 0.22
}
