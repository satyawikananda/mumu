package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Breakpoint is one of the three reference window sizes in the revamp design.
type Breakpoint int

const (
	// BreakpointCompact is the 1024x640 reference and anything smaller.
	BreakpointCompact Breakpoint = iota + 1
	// BreakpointMedium is the 1280x720 reference.
	BreakpointMedium
	// BreakpointWide is the 1470x956 reference.
	BreakpointWide
)

// Layout is the responsive policy for one window size. It changes spacing,
// type, rail and grid density from measurements, not from a single width.
type Layout struct {
	Breakpoint      Breakpoint
	Padding         float32
	HeroTitle       float32
	RailCards       int
	DiscoverColumns int
	TileHeight      float32
	SettingsRail    float32
	HintCount       int
	CategoryColumn  bool
}

// layoutFor returns the layout policy for width and height in DIPs.
func layoutFor(width, height float32) Layout {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	switch {
	case width >= 1400 && height >= 860:
		return Layout{
			Breakpoint:      BreakpointWide,
			Padding:         64,
			HeroTitle:       76,
			RailCards:       6,
			DiscoverColumns: 5,
			TileHeight:      206,
			SettingsRail:    286,
			HintCount:       4,
			CategoryColumn:  true,
		}
	case width >= 1160 && height >= 680:
		return Layout{
			Breakpoint:      BreakpointMedium,
			Padding:         48,
			HeroTitle:       60,
			RailCards:       5,
			DiscoverColumns: 4,
			TileHeight:      214,
			SettingsRail:    260,
			HintCount:       4,
			CategoryColumn:  true,
		}
	default:
		return Layout{
			Breakpoint:      BreakpointCompact,
			Padding:         28,
			HeroTitle:       48,
			RailCards:       4,
			DiscoverColumns: 4,
			TileHeight:      186,
			SettingsRail:    220,
			HintCount:       3,
			CategoryColumn:  false,
		}
	}
}

// Motion tokens from the revamp spec. MyGo turns these into an immediate
// target when the desktop asks for reduced motion.
var hoverMotion = ui.Animation{Duration: 160 * time.Millisecond, Ease: ui.EaseOut}

// Shape tokens. The theme keeps the control default at 10; these name the
// component exceptions instead of hardcoding hex-like values in views.
const (
	radiusKeycap float32 = 5
	radiusChip   float32 = 8
	radiusRow    float32 = 10
	radiusTile   float32 = 12

	hintBarHeight float32 = 34
	hintBarGap    float32 = 24
)

// baseFontSize is the unscaled body size the revamp type scale refers to.
const baseFontSize float32 = 14

// textSize scales a design size with the theme's text scale.
func textSize(t *ui.Theme, size float32) float32 {
	return t.Rem(size / baseFontSize)
}

// palette holds the semantic colors of one appearance.
type palette struct {
	canvas     ui.Color
	surface    ui.Color
	surface2   ui.Color
	border     ui.Color
	text       ui.Color
	secondary  ui.Color
	muted      ui.Color
	brand      ui.Color
	brandFill  ui.Color
	brandHover ui.Color
	brandPress ui.Color
	brandText  ui.Color
	focus      ui.Color
	success    ui.Color
	warning    ui.Color
	danger     ui.Color
	inverse    ui.Color
	inverseInk ui.Color
	scrollbar  ui.Color
}

// paletteFor returns the app palette for dark or light.
func paletteFor(dark bool) palette {
	if dark {
		return palette{
			canvas:     ui.Hex("#0A0A0C"),
			surface:    ui.Hex("#17171C"),
			surface2:   ui.Hex("#23232B"),
			border:     ui.Hex("#3A3A3F"),
			text:       ui.Hex("#F5F5F7"),
			secondary:  ui.Hex("#B8B8BF"),
			muted:      ui.Hex("#9298A1"),
			brand:      ui.Hex("#F82028"),
			brandFill:  ui.Hex("#D93636"),
			brandHover: ui.Hex("#C92C2C"),
			brandPress: ui.Hex("#B72525"),
			brandText:  ui.Hex("#FFFFFF"),
			focus:      ui.Hex("#FFFFFF"),
			success:    ui.Hex("#4DC19A"),
			warning:    ui.Hex("#F1B85B"),
			danger:     ui.Hex("#F36B68"),
			inverse:    ui.Hex("#3A3A3F"),
			inverseInk: ui.Hex("#F5F5F7"),
			scrollbar:  ui.RGBA(255, 255, 255, 0.28),
		}
	}

	return palette{
		canvas:     ui.Hex("#F7F7F8"),
		surface:    ui.Hex("#FFFFFF"),
		surface2:   ui.Hex("#E7E7EA"),
		border:     ui.Hex("#D5D5DA"),
		text:       ui.Hex("#18181B"),
		secondary:  ui.Hex("#52525B"),
		muted:      ui.Hex("#5C5C63"),
		brand:      ui.Hex("#D93636"),
		brandFill:  ui.Hex("#C22B2B"),
		brandHover: ui.Hex("#B72525"),
		brandPress: ui.Hex("#A11F1F"),
		brandText:  ui.Hex("#FFFFFF"),
		focus:      ui.Hex("#18181B"),
		success:    ui.Hex("#157F5F"),
		warning:    ui.Hex("#A16207"),
		danger:     ui.Hex("#C22B2B"),
		inverse:    ui.Hex("#262629"),
		inverseInk: ui.Hex("#F5F5F7"),
		scrollbar:  ui.RGBA(0, 0, 0, 0.28),
	}
}
