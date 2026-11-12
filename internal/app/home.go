package app

import (
	"strings"

	"mumu/internal/library"
	"mumu/internal/player"
	"mumu/internal/ui"
)

const (
	// homeRailLimit caps the cards one rail can show at the widest layout.
	homeRailLimit = 6
	// homeArtworkLimit bounds logo warm-up for Home and the Library rows.
	homeArtworkLimit = 60
)

// homeState builds the Home feed from the shared library cache and the
// playback state. It runs on the UI thread and performs no storage or network
// work.
func (a *App) homeState() ui.HomeState {
	a.mu.Lock()
	media := a.media
	countries := a.countryNames
	a.mu.Unlock()

	recent, favorites, recentCount, favoriteCount, loaded, loadErr := a.libraryData.homeSnapshot()

	state := ui.HomeState{
		Kind:          ui.HomeEmpty,
		RecentCount:   recentCount,
		FavoriteCount: favoriteCount,
	}
	if !loaded {
		state.Kind = ui.HomeLoading
		if loadErr != "" {
			state.Kind = ui.HomeFailed
			state.Message = loadErr
		}

		return state
	}

	if recentCount > 0 || favoriteCount > 0 {
		state.Kind = ui.HomeResume
	}
	for _, entry := range railEntries(favorites) {
		state.Favorites = append(state.Favorites, homeChannel(entry, true, countries))
	}
	for _, entry := range railEntries(recent) {
		state.Recent = append(state.Recent, homeChannel(entry, a.libraryData.favorite(entry.Key), countries))
	}

	if hero, ok := a.activeHomeChannel(media, countries); ok {
		state.Kind = ui.HomeActive
		state.Hero = hero

		return state
	}
	if len(recent) > 0 && recent[0].Playable {
		state.Hero = homeChannel(recent[0], a.libraryData.favorite(recent[0].Key), countries)
	}

	return state
}

// activeHomeChannel reports the channel the session is presenting.
func (a *App) activeHomeChannel(media mediaState, countries map[string]string) (ui.HomeChannel, bool) {
	if media.channel.ProviderID == "" {
		return ui.HomeChannel{}, false
	}
	switch media.snapshot.State {
	case player.StateLoading, player.StateBuffering, player.StatePlaying:
	default:
		return ui.HomeChannel{}, false
	}

	channel := media.channel
	quality := ""
	if stream, ok := pickStream(channel); ok {
		quality = strings.TrimSpace(stream.Quality)
	}

	return ui.HomeChannel{
		Key:           channelKey(channel),
		Name:          strings.TrimSpace(channel.Name),
		SourceName:    media.sourceName,
		Group:         strings.TrimSpace(channel.Group),
		Country:       countryName(channel.Country, countries),
		Quality:       quality,
		LogoURL:       strings.TrimSpace(channel.LogoURL),
		Live:          channelLive(channel),
		Favorite:      a.libraryData.favorite(channelKey(channel)),
		Playable:      true,
		SourceEnabled: true,
	}, true
}

// homeChannel maps one library entry to a Home card.
func homeChannel(entry library.Entry, favorite bool, countries map[string]string) ui.HomeChannel {
	quality := ""
	if stream, ok := pickStream(entry.Channel); ok {
		quality = strings.TrimSpace(stream.Quality)
	}

	return ui.HomeChannel{
		Key:           entry.Key,
		Name:          strings.TrimSpace(entry.Channel.Name),
		SourceName:    libraryDisplayName(entry.Source),
		Group:         strings.TrimSpace(entry.Channel.Group),
		Country:       countryName(entry.Channel.Country, countries),
		Quality:       quality,
		LogoURL:       strings.TrimSpace(entry.Channel.LogoURL),
		Live:          channelLive(entry.Channel),
		Favorite:      favorite,
		Playable:      entry.Playable,
		SourceEnabled: entry.Source.Enabled,
		PlayedAt:      entry.PlayedAt,
	}
}

// countryName resolves a stored region code through the catalog country list.
func countryName(code string, countries map[string]string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ""
	}

	return countries[code]
}

// railEntries caps one rail to the widest layout's card count.
func railEntries(entries []library.Entry) []library.Entry {
	return entries[:min(len(entries), homeRailLimit)]
}

// artworkURLs collects distinct logo URLs from the saved entries.
func artworkURLs(groups ...[]library.Entry) []string {
	urls := make([]string, 0, homeArtworkLimit)
	seen := make(map[string]bool, homeArtworkLimit)

	for _, entries := range groups {
		for _, entry := range entries {
			url := strings.TrimSpace(entry.Channel.LogoURL)
			if url == "" || seen[url] {
				continue
			}
			seen[url] = true
			urls = append(urls, url)
			if len(urls) == homeArtworkLimit {
				return urls
			}
		}
	}

	return urls
}
