package app

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/player"
	"mumu/internal/sources"
	"mumu/internal/ui"
	"mumu/internal/video"
)

const (
	// playbackSettingVolume stores the last confirmed volume.
	playbackSettingVolume = "playback.volume"
	// playbackSettingMuted stores the last confirmed mute state.
	playbackSettingMuted = "playback.muted"
	// playbackTimeout bounds a settings read or write.
	playbackTimeout = 5 * time.Second
	// videoAspect is the video area's target aspect ratio.
	videoAspect = 16.0 / 9.0
)

// watchResume is the last channel offered without autoplay.
type watchResume struct {
	channel    catalog.Channel
	sourceName string
}

// mediaState is the playback and channel state shared by workers and the UI.
type mediaState struct {
	channel     catalog.Channel
	stream      catalog.Stream
	sourceName  string
	snapshot    player.Snapshot
	favorite    bool
	playbackErr string
	drawer      []ui.WatchChannel
	resume      *watchResume
	markedGen   uint64
	attached    bool
}

// startWatch wires the Watch page, opens the player and offers a resume.
func (a *App) startWatch() {
	page := a.shell.Watch()
	page.OnFrame = a.applyVideoFrame
	page.OnResume = a.resumeWatching
	page.OnRetry = a.retryWatching
	page.OnBrowse = a.shell.ShowDiscover
	page.OnVolume = a.setPlaybackVolume
	page.OnToggleMute = a.togglePlaybackMute
	page.OnFullscreen = a.toggleFullscreen
	page.OnToggleFavorite = a.toggleWatchFavorite
	page.OnSelectChannel = a.selectWatchChannel

	a.openPlayback()

	a.Go(func(ctx context.Context) error {
		a.loadWatchResume(ctx)

		return nil
	})
}

// openPlayback builds one engine and one controller for the whole app.
func (a *App) openPlayback() {
	a.mu.Lock()
	stopped := a.stopped
	a.mu.Unlock()
	if stopped {
		return
	}

	output := "null"
	surfaceSupported := video.Supported()
	if surfaceSupported {
		output = "libmpv"
	}

	engine, err := newPlaybackEngine(output)
	if err != nil {
		slog.Warn("native playback unavailable", "error", err)
		a.setPlaybackError("Native playback is unavailable in this build.")
		a.publishWatch()

		return
	}

	controller := player.New(engine, player.WithFullscreenHandler(a.fullscreenHandler))

	var surface video.Surface
	if surfaceSupported {
		renderer, ok := engine.(player.Renderer)
		if ok {
			surface, err = video.New(renderer)
			if err != nil {
				slog.Warn("video surface unavailable", "error", err)
			}
		}
	}

	a.mu.Lock()
	a.engine = engine
	a.controller = controller
	a.surface = surface
	a.mu.Unlock()

	if surface != nil {
		go a.watchSurfaceErrors(surface)
	}

	controller.Subscribe(a.playerSnapshot)
	a.loadPlaybackSettings(controller)
}

// loadPlaybackSettings restores the last volume and mute choice.
func (a *App) loadPlaybackSettings(controller *player.Controller) {
	a.GoQuiet(func(ctx context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			return
		}

		readCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
		defer cancel()

		settings, err := store.Settings().All(readCtx)
		if err != nil {
			return
		}
		if volume, err := strconv.Atoi(settings[playbackSettingVolume]); err == nil {
			_ = controller.SetVolume(volume)
		}
		if muted, err := strconv.ParseBool(settings[playbackSettingMuted]); err == nil {
			_ = controller.SetMuted(muted)
		}
	})
}

// loadWatchResume offers the last played channel without starting it.
func (a *App) loadWatchResume(ctx context.Context) {
	store, err := a.Store()
	if err != nil || store == nil {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
	defer cancel()

	recent, found, err := store.Recent().Last(readCtx)
	if err != nil || !found {
		return
	}

	source, found, err := store.Sources().Get(readCtx, recent.Channel.SourceID)
	if err != nil || !found || !source.Enabled {
		return
	}

	streams, err := store.Channels().ListStreams(readCtx, recent.Channel.ID)
	if err != nil || len(streams) == 0 {
		return
	}

	channel := sources.FromStorageChannel(recent.Channel, streams)
	if _, ok := pickStream(channel); !ok {
		return
	}

	a.mu.Lock()
	a.media.resume = &watchResume{channel: channel, sourceName: source.DisplayName}
	a.mu.Unlock()

	a.publishWatch()
	a.publish(func() { a.shell.ShowWatch() })
}

// resumeWatching starts the offered last channel. It runs on the UI thread.
func (a *App) resumeWatching() {
	a.mu.Lock()
	resume := a.media.resume
	a.mu.Unlock()

	if resume != nil {
		a.playChannel(resume.channel, resume.sourceName)
	}
}

// retryWatching reloads the active channel. It runs on the UI thread.
func (a *App) retryWatching() {
	a.mu.Lock()
	channel, stream, sourceName := a.media.channel, a.media.stream, a.media.sourceName
	a.mu.Unlock()

	if channel.ProviderID == "" || stream.URL == "" {
		return
	}

	a.playStream(channel, stream, sourceName)
}

// playChannel starts a channel from the catalog. It runs on the UI thread.
func (a *App) playChannel(channel catalog.Channel, sourceName string) {
	stream, ok := pickStream(channel)
	if !ok {
		a.failWatch("This channel has no supported stream URL.")

		return
	}

	a.playStream(channel, stream, sourceName)
}

// playStream sends one stream to the shared controller.
func (a *App) playStream(channel catalog.Channel, stream catalog.Stream, sourceName string) {
	scheme, err := streamScheme(stream.URL)
	if err != nil {
		a.failWatch("Only http and https streams are supported.")

		return
	}

	a.mu.Lock()
	controller := a.controller
	a.media.channel = channel
	a.media.stream = stream
	a.media.sourceName = sourceName
	a.media.favorite = false
	a.media.markedGen = 0
	a.media.playbackErr = ""
	a.media.resume = nil
	a.mu.Unlock()

	a.shell.ShowWatch()
	a.publishWatch()
	a.refreshFavorite(channel)

	if controller == nil {
		a.failWatch("Native playback is unavailable in this build.")

		return
	}

	source := player.Source{
		URL:     stream.URL,
		Scheme:  scheme,
		Headers: streamHeaders(stream.Headers),
	}
	if err := controller.Play(source); err != nil {
		a.failWatch("The player could not start this stream.")
	}
}

// selectWatchChannel switches to a drawer row. It runs on the UI thread.
func (a *App) selectWatchChannel(key string) {
	channel, ok := a.discoverIndex.channel(key)
	if !ok {
		return
	}

	a.playChannel(channel, a.discoverIndex.sourceName(channel.SourceID))
}

// playerSnapshot receives controller state off the UI thread.
func (a *App) playerSnapshot(snapshot player.Snapshot) {
	a.mu.Lock()
	a.media.snapshot = snapshot
	a.mu.Unlock()

	a.publishWatch()
	a.markPlayingRecent(snapshot)
}

// markPlayingRecent records a real playback start once per generation.
func (a *App) markPlayingRecent(snapshot player.Snapshot) {
	if snapshot.State != player.StatePlaying || snapshot.Generation == 0 {
		return
	}

	a.mu.Lock()
	channel := a.media.channel
	if channel.ProviderID == "" || a.media.markedGen == snapshot.Generation {
		a.mu.Unlock()

		return
	}
	a.media.markedGen = snapshot.Generation
	a.mu.Unlock()

	a.GoQuiet(func(ctx context.Context) {
		a.recordRecent(ctx, channel)
	})
}

// recordRecent stores the channel once the stream genuinely played.
func (a *App) recordRecent(ctx context.Context, channel catalog.Channel) {
	store, err := a.Store()
	if err != nil || store == nil {
		return
	}

	writeCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
	defer cancel()

	stored, found, err := store.Channels().FindByProvider(writeCtx, channel.SourceID, channel.ProviderID)
	if err != nil || !found {
		return
	}
	if err := store.Recent().MarkPlayed(writeCtx, stored.ID, time.Now().UTC()); err != nil {
		slog.Debug("record recent channel", "error", err)
	}
}

// watchState builds the page state on the UI thread.
func (a *App) watchState() ui.WatchState {
	a.mu.Lock()
	media := a.media
	a.mu.Unlock()

	state := ui.WatchState{
		Kind:     ui.WatchEmpty,
		Channel:  media.channel.Name,
		Source:   media.sourceName,
		Volume:   media.snapshot.Volume,
		Muted:    media.snapshot.Muted,
		Favorite: media.favorite,
		Live:     media.stream.Live || media.snapshot.Capabilities.Live,
		Status:   media.snapshot.Detail,
	}

	if media.channel.ProviderID == "" {
		if media.resume != nil {
			state.Kind = ui.WatchResume
			state.Channel = media.resume.channel.Name
			state.Source = media.resume.sourceName
			for _, stream := range media.resume.channel.Streams {
				if stream.Live {
					state.Live = true

					break
				}
			}
		}
	} else {
		state.Kind = watchKind(media.snapshot.State)
		state.Error = media.snapshot.Error
	}

	if media.playbackErr != "" {
		state.Kind = ui.WatchError
		state.Error = media.playbackErr
	}

	state.Channels = make([]ui.WatchChannel, 0, len(media.drawer))
	for _, row := range media.drawer {
		row.Current = row.Key == channelKey(media.channel)
		state.Channels = append(state.Channels, row)
	}

	return state
}

// watchKind maps the player state to the page state.
func watchKind(state player.State) ui.WatchKind {
	switch state {
	case player.StateLoading:
		return ui.WatchLoading
	case player.StatePlaying:
		return ui.WatchPlaying
	case player.StateBuffering:
		return ui.WatchBuffering
	case player.StateError:
		return ui.WatchError
	case player.StateStopping, player.StateStopped:
		return ui.WatchStopped
	default:
		return ui.WatchLoading
	}
}

// publishWatch shows the current media state on the UI thread.
func (a *App) publishWatch() {
	a.publish(func() { a.shell.Watch().SetState(a.watchState()) })
}

// failWatch reports an actionable error on the Watch page.
func (a *App) failWatch(message string) {
	a.mu.Lock()
	a.media.playbackErr = message
	a.mu.Unlock()

	a.shell.ShowWatch()
	a.publishWatch()
}

// setPlaybackError records a build or runtime playback limitation.
func (a *App) setPlaybackError(message string) {
	a.mu.Lock()
	a.media.playbackErr = message
	a.mu.Unlock()
}

// applyVideoFrame places the native surface over the page's video box.
func (a *App) applyVideoFrame(frame ui.Rect) {
	a.mu.Lock()
	surface, win := a.surface, a.win
	active := a.media.channel.ProviderID != ""
	attached := a.media.attached
	a.mu.Unlock()

	if surface == nil || win == nil || !active {
		return
	}

	target := video.Fit(video.Rect{
		X: frame.X,
		Y: frame.Y,
		W: frame.W,
		H: frame.H,
	}, videoAspect)
	if target.W <= 0 || target.H <= 0 {
		return
	}

	if !attached {
		if err := surface.Attach(win.NativeHandle(), target); err != nil {
			slog.Warn("attach video surface", "error", err)
			a.failWatch("The video surface could not be attached.")

			return
		}

		a.mu.Lock()
		a.media.attached = true
		a.mu.Unlock()

		return
	}

	if err := surface.SetFrame(target); err != nil {
		slog.Debug("resize video surface", "error", err)
	}
}

// watchSurfaceErrors reports renderer failures without crashing the app.
func (a *App) watchSurfaceErrors(surface video.Surface) {
	for err := range surface.Errors() {
		if err == nil {
			continue
		}
		slog.Error("video renderer stopped", "error", err)
		a.failWatch("The video renderer stopped. Retry the channel.")
	}
}

// watchFavorite reports whether the active channel is a favorite.
func (a *App) refreshFavorite(channel catalog.Channel) {
	a.GoQuiet(func(ctx context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			return
		}

		readCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
		defer cancel()

		stored, found, err := store.Channels().FindByProvider(readCtx, channel.SourceID, channel.ProviderID)
		if err != nil || !found {
			return
		}
		favorite, err := store.Favorites().Has(readCtx, stored.ID)
		if err != nil {
			return
		}

		a.mu.Lock()
		if a.media.channel.ProviderID == channel.ProviderID {
			a.media.favorite = favorite
		}
		a.mu.Unlock()

		a.publishWatch()
	})
}

// toggleWatchFavorite adds or removes the active channel.
func (a *App) toggleWatchFavorite() {
	a.mu.Lock()
	channel, favorite := a.media.channel, a.media.favorite
	a.mu.Unlock()

	if channel.ProviderID == "" {
		return
	}

	a.GoQuiet(func(ctx context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			return
		}

		writeCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
		defer cancel()

		stored, found, err := store.Channels().FindByProvider(writeCtx, channel.SourceID, channel.ProviderID)
		if err != nil || !found {
			return
		}

		var errFavorite error
		if favorite {
			errFavorite = store.Favorites().Remove(writeCtx, stored.ID)
		} else {
			errFavorite = store.Favorites().Add(writeCtx, stored.ID, time.Now().UTC())
		}
		if errFavorite != nil {
			slog.Debug("toggle favorite", "error", errFavorite)

			return
		}

		a.mu.Lock()
		if a.media.channel.ProviderID == channel.ProviderID {
			a.media.favorite = !favorite
		}
		a.mu.Unlock()

		a.publishWatch()
	})
}

// setPlaybackVolume changes the volume and remembers it.
func (a *App) setPlaybackVolume(volume int) {
	a.mu.Lock()
	controller := a.controller
	a.mu.Unlock()

	if controller != nil {
		_ = controller.SetVolume(volume)
	}
	a.savePlaybackSetting(playbackSettingVolume, strconv.Itoa(volume))
}

// togglePlaybackMute flips the mute state and remembers it.
func (a *App) togglePlaybackMute() {
	a.mu.Lock()
	controller := a.controller
	muted := a.media.snapshot.Muted
	a.mu.Unlock()

	muted = !muted
	if controller != nil {
		_ = controller.SetMuted(muted)
	}
	a.savePlaybackSetting(playbackSettingMuted, strconv.FormatBool(muted))
}

// savePlaybackSetting stores one confirmed preference.
func (a *App) savePlaybackSetting(key, value string) {
	a.GoQuiet(func(ctx context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			return
		}

		writeCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
		defer cancel()

		if err := store.Settings().Set(writeCtx, key, value); err != nil {
			slog.Debug("save playback setting", "key", key, "error", err)
		}
	})
}

// toggleFullscreen asks the main window for native fullscreen.
func (a *App) toggleFullscreen() {
	a.publish(func() {
		if a.win != nil {
			a.win.ToggleFullScreen()
		}
	})
}

// fullscreenHandler lets the controller request native fullscreen.
func (a *App) fullscreenHandler() {
	a.toggleFullscreen()
}

// shutdownPlayback closes the surface before the engine, as required.
func (a *App) shutdownPlayback() {
	a.mu.Lock()
	surface, controller, engine := a.surface, a.controller, a.engine
	a.surface, a.controller, a.engine = nil, nil, nil
	a.mu.Unlock()

	if surface != nil {
		surface.Close()
	}
	if controller != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := controller.Close(ctx); err != nil {
			slog.Warn("close player", "error", err)
		}
		cancel()

		return
	}
	if engine != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = engine.Close(ctx)
		cancel()
	}
}

// pickStream chooses the first supported stream variant.
func pickStream(channel catalog.Channel) (catalog.Stream, bool) {
	var fallback catalog.Stream
	haveFallback := false

	for _, stream := range channel.Streams {
		if _, err := streamScheme(stream.URL); err != nil {
			continue
		}
		if stream.Live || stream.HLS {
			return stream, true
		}
		if !haveFallback {
			fallback = stream
			haveFallback = true
		}
	}

	return fallback, haveFallback
}

// streamScheme returns the normalized, supported scheme of rawURL.
func streamScheme(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("unsupported stream scheme")
	}
	if parsed.Host == "" {
		return "", errors.New("stream URL has no host")
	}

	return scheme, nil
}

// streamHeaders copies only the playback headers mumu keeps.
func streamHeaders(headers catalog.Headers) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	out := make(map[string]string, 2)
	if userAgent := headers.Get("User-Agent"); userAgent != "" {
		out["User-Agent"] = userAgent
	}
	if referer := headers.Get("Referer"); referer != "" {
		out["Referer"] = referer
	}
	if len(out) == 0 {
		return nil
	}

	return out
}

// channelKey is the shared row identity for a channel.
func channelKey(channel catalog.Channel) string {
	if channel.ProviderID == "" {
		return ""
	}
	if channel.SourceID == "" {
		return channel.ProviderID
	}

	return channel.SourceID + ":" + channel.ProviderID
}
