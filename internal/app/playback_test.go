package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/player"
	"mumu/internal/ui"
)

// A stored recent channel is offered, never started automatically.
func TestLoadWatchResume(t *testing.T) {
	a, _ := discoverTestApp(t)
	seedUserSource(t, a)

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}

	ctx := context.Background()
	sources, err := store.Sources().List(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources %+v, %v", sources, err)
	}

	stored, found, err := store.Channels().FindByProvider(ctx, sources[0].ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
	}
	if err := store.Recent().MarkPlayed(ctx, stored.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	a.loadLibrary(ctx)
	a.loadWatchResume(ctx)

	got := a.shell.Watch().State()
	if got.Kind != ui.WatchResume {
		t.Fatalf("watch kind %v, want Resume", got.Kind)
	}
	if got.Channel != "My News" {
		t.Fatalf("channel %q, want My News", got.Channel)
	}
	if a.shell.Destination() != "/home" {
		t.Fatalf("destination %q, want Home", a.shell.Destination())
	}
	if got := a.shell.Home().State(); got.Kind != ui.HomeResume || got.Hero.Name != "My News" {
		t.Fatalf("home state %+v, want the resume hero", got)
	}

	a.mu.Lock()
	started := a.controller != nil || a.engine != nil
	a.mu.Unlock()
	if started {
		t.Fatal("the resume card started playback")
	}
}

// Without a recent channel Watch stays idle and Discover stays current.
func TestLoadWatchResumeEmpty(t *testing.T) {
	a, _ := discoverTestApp(t)

	a.loadWatchResume(context.Background())

	if got := a.shell.Watch().State(); got.Kind != ui.WatchEmpty {
		t.Fatalf("watch kind %v, want Empty", got.Kind)
	}
	if a.shell.Destination() != "/discover" {
		t.Fatalf("destination %q, want Discover", a.shell.Destination())
	}
}

// A disabled source is not offered for resume.
func TestLoadWatchResumeSkipsDisabledSource(t *testing.T) {
	a, _ := discoverTestApp(t)
	seedUserSource(t, a)

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}

	ctx := context.Background()
	sources, err := store.Sources().List(ctx)
	if err != nil || len(sources) == 0 {
		t.Fatalf("sources %+v, %v", sources, err)
	}
	if err := store.Sources().SetEnabled(ctx, sources[0].ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	stored, found, err := store.Channels().FindByProvider(ctx, sources[0].ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
	}
	if err := store.Recent().MarkPlayed(ctx, stored.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	a.loadLibrary(ctx)
	a.loadWatchResume(ctx)

	if got := a.shell.Watch().State(); got.Kind != ui.WatchEmpty {
		t.Fatalf("watch kind %v, want Empty", got.Kind)
	}
	if got := a.shell.Destination(); got != "/discover" {
		t.Fatalf("destination %q, want Discover for an unusable recent entry", got)
	}
}

// Stream selection prefers a live or HLS variant and rejects other schemes.
func TestPickStream(t *testing.T) {
	channel := catalog.Channel{Streams: []catalog.Stream{
		{URL: "rtsp://example.com/live", Live: true},
		{URL: "https://example.com/plain.flv"},
		{URL: "https://example.com/live.m3u8", HLS: true, Live: true},
	}}

	stream, ok := pickStream(channel)
	if !ok || stream.URL != "https://example.com/live.m3u8" {
		t.Fatalf("stream %+v ok=%t, want the HLS variant", stream, ok)
	}

	if _, ok := pickStream(catalog.Channel{Streams: []catalog.Stream{{URL: "udp://1.2.3.4:1234"}}}); ok {
		t.Fatal("picked an unsupported scheme")
	}
}

// The scheme helper accepts only http and https with a host.
func TestStreamScheme(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want string
		ok   bool
	}{
		{url: "https://example.com/a.m3u8", want: "https", ok: true},
		{url: "HTTP://example.com/a.m3u8", want: "http", ok: true},
		{url: "rtsp://example.com/a", ok: false},
		{url: "file:///tmp/a.m3u8", ok: false},
		{url: "https:///no-host", ok: false},
	} {
		got, err := streamScheme(tc.url)
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("streamScheme(%q) = %q, %v; want %q ok=%t", tc.url, got, err, tc.want, tc.ok)
		}
	}
}

// Only sanitized playback headers are forwarded.
func TestStreamHeaders(t *testing.T) {
	headers := streamHeaders(catalog.Headers{
		"User-Agent": "mumu/0.1",
		"Referer":    "https://example.com/",
		"Cookie":     "session=secret",
	})

	if headers["User-Agent"] != "mumu/0.1" || headers["Referer"] != "https://example.com/" {
		t.Fatalf("headers %+v, want UA and Referer", headers)
	}
	if _, ok := headers["Cookie"]; ok {
		t.Fatalf("headers %+v leaked a credential", headers)
	}
	if streamHeaders(nil) != nil {
		t.Fatal("nil headers should stay nil")
	}
}

// A failed playback keeps the channel and reports an actionable error. The
// engine factory fails, so the result does not depend on a native runtime.
func TestPlayChannelWithoutEngineReportsError(t *testing.T) {
	a, _ := discoverTestApp(t)
	a.newEngine = func(string, int) (player.Engine, error) {
		return nil, fmt.Errorf("%w: test runtime", player.ErrRuntimeUnavailable)
	}

	channel := catalog.Channel{
		SourceID:   "source-a",
		ProviderID: "provider-a",
		Name:       "News",
		Streams:    []catalog.Stream{{URL: "https://example.com/live.m3u8", Live: true}},
	}

	a.playChannel(channel, "My list")

	got := a.shell.Watch().State()
	if got.Kind != ui.WatchError {
		t.Fatalf("watch kind %v, want Error", got.Kind)
	}
	if got.Channel != "News" {
		t.Fatalf("channel %q, want News", got.Channel)
	}
	if got.Error == "" {
		t.Fatal("error state has no message")
	}
	if a.shell.Destination() != "/watch" {
		t.Fatalf("destination %q, want Watch", a.shell.Destination())
	}
}

// A launch without playback must not open the engine, log or report an error.
func TestStartWatchDoesNotOpenPlayback(t *testing.T) {
	a, _ := discoverTestApp(t)

	attempts := 0
	a.newEngine = func(string, int) (player.Engine, error) {
		attempts++

		return nil, fmt.Errorf("%w: test runtime", player.ErrRuntimeUnavailable)
	}

	a.startWatch()

	if attempts != 0 {
		t.Fatalf("engine created %d times before a Play", attempts)
	}
	if a.controller != nil {
		t.Fatal("startWatch created a controller")
	}
	got := a.shell.Watch().State()
	if got.Kind == ui.WatchError || got.Error != "" {
		t.Fatalf("watch state %+v, want no playback error before a Play", got)
	}
}

// The engine, controller and stream load are created on the first Play.
func TestPlayOpensPlayerOnFirstPlay(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }

	if a.controller != nil {
		t.Fatal("controller exists before a Play")
	}

	a.playChannel(playbackTestChannel(), "My list")

	a.mu.Lock()
	controller := a.controller
	a.mu.Unlock()
	if controller == nil {
		t.Fatalf("no controller after a Play; watch state %+v", a.shell.Watch().State())
	}

	waitForLoads(t, engine, 1)
	if source := engine.loadSource(); source.URL != "https://example.com/live.m3u8" {
		t.Fatalf("loaded %q, want the channel stream", source.URL)
	}
}

// A missing runtime is reported only when the viewer asks to play, and a
// later attempt works once the runtime is available.
func TestPlayRetriesAfterRuntimeAppears(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)

	available := false
	a.newEngine = func(string, int) (player.Engine, error) {
		if !available {
			return nil, fmt.Errorf("%w: test runtime", player.ErrRuntimeUnavailable)
		}

		return engine, nil
	}

	a.playChannel(playbackTestChannel(), "My list")

	got := a.shell.Watch().State()
	if got.Kind != ui.WatchError {
		t.Fatalf("watch kind %v, want Error", got.Kind)
	}
	if !strings.Contains(got.Error, "libmpv") {
		t.Fatalf("error %q, want an actionable libmpv message", got.Error)
	}
	if strings.Contains(got.Error, "example.com") || strings.Contains(got.Error, "http") {
		t.Fatalf("error %q leaks the stream URL", got.Error)
	}
	if a.controller != nil {
		t.Fatal("a failed engine creation left a controller")
	}

	available = true
	a.playChannel(playbackTestChannel(), "My list")

	a.mu.Lock()
	controller := a.controller
	a.mu.Unlock()
	if controller == nil {
		t.Fatalf("no controller after the runtime appeared; watch state %+v", a.shell.Watch().State())
	}

	waitForLoads(t, engine, 1)
}

// playbackTestChannel is a live channel with one supported stream.
func playbackTestChannel() catalog.Channel {
	return catalog.Channel{
		SourceID:   "source-a",
		ProviderID: "provider-a",
		Name:       "News",
		Streams:    []catalog.Stream{{URL: "https://example.com/live.m3u8", Live: true}},
	}
}

// waitForLoads waits until the fake engine saw want Load calls.
func waitForLoads(t *testing.T, engine *fakeEngine, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engine.loadCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("engine loads = %d, want %d", engine.loadCount(), want)
}

// markPlayingRecent records only real playback, once per generation.
func TestMarkPlayingRecentNeedsPlaying(t *testing.T) {
	a, _ := discoverTestApp(t)

	a.media.channel = catalog.Channel{SourceID: "source-a", ProviderID: "provider-a", Name: "News"}
	a.markPlayingRecent(player.Snapshot{State: player.StateLoading, Generation: 1})
	if a.media.markedGen != 0 {
		t.Fatal("marked recent before playback")
	}
}
