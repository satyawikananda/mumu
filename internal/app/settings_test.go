package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"mumu/internal/catalog"
	"mumu/internal/sources"
)

// Hiding the community catalog persists the setting and republishes Discover
// from cache only: user channels stay, the built-in notice disappears.
func TestSettingsCommunityCatalogToggle(t *testing.T) {
	a, f := newLibraryTestApp(t)
	seedBuiltinCatalog(t, a)
	seedUserSource(t, a)

	var network atomic.Int32
	a.refreshBuiltin = func(context.Context, *sources.Service, string) (catalog.Source, error) {
		network.Add(1)

		return catalog.Source{}, errors.New("network disabled in tests")
	}

	// A discover generation exists; no discover job runs in this test.
	a.discover.begin(a.ctx)
	a.loadSettings(a.ctx)

	if !a.shell.Settings().Community() {
		t.Fatal("settings loaded as hidden, want visible")
	}

	a.setCommunityCatalog(false)
	waitFor(t, f, "community disabled", func() bool {
		state := a.shell.Discover().State()

		return !a.shell.Settings().Community() &&
			len(state.Channels) == 1 &&
			state.Channels[0].Name == "My News" &&
			a.shell.Discover().Notice() == ""
	})

	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if enabled, err := service.BuiltinEnabled(context.Background()); err != nil || enabled {
		t.Fatalf("BuiltinEnabled = %v, %v, want false", enabled, err)
	}

	a.setCommunityCatalog(true)
	waitFor(t, f, "community enabled", func() bool {
		state := a.shell.Discover().State()

		return a.shell.Settings().Community() &&
			len(state.Channels) == 2 &&
			a.shell.Discover().Notice() != ""
	})

	if enabled, err := service.BuiltinEnabled(context.Background()); err != nil || !enabled {
		t.Fatalf("BuiltinEnabled = %v, %v, want true", enabled, err)
	}
	if got := network.Load(); got != 0 {
		t.Fatalf("network refresh calls %d, want 0", got)
	}
}

// A write failure reverts the switch and shows an inline error.
func TestSettingsCommunityRevert(t *testing.T) {
	a, f := newLibraryTestApp(t)
	a.loadSettings(a.ctx)
	a.closeStore()

	a.setCommunityCatalog(false)
	waitFor(t, f, "reverted setting", func() bool {
		return a.shell.Settings().Community() && a.shell.Settings().CommunityError() != ""
	})
}
