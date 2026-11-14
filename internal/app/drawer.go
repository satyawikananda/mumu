package app

import (
	"context"
	"log/slog"
	"strings"

	"mumu/internal/catalog"
	"mumu/internal/sources"
	"mumu/internal/ui"
)

// drawerEntry is one Watch drawer row's identity and source attribution.
type drawerEntry struct {
	channel    catalog.Channel
	sourceName string
}

// drawerRow maps one catalog channel to a Watch drawer row and the identity
// that selecting it resolves. The key is the channel's playback identity, so a
// row reaches the same channel whether it came from the Discover index or from
// storage.
func drawerRow(channel catalog.Channel, position int, sourceName string) (ui.WatchChannel, drawerEntry) {
	sourceName = strings.TrimSpace(sourceName)
	meta := strings.TrimSpace(channel.Group)
	if sourceName != "" {
		if meta != "" {
			meta += " · "
		}
		meta += sourceName
	}

	return ui.WatchChannel{
		Key:     discoverKey(channel, position),
		Name:    channel.Name,
		Meta:    meta,
		LogoURL: channel.LogoURL,
		Live:    catalog.HasLiveStream(channel),
	}, drawerEntry{channel: channel, sourceName: sourceName}
}

// drawerRows maps one source's channels to drawer rows and their lookup. It
// keeps the caller's order, which is the order the rows are shown in.
func drawerRows(
	channels []catalog.Channel,
	sourceName string,
	limit int,
) ([]ui.WatchChannel, map[string]drawerEntry) {
	count := min(len(channels), limit)
	rows := make([]ui.WatchChannel, 0, count)
	entries := make(map[string]drawerEntry, count)
	for position := 0; position < count; position++ {
		row, entry := drawerRow(channels[position], position, sourceName)
		rows = append(rows, row)
		entries[row.Key] = entry
	}

	return rows, entries
}

// prepareChannelDrawer shows the drawer rows that belong to the active
// channel's source.
//
// The Discover index answers immediately when it already holds that source, so
// the common path never waits on storage. Otherwise the source is read in the
// background: the UI thread publishes an empty drawer until those rows arrive,
// never the previous source's channels. A newer preparation invalidates an
// older one, so a slow load cannot replace the current drawer.
func (a *App) prepareChannelDrawer(channel catalog.Channel, sourceName string) {
	sourceID := channel.SourceID

	a.mu.Lock()
	a.media.drawerGen++
	gen := a.media.drawerGen
	if sourceID == "" {
		a.media.drawer = nil
		a.media.drawerByKey = nil
		a.media.drawerSource = ""
		a.mu.Unlock()

		return
	}
	if a.media.drawerSource == sourceID && len(a.media.drawer) > 0 {
		// Switching inside the active source keeps the rows already shown.
		a.mu.Unlock()

		return
	}
	a.media.drawerSource = sourceID
	a.media.drawer = nil
	a.media.drawerByKey = nil
	a.mu.Unlock()

	if rows, entries, ok := a.discoverIndex.drawerForSource(sourceID, discoverResultLimit); ok {
		a.applyChannelDrawer(gen, sourceID, rows, entries)

		return
	}

	a.loadChannelDrawer(gen, sourceID, sourceName)
}

// applyChannelDrawer installs prepared rows only while their source is still
// the active drawer's source and no newer preparation ran.
func (a *App) applyChannelDrawer(
	gen uint64,
	sourceID string,
	rows []ui.WatchChannel,
	entries map[string]drawerEntry,
) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.media.drawerGen != gen || a.media.drawerSource != sourceID {
		return false
	}

	a.media.drawer = rows
	a.media.drawerByKey = entries

	return true
}

// loadChannelDrawer reads one source's stored channels off the UI thread.
func (a *App) loadChannelDrawer(gen uint64, sourceID, sourceName string) {
	a.GoQuiet(func(ctx context.Context) {
		service, err := a.Catalog()
		if err != nil || service == nil {
			return
		}

		readCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
		defer cancel()

		channels, err := service.Channels(readCtx, sourceID)
		if err != nil || len(channels) == 0 {
			if err != nil {
				slog.Debug("channel drawer unavailable", "source", sourceID, "error", err)
			}

			return
		}
		if sourceName == "" {
			sourceName = a.drawerSourceName(readCtx, service, sourceID)
		}

		rows, entries := drawerRows(channels, sourceName, discoverResultLimit)
		if !a.applyChannelDrawer(gen, sourceID, rows, entries) {
			return
		}

		a.publishWatch()
	})
}

// drawerSourceName resolves one stored source's display name for drawer
// attribution, preferring the name the Discover index already holds.
func (a *App) drawerSourceName(ctx context.Context, service *sources.Service, sourceID string) string {
	if name := a.discoverIndex.sourceName(sourceID); name != "" {
		return name
	}
	if service == nil {
		return ""
	}

	list, err := service.List(ctx)
	if err != nil {
		return ""
	}
	for _, source := range list {
		if source.ID == sourceID {
			return strings.TrimSpace(source.DisplayName)
		}
	}

	return ""
}
