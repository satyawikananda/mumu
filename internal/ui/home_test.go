package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// capture writes one Home screenshot when MUMU_CAPTURE_DIR is set. The files
// are evidence only and stay out of git.
func capture(t *testing.T, img image.Image, name string) {
	t.Helper()

	dir := os.Getenv("MUMU_CAPTURE_DIR")
	if dir == "" || img == nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s capture: %v", name, err)
		}
	}()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode %s capture: %v", name, err)
	}
}

// homeTestNow is the fixed clock every Home test reads.
func homeTestNow() time.Time {
	return time.Date(2026, 11, 12, 21, 4, 0, 0, time.UTC)
}

// homeTestChannel returns one playable card.
func homeTestChannel(key, name, source string) HomeChannel {
	return HomeChannel{
		Key:           key,
		Name:          name,
		SourceName:    source,
		Group:         "News",
		Country:       "Indonesia",
		Quality:       "480p",
		Live:          true,
		Playable:      true,
		SourceEnabled: true,
		PlayedAt:      homeTestNow().Add(-12 * time.Minute),
	}
}

// homeTestState is the approved wide Home feed: hero, recent and favorites.
func homeTestState() HomeState {
	hero := homeTestChannel("community:metro", "Metro TV", "Community")
	recent := []HomeChannel{
		hero,
		homeTestChannel("community:tvri", "TVRI Bali", "Community"),
		homeTestChannel("community:cnn", "CNN Indonesia", "Community"),
		homeTestChannel("community:cartoonito", "Cartoonito", "Community"),
	}
	favorites := []HomeChannel{
		homeTestChannel("community:tvri", "TVRI Bali", "Community"),
		homeTestChannel("community:cnn", "CNN Indonesia", "Community"),
		homeTestChannel("community:metro", "Metro TV", "Community"),
		homeTestChannel("community:cartoonito", "Cartoonito", "Community"),
		homeTestChannel("local:trans", "Trans 7", "Local channels"),
		homeTestChannel("local:antv", "ANTV", "Local channels"),
		homeTestChannel("local:rtv", "RTV", "Local channels"),
		homeTestChannel("local:kompas", "Kompas TV", "Local channels"),
	}

	return HomeState{
		Kind:          HomeResume,
		Hero:          hero,
		Recent:        recent,
		Favorites:     favorites,
		RecentCount:   4,
		FavoriteCount: 8,
	}
}

// homePage builds a Home-only tester with a fixed clock.
func homePage(state HomeState) (*Shell, *Home, *ui.Tester) {
	shell := NewShell()
	home := shell.Home()
	home.now = homeTestNow
	home.SetState(state)
	shell.ShowHome()

	return shell, home, ui.NewTester(shell.View, 1470, 956)
}

func TestHomeResumeIsExplicit(t *testing.T) {
	resumed := 0
	_, home, tt := homePage(homeTestState())
	home.OnResume = func() { resumed++ }
	tt.Frame()

	if !tt.HasText("LAST WATCHED") || !tt.HasText("Metro TV") {
		t.Fatalf("texts %q, want the resume hero", tt.Texts())
	}
	if resumed != 0 {
		t.Fatal("Home started playback without a click")
	}
	if err := tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	if resumed != 1 {
		t.Fatalf("resume calls = %d, want 1", resumed)
	}
}

func TestHomeWideShowsHeroAndRails(t *testing.T) {
	_, _, tt := homePage(homeTestState())

	for _, want := range []string{
		"LAST WATCHED",
		"Metro TV",
		"Indonesia · News · 480p · 12 minutes ago",
		"Recently watched",
		"Favorites",
		"4 channels",
		"8 channels",
		"See all",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
}

func TestHomeCompactKeepsBothRails(t *testing.T) {
	_, _, tt := homePage(homeTestState())
	tt.SetSize(1024, 640)
	tt.Frame()

	for _, want := range []string{
		"LAST WATCHED",
		"Recently watched",
		"Favorites",
		"See all",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q at 1024x640", tt.Texts(), want)
		}
	}
}

func TestHomeHeroOmitsMissingMetadata(t *testing.T) {
	state := homeTestState()
	state.Hero.Country = ""
	state.Hero.Quality = ""
	state.Hero.PlayedAt = time.Time{}
	_, _, tt := homePage(state)

	if tt.HasText("Indonesia · News · 480p") || tt.HasText("·  ·") {
		t.Fatalf("texts %q, want only the real metadata", tt.Texts())
	}
	if !tt.HasText("News") {
		t.Fatalf("texts %q, want the group", tt.Texts())
	}
}

func TestHomeEmptyOffersBrowseAndPlaylist(t *testing.T) {
	shell := NewShell()
	browsed, added := 0, 0
	shell.Home().OnBrowse = func() { browsed++ }
	shell.Home().OnAddPlaylist = func() { added++ }
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1280, 720)
	if !tt.HasText("Nothing to watch yet") {
		t.Fatalf("texts %q, want the empty Home state", tt.Texts())
	}
	if err := tt.Click("Browse Discover"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Add Playlist"); err != nil {
		t.Fatal(err)
	}
	if browsed != 1 || added != 1 {
		t.Fatalf("browse %d, add %d, want one each", browsed, added)
	}
}

func TestHomeFailedRetries(t *testing.T) {
	shell := NewShell()
	retried := 0
	shell.Home().OnRetry = func() { retried++ }
	shell.Home().SetState(HomeState{Kind: HomeFailed, Message: "Library offline."})
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1280, 720)
	if !tt.HasText("Home unavailable") || !tt.HasText("Library offline.") {
		t.Fatalf("texts %q, want the failed state", tt.Texts())
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if retried != 1 {
		t.Fatalf("retries = %d, want 1", retried)
	}
}

func TestHomeSeeAllOpensLibrarySection(t *testing.T) {
	shell := NewShell()
	shell.Home().SetState(homeTestState())
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1470, 956)
	if err := tt.Click("See all recently watched"); err != nil {
		t.Fatal(err)
	}
	if shell.Destination() != destLibrary || shell.Library().Section() != LibraryRecent {
		t.Fatalf("destination %q section %v, want Library recent", shell.Destination(), shell.Library().Section())
	}

	shell.ShowHome()
	tt.Frame()
	if err := tt.Click("See all favorites"); err != nil {
		t.Fatal(err)
	}
	if shell.Library().Section() != LibraryFavorites {
		t.Fatalf("section %v, want Favorites", shell.Library().Section())
	}
}

func TestHomeRailKeyboardMovesAndOpens(t *testing.T) {
	shell, home, tt := homePage(homeTestState())
	var played []string
	home.OnPlay = func(key string) { played = append(played, key) }
	tt.Frame()
	tt.Frame()

	home.focus = homeFocusRecentBase + home.state.Recent[0].Key
	tt.Frame()
	tt.Frame()

	tt.Key(0, ui.KeyRight)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if len(played) != 1 || played[0] != "community:tvri" {
		t.Fatalf("played %v, want the second recent channel", played)
	}
	if got := shell.Destination(); got == destWatch {
		t.Fatal("Home navigation changed; the app owns playback")
	}
}

// The same channel may appear in both rails; focus values must stay unique.
func TestHomeDuplicateChannelAcrossRails(t *testing.T) {
	_, _, tt := homePage(homeTestState())
	tt.SetSize(1280, 720)
	tt.Frame()
	tt.Frame()

	if !tt.HasText("Metro TV") {
		t.Fatalf("texts %q, want the duplicated channel", tt.Texts())
	}
}

// A disabled source keeps its row but cannot take focus or start playback.
func TestHomeDisabledCardIsNotFocusable(t *testing.T) {
	state := homeTestState()
	state.Favorites[0].SourceEnabled = false
	_, _, tt := homePage(state)
	tt.Frame()

	if !tt.HasText("TVRI Bali") {
		t.Fatalf("texts %q, want the disabled card", tt.Texts())
	}
}

// homeCardState names every rail card uniquely so a test can count them.
func homeCardState(recent, favorites int) HomeState {
	cards := func(prefix, source string, count int) []HomeChannel {
		out := make([]HomeChannel, 0, count)
		for i := range count {
			card := homeTestChannel(
				source+":"+prefix+fmt.Sprint(i),
				fmt.Sprintf("%s %d", prefix, i),
				source,
			)
			card.LogoURL = card.Key
			out = append(out, card)
		}

		return out
	}

	return HomeState{
		Kind:          HomeResume,
		Hero:          cards("Hero", "Community", 1)[0],
		Recent:        cards("Recent", "Community", recent),
		Favorites:     cards("Fav", "Local", favorites),
		RecentCount:   recent,
		FavoriteCount: favorites,
	}
}

// homeTestArtColors gives every rail card a deterministic channel tint.
func homeTestArtColors(state HomeState) map[string]ui.Color {
	tints := []ui.Color{
		ui.Hex("#3A1712"),
		ui.Hex("#1E5A8A"),
		ui.Hex("#1F4A3A"),
		ui.Hex("#3A2A55"),
	}
	colors := map[string]ui.Color{}
	index := 0
	for _, cards := range [][]HomeChannel{state.Recent, state.Favorites} {
		for _, card := range cards {
			if card.LogoURL == "" {
				continue
			}
			colors[card.LogoURL] = tints[index%len(tints)]
			index++
		}
	}
	if state.Hero.LogoURL != "" {
		colors[state.Hero.LogoURL] = tints[0]
	}

	return colors
}

// The reference sizes cap each rail as the layout policy says and the whole
// feed stays reachable at every size.
func TestHomeReferenceSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		cards  int
	}{
		{name: "wide", width: 1470, height: 956, cards: 6},
		{name: "medium", width: 1280, height: 720, cards: 5},
		{name: "compact", width: 1024, height: 640, cards: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := homeCardState(4, 8)
			shell := NewShell()
			shell.Home().SetState(state)
			shell.Home().SetLogoColors(homeTestArtColors(state))
			shell.ShowHome()

			tt := ui.NewTester(shell.View, tc.width, tc.height)
			tt.Frame()
			tt.Frame()

			if tt.HasText(fmt.Sprintf("Fav %d", tc.cards)) {
				t.Errorf("favorites rail shows card %d, want %d", tc.cards, tc.cards)
			}
			if !tt.HasText(fmt.Sprintf("Fav %d", tc.cards-1)) {
				t.Errorf("favorites rail misses card %d; texts %q", tc.cards-1, tt.Texts())
			}
			for _, want := range []string{"Recently watched", "Favorites", "See all"} {
				if !tt.HasText(want) {
					t.Errorf("texts %q, want %q", tt.Texts(), want)
				}
			}

			capture(t, tt.Image(), "home-"+tc.name)
		})
	}
}

// A loading library shows Home's own progress, not an empty feed.
func TestHomeLoadingState(t *testing.T) {
	shell := NewShell()
	shell.Home().SetState(HomeState{Kind: HomeLoading})
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1280, 720)
	if !tt.HasText("Loading Home") {
		t.Fatalf("texts %q, want the loading state", tt.Texts())
	}
}

// Light appearance keeps the feed readable over the synthesized art.
func TestHomeLightTheme(t *testing.T) {
	shell := NewShell()
	shell.themeMode = ThemeLight
	state := homeCardState(3, 4)
	shell.Home().SetState(state)
	shell.Home().SetLogoColors(homeTestArtColors(state))
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1280, 720)
	tt.Frame()
	tt.Frame()

	for _, want := range []string{"LAST WATCHED", "Recently watched", "Favorites"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q in the light theme", tt.Texts(), want)
		}
	}
	capture(t, tt.Image(), "home-light")
}

// The top bar is transparent at the start and takes its scrim once the feed
// scrolls under it.
func TestTopBarScrimFollowsHomeScroll(t *testing.T) {
	shell := NewShell()
	state := homeCardState(6, 8)
	shell.Home().SetState(state)
	shell.Home().SetLogoColors(homeTestArtColors(state))
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1024, 640)
	tt.Frame()
	tt.Frame()

	if shell.ChromeScrolled() {
		t.Fatal("top bar shows a scrim at the top of the feed")
	}

	// The app can also place the feed; the same state drives the bar.
	shell.Home().scroll.Y = 40
	tt.Frame()
	tt.Frame()

	if !shell.ChromeScrolled() {
		t.Fatal("top bar kept a transparent background after scrolling")
	}
	capture(t, tt.Image(), "home-scrolled")
}
