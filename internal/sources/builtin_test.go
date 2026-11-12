package sources

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mumu/internal/catalog/iptvorg"
	"mumu/internal/netguard"
	"mumu/internal/playlist"
	"mumu/internal/storage"
)

const testPlaylist = `#EXTM3U
#EXTINF:-1 tvg-id="good" group-title="News",Good (720p)
https://stream.example/good.m3u8
#EXTINF:-1 tvg-id="blocked" group-title="News",Blocked
https://bad.example/live.m3u8
#EXTINF:-1 tvg-id="private",Private
http://192.168.1.5/live.m3u8
`

type testServer struct {
	*httptest.Server
	mu         sync.Mutex
	requests   int
	playlist   string
	etag       string
	blocklist  string
	fail       bool
	failBlocks bool
	countries  int
	blocks     int
}

func (s *testServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	playlistBody, etag, fail := s.playlist, s.etag, s.fail
	blocklistBody := s.blocklist
	s.mu.Unlock()

	switch r.URL.Path {
	case "/api/countries.json":
		s.mu.Lock()
		s.countries++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Indonesia","code":"ID"}]`))
	case "/api/blocklist.json":
		s.mu.Lock()
		s.blocks++
		failBlocks := s.failBlocks
		s.mu.Unlock()
		if fail || failBlocks {
			http.Error(w, "boom", http.StatusInternalServerError)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		if blocklistBody == "" {
			blocklistBody = "[]"
		}
		_, _ = w.Write([]byte(blocklistBody))
	case "/iptv/countries/id.m3u", "/iptv/countries/us.m3u":
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
		_, _ = w.Write([]byte(playlistBody))
	default:
		http.NotFound(w, r)
	}
}

func newTestService(t *testing.T) (*Service, *testServer, *storage.Store) {
	t.Helper()

	store, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := &testServer{playlist: testPlaylist, etag: `"v1"`}
	server.Server = httptest.NewServer(http.HandlerFunc(server.handler))
	t.Cleanup(server.Close)

	policy := netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second}
	provider := iptvorg.NewProviderWithBase(netguard.New(policy), server.URL)
	loader := playlist.NewLoader(policy)

	service, err := New(store, loader, provider,
		WithBlocklist(iptvorg.Blocklist{
			Channels: map[string]struct{}{"blocked": {}},
			Hosts:    []string{"bad.example"},
		}),
		WithLogger(slog.New(slog.DiscardHandler)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return service, server, store
}

func TestBuiltinRefreshAndCache(t *testing.T) {
	service, _, _ := newTestService(t)

	if region, err := service.Region(t.Context()); err != nil || region != "" {
		t.Fatalf("Region = %q, %v, want no country before a choice", region, err)
	}
	if enabled, err := service.BuiltinEnabled(t.Context()); err != nil || !enabled {
		t.Fatalf("BuiltinEnabled = %v, %v, want true", enabled, err)
	}
	if err := service.SetRegion(t.Context(), "ID"); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	if region, err := service.Region(t.Context()); err != nil || region != "ID" {
		t.Fatalf("Region = %q, %v, want the stored ID", region, err)
	}

	source, err := service.RefreshBuiltin(t.Context(), "ID")
	if err != nil {
		t.Fatalf("RefreshBuiltin: %v", err)
	}
	if source.ItemCount != 1 {
		t.Fatalf("item count %d, want 1 (blocked and private dropped)", source.ItemCount)
	}

	cached, channels, found, err := service.BuiltinSource(t.Context(), "ID")
	if err != nil || !found {
		t.Fatalf("BuiltinSource = %v, %v, %v", found, cached, err)
	}
	if len(channels) != 1 || channels[0].Name != "Good (720p)" || len(channels[0].Streams) != 1 {
		t.Fatalf("channels = %+v", channels)
	}

	if stale := service.BuiltinStale(cached); stale {
		t.Fatal("just refreshed source reported stale")
	}
}

func TestBuiltinRefreshNotModified(t *testing.T) {
	service, server, _ := newTestService(t)

	if _, err := service.RefreshBuiltin(t.Context(), "ID"); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	server.mu.Lock()
	server.playlist = "#EXTM3U\n"
	server.mu.Unlock()

	source, err := service.RefreshBuiltin(t.Context(), "ID")
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if source.ItemCount != 1 {
		t.Fatalf("item count %d after 304, want 1", source.ItemCount)
	}

	_, channels, _, err := service.BuiltinSource(t.Context(), "ID")
	if err != nil {
		t.Fatalf("BuiltinSource: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels after 304 = %+v", channels)
	}
}

func TestBuiltinRefreshFailureKeepsCache(t *testing.T) {
	service, server, _ := newTestService(t)

	if _, err := service.RefreshBuiltin(t.Context(), "ID"); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	server.mu.Lock()
	server.fail = true
	server.mu.Unlock()

	if _, err := service.RefreshBuiltin(t.Context(), "ID"); err == nil {
		t.Fatal("refresh succeeded, want failure")
	}

	cached, channels, found, err := service.BuiltinSource(t.Context(), "ID")
	if err != nil || !found {
		t.Fatalf("cached source = %v, %v, %v", found, cached, err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels after failure = %+v", channels)
	}
	if cached.LastSuccessAt.IsZero() {
		t.Fatal("last success cleared by failure")
	}
	if cached.RefreshError == "" {
		t.Fatal("refresh error not recorded")
	}
}

func TestCountriesCache(t *testing.T) {
	service, server, _ := newTestService(t)

	countries, stale, err := service.Countries(t.Context())
	if err != nil || stale || len(countries) != 1 {
		t.Fatalf("Countries = %+v, %v, %v", countries, stale, err)
	}

	countries, stale, err = service.Countries(t.Context())
	if err != nil || stale || len(countries) != 1 {
		t.Fatalf("cached Countries = %+v, %v, %v", countries, stale, err)
	}

	server.mu.Lock()
	requests := server.countries
	server.mu.Unlock()
	if requests != 1 {
		t.Fatalf("country requests %d, want 1", requests)
	}
}

func TestRegionValidation(t *testing.T) {
	service, _, _ := newTestService(t)

	if err := service.SetRegion(t.Context(), "us"); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	if region, err := service.Region(t.Context()); err != nil || region != "US" {
		t.Fatalf("Region = %q, %v, want US", region, err)
	}
	if err := service.SetRegion(t.Context(), "USA"); !errors.Is(err, iptvorg.ErrInvalidCountry) {
		t.Fatalf("error %v, want ErrInvalidCountry", err)
	}
}

// A stored value is trimmed and upper-cased before validation.
func TestRegionStoredValueNormalized(t *testing.T) {
	service, _, store := newTestService(t)

	if err := store.Settings().Set(t.Context(), SettingRegion, " us "); err != nil {
		t.Fatalf("store region: %v", err)
	}
	if region, err := service.Region(t.Context()); err != nil || region != "US" {
		t.Fatalf("Region = %q, %v, want US", region, err)
	}
}

// A stored value that cannot identify a catalog country is reported as the
// empty string instead of being replaced by a guessed country.
func TestRegionInvalidStoredValue(t *testing.T) {
	service, _, store := newTestService(t)

	if err := store.Settings().Set(t.Context(), SettingRegion, "usa"); err != nil {
		t.Fatalf("store region: %v", err)
	}
	region, err := service.Region(t.Context())
	if err != nil {
		t.Fatalf("Region: %v", err)
	}
	if region != "" {
		t.Fatalf("Region = %q, want empty for an invalid stored value", region)
	}
}

// A catalog operation without a country reports ErrNoRegion before any
// provider or network call.
func TestBuiltinRequiresRegion(t *testing.T) {
	service, server, _ := newTestService(t)

	for _, tc := range []struct {
		name   string
		region string
		call   func(ctx context.Context, region string) error
	}{
		{
			name:   "source empty region",
			region: "",
			call: func(ctx context.Context, region string) error {
				_, _, _, err := service.BuiltinSource(ctx, region)

				return err
			},
		},
		{
			name:   "source blank region",
			region: "   ",
			call: func(ctx context.Context, region string) error {
				_, _, _, err := service.BuiltinSource(ctx, region)

				return err
			},
		},
		{
			name:   "refresh empty region",
			region: "",
			call: func(ctx context.Context, region string) error {
				_, err := service.RefreshBuiltin(ctx, region)

				return err
			},
		},
		{
			name:   "refresh blank region",
			region: "  ",
			call: func(ctx context.Context, region string) error {
				_, err := service.RefreshBuiltin(ctx, region)

				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(t.Context(), tc.region); !errors.Is(err, ErrNoRegion) {
				t.Fatalf("error %v, want ErrNoRegion", err)
			}
		})
	}

	server.mu.Lock()
	requests := server.requests
	server.mu.Unlock()
	if requests != 0 {
		t.Fatalf("HTTP requests %d, want none", requests)
	}
}

func TestBuiltinDisabledSetting(t *testing.T) {
	service, _, _ := newTestService(t)

	if err := service.SetBuiltinEnabled(t.Context(), false); err != nil {
		t.Fatalf("SetBuiltinEnabled: %v", err)
	}
	if enabled, err := service.BuiltinEnabled(t.Context()); err != nil || enabled {
		t.Fatalf("BuiltinEnabled = %v, %v, want false", enabled, err)
	}
}

func TestBuiltinStaleByClock(t *testing.T) {
	service, _, _ := newTestService(t)

	now := time.Unix(1_700_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	if _, err := service.RefreshBuiltin(t.Context(), "ID"); err != nil {
		t.Fatalf("RefreshBuiltin: %v", err)
	}

	source, _, _, err := service.BuiltinSource(t.Context(), "ID")
	if err != nil {
		t.Fatalf("BuiltinSource: %v", err)
	}
	if service.BuiltinStale(source) {
		t.Fatal("fresh source reported stale")
	}

	service.now = func() time.Time { return now.Add(builtinRefreshInterval) }
	if !service.BuiltinStale(source) {
		t.Fatal("source not stale after interval")
	}
}

// Upstream iptv-org exclusions are merged with the local blocklist and cached.
func TestBuiltinAppliesUpstreamBlocklist(t *testing.T) {
	service, server, _ := newTestService(t)

	server.mu.Lock()
	server.blocklist = `[{"channel":"blocked-upstream","reason":"dmca"}]`
	server.playlist = testPlaylist + `#EXTINF:-1 tvg-id="blocked-upstream",Upstream DMCA
https://stream.example/dmca.m3u8
`
	server.mu.Unlock()

	source, err := service.RefreshBuiltin(t.Context(), "ID")
	if err != nil {
		t.Fatalf("RefreshBuiltin: %v", err)
	}
	if source.ItemCount != 1 {
		t.Fatalf("item count %d, want only the good channel", source.ItemCount)
	}

	_, channels, _, err := service.BuiltinSource(t.Context(), "ID")
	if err != nil {
		t.Fatalf("BuiltinSource: %v", err)
	}
	for _, channel := range channels {
		if channel.ProviderID == "blocked-upstream" {
			t.Fatalf("upstream-blocked channel survived: %+v", channel)
		}
	}

	server.mu.Lock()
	firstBlocks := server.blocks
	server.mu.Unlock()
	if firstBlocks != 1 {
		t.Fatalf("blocklist requests %d, want 1", firstBlocks)
	}

	// The second refresh reuses the cached blocklist.
	if _, err := service.RefreshBuiltin(t.Context(), "ID"); err != nil {
		t.Fatalf("second RefreshBuiltin: %v", err)
	}
	server.mu.Lock()
	secondBlocks := server.blocks
	server.mu.Unlock()
	if secondBlocks != 1 {
		t.Fatalf("blocklist requests after cache %d, want 1", secondBlocks)
	}
}

// A failed blocklist refresh keeps the cached exclusions and never fails the
// catalog refresh.
func TestBlocklistFetchFailureUsesCache(t *testing.T) {
	service, server, _ := newTestService(t)

	server.mu.Lock()
	server.blocklist = `[{"channel":"blocked-upstream","reason":"dmca"}]`
	server.playlist = testPlaylist + `#EXTINF:-1 tvg-id="blocked-upstream",Upstream DMCA
https://stream.example/dmca.m3u8
`
	server.mu.Unlock()

	if _, err := service.RefreshBuiltin(t.Context(), "ID"); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	service.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	server.mu.Lock()
	server.failBlocks = true
	server.etag = `"v2"`
	server.mu.Unlock()

	source, err := service.RefreshBuiltin(t.Context(), "ID")
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if source.ItemCount != 1 {
		t.Fatalf("item count %d, want cached blocklist applied", source.ItemCount)
	}
}
