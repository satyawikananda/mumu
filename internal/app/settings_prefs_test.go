package app

import (
	"context"
	"testing"
	"time"

	"mumu/internal/player"
	"mumu/internal/ui"
	"mumu/internal/video"
)

// Startup preferences apply before the window opens.
func TestLoadStartupPreferences(t *testing.T) {
	a, _ := discoverTestApp(t)

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}

	ctx := context.Background()
	for key, value := range map[string]string{
		appearanceThemeSetting:    "light",
		playbackStartMutedSetting: "true",
		playbackSettingVolume:     "33",
	} {
		if err := store.Settings().Set(ctx, key, value); err != nil {
			t.Fatalf("Set %s: %v", key, err)
		}
	}

	a.loadStartupPreferences()

	if got := a.settingsData.themeValue(); got != ui.ThemeLight {
		t.Fatalf("theme = %v, want Light", got)
	}
	if got := a.shell.ThemeMode(); got != ui.ThemeLight {
		t.Fatalf("shell theme = %v, want Light", got)
	}
	if !a.settingsData.startMutedValue() {
		t.Fatal("start muted was not loaded")
	}
	if got := mediaSnapshot(a).Volume; got != 33 {
		t.Fatalf("volume = %d, want 33", got)
	}
	if mediaSnapshot(a).Muted {
		t.Fatal("startup restored mute instead of the start-muted preference")
	}
}

// Theme changes apply immediately and persist.
func TestSetThemeModePersists(t *testing.T) {
	a, f := discoverTestApp(t)

	a.setThemeMode(ui.ThemeLight)
	if got := a.shell.ThemeMode(); got != ui.ThemeLight {
		t.Fatalf("theme = %v, want Light", got)
	}

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}
	waitFor(t, f, "theme saved", func() bool {
		value, found, err := store.Settings().Get(context.Background(), appearanceThemeSetting)
		return err == nil && found && value == "light"
	})
}

// A failed theme write restores the previous appearance with an inline reason.
func TestSetThemeModeReverts(t *testing.T) {
	a, f := discoverTestApp(t)
	a.closeStore()

	a.setThemeMode(ui.ThemeLight)
	waitFor(t, f, "theme reverted", func() bool {
		return a.shell.ThemeMode() == ui.ThemeDark &&
			a.shell.Settings().Theme() == ui.ThemeDark &&
			a.shell.Settings().ThemeError() != ""
	})
}

// Start muted is persisted explicitly and does not mute the current session.
func TestSetStartMutedPersists(t *testing.T) {
	a, f := discoverTestApp(t)

	a.setStartMuted(true)

	if mediaSnapshot(a).Muted {
		t.Fatal("changing the preference muted the current session")
	}

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}
	waitFor(t, f, "start muted saved", func() bool {
		value, found, err := store.Settings().Get(context.Background(), playbackStartMutedSetting)
		return err == nil && found && value == "true"
	})
}

// A failed start-muted write restores the previous value.
func TestSetStartMutedReverts(t *testing.T) {
	a, f := discoverTestApp(t)
	a.closeStore()

	a.setStartMuted(true)
	waitFor(t, f, "start muted reverted", func() bool {
		return !a.shell.Settings().StartMuted() && a.shell.Settings().PlaybackError() != ""
	})
}

// Start muted applies to a new channel load, but not to a retry.
func TestStartMutedAppliesOnlyToNewChannel(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }
	a.settingsData.setStartMuted(true)

	a.playChannel(playbackTestChannel(), "My list")
	waitSnapshot(t, a, "new channel start muted", func() bool {
		return a.media.snapshot.Muted
	})
	waitControllerMuted(t, a, true)

	a.togglePlaybackMute()
	waitSnapshot(t, a, "runtime unmute", func() bool {
		return !a.media.snapshot.Muted
	})

	a.retryWatching()
	if mediaSnapshot(a).Muted {
		t.Fatal("retry rewrote the runtime audio intent")
	}
}

// waitControllerMuted waits for the one controller to hold want.
func waitControllerMuted(t *testing.T, a *App, want bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		controller := a.controller
		a.mu.Unlock()
		if controller != nil && controller.Snapshot().Muted == want {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("controller mute never became %t", want)
}

// Runtime mute never writes the start-muted preference.
func TestRuntimeMuteDoesNotRewritePreference(t *testing.T) {
	a, _ := discoverTestApp(t)
	a.settingsData.setStartMuted(false)

	a.togglePlaybackMute()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}
	if value, found, err := store.Settings().Get(context.Background(), playbackStartMutedSetting); err != nil {
		t.Fatalf("Get start muted: %v", err)
	} else if found {
		t.Fatalf("runtime mute wrote %q", value)
	}
}

// PiP's temporary mute never writes the start-muted preference.
func TestPiPMuteDoesNotRewritePreference(t *testing.T) {
	a, _ := discoverTestApp(t)
	engine := newFakeEngine(t)
	controller := player.New(engine)
	a.controller = controller
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = controller.Close(ctx)
	})
	a.presentation = video.PresentationPiP
	a.pipMainMuted = false
	a.pipMuted = false
	a.settingsData.setStartMuted(false)

	a.togglePiPMute()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v", store, err)
	}
	if value, found, err := store.Settings().Get(context.Background(), playbackStartMutedSetting); err != nil {
		t.Fatalf("Get start muted: %v", err)
	} else if found {
		t.Fatalf("PiP mute wrote %q", value)
	}
}

// themeFromSetting accepts only the three stored values.
func TestThemeFromSetting(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  ui.ThemeMode
	}{
		{value: "light", want: ui.ThemeLight},
		{value: "system", want: ui.ThemeSystem},
		{value: "dark", want: ui.ThemeDark},
		{value: "", want: ui.ThemeDark},
		{value: "sepia", want: ui.ThemeDark},
	} {
		if got := themeFromSetting(tc.value); got != tc.want {
			t.Fatalf("themeFromSetting(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
