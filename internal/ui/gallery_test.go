package ui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMain(m *testing.M) {
	if err := RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(m.Run())
}

// galleryTiles returns deterministic tiles for layout checks.
func galleryTiles(prefix string, count int) []ChannelTile {
	tiles := make([]ChannelTile, 0, count)
	for i := range count {
		tiles = append(tiles, ChannelTile{
			Key:    fmt.Sprintf("%s-%d", prefix, i),
			Name:   fmt.Sprintf("%s %d", prefix, i),
			Meta:   "Indonesia · News",
			Live:   i%2 == 0,
			Height: 186,
			Art: Artwork{
				Monogram: fmt.Sprintf("%s%d", prefix[:1], i),
				Color:    []ui.Color{ui.Hex("#1F3A5F"), ui.Hex("#4A1F2B"), ui.Hex("#1F4A3A"), ui.Hex("#3A2A55")}[i%4],
			},
		})
	}

	return tiles
}

// galleryView composes the shared components at one responsive size.
func galleryView(c *ui.Context) {
	galleryViewTheme(c, true)
}

func galleryViewTheme(c *ui.Context, dark bool) {
	layout := layoutFor(c.Size())
	mode := ThemeDark
	if !dark {
		mode = ThemeLight
	}

	ui.Themed(c, themeFor(mode, dark)).Fill().Children(func() {
		t := c.Theme()
		ui.Scroll(c).Fill().Background(t.Background).Padding(layout.Padding).Gap(16).Children(func() {
			BrandLockup(c, nil)

			ui.Row(c).Gap(8).Children(func() {
				Chip{Label: "Semua", Active: true}.View(c)
				Chip{Label: "Berita"}.View(c)
				Chip{Label: "Hanya LIVE"}.View(c)
			})

			Rail{Title: "Favorit", Count: "12 channel", Tiles: galleryTiles("Rail", 8)}.View(c)

			if layout.CategoryColumn {
				ui.Row(c).Gap(16).Children(func() {
					ui.Column(c).Width(180).Gap(6).Children(func() {
						ui.Text(c, "Kategori").Font(displayFont()).FontSize(textSize(t, 19)).FontWeight(700)
						for _, name := range []string{"Semua channel", "Berita", "Anak", "Olahraga"} {
							ui.Text(c, name).Font(bodyFont()).FontSize(textSize(t, 14)).TextColor(t.TextMuted)
						}
					})

					ui.Column(c).Grow(1).Gap(12).Children(func() {
						ui.Text(c, "Indonesian channels").Font(displayFont()).FontSize(textSize(t, 19)).FontWeight(700)
						ChannelGrid{Tiles: galleryTiles("Grid", 10), Columns: layout.DiscoverColumns, Height: layout.TileHeight}.View(c)
					})
				})
			} else {
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, "Indonesian channels").Font(displayFont()).FontSize(textSize(t, 19)).FontWeight(700)
					ChannelGrid{Tiles: galleryTiles("Grid", 10), Columns: layout.DiscoverColumns, Height: layout.TileHeight}.View(c)
				})
			}

			ui.Row(c).Gap(14).Children(func() {
				ControlCenterCard{Key: "continue", Label: "Lanjutkan", Title: "Metro TV", Meta: "12 menit lalu"}.View(c)
				ControlCenterCard{Key: "favorite", Label: "Favorit", Title: "TVRI Bali", Meta: "12 channel", Icon: icons().heart}.View(c)
				ControlCenterCard{Key: "playlist", Label: "Playlist", Title: "My Own Channels", Meta: "3 sumber", Icon: icons().listVideo}.View(c)
			})

			HintBar{Hints: []Hint{
				{Key: "/", Action: "Cari"},
				{Key: "↑↓", Action: "Pindah"},
				{Key: "⏎", Action: "Tonton"},
				{Key: "Esc", Action: "Tutup"},
			}}.View(c)
		})
	})
}

func TestGalleryLightTheme(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		galleryViewTheme(c, false)
	}, 1280, 720)
	if !tt.HasText("Favorit") || !tt.HasText("Kategori") {
		t.Fatalf("texts %q, want the light gallery", tt.Texts())
	}
	if dir := os.Getenv("MUMU_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(dir, "gallery-light.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				t.Errorf("close light gallery capture: %v", err)
			}
		}()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGalleryReferenceSizes(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		columns int
		rail    int
	}{
		{name: "wide", width: 1470, height: 956, columns: 5, rail: 6},
		{name: "medium", width: 1280, height: 720, columns: 4, rail: 5},
		{name: "compact", width: 1024, height: 640, columns: 4, rail: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := ui.NewTester(galleryView, tc.width, tc.height)
			texts := tt.Texts()

			for i := range tc.rail {
				if !tt.HasText(fmt.Sprintf("Rail %d", i)) {
					t.Errorf("rail misses card %d; texts %q", i, texts)
				}
			}
			if tt.HasText(fmt.Sprintf("Rail %d", tc.rail)) {
				t.Errorf("rail shows card %d, want %d cards; texts %q", tc.rail, tc.rail, texts)
			}

			category := tc.columns != 4 || tc.width >= 1160
			if got := tt.HasText("Kategori"); got != category {
				t.Errorf("Kategori present = %v, want %v", got, category)
			}

			first, ok := tt.Find("Grid 0")
			if !ok {
				t.Fatalf("texts %q, want Grid 0", texts)
			}
			last, ok := tt.Find(fmt.Sprintf("Grid %d", tc.columns-1))
			if !ok {
				t.Fatalf("texts %q, want the last column tile", texts)
			}
			if first.Y != last.Y {
				t.Errorf("first row tiles Y = %v and %v, want one row", first.Y, last.Y)
			}
			next, ok := tt.Find(fmt.Sprintf("Grid %d", tc.columns))
			if !ok {
				t.Fatalf("texts %q, want the second row tile", texts)
			}
			if next.Y <= first.Y {
				t.Errorf("second row Y = %v, want below %v", next.Y, first.Y)
			}

			if dir := os.Getenv("MUMU_CAPTURE_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(dir, "gallery-"+tc.name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := file.Close(); err != nil {
						t.Errorf("close gallery capture: %v", err)
					}
				}()
				if err := png.Encode(file, tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
