package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/library"
	"mumu/internal/player"
	"mumu/internal/ui"
	"mumu/internal/video"
)

const (
	// playbackSettingVolume stores the last confirmed volume.
	playbackSettingVolume = "playback.volume"
	// legacyPlaybackSettingMuted is the mute preference older releases wrote.
	// Mute is never restored now: every launch starts audible, so the key is
	// only read once to delete it and stop it being inherited.
	legacyPlaybackSettingMuted = "playback.muted"
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

// previousChannel is the channel a failed switch can return to.
type previousChannel struct {
	channel    catalog.Channel
	stream     catalog.Stream
	sourceName string
}

// mediaState is the playback and channel state shared by workers and the UI.
type mediaState struct {
	channel     catalog.Channel
	stream      catalog.Stream
	sourceName  string
	snapshot    player.Snapshot
	playbackErr string
	drawer      []ui.WatchChannel
	resume      *watchResume
	previous    *previousChannel
	markedGen   uint64
	// audioGen is the playback generation whose effective volume and mute were
	// already logged, so a silent stream leaves one line instead of none.
	audioGen uint64
	// attached reports the retained view showing in the main window, and
	// pipAttached reports it showing in the floating player. Both are false
	// while the view is detached, so neither can claim a drawable it lost.
	attached    bool
	pipAttached bool
}

// playbackUnavailableMessage tells the viewer what to fix. A release bundles
// its runtime, so the usual cause is a damaged or incomplete install; the
// environment override stays available for a custom runtime. Watch shows it
// only after a Play request, never at launch.
const playbackUnavailableMessage = "Playback needs the libmpv runtime bundled with mumu. Reinstall mumu, or set MUMU_MPV_LIBRARY, then retry."

// startWatch wires the Watch page and offers a resume.
//
// Playback is not opened here: the engine and its runtime are created on the
// first Play attempt, so a launch that never plays stays quiet and a machine
// without libmpv only hears about it when the viewer asks for playback.
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
	page.OnEnterPiP = a.enterPiP
	page.OnReturn = a.returnFromPiP
	page.OnBack = a.backToPrevious
	a.shell.OnDestinationChanged(a.watchVisibilityChanged)
	a.shell.Home().OnResume = a.resumeWatching
	a.shell.Home().OnWatch = a.shell.ShowWatch
	a.shell.Home().OnBrowse = a.shell.ShowDiscover

	pip := a.shell.PiP()
	pip.OnReturn = a.returnFromPiP
	pip.OnClose = a.closePiP
	pip.OnToggleMute = a.togglePiPMute

	a.loadPlaybackSettings()

	a.Go(func(ctx context.Context) error {
		a.loadWatchResume(ctx)

		return nil
	})
}

// playerController returns the one controller, creating it on demand.
//
// The first successful call builds the engine, the controller and, on a
// platform with a native renderer, the video surface. A failure leaves nothing
// half-built, so a later attempt (Retry, or a new channel) can succeed after
// the viewer installs the runtime.
func (a *App) playerController() (*player.Controller, error) {
	a.mu.Lock()
	controller := a.controller
	stopped := a.stopped
	a.mu.Unlock()

	if controller != nil {
		return controller, nil
	}
	if stopped {
		return nil, player.ErrClosed
	}

	a.openMu.Lock()
	defer a.openMu.Unlock()

	a.mu.Lock()
	controller = a.controller
	a.mu.Unlock()
	if controller != nil {
		return controller, nil
	}

	return a.openPlayer()
}

// openPlayer builds one engine and one controller for the whole app. The
// caller holds openMu.
//
// Mute is never restored: a launch always starts audible, and only the last
// volume carries over. The effective volume also reaches the native engine,
// whose own configuration would otherwise start at its zero value and play
// silently even when the controller reports a different volume.
func (a *App) openPlayer() (*player.Controller, error) {
	output := "null"
	surfaceSupported := video.Supported()
	if surfaceSupported {
		output = "libmpv"
	}

	a.mu.Lock()
	volume := effectiveVolume(a.media.snapshot.Volume)
	a.mu.Unlock()

	engine, err := a.newEngine(output, volume)
	if err != nil {
		return nil, err
	}

	controller := player.New(
		engine,
		player.WithInitialVolume(volume),
		player.WithInitialMuted(false),
		player.WithFullscreenHandler(a.fullscreenHandler),
	)

	var surface video.Surface
	if surfaceSupported {
		renderer, ok := engine.(player.Renderer)
		if ok {
			surface, err = video.New(renderer)
			if err != nil {
				slog.Debug("video surface unavailable", "error", err)
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

	return controller, nil
}

// loadPlaybackSettings restores the last volume. Mute is deliberately not
// restored: the approved policy is that every launch starts audible, so a
// viewer who muted before quitting still hears the next launch.
//
// It runs before any controller usually exists, so it also primes the state the
// Watch page and a later controller read.
func (a *App) loadPlaybackSettings() {
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

		volume, errVolume := strconv.Atoi(settings[playbackSettingVolume])
		if errVolume != nil || volume < 0 || volume > 100 {
			// A missing or unusable key must not leave the engine at zero.
			volume = player.DefaultVolume
		}

		a.mu.Lock()
		a.media.snapshot.Volume = volume
		a.media.snapshot.Muted = false
		controller := a.controller
		a.mu.Unlock()

		if controller != nil {
			_ = controller.SetVolume(volume)
			_ = controller.SetMuted(false)
		}

		if _, stale := settings[legacyPlaybackSettingMuted]; stale {
			writeCtx, writeCancel := context.WithTimeout(ctx, playbackTimeout)
			if err := store.Settings().Delete(writeCtx, legacyPlaybackSettingMuted); err != nil {
				slog.Debug("clear legacy mute setting", "error", err)
			}
			writeCancel()
		}

		a.publishWatch()
	})
}

// effectiveVolume reports a usable launch volume. A value outside the player's
// 0..100 range falls back to the default; zero is kept because the viewer may
// have saved it deliberately.
func effectiveVolume(volume int) int {
	if volume < 0 || volume > 100 {
		return player.DefaultVolume
	}

	return volume
}

// loadWatchResume offers the last played channel without starting it.
func (a *App) loadWatchResume(ctx context.Context) {
	service, err := a.UserLibrary()
	if err != nil {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
	defer cancel()

	entry, found, err := service.Last(readCtx)
	if err != nil || !found || !entry.Playable {
		return
	}

	a.mu.Lock()
	a.media.resume = &watchResume{
		channel:    entry.Channel,
		sourceName: libraryDisplayName(entry.Source),
	}
	a.mu.Unlock()

	a.publishWatch()
	a.publish(func() {
		if a.shell.IsDiscover() {
			a.shell.ShowHome()
		}
	})
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
//
// Before the new channel replaces the active one, a channel that was really
// presenting frames becomes the "previous" one, so a failed switch can offer a
// way back. A channel that never played, the same channel, or a state that
// never showed video (loading, error) is not remembered.
func (a *App) playStream(channel catalog.Channel, stream catalog.Stream, sourceName string) {
	scheme, err := streamScheme(stream.URL)
	if err != nil {
		a.failWatch("Only http and https streams are supported.")

		return
	}

	a.mu.Lock()
	old := a.media.channel
	if old.ProviderID != "" &&
		channelKey(old) != channelKey(channel) &&
		presentingState(a.media.snapshot.State) {
		a.media.previous = &previousChannel{
			channel:    old,
			stream:     a.media.stream,
			sourceName: a.media.sourceName,
		}
	}

	a.media.channel = channel
	a.media.stream = stream
	a.media.sourceName = sourceName
	a.media.markedGen = 0
	a.media.playbackErr = ""
	a.media.resume = nil
	a.mu.Unlock()

	a.shell.ShowWatch()
	a.publishWatch()

	// Playback is the first thing that may touch the native runtime, so a
	// machine without libmpv reports it here and only here.
	controller, err := a.playerController()
	if err != nil {
		if errors.Is(err, player.ErrClosed) {
			return
		}

		slog.Warn("native playback unavailable", "error", err)
		a.failWatch(playbackUnavailableMessage)

		return
	}

	// The surface exists now. It is attached once the controller reports real
	// frames; until then the page keeps its glass loading overlay above it.
	source := player.Source{
		URL:     stream.URL,
		Scheme:  scheme,
		Headers: streamHeaders(stream.Headers),
	}
	if err := controller.Play(source); err != nil {
		a.failWatch("The player could not start this stream.")
	}
}

// backToPrevious returns to the channel that played before the failed switch.
// It runs on the UI thread.
func (a *App) backToPrevious() {
	a.mu.Lock()
	previous := a.media.previous
	a.media.previous = nil
	a.mu.Unlock()

	if previous == nil || previous.stream.URL == "" {
		return
	}

	a.playStream(previous.channel, previous.stream, previous.sourceName)
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

	a.logPlaybackAudio(snapshot)
	a.publishWatch()
	a.markPlayingRecent(snapshot)
}

// logPlaybackAudio records the effective volume and mute once per playback
// generation, at the first real frame. A silent stream then leaves evidence of
// what the app asked for instead of an unexplained absence of sound. It logs no
// URL, header or credential.
func (a *App) logPlaybackAudio(snapshot player.Snapshot) {
	if snapshot.State != player.StatePlaying || snapshot.Generation == 0 {
		return
	}

	a.mu.Lock()
	if a.media.audioGen == snapshot.Generation {
		a.mu.Unlock()

		return
	}
	a.media.audioGen = snapshot.Generation
	a.mu.Unlock()

	slog.Debug("playback audio state",
		"generation", snapshot.Generation,
		"volume", snapshot.Volume,
		"muted", snapshot.Muted,
		"audio_output", snapshot.Metadata.AudioOutput,
		"video_output", snapshot.Metadata.VideoOutput,
	)
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

		return
	}

	// Keep an open Library in step without waiting for a manual reload.
	if err := a.loadLibraryEntries(ctx); err != nil {
		slog.Debug("reload recent entries", "error", err)

		return
	}
	a.publishLibrary()
}

// watchState builds the page state on the UI thread.
func (a *App) watchState() ui.WatchState {
	a.mu.Lock()
	media := a.media
	pipActive := a.presentation == video.PresentationPiP
	a.mu.Unlock()

	state := ui.WatchState{
		PiPActive: media.snapshot.State.Active() && pipActive,
		Kind:      ui.WatchEmpty,
		Channel:   media.channel.Name,
		Source:    media.sourceName,
		Volume:    media.snapshot.Volume,
		Muted:     media.snapshot.Muted,
		Favorite:  a.libraryData.favorite(channelKey(media.channel)),
		Live:      media.stream.Live || media.snapshot.Capabilities.Live,
		Status:    media.snapshot.Detail,
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

	// A failed switch offers a way back only when a different channel was
	// really presenting frames before it.
	if state.Kind == ui.WatchError &&
		media.previous != nil &&
		channelKey(media.previous.channel) != channelKey(media.channel) {
		state.Previous = media.previous.channel.Name
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
//
// The native view is moved before the page state is set: MyGo draws its glass
// loading and error panels inside the stage box, and a native subview is always
// above that layer. Detaching first means the overlay is never painted under a
// surface that is still attached.
func (a *App) publishWatch() {
	a.publish(func() {
		a.syncSurfaceVisibility()
		a.shell.Watch().SetState(a.watchState())
		a.shell.Home().SetState(a.homeState())
		a.shell.PiP().SetState(a.pipState())
	})
}

// homeState is the small Home contract for #34. #35 expands the feed.
func (a *App) homeState() ui.HomeState {
	a.mu.Lock()
	media := a.media
	a.mu.Unlock()

	switch {
	case media.channel.ProviderID != "" &&
		(media.snapshot.State == player.StateLoading ||
			media.snapshot.State == player.StateBuffering ||
			media.snapshot.State == player.StatePlaying):
		return ui.HomeState{
			Kind:    ui.HomeActive,
			Channel: media.channel.Name,
			Source:  media.sourceName,
		}
	case media.resume != nil:
		return ui.HomeState{
			Kind:    ui.HomeResume,
			Channel: media.resume.channel.Name,
			Source:  media.resume.sourceName,
		}
	default:
		return ui.HomeState{Kind: ui.HomeEmpty}
	}
}

// failWatch reports an actionable error on the Watch page. It can run on any
// goroutine, so navigation is posted to the UI thread like every other shell
// change; the two posts stay ordered because they use the same window.
func (a *App) failWatch(message string) {
	a.mu.Lock()
	a.media.playbackErr = message
	a.mu.Unlock()

	a.publish(func() { a.shell.ShowWatch() })
	a.publishWatch()
}

// reportSurfaceFailure shows one actionable message when the native view cannot
// be bound. Publishing once is enough: the reported error also stops further
// attach attempts until the next Play, so a persistent failure cannot loop
// through the publish closure.
func (a *App) reportSurfaceFailure(err error) {
	slog.Warn("attach video surface", "error", err)

	a.mu.Lock()
	changed := a.media.playbackErr == ""
	a.media.playbackErr = "The video surface could not be attached."
	a.mu.Unlock()

	if !changed {
		return
	}

	a.publish(func() { a.shell.ShowWatch() })
	a.publishWatch()
}

// presentingState reports whether the player is showing real frames. Those are
// the only states where the native view may cover the page: buffering keeps the
// last frame, and its badge lives in the control strip below the video.
func presentingState(state player.State) bool {
	return state == player.StatePlaying || state == player.StateBuffering
}

// applyVideoFrame places the native surface over the page's video box.
func (a *App) applyVideoFrame(frame ui.Rect) {
	a.mu.Lock()
	// Keep the box even before a channel is chosen: the first Play attaches
	// the surface with this frame instead of waiting for another layout pass.
	if frame.W > 0 && frame.H > 0 {
		a.mainFrame = video.Rect{X: frame.X, Y: frame.Y, W: frame.W, H: frame.H}
	}
	a.mu.Unlock()

	if err := a.attachVideoSurface(); err != nil {
		a.reportSurfaceFailure(err)
	}
}

// watchVisibilityChanged detaches the native view when Watch is left, so it
// cannot cover another page, and re-attaches on return without a reload.
func (a *App) watchVisibilityChanged(watch bool) {
	if watch {
		a.syncSurfaceVisibility()

		return
	}

	a.detachVideoSurface()
}

// syncSurfaceVisibility binds the retained view only while a channel really
// presents frames, and releases it for every overlay state. It runs on the UI
// thread, inside the publish closure, because Attach must be on the main
// thread.
func (a *App) syncSurfaceVisibility() {
	a.mu.Lock()
	surface := a.surface
	state := a.presentation
	active := a.media.channel.ProviderID != "" &&
		presentingState(a.media.snapshot.State) &&
		a.media.playbackErr == ""
	mainReady := a.win != nil
	pipReady := a.pipWindow != nil
	a.mu.Unlock()

	if surface == nil {
		return
	}

	switch state {
	case video.PresentationTransitioning:
		// A transfer owns the drawable for the length of one attach; it is
		// never observable from another publish on the same thread.
		return
	case video.PresentationMain:
		// Watch must be the current page: a stream that starts playing while
		// another page is open must not cover it.
		if !active || !mainReady || !a.shell.IsWatch() {
			a.detachVideoSurface()

			return
		}
		if err := a.attachVideoSurface(); err != nil {
			a.reportSurfaceFailure(err)
		}
	case video.PresentationPiP:
		if !active || !pipReady {
			a.detachPiPSurface()

			return
		}
		a.attachPiPSurface()
	}
}

// detachVideoSurface hides the retained view but keeps the session alive.
//
// The attached flag is the only guard: it is cleared whenever the view is not
// in the main window, including a move to PiP or a closed window, so a detach
// can never knock the floating player's drawable out.
func (a *App) detachVideoSurface() {
	a.mu.Lock()
	surface := a.surface
	attached := a.media.attached
	if surface == nil || !attached {
		a.mu.Unlock()

		return
	}
	a.media.attached = false
	a.mu.Unlock()

	if err := surface.Detach(); err != nil {
		slog.Debug("detach video surface", "error", err)
	}
}

// attachVideoSurface binds the surface to the main window when it is missing
// or has moved. It runs on the UI thread and never attaches over an overlay.
func (a *App) attachVideoSurface() error {
	a.mu.Lock()
	surface, win := a.surface, a.win
	state := a.presentation
	active := a.media.channel.ProviderID != "" &&
		presentingState(a.media.snapshot.State) &&
		a.media.playbackErr == ""
	attached := a.media.attached
	frame := a.mainFrame
	a.mu.Unlock()

	if surface == nil || win == nil || state != video.PresentationMain || !active {
		return nil
	}

	target := video.Fit(frame, videoAspect)
	if target.W <= 0 || target.H <= 0 {
		// The page has not reported its box yet; the next frame does.
		return nil
	}

	if !attached {
		if err := surface.Attach(win.NativeHandle(), target); err != nil {
			return fmt.Errorf("attach main window: %w", err)
		}

		a.mu.Lock()
		a.media.attached = true
		a.mu.Unlock()

		return nil
	}

	if err := surface.SetFrame(target); err != nil {
		slog.Debug("resize video surface", "error", err)
	}

	return nil
}

// attachPiPSurface binds the retained view to the floating window, or resizes
// it there. It runs on the UI thread.
func (a *App) attachPiPSurface() {
	a.mu.Lock()
	surface, pip := a.surface, a.pipWindow
	state := a.presentation
	attached := a.media.pipAttached
	a.mu.Unlock()

	if surface == nil || pip == nil || state != video.PresentationPiP {
		return
	}

	target := pipFrame(pip)
	if !attached {
		if err := surface.Attach(pip.NativeHandle(), target); err != nil {
			slog.Warn("attach video surface to picture-in-picture", "error", err)

			return
		}

		a.mu.Lock()
		a.media.pipAttached = true
		a.mu.Unlock()

		return
	}

	if err := surface.SetFrame(target); err != nil {
		slog.Debug("resize picture-in-picture surface", "error", err)
	}
}

// detachPiPSurface releases the floating window's drawable while the session
// keeps running, so the panel's status text is visible instead of a stale
// frame.
func (a *App) detachPiPSurface() {
	a.mu.Lock()
	surface := a.surface
	attached := a.media.pipAttached
	a.media.pipAttached = false
	a.mu.Unlock()

	if surface == nil || !attached {
		return
	}

	if err := surface.Detach(); err != nil {
		slog.Debug("detach picture-in-picture surface", "error", err)
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

// toggleWatchFavorite adds or removes the active channel.
func (a *App) toggleWatchFavorite() {
	a.mu.Lock()
	channel := a.media.channel
	a.mu.Unlock()

	if channel.ProviderID == "" {
		return
	}

	a.toggleFavorite(library.Entry{
		Key:     channelKey(channel),
		Channel: channel,
	})
}

// setPlaybackVolume changes the volume and remembers it. Without a controller
// it still updates the page, so the choice survives until the first Play.
func (a *App) setPlaybackVolume(volume int) {
	a.mu.Lock()
	controller := a.controller
	a.media.snapshot.Volume = volume
	a.mu.Unlock()

	if controller != nil {
		_ = controller.SetVolume(volume)
	} else {
		a.publishWatch()
	}
	a.savePlaybackSetting(playbackSettingVolume, strconv.Itoa(volume))
}

// togglePlaybackMute flips the mute state for this session.
//
// Mute is not persisted: the approved policy is that every launch starts
// audible, so the toggle changes the running player only.
func (a *App) togglePlaybackMute() {
	a.mu.Lock()
	controller := a.controller
	muted := !a.media.snapshot.Muted
	a.media.snapshot.Muted = muted
	a.mu.Unlock()

	if controller != nil {
		_ = controller.SetMuted(muted)
	} else {
		a.publishWatch()
	}
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
	a.pipDispatcher = nil
	a.media.attached = false
	a.media.pipAttached = false
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
