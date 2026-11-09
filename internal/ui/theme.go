package ui

import "github.com/egoist/mygo/ui"

// WindowBackground is the color behind the first frame.
func WindowBackground() string { return "#151517" }

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

// palette holds app colors that MyGo's Theme does not name.
type palette struct {
	canvas    ui.Color
	sidebar   ui.Color
	raised    ui.Color
	hover     ui.Color
	pressed   ui.Color
	border    ui.Color
	text      ui.Color
	secondary ui.Color
	muted     ui.Color
	tomato    ui.Color
	video     ui.Color
}

// paletteFor returns the app palette for dark or light.
func paletteFor(dark bool) palette {
	if dark {
		return palette{
			canvas:    ui.Hex("#151517"),
			sidebar:   ui.Hex("#1D1D20"),
			raised:    ui.Hex("#262629"),
			hover:     ui.Hex("#333338"),
			pressed:   ui.Hex("#3D3D43"),
			border:    ui.Hex("#3A3A3F"),
			text:      ui.Hex("#F5F5F6"),
			secondary: ui.Hex("#B8B8BF"),
			muted:     ui.Hex("#92929A"),
			tomato:    ui.Hex("#FF4D4D"),
			video:     ui.Hex("#09090B"),
		}
	}

	return palette{
		canvas:    ui.Hex("#F7F7F8"),
		sidebar:   ui.Hex("#EFEFF1"),
		raised:    ui.Hex("#FFFFFF"),
		hover:     ui.Hex("#E7E7EA"),
		pressed:   ui.Hex("#DDDDE1"),
		border:    ui.Hex("#D5D5DA"),
		text:      ui.Hex("#18181B"),
		secondary: ui.Hex("#52525B"),
		muted:     ui.Hex("#62626B"),
		tomato:    ui.Hex("#FF4D4D"),
		video:     ui.Hex("#09090B"),
	}
}

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
		Surface:         p.raised,
		SurfaceHover:    p.hover,
		SurfacePressed:  p.pressed,
		Border:          p.border,
		Text:            p.text,
		TextMuted:       p.secondary,
		Accent:          ui.Hex("#D93636"),
		AccentHover:     ui.Hex("#C92C2C"),
		AccentPressed:   ui.Hex("#B72525"),
		AccentText:      ui.Hex("#FFFFFF"),
		Danger:          ui.Hex("#F36B68"),
		Warning:         ui.Hex("#F1B85B"),
		Success:         ui.Hex("#4DC19A"),
		Selection:       ui.RGBA(255, 77, 77, 0.30),
		Focus:           ui.Hex("#FF4D4D"),
		FocusRingWidth:  2,
		FocusRingOffset: 2,
		Inverse:         ui.Hex("#3A3A3F"),
		InverseText:     ui.Hex("#F5F5F6"),
		Scrollbar:       ui.RGBA(255, 255, 255, 0.28),
		Radius:          8,
		Spacing:         4,
		FontSize:        13,
	}

	if !dark {
		t.Danger = ui.Hex("#C22B2B")
		t.Warning = ui.Hex("#A16207")
		t.Success = ui.Hex("#157F5F")
		t.Selection = ui.RGBA(217, 54, 54, 0.22)
		t.Focus = ui.Hex("#D93636")
		t.Inverse = ui.Hex("#262629")
		t.InverseText = ui.Hex("#F5F5F6")
		t.Scrollbar = ui.RGBA(0, 0, 0, 0.28)
	}

	return t
}
