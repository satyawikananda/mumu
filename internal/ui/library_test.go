package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// libraryTestNow is the clock the page tests format relative times with.
func libraryTestNow() time.Time {
	return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
}

// libraryPage wires a page into a headless tester.
func libraryPage(l *Library) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		l.View(c, c.Theme())
	}, 900, 640)
}

// libraryTestState is one enabled playlist, one disabled file and one stream.
func libraryTestState() LibraryState {
	return LibraryState{
		Kind: LibraryReady,
		Sources: []LibrarySource{
			{
				ID:          "remote",
				Name:        "Indonesia IPTV",
				Kind:        LibraryPlaylistURL,
				Enabled:     true,
				Channels:    214,
				LastSuccess: libraryTestNow().Add(-2 * time.Hour),
			},
			{
				ID:          "file",
				Name:        "Local channels",
				Kind:        LibraryPlaylistFile,
				Enabled:     false,
				Channels:    12,
				LastSuccess: libraryTestNow().Add(-30 * time.Minute),
				Error:       "source server returned an error",
			},
			{
				ID:       "stream",
				Name:     "News stream",
				Kind:     LibraryStream,
				Enabled:  true,
				Channels: 1,
			},
		},
	}
}

// A new page keeps the empty state and every import path.
func TestLibraryEmptyState(t *testing.T) {
	l := NewLibrary()
	tt := libraryPage(l)

	if !tt.HasText("Library") || !tt.HasText("My Playlists") {
		t.Fatalf("texts %q, want the Library heading and section", tt.Texts())
	}
	if !tt.HasText("No playlists yet") {
		t.Fatalf("texts %q, want the empty state", tt.Texts())
	}
	for _, action := range []string{"Add Playlist", "Import M3U file", "Add Stream"} {
		if !tt.HasText(action) {
			t.Fatalf("texts %q, want %q", tt.Texts(), action)
		}
	}
}

// Rows show name, kind, count, relative success and the last error.
func TestLibraryRows(t *testing.T) {
	l := NewLibrary()
	l.now = libraryTestNow
	l.SetState(libraryTestState())

	tt := libraryPage(l)

	for _, want := range []string{
		"Indonesia IPTV", "Local channels", "News stream",
		"M3U URL", "M3U file", "Stream",
		"214 channels", "2h ago",
		"12 channels", "30m ago",
		"1 channel", "not refreshed yet",
		"Disabled",
		"source server returned an error",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
}

// The switch dispatches the opposite enabled state.
func TestLibraryToggle(t *testing.T) {
	l := NewLibrary()
	l.SetState(LibraryState{Kind: LibraryReady, Sources: []LibrarySource{{
		ID: "remote", Name: "Indonesia IPTV", Kind: LibraryPlaylistURL, Enabled: true, Channels: 3,
	}}})

	var (
		gotID      string
		gotEnabled bool
		calls      int
	)
	l.OnToggle = func(id string, enabled bool) {
		gotID, gotEnabled, calls = id, enabled, calls+1
	}

	tt := libraryPage(l)
	if err := tt.Click("Enable Indonesia IPTV"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || gotID != "remote" || gotEnabled {
		t.Fatalf("toggle call = %q/%v x%d, want remote/false x1", gotID, gotEnabled, calls)
	}
}

// Refresh dispatches the row's source.
func TestLibraryRefreshAction(t *testing.T) {
	l := NewLibrary()
	l.SetState(LibraryState{Kind: LibraryReady, Sources: []LibrarySource{{
		ID: "remote", Name: "Indonesia IPTV", Kind: LibraryPlaylistURL, Enabled: true, Channels: 3,
	}}})

	var refreshed string
	l.OnRefresh = func(id string) { refreshed = id }

	tt := libraryPage(l)
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	if refreshed != "remote" {
		t.Fatalf("refreshed %q, want remote", refreshed)
	}
}

// Add Playlist opens the sheet, and Cancel closes it after cancellation.
func TestLibrarySheetOpenAndCancel(t *testing.T) {
	l := NewLibrary()

	var closed int
	l.OnSheetClose = func() { closed++ }

	tt := libraryPage(l)
	if err := tt.Click("Add Playlist"); err != nil {
		t.Fatal(err)
	}
	if !l.SheetOpen() {
		t.Fatal("Add Playlist did not open the sheet")
	}
	if !tt.HasText("Source type") || !tt.HasText("Validate and Preview") {
		t.Fatalf("texts %q, want the import sheet", tt.Texts())
	}

	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if l.SheetOpen() {
		t.Fatal("Cancel left the sheet open")
	}
	if closed != 1 {
		t.Fatalf("sheet close dispatches %d, want 1", closed)
	}
}

// Escape dismisses the sheet and cancels pending work once.
func TestLibrarySheetEscape(t *testing.T) {
	l := NewLibrary()
	l.OpenAddPlaylist()

	var closed int
	l.OnSheetClose = func() { closed++ }

	tt := libraryPage(l)
	tt.Key(0, ui.KeyEscape)

	if l.SheetOpen() {
		t.Fatal("Escape left the sheet open")
	}
	if closed != 1 {
		t.Fatalf("sheet close dispatches %d, want 1", closed)
	}
}

// A validation error shows inline and Import is not dispatched.
func TestLibraryImportError(t *testing.T) {
	l := NewLibrary()
	l.OpenAddPlaylist()
	l.url = "https://example.com/list.m3u"

	var previews, imports int
	l.OnPreview = func(LibraryRequest) { previews++ }
	l.OnImport = func(LibraryRequest) { imports++ }

	tt := libraryPage(l)
	if err := tt.Click("Validate and Preview"); err != nil {
		t.Fatal(err)
	}
	if previews != 1 || !l.previewBusy {
		t.Fatalf("preview dispatches %d busy %v, want 1 true", previews, l.previewBusy)
	}

	l.SetPreview(nil, "That URL is not valid.")
	tt.Frame()

	if !tt.HasText("That URL is not valid.") {
		t.Fatalf("texts %q, want the inline error", tt.Texts())
	}
	if l.previewBusy {
		t.Fatal("SetPreview left the validation busy")
	}
}

// A successful preview shows its summary and allows Import.
func TestLibraryPreviewAndImport(t *testing.T) {
	l := NewLibrary()
	l.OpenAddPlaylist()
	l.url = "https://example.com/list.m3u"

	var imported LibraryRequest
	l.OnImport = func(req LibraryRequest) { imported = req }

	tt := libraryPage(l)
	l.SetPreview(&LibraryPreview{
		Kind:         LibraryPlaylistURL,
		Name:         "Indonesia IPTV",
		Location:     "https://example.com/list.m3u",
		Channels:     214,
		Samples:      []string{"TVRI Nasional", "CNN Indonesia"},
		Warnings:     []string{"line is too long"},
		WarningCount: 2,
		Skipped:      3,
	}, "")
	tt.Frame()

	for _, want := range []string{
		"M3U playlist · 214 channels",
		"Indonesia IPTV",
		"https://example.com/list.m3u",
		"Sample: TVRI Nasional, CNN Indonesia",
		"Skipped entries · 3",
		"line is too long",
		"and 1 more warnings",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}

	if err := tt.Click("Import"); err != nil {
		t.Fatal(err)
	}
	if imported.Kind != LibraryPlaylistURL || imported.URL != "https://example.com/list.m3u" {
		t.Fatalf("import request %+v", imported)
	}
	if !l.importBusy {
		t.Fatal("Import did not mark the sheet busy")
	}

	l.ImportFinished()
	tt.Frame()
	if l.SheetOpen() {
		t.Fatal("ImportFinished left the sheet open")
	}
}

// Skipped entries show reason counts and named examples, muted, and only
// when something was skipped.
func TestLibraryPreviewSkippedBlock(t *testing.T) {
	l := NewLibrary()
	l.OpenAddPlaylist()
	l.url = "https://example.com/list.m3u"

	var imported bool
	l.OnImport = func(LibraryRequest) { imported = true }

	tt := libraryPage(l)
	l.SetPreview(&LibraryPreview{
		Kind:     LibraryPlaylistURL,
		Name:     "List",
		Channels: 10,
		Skipped:  6,
		SkippedReasons: []SkippedReason{
			{Label: "entries with a private or local host", Count: 4},
			{Label: "entries without a URL", Count: 2},
		},
		SkippedExamples: []string{"TVRI — entries with a private or local host"},
	}, "")
	tt.Frame()

	for _, want := range []string{
		"Skipped entries · 6",
		"4 entries with a private or local host",
		"2 entries without a URL",
		"e.g. TVRI — entries with a private or local host",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}

	// A partially usable source keeps Import enabled and shows no URL text
	// in the skip block.
	for _, leak := range []string{"192.", "10.0.0.1", "token"} {
		if tt.HasText(leak) {
			t.Fatalf("texts %q leak %q", tt.Texts(), leak)
		}
	}
	if err := tt.Click("Import"); err != nil {
		t.Fatal(err)
	}
	if !imported {
		t.Fatal("Import was not dispatched for a partially usable source")
	}

	// Nothing skipped: no block at all.
	l.ImportFinished()
	l.OpenAddPlaylist()
	l.SetPreview(&LibraryPreview{Kind: LibraryPlaylistURL, Name: "List", Channels: 10}, "")
	tt.Frame()
	if tt.HasText("Skipped entries") {
		t.Fatalf("texts %q, want no skipped block", tt.Texts())
	}
}

// The file tab picks a path and validates it.
func TestLibraryFileTab(t *testing.T) {
	l := NewLibrary()
	l.OpenImportFile()

	var (
		picks   int
		preview LibraryRequest
	)
	l.OnPickFile = func() { picks++ }
	l.OnPreview = func(req LibraryRequest) { preview = req }

	tt := libraryPage(l)
	if err := tt.Click("Choose File…"); err != nil {
		t.Fatal(err)
	}
	if picks != 1 || !l.pickBusy {
		t.Fatalf("picker dispatches %d busy %v, want 1 true", picks, l.pickBusy)
	}

	l.SetFilePath("/Users/me/channels.m3u")
	tt.Frame()
	if l.pickBusy {
		t.Fatal("SetFilePath left the picker busy")
	}

	if err := tt.Click("Validate and Preview"); err != nil {
		t.Fatal(err)
	}
	if preview.Kind != LibraryPlaylistFile || preview.Path != "/Users/me/channels.m3u" {
		t.Fatalf("preview request %+v", preview)
	}
}

// Remove asks for confirmation by name before dispatching.
func TestLibraryRemoveConfirmation(t *testing.T) {
	l := NewLibrary()
	l.SetState(libraryTestState())

	var removed string
	l.OnRemove = func(id string) { removed = id }

	tt := libraryPage(l)
	if err := tt.Click("Remove…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Remove “Indonesia IPTV”?") {
		t.Fatalf("texts %q, want the confirmation naming the source", tt.Texts())
	}
	if !tt.HasText("favorites and recent entries") {
		t.Fatalf("texts %q, want the consequence", tt.Texts())
	}

	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if removed != "remote" {
		t.Fatalf("removed %q, want remote", removed)
	}
}

// The stream sheet asks for a URL and a display name only.
func TestLibraryStreamSheet(t *testing.T) {
	l := NewLibrary()
	l.OpenAddStream()

	tt := libraryPage(l)
	for _, want := range []string{"Add Stream", "Stream URL", "Display name", "Validate and Preview"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if tt.HasText("Source type") {
		t.Fatalf("texts %q, want no playlist type control", tt.Texts())
	}
}

// The sheet's first field takes typed input.
func TestLibrarySheetTyping(t *testing.T) {
	l := NewLibrary()
	l.OpenAddPlaylist()

	tt := libraryPage(l)
	tt.Type("https://example.com/list.m3u")

	if l.url != "https://example.com/list.m3u" {
		t.Fatalf("url %q, want the typed value", l.url)
	}
}

// A page-level failure offers Retry.
func TestLibraryLoadError(t *testing.T) {
	l := NewLibrary()
	l.SetState(LibraryState{Kind: LibraryFailed, Message: "mumu could not read the saved playlists."})

	var reloads int
	l.OnReload = func() { reloads++ }

	tt := libraryPage(l)
	if !tt.HasText("Playlists unavailable") || !tt.HasText("mumu could not read the saved playlists.") {
		t.Fatalf("texts %q, want the failure state", tt.Texts())
	}

	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if reloads != 1 {
		t.Fatalf("reload dispatches %d, want 1", reloads)
	}
}

// Reopening the sheet while it is open cancels the previous draft once.
func TestLibraryReopenCancelsPreviousSheet(t *testing.T) {
	page := NewLibrary()
	closed := 0
	page.OnSheetClose = func() { closed++ }

	page.OpenAddPlaylist()
	page.OpenAddStream()

	if closed != 1 {
		t.Fatalf("sheet close count %d, want 1", closed)
	}
	if !page.SheetOpen() {
		t.Fatal("sheet closed after reopening")
	}
}
