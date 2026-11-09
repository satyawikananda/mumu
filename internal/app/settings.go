package app

import (
	"context"
	"sync"
	"time"
)

// settingsTimeout bounds one catalog-setting read or write.
const settingsTimeout = 10 * time.Second

// settingsCache holds the community catalog setting shown by Settings.
type settingsCache struct {
	mu        sync.Mutex
	community bool
}

func newSettingsCache() *settingsCache {
	// The service treats a missing setting as enabled.
	return &settingsCache{community: true}
}

// community returns the last value the store confirmed.
func (c *settingsCache) communityValue() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.community
}

// setCommunity records a confirmed value.
func (c *settingsCache) setCommunity(community bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.community = community
}

// startSettings wires the Settings page and publishes the saved setting.
func (a *App) startSettings() {
	a.shell.Settings().OnCommunity = a.setCommunityCatalog

	a.Go(func(ctx context.Context) error {
		a.loadSettings(ctx)

		return nil
	})
}

// loadSettings reads the community catalog setting and publishes it.
func (a *App) loadSettings(ctx context.Context) {
	service, err := a.Catalog()
	if err != nil {
		a.publishSettingsError("mumu could not open its local library.")

		return
	}

	readCtx, cancel := context.WithTimeout(ctx, settingsTimeout)
	community, err := service.BuiltinEnabled(readCtx)
	cancel()

	if err != nil {
		a.publishSettingsError("mumu could not read the catalog setting. Try again.")

		return
	}

	// A toggle that started meanwhile owns the shown value.
	if a.settings.generation() != 0 {
		return
	}

	a.settingsData.setCommunity(community)
	a.publish(func() {
		a.shell.Settings().SetCommunity(community)
		a.shell.Settings().SetCommunityError("")
	})
}

// setCommunityCatalog shows or hides the community catalog. It runs on the
// UI thread; the write runs in the background.
func (a *App) setCommunityCatalog(enabled bool) {
	gen, ctx := a.settings.begin(a.ctx)

	a.GoQuiet(func(context.Context) {
		service, err := a.Catalog()
		if err != nil {
			a.revertCommunity(gen, "mumu could not open its local library.")

			return
		}

		writeCtx, cancel := context.WithTimeout(ctx, settingsTimeout)
		err = service.SetBuiltinEnabled(writeCtx, enabled)
		cancel()

		if !a.settings.current(gen) {
			return
		}
		if err != nil {
			a.revertCommunity(gen, "mumu could not save the setting. Try again.")

			return
		}

		a.settingsData.setCommunity(enabled)
		a.publish(func() {
			if !a.settings.current(gen) {
				return
			}
			a.shell.Settings().SetCommunity(enabled)
			a.shell.Settings().SetCommunityError("")
		})
		a.refreshDiscoverFromCache()
	})
}

// revertCommunity restores the last confirmed value with an inline reason.
func (a *App) revertCommunity(gen uint64, message string) {
	previous := a.settingsData.communityValue()

	a.publish(func() {
		if !a.settings.current(gen) {
			return
		}
		a.shell.Settings().SetCommunity(previous)
		a.shell.Settings().SetCommunityError(message)
	})
}

// publishSettingsError shows a settings failure without changing the value.
func (a *App) publishSettingsError(message string) {
	a.publish(func() { a.shell.Settings().SetCommunityError(message) })
}
