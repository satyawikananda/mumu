package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
	"mumu/internal/playlist"
	"mumu/internal/sources"
	"mumu/internal/ui"
)

// newLibraryTestApp opens a temporary library and runs UI updates on the
// calling goroutine.
func newLibraryTestApp(t *testing.T) (*App, *fakeDispatcher) {
	t.Helper()

	a := New()
	a.openStoreAt(filepath.Join(t.TempDir(), "mumu.db"))
	a.openCatalog()
	t.Cleanup(a.closeStore)

	f := &fakeDispatcher{}
	a.dispatcher = f

	return a, f
}

// waitFor polls cond under the dispatcher's lock, which serializes it with
// UI updates.
func waitFor(t *testing.T, f *fakeDispatcher, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		ok := cond()
		f.mu.Unlock()

		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

// libraryState reads the page state on the UI thread.
func libraryState(a *App, f *fakeDispatcher) ui.LibraryState {
	var state ui.LibraryState
	f.mu.Lock()
	state = a.shell.Library().State()
	f.mu.Unlock()

	return state
}

// writeTestPlaylist writes a local M3U file with two channels.
func writeTestPlaylist(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "channels.m3u")
	body := strings.Join([]string{
		"#EXTM3U",
		`#EXTINF:-1 tvg-id="tvri" group-title="News",TVRI Nasional`,
		"https://stream.example/tvri.m3u8",
		"#EXTINF:-1,Metro TV",
		"https://stream.example/metro.m3u8",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write playlist: %v", err)
	}

	return path
}

// A local playlist imports through preview and save, then survives a failed
// refresh, disable and remove.
func TestLibrarySourceLifecycle(t *testing.T) {
	a, f := newLibraryTestApp(t)
	path := writeTestPlaylist(t)
	req := ui.LibraryRequest{Kind: ui.LibraryPlaylistFile, Path: path, Name: "My channels"}

	a.previewLibrarySource(req)
	waitFor(t, f, "preview", func() bool { return a.shell.Library().Preview() != nil })

	f.mu.Lock()
	preview := a.shell.Library().Preview()
	f.mu.Unlock()

	if preview.Kind != ui.LibraryPlaylistFile {
		t.Fatalf("preview kind %v, want a file playlist", preview.Kind)
	}
	if preview.Channels != 2 || len(preview.Samples) != 2 {
		t.Fatalf("preview %+v, want two channels and samples", preview)
	}
	if preview.Name != "My channels" {
		t.Fatalf("preview name %q, want My channels", preview.Name)
	}
	if preview.Location != "channels.m3u" {
		t.Fatalf("preview location %q, want the file name only", preview.Location)
	}

	a.importLibrarySource(req)
	waitFor(t, f, "imported source", func() bool {
		state := a.shell.Library().State()

		return state.Kind == ui.LibraryReady && len(state.Sources) == 1 && !state.Sources[0].Busy
	})

	state := libraryState(a, f)
	source := state.Sources[0]
	if source.Name != "My channels" || source.Kind != ui.LibraryPlaylistFile {
		t.Fatalf("row %+v, want the imported file playlist", source)
	}
	if !source.Enabled || source.Channels != 2 || source.Error != "" {
		t.Fatalf("row %+v, want an enabled source with two channels", source)
	}

	// A refresh that cannot read the file keeps the rows and shows the error.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove playlist: %v", err)
	}

	a.refreshLibrarySource(source.ID)
	waitFor(t, f, "failed refresh", func() bool {
		state := a.shell.Library().State()
		if len(state.Sources) != 1 || state.Sources[0].Busy {
			return false
		}

		return state.Sources[0].Error != ""
	})

	state = libraryState(a, f)
	if got := state.Sources[0].Channels; got != 2 {
		t.Fatalf("channels after failed refresh %d, want the retained 2", got)
	}
	if !strings.Contains(state.Sources[0].Error, "file") {
		t.Fatalf("row error %q, want a readable file error", state.Sources[0].Error)
	}

	// Disabling updates the row without touching the channels.
	a.toggleLibrarySource(source.ID, false)
	waitFor(t, f, "disabled source", func() bool {
		state := a.shell.Library().State()

		return len(state.Sources) == 1 && !state.Sources[0].Busy && !state.Sources[0].Enabled
	})

	// Removing deletes the row and its stored source.
	a.removeLibrarySource(source.ID)
	waitFor(t, f, "removed source", func() bool {
		state := a.shell.Library().State()

		return state.Kind == ui.LibraryReady && len(state.Sources) == 0
	})

	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	list, err := service.ListUserSources(context.Background())
	if err != nil {
		t.Fatalf("ListUserSources: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("stored sources %+v, want none after removal", list)
	}
}

// An empty draft reports an input error without network access.
func TestLibraryPreviewNeedsURL(t *testing.T) {
	a, f := newLibraryTestApp(t)

	a.previewLibrarySource(ui.LibraryRequest{Kind: ui.LibraryPlaylistURL})
	waitFor(t, f, "input error", func() bool {
		return a.shell.Library().PreviewError() == "Enter a playlist URL."
	})
}

// Preview locations drop credentials and query tokens.
func TestLibraryLocationRedactsSecrets(t *testing.T) {
	tests := []struct {
		name     string
		kind     catalog.SourceKind
		location string
		want     string
	}{
		{
			name:     "remote",
			kind:     catalog.SourceRemotePlaylist,
			location: "https://user:pass@example.com/list.m3u?token=secret#top",
			want:     "https://example.com/list.m3u",
		},
		{
			name:     "file",
			kind:     catalog.SourceLocalPlaylist,
			location: "/Users/me/Private/list.m3u",
			want:     "list.m3u",
		},
		{
			name:     "invalid",
			kind:     catalog.SourceCustomStream,
			location: "://bad",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := libraryLocation(tc.kind, tc.location); got != tc.want {
				t.Fatalf("libraryLocation = %q, want %q", got, tc.want)
			}
		})
	}
}

// Errors shown to the viewer are actionable and credential-free.
func TestLibraryErrorText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "playlist",
			err:  &playlist.Error{Message: "URL is a single stream; add it as a stream instead"},
			want: "URL is a single stream; add it as a stream instead",
		},
		{
			name: "private",
			err:  fmt.Errorf("fetch: %w", netguard.ErrPrivateAddress),
			want: "Local and private addresses are not allowed for remote playlists.",
		},
		{
			name: "deadline",
			err:  fmt.Errorf("load: %w", context.DeadlineExceeded),
			want: "The source took too long to answer. Try again.",
		},
		{
			name: "unknown",
			err:  errors.New("dial https://example.com/?token=secret: refused"),
			want: "Could not read that source. Check it and try again.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := libraryErrorText(tc.err)
			if got != tc.want {
				t.Fatalf("libraryErrorText = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "token=") || strings.Contains(got, "http") {
				t.Fatalf("error text %q leaks a location", got)
			}
		})
	}
}

// Skipped entries become reason counts and named examples without a URL.
func TestLibraryPreviewSkippedReasons(t *testing.T) {
	preview := libraryPreview(sources.Preview{
		Kind:        catalog.SourceRemotePlaylist,
		DisplayName: "List",
		Channels:    []catalog.Channel{{Name: "TVRI"}},
		Warnings:    []playlist.Warning{{Message: "line is too long"}},
		Skipped:     5,
		SkippedReasons: map[playlist.Code]int{
			playlist.CodePrivateAddress: 4,
			playlist.CodeMissingURL:     1,
		},
		SkippedEntries: []playlist.SkippedEntry{
			{Line: 12, Name: "TVRI", Reason: playlist.CodePrivateAddress, Scheme: "http", Host: "192.168.1.10"},
			{Line: 15, Name: "Secret", Reason: playlist.CodeCredentials, Host: "example.com"},
			{Line: 18, Reason: playlist.CodeMissingURL, Host: "10.0.0.1"},
			{Line: 20, Name: "Fourth", Reason: playlist.CodeInvalidURL},
			{Line: 21, Name: "Fifth", Reason: playlist.CodeInvalidURL},
		},
	})

	if preview.Skipped != 5 {
		t.Fatalf("skipped %d, want 5", preview.Skipped)
	}
	if len(preview.SkippedReasons) != 2 {
		t.Fatalf("reasons %+v, want two", preview.SkippedReasons)
	}
	if got := preview.SkippedReasons[0]; got.Count != 4 || got.Label != "entries with a private or local host" {
		t.Fatalf("first reason %+v, want the private host count", got)
	}
	if got := preview.SkippedReasons[1]; got.Count != 1 || got.Label != "entries without a URL" {
		t.Fatalf("second reason %+v, want the missing URL count", got)
	}

	wantExamples := []string{
		"TVRI — entries with a private or local host",
		"Secret — entries with embedded credentials",
		"Fourth — entries with an invalid URL",
	}
	if len(preview.SkippedExamples) != len(wantExamples) {
		t.Fatalf("examples %+v, want %d", preview.SkippedExamples, len(wantExamples))
	}
	for i, want := range wantExamples {
		if preview.SkippedExamples[i] != want {
			t.Fatalf("example %d = %q, want %q", i, preview.SkippedExamples[i], want)
		}
	}

	// Parser warnings are counted separately from skipped entries.
	if preview.WarningCount != 1 || len(preview.Warnings) != 1 || preview.Warnings[0] != "line is too long" {
		t.Fatalf("warnings %+v count %d, want the parser warning only", preview.Warnings, preview.WarningCount)
	}

	joined := strings.Join(preview.SkippedExamples, " ")
	for _, leak := range []string{"http", "192.", "10.0.0.1", "example.com", "token", "/"} {
		if strings.Contains(joined, leak) {
			t.Fatalf("examples %q leak %q", joined, leak)
		}
	}
}
