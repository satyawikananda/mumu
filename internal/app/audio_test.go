package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/player"
)

// mediaSnapshot reads the shared playback state under its lock.
func mediaSnapshot(a *App) player.Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.media.snapshot
}

// A stored mute preference is ignored: the approved policy is that every launch
// starts audible. Its legacy key is deleted once so it is not inherited.
func TestLoadPlaybackSettingsNeverRestoresMute(t *testing.T) {
	a, f := discoverTestApp(t)

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}

	ctx := context.Background()
	if err := store.Settings().Set(ctx, playbackSettingVolume, "55"); err != nil {
		t.Fatalf("Set volume: %v", err)
	}
	if err := store.Settings().Set(ctx, legacyPlaybackSettingMuted, "true"); err != nil {
		t.Fatalf("Set legacy mute: %v", err)
	}

	a.loadPlaybackSettings()
	f.waitUpdates(t, 1)

	if got := mediaSnapshot(a).Volume; got != 55 {
		t.Fatalf("volume = %d, want the saved 55", got)
	}
	if mediaSnapshot(a).Muted {
		t.Fatal("a stored mute was restored at launch")
	}

	if value, found, err := store.Settings().Get(ctx, legacyPlaybackSettingMuted); err != nil {
		t.Fatalf("Get legacy mute: %v", err)
	} else if found {
		t.Fatalf("legacy mute key = %q, want it deleted", value)
	}
}

// A missing or unusable volume key falls back to the default instead of the
// zero value, which would launch silently.
func TestLoadPlaybackSettingsVolumeDefault(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		set   bool
	}{
		{name: "missing"},
		{name: "not a number", value: "loud", set: true},
		{name: "out of range", value: "140", set: true},
		{name: "saved", value: "12", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, f := discoverTestApp(t)

			store, err := a.Store()
			if err != nil || store == nil {
				t.Fatalf("Store = %v, %v", store, err)
			}
			if tc.set {
				if err := store.Settings().Set(context.Background(), playbackSettingVolume, tc.value); err != nil {
					t.Fatalf("Set volume: %v", err)
				}
			}

			// A zero value from an unset field must not become the launch
			// volume.
			a.mu.Lock()
			a.media.snapshot.Volume = 0
			a.mu.Unlock()

			a.loadPlaybackSettings()
			f.waitUpdates(t, 1)

			want := player.DefaultVolume
			if tc.name == "saved" {
				want = 12
			}
			if got := mediaSnapshot(a).Volume; got != want {
				t.Fatalf("volume = %d, want %d", got, want)
			}
		})
	}
}

// The engine receives the effective launch volume, and mute is always off.
func TestOpenPlayerUsesUnmutedEffectiveAudio(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)

	var gotVolume int
	a.newEngine = func(_ string, volume int) (player.Engine, error) {
		gotVolume = volume

		return engine, nil
	}

	a.mu.Lock()
	a.media.snapshot.Volume = 42
	a.media.snapshot.Muted = true
	a.mu.Unlock()

	controller, err := a.playerController()
	if err != nil {
		t.Fatalf("playerController: %v", err)
	}

	if gotVolume != 42 {
		t.Fatalf("engine volume = %d, want 42", gotVolume)
	}
	snapshot := controller.Snapshot()
	if snapshot.Volume != 42 {
		t.Fatalf("controller volume = %d, want 42", snapshot.Volume)
	}
	if snapshot.Muted {
		t.Fatal("controller started muted")
	}
}

// effectiveVolume keeps valid values and never lets an unusable one through.
func TestEffectiveVolume(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   int
		want int
	}{
		{name: "zero is valid", in: 0, want: 0},
		{name: "in range", in: 70, want: 70},
		{name: "negative", in: -5, want: player.DefaultVolume},
		{name: "too loud", in: 101, want: player.DefaultVolume},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveVolume(tc.in); got != tc.want {
				t.Fatalf("effectiveVolume(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// Muting changes the running session only; nothing is written for the next
// launch.
func TestToggleMuteIsNotPersisted(t *testing.T) {
	a, f := discoverTestApp(t)

	a.togglePlaybackMute()
	f.waitUpdates(t, 1)

	if !mediaSnapshot(a).Muted {
		t.Fatal("the toggle did not mute the session")
	}

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}
	if _, found, err := store.Settings().Get(context.Background(), legacyPlaybackSettingMuted); err != nil {
		t.Fatalf("Get mute setting: %v", err)
	} else if found {
		t.Fatal("the mute toggle persisted a launch preference")
	}
}

// previousChannelFor builds a ready-to-play remembered channel.
func previousChannelFor(providerID, name, streamURL string) *previousChannel {
	return &previousChannel{
		channel: catalog.Channel{
			SourceID:   "source-a",
			ProviderID: providerID,
			Name:       name,
			Streams:    []catalog.Stream{{URL: streamURL, Live: true}},
		},
		stream:     catalog.Stream{URL: streamURL, Live: true},
		sourceName: "My list",
	}
}

// Only a channel that was really presenting frames becomes the one a failed
// switch can return to.
func TestPreviousTracksOnlyPlayingChannel(t *testing.T) {
	next := catalog.Channel{
		SourceID:   "source-a",
		ProviderID: "provider-next",
		Name:       "Next",
		Streams:    []catalog.Stream{{URL: "https://example.com/next.m3u8", Live: true}},
	}

	for _, tc := range []struct {
		name     string
		old      catalog.Channel
		state    player.State
		wantKeep bool
	}{
		{
			name:     "playing",
			old:      catalog.Channel{SourceID: "source-a", ProviderID: "provider-old", Name: "Old"},
			state:    player.StatePlaying,
			wantKeep: true,
		},
		{
			name:     "buffering",
			old:      catalog.Channel{SourceID: "source-a", ProviderID: "provider-old", Name: "Old"},
			state:    player.StateBuffering,
			wantKeep: true,
		},
		{
			name:  "loading",
			old:   catalog.Channel{SourceID: "source-a", ProviderID: "provider-old", Name: "Old"},
			state: player.StateLoading,
		},
		{
			name:  "error",
			old:   catalog.Channel{SourceID: "source-a", ProviderID: "provider-old", Name: "Old"},
			state: player.StateError,
		},
		{
			name:  "never played",
			old:   catalog.Channel{},
			state: player.StateIdle,
		},
		{
			name:  "same channel",
			old:   catalog.Channel{SourceID: "source-a", ProviderID: "provider-next", Name: "Next"},
			state: player.StatePlaying,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := discoverTestApp(t)
			a.newEngine = func(string, int) (player.Engine, error) {
				return nil, fmt.Errorf("%w: test runtime", player.ErrRuntimeUnavailable)
			}

			a.mu.Lock()
			a.media.channel = tc.old
			a.media.stream = catalog.Stream{URL: "https://example.com/old.m3u8", Live: true}
			a.media.sourceName = "My list"
			a.media.snapshot = player.Snapshot{State: tc.state}
			a.mu.Unlock()

			a.playStream(next, next.Streams[0], "My list")

			a.mu.Lock()
			previous := a.media.previous
			a.mu.Unlock()

			if (previous != nil) != tc.wantKeep {
				t.Fatalf("previous = %+v, want kept = %t", previous, tc.wantKeep)
			}
			if previous != nil && previous.channel.ProviderID != "provider-old" {
				t.Fatalf("previous channel %q, want provider-old", previous.channel.ProviderID)
			}
		})
	}
}

// Back to Previous restores the channel, its stream and its source, then plays
// it through the same controller.
func TestBackToPreviousReplaysChannel(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }

	previous := previousChannelFor("provider-old", "Old", "https://example.com/old.m3u8")
	a.mu.Lock()
	a.media.previous = previous
	a.media.channel = catalog.Channel{SourceID: "source-a", ProviderID: "provider-bad", Name: "Bad"}
	a.media.snapshot = player.Snapshot{State: player.StateError}
	a.mu.Unlock()

	a.backToPrevious()

	waitForLoads(t, engine, 1)

	a.mu.Lock()
	channel, sourceName, kept := a.media.channel, a.media.sourceName, a.media.previous
	a.mu.Unlock()

	if channel.ProviderID != "provider-old" {
		t.Fatalf("channel %q, want provider-old", channel.ProviderID)
	}
	if sourceName != "My list" {
		t.Fatalf("source %q, want My list", sourceName)
	}
	if kept != nil {
		t.Fatal("the previous channel was kept after returning to it")
	}
	if source := engine.loadSource(); source.URL != "https://example.com/old.m3u8" {
		t.Fatalf("loaded %q, want the previous stream", source.URL)
	}
}

// The Watch page shows a way back only after a failure with a different channel
// that really played.
func TestWatchStateOffersPreviousOnlyAfterFailure(t *testing.T) {
	a, _ := newTestApp()
	a.media.channel = catalog.Channel{SourceID: "source-a", ProviderID: "provider-bad", Name: "Bad"}
	a.media.previous = previousChannelFor("provider-old", "Old", "https://example.com/old.m3u8")

	a.media.snapshot = player.Snapshot{State: player.StatePlaying}
	if got := a.watchState().Previous; got != "" {
		t.Fatalf("Previous = %q while playing, want empty", got)
	}

	a.media.snapshot = player.Snapshot{State: player.StateError, Error: "Stream failed"}
	if got := a.watchState().Previous; got != "Old" {
		t.Fatalf("Previous = %q, want Old", got)
	}

	a.media.previous = previousChannelFor("provider-bad", "Bad", "https://example.com/bad.m3u8")
	if got := a.watchState().Previous; got != "" {
		t.Fatalf("Previous = %q for the failing channel, want empty", got)
	}
}

// startWatch wires the secondary error action to the return path.
func TestWatchBackReplaysPreviousChannel(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }

	a.startWatch()

	page := a.shell.Watch()
	if page.OnBack == nil {
		t.Fatal("startWatch did not wire OnBack")
	}

	a.mu.Lock()
	a.media.channel = catalog.Channel{SourceID: "source-a", ProviderID: "provider-bad", Name: "Bad"}
	a.media.previous = previousChannelFor("provider-old", "Old", "https://example.com/old.m3u8")
	a.media.snapshot = player.Snapshot{State: player.StateError, Error: "Stream failed"}
	a.mu.Unlock()

	page.OnBack()

	waitForLoads(t, engine, 1)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if mediaSnapshot(a).Source.URL == "https://example.com/old.m3u8" {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("controller source %q, want the previous stream", mediaSnapshot(a).Source.URL)
}

// The one-time cleanup must not touch an unrelated settings row.
func TestLegacyMuteCleanupKeepsOtherSettings(t *testing.T) {
	a, f := discoverTestApp(t)

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}
	ctx := context.Background()
	if err := store.Settings().Set(ctx, "catalog.community", "false"); err != nil {
		t.Fatalf("Set community: %v", err)
	}

	a.loadPlaybackSettings()
	f.waitUpdates(t, 1)

	settings, err := store.Settings().All(ctx)
	if err != nil {
		t.Fatalf("Settings.All: %v", err)
	}
	if settings["catalog.community"] != "false" {
		t.Fatalf("settings %+v, want the community row kept", settings)
	}
	if _, ok := settings[legacyPlaybackSettingMuted]; ok {
		t.Fatal("legacy mute key was not removed")
	}
}
