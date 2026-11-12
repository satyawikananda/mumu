package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRegisterFonts(t *testing.T) {
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}

	families := ui.FontFamilies()
	for _, want := range []string{bodyFamily, displayFamily} {
		found := false
		for _, family := range families {
			if family.Name == want && family.Registered {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("font family %q is not registered", want)
		}
	}
}

func TestMonogram(t *testing.T) {
	tests := map[string]string{
		"":                  "TV",
		"TVRI Bali":         "TV",
		"CNN Indonesia":     "CN",
		"Metro TV":          "ME",
		"tvOne":             "TV",
		"---":               "TV",
		"7 News":            "7N",
		"Cartoonito Asia":   "CA",
		"  Metro   TV  ":    "ME",
		"channel-with-dash": "CH",
	}

	for name, want := range tests {
		if got := monogram(name); got != want {
			t.Errorf("monogram(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestBadgeTexts(t *testing.T) {
	tests := []struct {
		badge Badge
		want  string
	}{
		{Badge{Kind: BadgeLive}, "LIVE"},
		{Badge{Kind: BadgeWarning}, "BUFFERING"},
		{Badge{Kind: BadgeDanger}, "UNAVAILABLE"},
		{Badge{Kind: BadgeNeutral, Text: "720p"}, "720p"},
	}

	for _, tc := range tests {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
				tc.badge.View(c)
			})
		}, 200, 80)
		if !tt.HasText(tc.want) {
			t.Errorf("texts %q, want %q", tt.Texts(), tc.want)
		}
	}
}

func TestChipActiveAndClick(t *testing.T) {
	clicked := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			Chip{Key: "country", Label: "Indonesia", Active: true, OnClick: func() {
				clicked++
			}}.View(c)
		})
	}, 240, 80)

	if !tt.HasText("Indonesia") {
		t.Fatalf("texts %q, want the chip label", tt.Texts())
	}
	if err := tt.Click("Indonesia"); err != nil {
		t.Fatal(err)
	}
	if clicked != 1 {
		t.Fatalf("chip clicked %d times, want 1", clicked)
	}
}

func TestHintBarLimit(t *testing.T) {
	hints := []Hint{
		{Key: "/", Action: "Search"},
		{Key: "↑↓", Action: "Move"},
		{Key: "⏎", Action: "Watch"},
		{Key: "Esc", Action: "Close"},
		{Key: "P", Action: "PiP"},
		{Key: "F", Action: "Fullscreen"},
		{Key: "M", Action: "Mute"},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			HintBar{Hints: hints}.View(c)
		})
	}, 800, 80)

	for _, hint := range hints[:5] {
		if !tt.HasText(hint.Action) {
			t.Errorf("texts %q, want %q", tt.Texts(), hint.Action)
		}
	}
	for _, hint := range hints[5:] {
		if tt.HasText(hint.Action) {
			t.Errorf("texts %q, want no %q", tt.Texts(), hint.Action)
		}
	}
}

func TestChannelTileStates(t *testing.T) {
	var activated int
	tile := ChannelTile{
		Key:        "metro",
		Name:       "Metro TV",
		Meta:       "News · iptv-org",
		Live:       true,
		Width:      206,
		Height:     206,
		OnActivate: func() { activated++ },
	}

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			tile.View(c)
		})
	}, 300, 300)

	if !tt.HasText("Metro TV") || !tt.HasText("LIVE") || !tt.HasText("ME") {
		t.Fatalf("texts %q, want title, status and monogram", tt.Texts())
	}
	box, ok := tt.Find("Metro TV")
	if !ok || box.W <= 0 || box.W > 206 {
		t.Fatalf("title box %+v, want inside the tile", box)
	}
	if err := tt.Click("Metro TV"); err != nil {
		t.Fatal(err)
	}
	if activated != 1 {
		t.Fatalf("tile activated %d times, want 1", activated)
	}

	disabled := tile
	disabled.Disabled = true
	tt = ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			disabled.View(c)
		})
	}, 300, 300)
	if err := tt.Click("Metro TV"); err != nil {
		t.Fatal(err)
	}
	if activated != 1 {
		t.Fatalf("disabled tile activated %d times, want no change", activated)
	}

	loading := tile
	loading.Art.Loading = true
	tt = ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			loading.View(c)
		})
	}, 300, 300)
	if !tt.HasText("ME") {
		t.Fatalf("texts %q, want the monogram while loading", tt.Texts())
	}
}

func TestChannelTileLongName(t *testing.T) {
	name := strings.TrimSpace(strings.Repeat("Very long channel name ", 8))
	tile := ChannelTile{Name: name, Height: 186}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			tile.View(c)
		})
	}, 260, 260)

	box, ok := tt.Find(name)
	if !ok {
		t.Fatalf("texts %q, want the long title", tt.Texts())
	}
	if box.X < 0 || box.W <= 0 || box.X+box.W > 260 {
		t.Fatalf("long title box %+v, want inside the window", box)
	}
}

func TestChannelTileLogoSwapKeepsBounds(t *testing.T) {
	art := Artwork{}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			ChannelTile{Name: "Metro TV", Live: true, Height: 206, Art: art}.View(c)
		})
	}, 300, 300)

	before, ok := tt.Find("Metro TV")
	if !ok {
		t.Fatalf("texts %q, want the title", tt.Texts())
	}

	art.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 32, 32)))
	tt.Frame()

	after, ok := tt.Find("Metro TV")
	if !ok {
		t.Fatalf("texts %q, want the title after the logo arrived", tt.Texts())
	}
	if before != after {
		t.Fatalf("title bounds moved from %+v to %+v after logo load", before, after)
	}
}

func TestControlCenterCard(t *testing.T) {
	var activated int
	card := ControlCenterCard{
		Key:        "continue",
		Label:      "Lanjutkan",
		Title:      "Metro TV",
		Meta:       "12 menit lalu",
		OnActivate: func() { activated++ },
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			card.View(c)
		})
	}, 360, 240)

	if !tt.HasText("LANJUTKAN") || !tt.HasText("Metro TV") || !tt.HasText("12 menit lalu") {
		t.Fatalf("texts %q, want label, title and meta", tt.Texts())
	}
	if err := tt.Click("Metro TV"); err != nil {
		t.Fatal(err)
	}
	if activated != 1 {
		t.Fatalf("card activated %d times, want 1", activated)
	}
}

func TestChannelRowStates(t *testing.T) {
	row := ChannelRow{Name: "CNN Indonesia", Meta: "News · iptv-org", Live: true, Art: Artwork{Monogram: "CN"}}
	states := []RowState{
		{},
		{Hovered: true},
		{Focused: true},
		{Selected: true, Focused: true},
	}

	for _, state := range states {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
				row.View(c, state)
			})
		}, 480, 80)
		if !tt.HasText("CNN Indonesia") || !tt.HasText("LIVE") {
			t.Errorf("state %+v texts %q, want name and live badge", state, tt.Texts())
		}
	}
}

func TestReducedMotionGallery(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			ChannelTile{Name: "Metro TV", Height: 206}.View(c)
		})
	}, 300, 300)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Move(100, 100)
	tt.Frame()
	if !tt.HasText("Metro TV") {
		t.Fatalf("texts %q, want the tile after reduced motion", tt.Texts())
	}
}

func TestChannelTileFocusRingIsKeyboardOnly(t *testing.T) {
	tile := ChannelTile{Key: "metro", Name: "Metro TV", Height: 206, Art: Artwork{Color: ui.Hex("#1F3A5F")}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			ui.Box(c).Padding(20).Children(func() {
				tile.View(c)
			})
		})
	}, 260, 260)

	tt.Move(100, 100)
	tt.Frame()
	hover := tt.Image().RGBAAt(18, 100)

	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if !tt.Focused("Metro TV") {
		t.Fatalf("focus did not reach the tile; texts %q", tt.Texts())
	}
	focused := tt.Image().RGBAAt(21, 100)

	if nearWhite(hover) {
		t.Fatalf("hover pixel %+v looks like a focus ring", hover)
	}
	if !nearWhite(focused) {
		t.Fatalf("focus pixel %+v is not the white ring", focused)
	}
}

func nearWhite(c color.RGBA) bool {
	return c.R > 230 && c.G > 230 && c.B > 230
}

func TestBoundedArtworkColorKeepsWhiteReadable(t *testing.T) {
	for _, tint := range []ui.Color{ui.Hex("#FFFFFF"), ui.Hex("#FFD400"), ui.Hex("#F82028"), ui.Hex("#1F3A5F")} {
		got := boundedArtworkColor(tint)
		if ratio := contrast(ui.Hex("#FFFFFF"), got); ratio < 4.5 {
			t.Errorf("boundedArtworkColor(%v) contrast %.2f, want at least 4.5", tint, ratio)
		}
	}
}

func TestIconsParse(t *testing.T) {
	set := icons()
	all := []*ui.SVG{
		set.watch, set.discover, set.library, set.settings, set.home, set.search,
		set.user, set.arrowLeft, set.plus, set.globe, set.tv, set.heart, set.volume,
		set.mute, set.play, set.pause, set.pip, set.fullscreen, set.chevron,
		set.loader, set.check, set.close, set.listVideo, set.folder, set.clock,
		set.signal, set.offline, set.alert, set.refresh, set.panelLeft, set.monitor,
		set.compass, set.liveDot, set.rotateLeft, set.copy, set.external, set.trash,
		set.eye, set.eyeOff, set.sliders,
	}
	for i, icon := range all {
		if icon == nil {
			t.Fatalf("icon %d did not parse", i)
		}
	}
}
