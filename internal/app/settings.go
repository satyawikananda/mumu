package app

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"mumu/internal/player"
	"mumu/internal/ui"
)

// settingsTimeout bounds one catalog-setting read or write.
const settingsTimeout = 10 * time.Second

// startupSettingsTimeout bounds the synchronous read before the window opens.
const startupSettingsTimeout = 3 * time.Second

// appearanceThemeSetting stores the selected Dark, Light or System mode.
const appearanceThemeSetting = "appearance.theme"

// settingsCache holds the preferences shown by Settings.
type settingsCache struct {
	mu         sync.Mutex
	theme      ui.ThemeMode
	startMuted bool
}

func newSettingsCache() *settingsCache {
	return &settingsCache{theme: ui.ThemeDark}
}

// themeValue returns the current appearance.
func (c *settingsCache) themeValue() ui.ThemeMode {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.theme
}

// setTheme records an appearance.
func (c *settingsCache) setTheme(mode ui.ThemeMode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.theme = mode
}

// startMutedValue reports whether new channels begin muted.
func (c *settingsCache) startMutedValue() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.startMuted
}

// setStartMuted records the start-muted preference.
func (c *settingsCache) setStartMuted(muted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.startMuted = muted
}

// startSettings wires the Settings page and publishes the saved preferences.
func (a *App) startSettings() {
	page := a.shell.Settings()
	page.OnTheme = a.setThemeMode
	page.OnStartMuted = a.setStartMuted
	page.OnCheckUpdates = a.checkForUpdates

	a.publishSettingsValues()
	a.startAbout()
}

// loadStartupPreferences applies theme, volume and playback preferences before
// the window opens, so the first frame uses the stored appearance and the
// first Play cannot race the read.
func (a *App) loadStartupPreferences() {
	store, err := a.Store()
	if err != nil || store == nil {
		return
	}

	ctx, cancel := context.WithTimeout(a.ctx, startupSettingsTimeout)
	defer cancel()

	settings, err := store.Settings().All(ctx)
	if err != nil {
		slog.Debug("read startup preferences", "error", err)

		return
	}

	a.applyPlaybackPreferences(settings)

	theme := themeFromSetting(settings[appearanceThemeSetting])
	a.settingsData.setTheme(theme)
	a.shell.SetThemeMode(theme)

	if _, stale := settings[legacyPlaybackSettingMuted]; stale {
		if err := store.Settings().Delete(ctx, legacyPlaybackSettingMuted); err != nil {
			slog.Debug("clear legacy mute setting", "error", err)
		}
	}
}

// applyPlaybackPreferences primes the state the Watch page and a later
// controller read. Mute is deliberately not restored: a launch starts
// audible; the start-muted preference applies to the next channel load.
func (a *App) applyPlaybackPreferences(settings map[string]string) {
	volume := player.DefaultVolume
	if parsed, err := strconv.Atoi(settings[playbackSettingVolume]); err == nil &&
		parsed >= 0 && parsed <= 100 {
		volume = parsed
	}

	startMuted, _ := strconv.ParseBool(settings[playbackStartMutedSetting])

	a.mu.Lock()
	a.media.snapshot.Volume = volume
	a.media.snapshot.Muted = false
	a.mu.Unlock()

	a.settingsData.setStartMuted(startMuted)
}

// publishSettingsValues shows the loaded playback preferences on the page.
func (a *App) publishSettingsValues() {
	startMuted := a.settingsData.startMutedValue()
	a.publish(func() {
		a.shell.Settings().SetStartMuted(startMuted)
	})
}

// setThemeMode applies a new appearance immediately, then persists it. A
// failed write restores the previous appearance with an inline reason.
func (a *App) setThemeMode(mode ui.ThemeMode) {
	mode = validThemeMode(mode)
	previous := a.settingsData.themeValue()
	a.shell.SetThemeMode(mode)
	a.settingsData.setTheme(mode)
	if mode == previous {
		return
	}

	gen, ctx := a.appearance.begin(a.ctx)
	a.GoQuiet(func(context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			a.revertTheme(gen, previous, ui.ProductName+" could not open its local library.")

			return
		}

		writeCtx, cancel := context.WithTimeout(ctx, settingsTimeout)
		err = store.Settings().Set(writeCtx, appearanceThemeSetting, themeSettingValue(mode))
		cancel()

		if !a.appearance.current(gen) {
			return
		}
		if err != nil {
			a.revertTheme(gen, previous, ui.ProductName+" could not save the appearance. Try again.")

			return
		}
		a.publish(func() {
			if a.appearance.current(gen) {
				a.shell.Settings().SetThemeError("")
			}
		})
	})
}

// revertTheme restores the last confirmed appearance with an inline reason.
func (a *App) revertTheme(gen uint64, previous ui.ThemeMode, message string) {
	a.settingsData.setTheme(previous)
	a.publish(func() {
		if !a.appearance.current(gen) {
			return
		}
		a.shell.SetThemeMode(previous)
		a.shell.Settings().SetThemeError(message)
	})
}

// setStartMuted persists the preference for new channel loads. It never
// changes the current playback and never writes runtime or PiP mute.
func (a *App) setStartMuted(muted bool) {
	previous := a.settingsData.startMutedValue()
	a.settingsData.setStartMuted(muted)
	if muted == previous {
		return
	}

	gen, ctx := a.startMuted.begin(a.ctx)
	a.GoQuiet(func(context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			a.revertStartMuted(gen, previous, ui.ProductName+" could not open its local library.")

			return
		}

		writeCtx, cancel := context.WithTimeout(ctx, settingsTimeout)
		err = store.Settings().Set(writeCtx, playbackStartMutedSetting,
			strconv.FormatBool(muted))
		cancel()

		if !a.startMuted.current(gen) {
			return
		}
		if err != nil {
			a.revertStartMuted(gen, previous, ui.ProductName+" could not save the playback setting. Try again.")

			return
		}
		a.publish(func() {
			if a.startMuted.current(gen) {
				a.shell.Settings().SetPlaybackError("")
			}
		})
	})
}

// revertStartMuted restores the last confirmed preference.
func (a *App) revertStartMuted(gen uint64, previous bool, message string) {
	a.settingsData.setStartMuted(previous)
	a.publish(func() {
		if !a.startMuted.current(gen) {
			return
		}
		a.shell.Settings().SetStartMuted(previous)
		a.shell.Settings().SetPlaybackError(message)
	})
}

// themeFromSetting parses a stored appearance, falling back to Dark.
func themeFromSetting(value string) ui.ThemeMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "light":
		return ui.ThemeLight
	case "system":
		return ui.ThemeSystem
	default:
		return ui.ThemeDark
	}
}

// themeSettingValue encodes an appearance for storage.
func themeSettingValue(mode ui.ThemeMode) string {
	switch mode {
	case ui.ThemeLight:
		return "light"
	case ui.ThemeSystem:
		return "system"
	default:
		return "dark"
	}
}

// validThemeMode keeps an unknown mode on the dark default.
func validThemeMode(mode ui.ThemeMode) ui.ThemeMode {
	if mode < ui.ThemeDark || mode > ui.ThemeSystem {
		return ui.ThemeDark
	}

	return mode
}
