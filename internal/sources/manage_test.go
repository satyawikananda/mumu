package sources

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/catalog/iptvorg"
	"mumu/internal/netguard"
	"mumu/internal/playlist"
	"mumu/internal/storage"
)

type manageServer struct {
	*httptest.Server
	mu       sync.Mutex
	playlist string
	etag     string
	fail     bool
}

func (s *manageServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	body, etag, fail := s.playlist, s.etag, s.fail
	s.mu.Unlock()

	switch r.URL.Path {
	case "/api/countries.json":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Indonesia","code":"ID"}]`))
	case "/api/blocklist.json":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	case "/list.m3u", "/list2.m3u":
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)

			return
		}
		if etag != "" && r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)

			return
		}
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte(body))
	case "/live.m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n"))
	case "/iptv/countries/id.m3u":
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte(testPlaylist))
	case "/page":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	default:
		http.NotFound(w, r)
	}
}

func newManageService(t *testing.T) (*Service, *manageServer, *storage.Store) {
	t.Helper()

	store, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := &manageServer{
		playlist: `#EXTM3U
#EXTINF:-1 tvg-id="one",One (720p)
https://stream.example/one.m3u8
#EXTINF:-1 tvg-id="two",Two
https://stream.example/two.m3u8
`,
		etag: `"v1"`,
	}
	server.Server = httptest.NewServer(http.HandlerFunc(server.handler))
	t.Cleanup(server.Close)

	policy := netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second}
	provider := iptvorg.NewProviderWithBase(netguard.New(policy), server.URL)
	service, err := New(
		store,
		playlist.NewLoader(policy),
		provider,
		WithLogger(slog.New(slog.DiscardHandler)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return service, server, store
}

func TestPreviewRemoteAndSave(t *testing.T) {
	service, server, _ := newManageService(t)
	ctx := t.Context()

	preview, err := service.PreviewRemote(ctx, server.URL+"/list.m3u", "My list")
	if err != nil {
		t.Fatalf("PreviewRemote: %v", err)
	}
	if preview.Kind != catalog.SourceRemotePlaylist || len(preview.Channels) != 2 || preview.ETag != `"v1"` {
		t.Fatalf("preview = %+v", preview)
	}

	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if source.ItemCount != 2 || source.DisplayName != "My list" || !source.Enabled {
		t.Fatalf("source = %+v", source)
	}

	channels, err := service.Channels(ctx, source.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 2 || len(channels[0].Streams) != 1 {
		t.Fatalf("channels = %+v", channels)
	}

	userSources, err := service.ListUserSources(ctx)
	if err != nil {
		t.Fatalf("ListUserSources: %v", err)
	}
	if len(userSources) != 1 || userSources[0].ID != source.ID {
		t.Fatalf("user sources = %+v", userSources)
	}
}

func TestPreviewRemoteRejectsHLS(t *testing.T) {
	service, server, _ := newManageService(t)

	_, err := service.PreviewRemote(t.Context(), server.URL+"/live.m3u8", "")
	if !errors.Is(err, playlist.ErrPlaylistNotStream) {
		t.Fatalf("error %v, want ErrPlaylistNotStream", err)
	}
}

func TestPreviewLocal(t *testing.T) {
	service, _, _ := newManageService(t)
	path := filepath.Join(t.TempDir(), "channels.m3u")
	if err := os.WriteFile(path, []byte(`#EXTM3U
#EXTINF:-1 tvg-id="local",Local
local.m3u8
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	preview, err := service.PreviewLocal(t.Context(), path, "")
	if err != nil {
		t.Fatalf("PreviewLocal: %v", err)
	}
	if preview.Kind != catalog.SourceLocalPlaylist || preview.DisplayName != "channels" {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Channels) != 1 {
		t.Fatalf("channels = %+v", preview.Channels)
	}
}

func TestPreviewStreamAndSave(t *testing.T) {
	service, server, _ := newManageService(t)
	ctx := t.Context()

	preview, err := service.PreviewStream(ctx, server.URL+"/live.m3u8", "Live")
	if err != nil {
		t.Fatalf("PreviewStream: %v", err)
	}
	if preview.Kind != catalog.SourceCustomStream || len(preview.Channels) != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if !preview.Channels[0].Streams[0].HLS {
		t.Fatal("HLS stream not marked")
	}
	// The loopback test server is a private host, so the preview asks for
	// consent and Save drops the channel without it.
	if !preview.NeedsPrivate || len(preview.Schemes) != 1 || preview.Schemes[0] != "http" {
		t.Fatalf("preview policy = %v %v, want private http", preview.NeedsPrivate, preview.Schemes)
	}
	preview.AllowPrivate = true

	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	channels, err := service.Channels(ctx, source.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 1 || channels[0].Name != "Live" {
		t.Fatalf("channels = %+v", channels)
	}
}

func TestPreviewStreamRejectsPlaylistAndPage(t *testing.T) {
	service, server, _ := newManageService(t)

	if _, err := service.PreviewStream(t.Context(), server.URL+"/list.m3u", ""); !errors.Is(err, playlist.ErrPlaylistNotStream) {
		t.Fatalf("playlist error %v, want ErrPlaylistNotStream", err)
	}
	if _, err := service.PreviewStream(t.Context(), server.URL+"/page", ""); !errors.Is(err, playlist.ErrUnsupported) {
		t.Fatalf("page error %v, want ErrUnsupported", err)
	}
}

func TestRefreshSourceAndFailureKeepsCache(t *testing.T) {
	service, server, _ := newManageService(t)
	ctx := t.Context()

	preview, err := service.PreviewRemote(ctx, server.URL+"/list.m3u", "My list")
	if err != nil {
		t.Fatalf("PreviewRemote: %v", err)
	}
	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	server.mu.Lock()
	server.playlist = `#EXTM3U
#EXTINF:-1 tvg-id="one",One (720p)
https://stream.example/one.m3u8
`
	server.etag = `"v2"`
	server.mu.Unlock()

	result, err := service.Refresh(ctx, source.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Channels != 1 || result.Removed != 1 {
		t.Fatalf("result = %+v", result)
	}

	server.mu.Lock()
	server.fail = true
	server.mu.Unlock()

	if _, err := service.Refresh(ctx, source.ID); err == nil {
		t.Fatal("refresh succeeded, want failure")
	}

	sources, err := service.ListUserSources(ctx)
	if err != nil {
		t.Fatalf("ListUserSources: %v", err)
	}
	if len(sources) != 1 || sources[0].RefreshError == "" {
		t.Fatalf("source after failure = %+v", sources)
	}

	channels, err := service.Channels(ctx, source.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels after failure = %+v", channels)
	}
}

func TestDisableRemoveAndBuiltinProtection(t *testing.T) {
	service, server, _ := newManageService(t)
	ctx := t.Context()

	preview, err := service.PreviewRemote(ctx, server.URL+"/list.m3u", "My list")
	if err != nil {
		t.Fatalf("PreviewRemote: %v", err)
	}
	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := service.SetEnabled(ctx, source.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	sources, err := service.ListUserSources(ctx)
	if err != nil {
		t.Fatalf("ListUserSources: %v", err)
	}
	if sources[0].Enabled {
		t.Fatal("source still enabled")
	}

	if err := service.Remove(ctx, source.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	channels, err := service.Channels(ctx, source.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("channels after remove = %+v", channels)
	}

	builtin, err := service.RefreshBuiltin(ctx, "ID")
	if err != nil {
		t.Fatalf("RefreshBuiltin: %v", err)
	}
	if err := service.SetEnabled(ctx, builtin.ID, false); !errors.Is(err, ErrBuiltinManaged) {
		t.Fatalf("builtin disable error %v, want ErrBuiltinManaged", err)
	}
	if err := service.Remove(ctx, builtin.ID); !errors.Is(err, ErrBuiltinManaged) {
		t.Fatalf("builtin remove error %v, want ErrBuiltinManaged", err)
	}
}

func TestSaveRejectsEmptyPreview(t *testing.T) {
	service, _, _ := newManageService(t)

	_, err := service.Save(t.Context(), Preview{
		Kind:        catalog.SourceRemotePlaylist,
		Location:    "https://example.com/list.m3u",
		DisplayName: "Empty",
	})
	if !errors.Is(err, ErrEmptyPreview) {
		t.Fatalf("error %v, want ErrEmptyPreview", err)
	}
}

func TestRefreshCustomStream(t *testing.T) {
	service, server, _ := newManageService(t)
	ctx := t.Context()

	preview, err := service.PreviewStream(ctx, server.URL+"/live.m3u8", "Live")
	if err != nil {
		t.Fatalf("PreviewStream: %v", err)
	}
	preview.AllowPrivate = true
	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	result, err := service.Refresh(ctx, source.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Channels != 1 {
		t.Fatalf("result = %+v", result)
	}
	if !result.Source.AllowPrivate {
		t.Fatalf("refresh lost the stored consent: %+v", result.Source)
	}
}

func TestRefreshLocalFile(t *testing.T) {
	service, _, _ := newManageService(t)
	ctx := t.Context()

	path := filepath.Join(t.TempDir(), "local.m3u")
	if err := os.WriteFile(path, []byte(`#EXTM3U
#EXTINF:-1 tvg-id="one",One
https://stream.example/one.m3u8
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	preview, err := service.PreviewLocal(ctx, path, "Local")
	if err != nil {
		t.Fatalf("PreviewLocal: %v", err)
	}
	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.WriteFile(path, []byte(`#EXTM3U
#EXTINF:-1 tvg-id="one",One
https://stream.example/one.m3u8
#EXTINF:-1 tvg-id="two",Two
https://stream.example/two.m3u8
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := service.Refresh(ctx, source.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Channels != 2 {
		t.Fatalf("channels %d, want 2", result.Channels)
	}
}

func TestRemoveCascadesFavorites(t *testing.T) {
	service, server, store := newManageService(t)
	ctx := t.Context()

	preview, err := service.PreviewRemote(ctx, server.URL+"/list.m3u", "My list")
	if err != nil {
		t.Fatalf("PreviewRemote: %v", err)
	}
	source, err := service.Save(ctx, preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	channels, err := service.Channels(ctx, source.ID)
	if err != nil || len(channels) == 0 {
		t.Fatalf("Channels = %+v, %v", channels, err)
	}
	stored, found, err := store.Channels().FindByProvider(ctx, source.ID, channels[0].ProviderID)
	if err != nil || !found {
		t.Fatalf("FindByProvider = %v, %v", found, err)
	}
	if err := store.Favorites().Add(ctx, stored.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(ctx, stored.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	if err := service.Remove(ctx, source.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	favorites, err := store.Favorites().List(ctx)
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	recent, err := store.Recent().List(ctx, 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(favorites) != 0 || len(recent) != 0 {
		t.Fatalf("favorites=%d recent=%d, want none", len(favorites), len(recent))
	}
}

func TestUserChannelsOnlyEnabledSources(t *testing.T) {
	service, server, _ := newManageService(t)
	ctx := t.Context()

	first, err := service.PreviewRemote(ctx, server.URL+"/list.m3u", "First")
	if err != nil {
		t.Fatalf("PreviewRemote first: %v", err)
	}
	savedFirst, err := service.Save(ctx, first)
	if err != nil {
		t.Fatalf("Save first: %v", err)
	}
	second, err := service.PreviewRemote(ctx, server.URL+"/list2.m3u", "Second")
	if err != nil {
		t.Fatalf("PreviewRemote second: %v", err)
	}
	savedSecond, err := service.Save(ctx, second)
	if err != nil {
		t.Fatalf("Save second: %v", err)
	}

	if err := service.SetEnabled(ctx, savedFirst.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	channels, err := service.UserChannels(ctx)
	if err != nil {
		t.Fatalf("UserChannels: %v", err)
	}
	if len(channels) != len(second.Channels) {
		t.Fatalf("channels %d, want only the enabled source's %d", len(channels), len(second.Channels))
	}
	for _, channel := range channels {
		if channel.SourceID != savedSecond.ID {
			t.Fatalf("channel %+v from disabled source %q", channel, savedFirst.ID)
		}
	}
}
