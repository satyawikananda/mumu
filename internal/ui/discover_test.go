package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// discoverTestCountries is the selector list used by the page tests.
func discoverTestCountries() []DiscoverCountry {
	return []DiscoverCountry{
		{Code: "ID", Name: "Indonesia"},
		{Code: "MY", Name: "Malaysia"},
	}
}

// discoverTestChannels is one live and one non-live row.
func discoverTestChannels() []DiscoverChannel {
	return []DiscoverChannel{
		{Key: "tvri", Name: "TVRI Nasional", Country: "ID", Category: "General", Live: true},
		{Key: "bernama", Name: "Bernama News", Country: "MY", Category: "News", LogoURL: "https://logos.example/bernama.png"},
	}
}

// discoverPage wires a page into a headless tester.
func discoverPage(d *Discover) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		d.View(c, c.Theme())
	}, 800, 600)
}

// A new page keeps the confirmed empty state and its actions.
func TestDiscoverDefaultEmpty(t *testing.T) {
	d := NewDiscover()
	tt := discoverPage(d)

	if !tt.HasText("Discover") {
		t.Fatalf("texts %q, want the page heading", tt.Texts())
	}
	if !tt.HasText("No channels yet") {
		t.Fatalf("texts %q, want the empty state", tt.Texts())
	}
	if !tt.HasText("Add Playlist") {
		t.Fatalf("texts %q, want Add Playlist", tt.Texts())
	}
}

// Loading names the region instead of showing a blank list.
func TestDiscoverLoading(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{Kind: DiscoverLoading, Region: "ID"})

	tt := discoverPage(d)

	if !tt.HasText("Loading channels") || !tt.HasText("Indonesia") {
		t.Fatalf("texts %q, want the loading state for Indonesia", tt.Texts())
	}
	if tt.HasText("LIVE") {
		t.Fatalf("texts %q, want no rows while loading", tt.Texts())
	}
}

// The channel list shows title, resolved country, category and a LIVE label
// only for live formats.
func TestDiscoverChannelRows(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(),
	})

	tt := discoverPage(d)

	if !tt.HasText("TVRI Nasional") || !tt.HasText("Bernama News") {
		t.Fatalf("texts %q, want both channel titles", tt.Texts())
	}
	if !tt.HasText("Indonesia · General") || !tt.HasText("Malaysia · News") {
		t.Fatalf("texts %q, want country and category metadata", tt.Texts())
	}
	if got := countText(tt.Texts(), "LIVE"); got != 1 {
		t.Fatalf("texts %q show LIVE %d times, want only the live channel", tt.Texts(), got)
	}
	// Without a bitmap the avatar falls back to the channel's initials.
	if !tt.HasText("TN") || !tt.HasText("BN") {
		t.Fatalf("texts %q, want avatar initials", tt.Texts())
	}
}

// A real bitmap replaces the initials for its URL.
func TestDiscoverChannelBitmap(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(),
	})

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	d.SetLogos(map[string]*ui.Bitmap{"https://logos.example/bernama.png": ui.NewBitmap(img)})

	tt := discoverPage(d)

	if !tt.HasText("Bernama News") {
		t.Fatalf("texts %q, want the channel with a logo", tt.Texts())
	}
	if tt.HasText("BN") {
		t.Fatalf("texts %q, want the bitmap instead of initials", tt.Texts())
	}
	if !tt.HasText("TN") {
		t.Fatalf("texts %q, want initials for the channel without a logo", tt.Texts())
	}
}

// The region selector reports the chosen code.
func TestDiscoverRegionCallback(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{Kind: DiscoverEmpty, Region: "ID"})

	var got []string
	d.OnRegion = func(code string) { got = append(got, code) }

	tt := discoverPage(d)

	if err := tt.Click("Indonesia"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Malaysia"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "MY" {
		t.Fatalf("region callbacks %v, want [MY]", got)
	}
	if d.Region() != "MY" {
		t.Fatalf("Region() = %q, want MY", d.Region())
	}
}

// Empty and error states offer Retry, and offline keeps cached rows visible.
func TestDiscoverStatesAndRetry(t *testing.T) {
	tests := []struct {
		name  string
		state DiscoverState
		text  string
	}{
		{"empty", DiscoverState{Kind: DiscoverEmpty, Region: "ID"}, "No channels yet"},
		{"error", DiscoverState{Kind: DiscoverError, Region: "ID"}, "Channels unavailable"},
		{
			"offline",
			DiscoverState{Kind: DiscoverOffline, Region: "ID", Channels: discoverTestChannels()},
			"Showing saved channels",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDiscover()
			d.SetCountries(discoverTestCountries())
			d.SetState(tc.state)

			retries := 0
			d.OnRetry = func() { retries++ }

			tt := discoverPage(d)
			if !tt.HasText(tc.text) {
				t.Fatalf("texts %q, want %q", tt.Texts(), tc.text)
			}
			if tc.name == "offline" && !tt.HasText("TVRI Nasional") {
				t.Fatalf("texts %q, want the cached rows", tt.Texts())
			}

			if err := tt.Click("Retry"); err != nil {
				t.Fatal(err)
			}
			if retries != 1 {
				t.Fatalf("retry clicked %d times, want 1", retries)
			}
		})
	}
}

// Add Playlist dispatches its intent without touching playback.
func TestDiscoverAddPlaylist(t *testing.T) {
	d := NewDiscover()
	var clicks int
	d.OnAddPlaylist = func() { clicks++ }

	tt := discoverPage(d)
	if err := tt.Click("Add Playlist"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("Add Playlist clicked %d times, want 1", clicks)
	}
}

// In the shell, Add Playlist opens the Library placeholder.
func TestShellDiscoverAddPlaylistOpensLibrary(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 960, 640)

	if err := tt.Click("Add Playlist"); err != nil {
		t.Fatal(err)
	}
	if shell.nav != destLibrary {
		t.Fatalf("navigation %q, want Library", shell.nav)
	}
	if !shell.Library().SheetOpen() {
		t.Fatal("Add Playlist did not open the import sheet")
	}
	if !tt.HasText("Source type") || !tt.HasText("Validate and Preview") {
		t.Fatalf("texts %q, want the import sheet", tt.Texts())
	}
}

// The list is reachable from the keyboard and the arrows choose rows.
func TestDiscoverKeyboardFocus(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(),
	})

	tt := discoverPage(d)

	// Region selector, Add Playlist, search field, then the list.
	for range 4 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Key(0, ui.KeyDown)

	if d.selected != 0 {
		t.Fatalf("selected row %d after Tab and Down, want 0", d.selected)
	}
}

// Typing in the search field reports the new narrowing once.
func TestDiscoverSearchFilter(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{Kind: DiscoverReady, Region: "ID", Channels: discoverTestChannels()})

	var got []DiscoverFilters
	d.OnFilter = func(filters DiscoverFilters) { got = append(got, filters) }

	tt := discoverPage(d)
	for range 3 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Type("tvri")

	if len(got) != 1 {
		t.Fatalf("filters %+v, want one report", got)
	}
	if got[0].Query != "tvri" {
		t.Fatalf("query %q, want tvri", got[0].Query)
	}
}

// A narrowed empty result offers Clear filters instead of Retry.
func TestDiscoverFilteredEmpty(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{Kind: DiscoverReady, Region: "ID"})

	var got []DiscoverFilters
	d.OnFilter = func(filters DiscoverFilters) { got = append(got, filters) }

	tt := discoverPage(d)
	for range 3 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Type("nothing")

	if !tt.HasText("No channels match") {
		t.Fatalf("texts %q, want the filtered empty state", tt.Texts())
	}
	if err := tt.Click("Clear filters"); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || !got[len(got)-1].Empty() {
		t.Fatalf("filters %+v, want a cleared report", got)
	}
	if d.Filters().Query != "" {
		t.Fatalf("query %q, want cleared", d.Filters().Query)
	}
}

// Enter opens the chosen row, keeping its source-scoped key.
func TestDiscoverSelectRow(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(),
	})

	var opened []string
	d.OnSelect = func(key string) { opened = append(opened, key) }

	tt := discoverPage(d)
	for range 4 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)

	if len(opened) != 1 || opened[0] != "tvri" {
		t.Fatalf("opened %v, want [tvri]", opened)
	}
}

// Rows attribute their source, and a capped list says so.
func TestDiscoverRowSourceAndCap(t *testing.T) {
	d := NewDiscover()
	rows := []DiscoverChannel{{
		Key:        "source-a:tvri",
		Name:       "TVRI Nasional",
		Country:    "ID",
		Category:   "General",
		SourceName: "Community list",
		Live:       true,
	}}
	d.SetState(DiscoverState{
		Kind:      DiscoverReady,
		Region:    "ID",
		Channels:  rows,
		Total:     42,
		Truncated: true,
	})

	tt := discoverPage(d)
	if !tt.HasText("Community list") {
		t.Fatalf("texts %q, want the source attribution", tt.Texts())
	}
	if !tt.HasText("Showing first 1 of 42 channels") {
		t.Fatalf("texts %q, want the cap summary", tt.Texts())
	}
}

// A facet list drops a selection the new catalog no longer offers.
func TestDiscoverFacetsDropStaleSelection(t *testing.T) {
	d := NewDiscover()
	d.SetFacets([]string{"News"}, []DiscoverFacet{{ID: "source-a", Name: "Community list"}})
	d.category = "News"
	d.sourceID = "source-a"

	d.SetFacets([]string{"Sports"}, []DiscoverFacet{{ID: "source-b", Name: "My list"}})

	filters := d.Filters()
	if filters.Category != "" || filters.SourceID != "" {
		t.Fatalf("filters %+v, want cleared", filters)
	}
}

// The community catalog carries a visible, unverified attribution line.
func TestDiscoverNotice(t *testing.T) {
	d := NewDiscover()
	d.SetState(DiscoverState{Kind: DiscoverReady, Region: "ID", Channels: discoverTestChannels()})
	d.SetNotice("Community channel list · iptv-org · not verified")

	tt := discoverPage(d)
	if !tt.HasText("Community channel list · iptv-org · not verified") {
		t.Fatalf("texts %q, want the community notice", tt.Texts())
	}
}

// Selection follows the row key when new results reorder the list.
func TestDiscoverSelectionFollowsKey(t *testing.T) {
	first := DiscoverChannel{Key: "a", Name: "Alpha"}
	second := DiscoverChannel{Key: "b", Name: "Bravo"}

	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{Kind: DiscoverReady, Region: "ID", Channels: []DiscoverChannel{first, second}})

	tt := discoverPage(d)
	for range 4 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Key(0, ui.KeyDown)

	d.SetState(DiscoverState{Kind: DiscoverReady, Region: "ID", Channels: []DiscoverChannel{second, first}})
	tt.Frame()

	if d.selected != 1 {
		t.Fatalf("selected row %d, want the same key at row 1", d.selected)
	}
}
