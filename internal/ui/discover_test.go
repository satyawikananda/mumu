package ui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// discoverTestCountries is the picker list used by the page tests.
func discoverTestCountries() []DiscoverCountry {
	return []DiscoverCountry{
		{Code: "ID", Name: "Indonesia"},
		{Code: "MY", Name: "Malaysia"},
	}
}

// discoverTestChannels returns n bare channels without metadata, so a tile's
// accessible name is just its own title.
func discoverTestChannels(n int) []DiscoverChannel {
	channels := make([]DiscoverChannel, 0, n)
	for i := range n {
		channels = append(channels, DiscoverChannel{
			Key:  fmt.Sprintf("key-%d", i),
			Name: fmt.Sprintf("Channel %d", i),
		})
	}

	return channels
}

// discoverTestFacets is the category rail.
func discoverTestFacets() []DiscoverCategory {
	return []DiscoverCategory{
		{Name: "News", Count: 2},
		{Name: "Sports", Count: 1},
	}
}

// discoverPage wires a page into a headless tester.
func discoverPage(d *Discover, width, height int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		d.View(c, c.Theme())
	}, width, height)
}

// A new page shows the approved no-country state, not a loading or empty one.
func TestDiscoverDefaultNoCountry(t *testing.T) {
	d := NewDiscover()
	tt := discoverPage(d, 1470, 956)

	for _, want := range []string{
		"Choose a country to get started",
		"No country is selected yet",
		"Select a country",
		"Add Playlist",
		"Choose a country",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if tt.HasText("No channels yet") {
		t.Fatalf("texts %q, presented the no-country state as an empty catalog", tt.Texts())
	}
	if tt.HasText("All sources") || tt.HasText("Live only") {
		t.Fatalf("texts %q, kept the removed source and live filters", tt.Texts())
	}
	if !tt.HasText("Country") {
		t.Fatalf("texts %q, want the country field label", tt.Texts())
	}
	if got := d.Region(); got != "" {
		t.Fatalf("Region() = %q, want no country", got)
	}
}

// The country chip opens a searchable picker and reports the chosen code.
func TestDiscoverCountryPicker(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())

	var picked []string
	d.OnRegion = func(code string) { picked = append(picked, code) }

	tt := discoverPage(d, 1470, 956)
	if err := tt.Click("Choose a country"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Malaysia") {
		t.Fatalf("texts %q, want the country list", tt.Texts())
	}

	tt.Type("mala")
	if !tt.HasText("Malaysia") {
		t.Fatalf("texts %q, want the typed match", tt.Texts())
	}
	if err := tt.Click("Malaysia, MY"); err != nil {
		t.Fatal(err)
	}

	if len(picked) != 1 || picked[0] != "MY" {
		t.Fatalf("picked %v, want [MY]", picked)
	}
	if got := d.Region(); got != "MY" {
		t.Fatalf("Region() = %q, want MY", got)
	}
}

// A chosen country keeps its real name in the filter chip.
func TestDiscoverCountryChipShowsSelection(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(2),
		Total:    2,
	})

	tt := discoverPage(d, 1470, 956)
	if !tt.HasText("Indonesia") {
		t.Fatalf("texts %q, want the selected country", tt.Texts())
	}
	if tt.HasText("Choose a country to get started") {
		t.Fatalf("texts %q, kept the no-country prompt", tt.Texts())
	}
}

// The country list failure explains itself in the picker and offers a retry.
func TestDiscoverCountriesError(t *testing.T) {
	d := NewDiscover()
	d.SetCountriesError("mumu could not load the country list. Check your connection and retry.")

	reloads := 0
	d.OnReloadCountries = func() { reloads++ }

	tt := discoverPage(d, 1470, 956)
	if err := tt.Click("Choose a country"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("could not load the country list") {
		t.Fatalf("texts %q, want the country-list failure", tt.Texts())
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if reloads != 1 {
		t.Fatalf("reloads %d, want 1", reloads)
	}
}

// The browse grid uses the responsive 5/4/4 columns and scrolls through every
// match without pagination.
func TestDiscoverGridColumns(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		columns int
	}{
		{name: "wide", width: 1470, height: 956, columns: 5},
		{name: "medium", width: 1280, height: 720, columns: 4},
		{name: "compact", width: 1024, height: 640, columns: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDiscover()
			d.SetState(DiscoverState{
				Kind:     DiscoverReady,
				Region:   "ID",
				Channels: discoverTestChannels(40),
				Total:    40,
			})

			tt := discoverPage(d, tc.width, tc.height)

			first, ok := tt.Find("Channel 0")
			if !ok {
				t.Fatalf("texts %q, want the first tile", tt.Texts())
			}
			last, ok := tt.Find(fmt.Sprintf("Channel %d", tc.columns-1))
			if !ok {
				t.Fatalf("texts %q, want the last column tile", tt.Texts())
			}
			if first.Y != last.Y {
				t.Errorf("first row tiles Y = %v and %v, want one row", first.Y, last.Y)
			}

			next, ok := tt.Find(fmt.Sprintf("Channel %d", tc.columns))
			if !ok {
				t.Fatalf("texts %q, want the second row tile", tt.Texts())
			}
			if next.Y <= first.Y {
				t.Errorf("second row Y = %v, want below %v", next.Y, first.Y)
			}

			if tt.HasText("Next") || tt.HasText("Previous") {
				t.Errorf("texts %q, kept pagination controls", tt.Texts())
			}
		})
	}
}

// Infinite scroll asks for the next batch only when the grid nears its end.
func TestDiscoverInfiniteScroll(t *testing.T) {
	d := NewDiscover()
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(40),
		Total:    200,
		HasMore:  true,
	})

	loads := 0
	d.OnLoadMore = func() { loads++ }

	tt := discoverPage(d, 1470, 956)
	tt.Frame()
	if loads != 0 {
		t.Fatalf("loads %d before scrolling, want 0", loads)
	}

	d.scroll.MaxY = 1200
	d.scroll.Y = 900
	tt.Frame()
	if loads != 1 {
		t.Fatalf("loads %d near the end, want 1", loads)
	}

	tt.Frame()
	if loads != 1 {
		t.Fatalf("loads %d for one scroll, want 1", loads)
	}
}

// A query switches to the virtualized row list and reports a list view.
func TestDiscoverSearchUsesRows(t *testing.T) {
	d := NewDiscover()
	d.query = "chan"
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(3),
		Total:    3,
	})

	tt := discoverPage(d, 1470, 956)
	if !tt.HasText("Search results") || !tt.HasText("list view") {
		t.Fatalf("texts %q, want the search results heading", tt.Texts())
	}
	if !tt.HasText("Add to Favorites") {
		t.Fatalf("texts %q, want the shared row action", tt.Texts())
	}
	if tt.HasText("Next") {
		t.Fatalf("texts %q, paged a search result", tt.Texts())
	}
}

// A category choice narrows results and the rail reports the real counts.
func TestDiscoverCategoryFilter(t *testing.T) {
	d := NewDiscover()
	d.SetFacets(discoverTestFacets())
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(2),
		Total:    2,
	})

	var reports []DiscoverFilters
	d.OnFilter = func(filters DiscoverFilters) { reports = append(reports, filters) }

	tt := discoverPage(d, 1470, 956)
	if !tt.HasText("News") || !tt.HasText("Sports") {
		t.Fatalf("texts %q, want the category rail", tt.Texts())
	}
	if err := tt.Click("News, 2 channels"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Clear filters"); err != nil {
		t.Fatal(err)
	}

	if len(reports) != 2 {
		t.Fatalf("reports %+v, want two", reports)
	}
	if reports[0].Category != "News" {
		t.Fatalf("first report %+v, want the News category", reports[0])
	}
	if !reports[1].Empty() {
		t.Fatalf("second report %+v, want cleared filters", reports[1])
	}
}

// Each state keeps its own copy and action.
func TestDiscoverStates(t *testing.T) {
	tests := []struct {
		name   string
		state  DiscoverState
		texts  []string
		action string
	}{
		{
			name:   "loading",
			state:  DiscoverState{Kind: DiscoverLoading, Region: "ID"},
			texts:  []string{"Loading channels", "Indonesia"},
			action: "",
		},
		{
			name:   "error",
			state:  DiscoverState{Kind: DiscoverError, Region: "ID"},
			texts:  []string{"Channels unavailable", "could not load the community catalog"},
			action: "Retry",
		},
		{
			name:   "offline",
			state:  DiscoverState{Kind: DiscoverOffline, Region: "ID"},
			texts:  []string{"You are offline"},
			action: "Retry",
		},
		{
			name: "offline cache",
			state: DiscoverState{
				Kind:     DiscoverOffline,
				Region:   "ID",
				Channels: discoverTestChannels(1),
				Total:    1,
			},
			texts:  []string{"Showing saved channels", "Channel 0"},
			action: "Retry",
		},
		{
			name:   "empty",
			state:  DiscoverState{Kind: DiscoverEmpty, Region: "ID"},
			texts:  []string{"No channels yet", "has no channels"},
			action: "Retry",
		},
		{
			name:   "disabled",
			state:  DiscoverState{Kind: DiscoverDisabled, Region: "ID"},
			texts:  []string{"Community catalog is off"},
			action: "Open Settings",
		},
		{
			name:   "no country",
			state:  DiscoverState{Kind: DiscoverNoCountry},
			texts:  []string{"Choose a country to get started"},
			action: "Select a country",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDiscover()
			d.SetCountries(discoverTestCountries())
			d.SetState(tc.state)

			retries := 0
			d.OnRetry = func() { retries++ }
			settings := 0
			d.OnOpenSettings = func() { settings++ }

			tt := discoverPage(d, 1470, 956)
			for _, want := range tc.texts {
				if !tt.HasText(want) {
					t.Fatalf("texts %q, want %q", tt.Texts(), want)
				}
			}

			if tc.action == "" {
				return
			}
			if err := tt.Click(tc.action); err != nil {
				t.Fatal(err)
			}
			if tc.action == "Retry" && retries != 1 {
				t.Fatalf("retries %d, want 1", retries)
			}
			if tc.action == "Open Settings" && settings != 1 {
				t.Fatalf("settings %d, want 1", settings)
			}
		})
	}
}

// A narrowed empty result offers Clear filters instead of Retry.
func TestDiscoverFilteredEmpty(t *testing.T) {
	d := NewDiscover()
	d.SetState(DiscoverState{Kind: DiscoverReady, Region: "ID"})
	d.query = "nothing"

	tt := discoverPage(d, 1470, 956)
	if !tt.HasText("No channels match") {
		t.Fatalf("texts %q, want the filtered empty state", tt.Texts())
	}
	if err := tt.Click("Clear filters"); err != nil {
		t.Fatal(err)
	}
	if got := d.Filters(); !got.Empty() {
		t.Fatalf("filters %+v, want cleared", got)
	}
}

// Clicking a tile opens it and leaves the grid's choice on that channel.
func TestDiscoverGridClick(t *testing.T) {
	d := NewDiscover()
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(4),
		Total:    4,
	})

	var opened []string
	d.OnSelect = func(key string) { opened = append(opened, key) }

	tt := discoverPage(d, 1280, 720)
	if err := tt.Click("Channel 2"); err != nil {
		t.Fatal(err)
	}

	if len(opened) != 1 || opened[0] != "key-2" {
		t.Fatalf("opened %v, want [key-2]", opened)
	}
	if got := d.currentKey(); got != "key-2" {
		t.Fatalf("current key %q, want key-2", got)
	}
}

// F toggles the grid's chosen channel inside the shell.
func TestDiscoverShellFavorite(t *testing.T) {
	shell := NewShell()
	d := shell.Discover()
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(4),
		Total:    4,
	})
	d.selectedKey = "key-1"
	d.focus = "cell:key-1"

	var toggled string
	d.OnToggleFavorite = func(key string) { toggled = key }

	tt := ui.NewTester(shell.View, 1280, 720)
	tt.Frame()
	tt.Key(0, ui.KeyF)

	if toggled != "key-1" {
		t.Fatalf("favorite key %q, want key-1", toggled)
	}
}

// A search row's own action uses its stable key.
func TestDiscoverSearchFavorite(t *testing.T) {
	d := NewDiscover()
	d.query = "chan"
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(2),
		Total:    2,
	})
	d.SetFavorites(map[string]bool{"key-0": true})

	var toggled []string
	d.OnToggleFavorite = func(key string) { toggled = append(toggled, key) }

	tt := discoverPage(d, 1470, 956)
	if !tt.HasText("Remove from Favorites") || !tt.HasText("Add to Favorites") {
		t.Fatalf("texts %q, want both favorite actions", tt.Texts())
	}
	if err := tt.Click("Add to Favorites"); err != nil {
		t.Fatal(err)
	}
	if len(toggled) != 1 || toggled[0] != "key-1" {
		t.Fatalf("toggled %v, want [key-1]", toggled)
	}
}

// A category list drops a selection the new catalog no longer offers.
func TestDiscoverFacetsDropStaleSelection(t *testing.T) {
	d := NewDiscover()
	d.SetFacets([]DiscoverCategory{{Name: "News", Count: 2}})
	d.category = "News"

	d.SetFacets([]DiscoverCategory{{Name: "Sports", Count: 1}})

	if got := d.Filters().Category; got != "" {
		t.Fatalf("category %q, want cleared", got)
	}
}

// The community catalog carries a visible, unverified attribution line.
func TestDiscoverNotice(t *testing.T) {
	d := NewDiscover()
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(1),
		Total:    1,
	})
	d.SetNotice("Community channel list · iptv-org · not verified")

	tt := discoverPage(d, 1470, 956)
	if !tt.HasText("Community channel list · iptv-org · not verified") {
		t.Fatalf("texts %q, want the community notice", tt.Texts())
	}
}

// Reference-size captures stay out of git; set MUMU_CAPTURE_DIR to write them.
func TestDiscoverReferenceSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 1470, height: 956},
		{name: "medium", width: 1280, height: 720},
		{name: "compact", width: 1024, height: 640},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDiscover()
			d.SetCountries(discoverTestCountries())
			d.SetNotice("Community channel list · iptv-org · not verified")
			d.SetFacets([]DiscoverCategory{{Name: "News", Count: 6}, {Name: "Sports", Count: 4}})
			rows := discoverTestChannels(20)
			for i := range 3 {
				rows[i].Live = true
			}

			d.SetState(DiscoverState{
				Kind:     DiscoverReady,
				Region:   "ID",
				Channels: rows,
				Total:    348,
			})

			tt := discoverPage(d, tc.width, tc.height)
			if !tt.HasText("Indonesia channels") || !tt.HasText("348 results") {
				t.Fatalf("texts %q, want the reference layout", tt.Texts())
			}

			dir := os.Getenv("MUMU_CAPTURE_DIR")
			if dir == "" {
				return
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(dir, "discover-"+tc.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					t.Errorf("close discover capture: %v", err)
				}
			}()
			if err := png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A dark-appearance capture shows the glass search field as the app draws it.
func TestDiscoverDarkCapture(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	d.SetFacets(discoverTestFacets())
	rows := discoverTestChannels(20)
	for i := range 3 {
		rows[i].Live = true
	}
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: rows,
		Total:    348,
	})

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Themed(c, themeFor(ThemeDark, true)).Fill().Children(func() {
			d.View(c, c.Theme())
		})
	}, 1470, 956)

	dir := os.Getenv("MUMU_CAPTURE_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "discover-wide-dark.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close dark capture: %v", err)
		}
	}()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// The approved no-country frame is capturable too.
func TestDiscoverNoCountryCapture(t *testing.T) {
	d := NewDiscover()
	d.SetCountries(discoverTestCountries())
	tt := discoverPage(d, 1470, 956)

	dir := os.Getenv("MUMU_CAPTURE_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "discover-no-country.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close no-country capture: %v", err)
		}
	}()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// A discovered live channel carries its LIVE tag in the tile's top-right
// corner, above and right of the title.
func TestDiscoverLiveTagCorner(t *testing.T) {
	d := NewDiscover()
	rows := discoverTestChannels(2)
	rows[0].Live = true
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: rows,
		Total:    2,
	})

	tt := discoverPage(d, 1470, 956)
	live, ok := tt.Find("LIVE")
	if !ok {
		t.Fatalf("texts %q, want the LIVE tag", tt.Texts())
	}
	title, ok := tt.Find("Channel 0")
	if !ok {
		t.Fatalf("texts %q, want the tile title", tt.Texts())
	}
	if live.Y >= title.Y {
		t.Errorf("LIVE at Y %v, want above the title at Y %v", live.Y, title.Y)
	}
	if live.X <= title.X {
		t.Errorf("LIVE at X %v, want right of the title at X %v", live.X, title.X)
	}
}

// The grid keeps its choice on the same channel when results reorder.
func TestDiscoverSelectionFollowsKey(t *testing.T) {
	d := NewDiscover()
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(3),
		Total:    3,
	})

	tt := discoverPage(d, 1280, 720)
	d.selectedKey = "key-1"
	d.focus = "cell:key-1"
	tt.Frame()

	next := []DiscoverChannel{
		{Key: "key-3", Name: "Channel 3"},
		{Key: "key-1", Name: "Channel 1"},
	}
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: next,
		Total:    5,
	})
	tt.Frame()

	if got := d.currentKey(); got != "key-1" {
		t.Fatalf("key %q, want the same channel after the reorder", got)
	}
}
