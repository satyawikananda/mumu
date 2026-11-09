package app

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
	"mumu/internal/playlist"
	"mumu/internal/sources"
	"mumu/internal/storage"
	"mumu/internal/ui"
)

// libraryTimeout bounds one Library operation.
const libraryTimeout = 30 * time.Second

// libraryInputError is a draft problem the page shows as typed.
type libraryInputError string

func (e libraryInputError) Error() string { return string(e) }

// libraryPending is the validated draft the Import button may save.
type libraryPending struct {
	req     ui.LibraryRequest
	preview sources.Preview
	valid   bool
}

// libraryCache is the Library page's shared state. The mutex keeps the
// source rows, per-row busy flags and the pending preview consistent while
// background jobs and the UI thread publish them.
type libraryCache struct {
	mu      sync.Mutex
	sources []catalog.Source
	busy    map[string]bool
	loaded  bool
	loadErr string
	message string
	pending libraryPending
}

func newLibraryCache() *libraryCache {
	return &libraryCache{busy: map[string]bool{}}
}

// snapshot builds the page state; callers publish it on the UI thread.
func (c *libraryCache) snapshot() ui.LibraryState {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.loaded {
		state := ui.LibraryState{Kind: ui.LibraryLoading}
		if c.loadErr != "" {
			state.Kind = ui.LibraryFailed
			state.Message = c.loadErr
		}

		return state
	}

	rows := make([]ui.LibrarySource, 0, len(c.sources))
	for _, source := range c.sources {
		if source.Kind == catalog.SourceBuiltin {
			continue
		}

		rows = append(rows, ui.LibrarySource{
			ID:          source.ID,
			Name:        libraryDisplayName(source),
			Kind:        libraryKind(source.Kind),
			Enabled:     source.Enabled,
			Channels:    source.ItemCount,
			LastSuccess: source.LastSuccessAt,
			Error:       source.RefreshError,
			Busy:        c.busy[source.ID],
		})
	}

	return ui.LibraryState{Kind: ui.LibraryReady, Sources: rows, Message: c.message}
}

// setSources replaces the cached rows; loaded reports a successful read.
func (c *libraryCache) setSources(list []catalog.Source, loaded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sources = list
	if loaded {
		c.loaded = true
		c.loadErr = ""
	}
}

// setLoadError records why the list could not be read.
func (c *libraryCache) setLoadError(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.loadErr = message
	if c.loaded {
		c.message = message
	}
}

// setMessage sets the page's inline banner.
func (c *libraryCache) setMessage(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.message = message
}

// markBusy claims one row's work, reporting false if it is already busy.
func (c *libraryCache) markBusy(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.busy[id] {
		return false
	}
	c.busy[id] = true

	return true
}

// clearBusy releases one row.
func (c *libraryCache) clearBusy(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.busy, id)
}

// setEnabled updates one cached row optimistically.
func (c *libraryCache) setEnabled(id string, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.sources {
		if c.sources[i].ID == id {
			c.sources[i].Enabled = enabled

			return
		}
	}
}

// drop removes one cached row after deletion.
func (c *libraryCache) drop(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	kept := c.sources[:0]
	for _, source := range c.sources {
		if source.ID != id {
			kept = append(kept, source)
		}
	}
	c.sources = kept
}

// setPending remembers the validated draft Import may save.
func (c *libraryCache) setPending(req ui.LibraryRequest, preview sources.Preview) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pending = libraryPending{req: req, preview: preview, valid: true}
}

// pendingPreview returns the draft for req, if it still matches.
func (c *libraryCache) pendingPreview(req ui.LibraryRequest) (sources.Preview, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.pending.valid || c.pending.req != req {
		return sources.Preview{}, false
	}

	return c.pending.preview, true
}

// clearPending forgets any validated draft.
func (c *libraryCache) clearPending() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pending = libraryPending{}
}

// startLibrary wires the Library page and publishes its saved sources.
func (a *App) startLibrary() {
	page := a.shell.Library()
	page.OnReload = a.reloadLibrary
	page.OnRefresh = a.refreshLibrarySource
	page.OnToggle = a.toggleLibrarySource
	page.OnRemove = a.removeLibrarySource
	page.OnPickFile = a.pickLibraryFile
	page.OnPreview = a.previewLibrarySource
	page.OnImport = a.importLibrarySource
	page.OnSheetClose = a.cancelLibraryPreview
	page.SetState(ui.LibraryState{Kind: ui.LibraryLoading})

	a.Go(func(ctx context.Context) error {
		a.loadLibrary(ctx)

		return nil
	})
}

// reloadLibrary re-reads the source list in the background.
func (a *App) reloadLibrary() {
	a.Go(func(ctx context.Context) error {
		a.loadLibrary(ctx)

		return nil
	})
}

// loadLibrary reads every user source and publishes it.
func (a *App) loadLibrary(ctx context.Context) {
	service, err := a.Catalog()
	if err != nil {
		a.libraryData.setLoadError("mumu could not open its local library. Retry, or add a playlist.")
		a.publishLibrary()

		return
	}

	listCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
	list, err := service.ListUserSources(listCtx)
	cancel()

	if err != nil {
		a.libraryData.setLoadError("mumu could not read the saved playlists. Retry, or add a playlist.")
		a.publishLibrary()

		return
	}

	a.libraryData.setSources(list, true)
	a.libraryData.setMessage("")
	a.publishLibrary()
}

// loadLibraryRows refreshes the rows without changing a loaded page's kind.
func (a *App) loadLibraryRows(ctx context.Context, service *sources.Service) {
	if service == nil {
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
	list, err := service.ListUserSources(listCtx)
	cancel()

	if err != nil {
		return
	}

	a.libraryData.setSources(list, true)
}

// publishLibrary shows the cached rows on the UI thread.
func (a *App) publishLibrary() {
	state := a.libraryData.snapshot()
	a.publish(func() { a.shell.Library().SetState(state) })
}

// refreshLibrarySource re-reads one source without blocking the page.
func (a *App) refreshLibrarySource(id string) {
	if !a.libraryData.markBusy(id) {
		return
	}
	a.publishLibrary()

	a.Go(func(ctx context.Context) error {
		service, err := a.Catalog()
		if err == nil {
			opCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
			_, err = service.Refresh(opCtx, id)
			cancel()
		}

		if err != nil && ctx.Err() == nil {
			// The row keeps its channels; Refresh recorded the failure.
			slog.Debug("source refresh unavailable", "source", id)
		} else if err == nil {
			a.libraryData.setMessage("")
		}

		a.libraryData.clearBusy(id)
		a.loadLibraryRows(ctx, service)
		a.publishLibrary()

		if err == nil {
			a.refreshDiscoverFromCache()
		}

		return nil
	})
}

// toggleLibrarySource enables or disables one source.
func (a *App) toggleLibrarySource(id string, enabled bool) {
	if !a.libraryData.markBusy(id) {
		return
	}

	// Optimistic switch; revert when the store refuses.
	a.libraryData.setEnabled(id, enabled)
	a.publishLibrary()

	a.Go(func(ctx context.Context) error {
		service, err := a.Catalog()
		if err == nil {
			opCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
			err = service.SetEnabled(opCtx, id, enabled)
			cancel()
		}

		if err != nil {
			a.libraryData.setEnabled(id, !enabled)
			a.libraryData.setMessage("mumu could not update that source. Try again.")
		} else {
			a.libraryData.setMessage("")
			a.loadLibraryRows(ctx, service)
		}

		a.libraryData.clearBusy(id)
		a.publishLibrary()

		if err == nil {
			a.refreshDiscoverFromCache()
		}

		return nil
	})
}

// removeLibrarySource deletes a source after the page confirmed it.
func (a *App) removeLibrarySource(id string) {
	if !a.libraryData.markBusy(id) {
		return
	}
	a.publishLibrary()

	a.Go(func(ctx context.Context) error {
		service, err := a.Catalog()
		if err == nil {
			opCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
			err = service.Remove(opCtx, id)
			cancel()
		}

		if err != nil {
			if ctx.Err() == nil {
				a.libraryData.setMessage("mumu could not remove that source. Try again.")
			}
		} else {
			a.libraryData.drop(id)
			a.libraryData.setMessage("")
		}

		a.libraryData.clearBusy(id)
		a.publishLibrary()

		if err == nil {
			a.refreshDiscoverFromCache()
		}

		return nil
	})
}

// pickLibraryFile opens the native file dialog off the UI thread and
// publishes the chosen path back.
func (a *App) pickLibraryFile() {
	a.GoQuiet(func(context.Context) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:       "Choose an M3U playlist",
			ButtonLabel: "Choose",
			Message:     "The playlist is read from this Mac. Nothing is uploaded.",
			Filters: []mygo.FileFilter{{
				Name:       "M3U playlists",
				Extensions: []string{"m3u", "m3u8"},
			}},
		})
		if err != nil {
			slog.Debug("file dialog unavailable")
			a.publish(func() {
				a.shell.Library().SetFilePathError("The file dialog could not open. Try again.")
			})

			return
		}

		path := ""
		if len(paths) > 0 {
			path = paths[0]
		}
		a.publish(func() { a.shell.Library().SetFilePath(path) })
	})
}

// previewLibrarySource validates a draft and publishes a summary.
func (a *App) previewLibrarySource(req ui.LibraryRequest) {
	gen, ctx := a.library.begin(a.ctx)

	a.GoQuiet(func(context.Context) {
		service, err := a.Catalog()
		if err != nil {
			a.publishPreview(gen, nil, "mumu could not open its local library. Try again.")

			return
		}

		preview, err := libraryPreviewRequest(ctx, service, req)
		if ctx.Err() != nil || !a.library.current(gen) {
			return
		}
		if err != nil {
			a.publishPreview(gen, nil, libraryErrorText(err))

			return
		}

		a.publish(func() {
			if !a.library.current(gen) {
				return
			}
			a.libraryData.setPending(req, preview)
			a.shell.Library().SetPreview(libraryPreview(preview), "")
		})
	})
}

// importLibrarySource saves the validated draft.
func (a *App) importLibrarySource(req ui.LibraryRequest) {
	preview, ok := a.libraryData.pendingPreview(req)
	if !ok {
		a.publish(func() {
			a.shell.Library().SetImportError("Validate and preview the source again before importing.")
		})

		return
	}

	gen, ctx := a.library.begin(a.ctx)

	a.GoQuiet(func(context.Context) {
		service, err := a.Catalog()
		if err != nil {
			a.publishImport(gen, "mumu could not open its local library. Try again.")

			return
		}

		opCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
		_, err = service.Save(opCtx, preview)
		cancel()

		if err != nil {
			if ctx.Err() == nil && a.library.current(gen) {
				a.publishImport(gen, libraryErrorText(err))
			}

			return
		}

		a.libraryData.clearPending()
		a.libraryData.setMessage("")
		// The reload must outlive the import session the closed sheet cancels.
		a.loadLibraryRows(a.ctx, service)
		a.publishLibrary()
		a.publish(func() {
			if a.library.current(gen) {
				a.shell.Library().ImportFinished()
			}
		})
		a.refreshDiscoverFromCache()
	})
}

// cancelLibraryPreview drops pending preview work when the sheet closes.
func (a *App) cancelLibraryPreview() {
	a.library.begin(a.ctx)
	a.libraryData.clearPending()
}

// publishPreview shows a validation outcome on the UI thread.
func (a *App) publishPreview(gen uint64, preview *ui.LibraryPreview, errText string) {
	a.publish(func() {
		if !a.library.current(gen) {
			return
		}
		a.shell.Library().SetPreview(preview, errText)
	})
}

// publishImport reports a failed import so the sheet stays usable.
func (a *App) publishImport(gen uint64, message string) {
	a.publish(func() {
		if !a.library.current(gen) {
			return
		}
		a.shell.Library().SetImportError(message)
	})
}

// refreshDiscoverFromCache republishes Discover from local storage only, so
// catalog views follow source and visibility changes without network work.
func (a *App) refreshDiscoverFromCache() {
	gen := a.discover.generation()
	if gen == 0 {
		return
	}

	a.GoQuiet(func(ctx context.Context) {
		service, err := a.Catalog()
		if err != nil {
			return
		}

		readCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
		region, err := service.Region(readCtx)
		if err != nil {
			cancel()

			return
		}

		community, settingErr := service.BuiltinEnabled(readCtx)
		var builtin []catalog.Channel
		if settingErr == nil && community {
			_, builtin, _, _ = service.BuiltinSource(readCtx, region)
		}
		users, _ := service.UserChannels(readCtx)
		cancel()

		channels := mergeDiscoverChannels(builtin, users)
		if !a.discover.current(gen) {
			return
		}

		a.publish(func() {
			if a.discover.current(gen) {
				a.shell.Discover().SetNotice(discoverNotice(community))
			}
		})

		if len(channels) == 0 {
			state := ui.DiscoverState{Kind: ui.DiscoverEmpty, Region: region}
			if settingErr == nil && !community {
				state.Message = "The community catalog is turned off. Add your own playlist, or show it again in Settings."
			}
			a.publishDiscover(gen, state)

			return
		}

		a.publishDiscover(gen, discoverState(ui.DiscoverReady, region, channels))
		a.prefetchLogos(ctx, gen, channels)
	})
}

// libraryPreviewRequest validates a draft through the catalog service.
func libraryPreviewRequest(
	ctx context.Context,
	service *sources.Service,
	req ui.LibraryRequest,
) (sources.Preview, error) {
	opCtx, cancel := context.WithTimeout(ctx, libraryTimeout)
	defer cancel()

	switch req.Kind {
	case ui.LibraryPlaylistURL:
		if strings.TrimSpace(req.URL) == "" {
			return sources.Preview{}, libraryInputError("Enter a playlist URL.")
		}

		return service.PreviewRemote(opCtx, req.URL, req.Name)
	case ui.LibraryPlaylistFile:
		if strings.TrimSpace(req.Path) == "" {
			return sources.Preview{}, libraryInputError("Choose an M3U file first.")
		}

		return service.PreviewLocal(opCtx, req.Path, req.Name)
	case ui.LibraryStream:
		if strings.TrimSpace(req.URL) == "" {
			return sources.Preview{}, libraryInputError("Enter a stream URL.")
		}

		return service.PreviewStream(opCtx, req.URL, req.Name)
	default:
		return sources.Preview{}, libraryInputError("Choose a source type.")
	}
}

// libraryErrorText turns a failure into credential-free, actionable text.
func libraryErrorText(err error) string {
	if err == nil {
		return ""
	}

	var input libraryInputError
	if errors.As(err, &input) {
		return string(input)
	}

	var playlistErr *playlist.Error
	if errors.As(err, &playlistErr) && playlistErr.Message != "" {
		return playlistErr.Message
	}

	switch {
	case errors.Is(err, sources.ErrBuiltinManaged):
		return "Built-in channels are managed from Discover."
	case errors.Is(err, netguard.ErrPrivateAddress):
		return "Local and private addresses are not allowed for remote playlists."
	case errors.Is(err, netguard.ErrCredentials):
		return "URLs with embedded credentials are not supported."
	case errors.Is(err, netguard.ErrUnsupportedScheme):
		return "Only http and https URLs are supported."
	case errors.Is(err, netguard.ErrInvalidURL):
		return "That URL is not valid."
	case errors.Is(err, netguard.ErrResponseTooLarge):
		return "The source is larger than the supported limit."
	case errors.Is(err, context.DeadlineExceeded):
		return "The source took too long to answer. Try again."
	case errors.Is(err, playlist.ErrUnreadable):
		return "The selected file could not be read."
	case errors.Is(err, storage.ErrNotFound):
		return "That source no longer exists."
	default:
		return "Could not read that source. Check it and try again."
	}
}

// libraryPreview maps a validated source to a credential-free summary.
func libraryPreview(preview sources.Preview) *ui.LibraryPreview {
	samples := make([]string, 0, 5)
	for _, channel := range preview.Channels {
		name := strings.TrimSpace(channel.Name)
		if name == "" {
			continue
		}

		samples = append(samples, name)
		if len(samples) == 5 {
			break
		}
	}

	warnings := make([]string, 0, 3)
	for _, warning := range preview.Warnings {
		if len(warnings) == 3 {
			break
		}

		warnings = append(warnings, warning.Message)
	}

	return &ui.LibraryPreview{
		Kind:            libraryKind(preview.Kind),
		Name:            preview.DisplayName,
		Location:        libraryLocation(preview.Kind, preview.Location),
		Channels:        len(preview.Channels),
		Samples:         samples,
		Warnings:        warnings,
		WarningCount:    len(preview.Warnings),
		Skipped:         preview.Skipped,
		SkippedReasons:  librarySkippedReasons(preview.SkippedReasons),
		SkippedExamples: librarySkippedExamples(preview.SkippedEntries),
		Truncated:       preview.Truncated,
	}
}

// librarySkippedReasons counts skips by reason, most frequent first.
func librarySkippedReasons(counts map[playlist.Code]int) []ui.SkippedReason {
	reasons := make([]ui.SkippedReason, 0, len(counts))
	seen := make(map[playlist.Code]bool, len(counts))

	for _, code := range skippedReasonOrder {
		count := counts[code]
		if count <= 0 {
			continue
		}

		reasons = append(reasons, ui.SkippedReason{Label: skippedReasonLabel(code), Count: count})
		seen[code] = true
	}

	extra := make([]playlist.Code, 0, len(counts))
	for code, count := range counts {
		if count > 0 && !seen[code] {
			extra = append(extra, code)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	for _, code := range extra {
		reasons = append(reasons, ui.SkippedReason{Label: skippedReasonLabel(code), Count: counts[code]})
	}

	// Stable ties keep skippedReasonOrder's readable grouping.
	sort.SliceStable(reasons, func(i, j int) bool { return reasons[i].Count > reasons[j].Count })

	return reasons
}

// librarySkippedExamples names at most three skipped entries. Only the name
// and reason label leave the parser; scheme and host are never shown.
func librarySkippedExamples(entries []playlist.SkippedEntry) []string {
	examples := make([]string, 0, 3)
	for _, entry := range entries {
		if len(examples) == 3 {
			break
		}

		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}

		examples = append(examples, name+" — "+skippedReasonLabel(entry.Reason))
	}

	return examples
}

var skippedReasonOrder = []playlist.Code{
	playlist.CodeMissingURL,
	playlist.CodeMissingName,
	playlist.CodeMalformedLine,
	playlist.CodeInvalidHeader,
	playlist.CodeCredentials,
	playlist.CodePrivateAddress,
	playlist.CodeUnsupportedScheme,
	playlist.CodeInvalidURL,
}

func skippedReasonLabel(code playlist.Code) string {
	switch code {
	case playlist.CodeMissingURL:
		return "entries without a URL"
	case playlist.CodeMissingName:
		return "entries without a name"
	case playlist.CodeMalformedLine:
		return "malformed entries"
	case playlist.CodeInvalidHeader:
		return "entries with invalid headers"
	case playlist.CodeCredentials:
		return "entries with embedded credentials"
	case playlist.CodePrivateAddress:
		return "entries with a private or local host"
	case playlist.CodeUnsupportedScheme:
		return "entries with an unsupported scheme"
	case playlist.CodeInvalidURL:
		return "entries with an invalid URL"
	default:
		return "skipped entries"
	}
}

// libraryLocation shows a source location without credentials or query.
func libraryLocation(kind catalog.SourceKind, location string) string {
	if kind == catalog.SourceLocalPlaylist {
		return filepath.Base(location)
	}

	parsed, err := url.Parse(strings.TrimSpace(location))
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

// libraryKind maps the domain source kind to the page's label.
func libraryKind(kind catalog.SourceKind) ui.LibraryKind {
	switch kind {
	case catalog.SourceLocalPlaylist:
		return ui.LibraryPlaylistFile
	case catalog.SourceCustomStream:
		return ui.LibraryStream
	default:
		return ui.LibraryPlaylistURL
	}
}

// libraryDisplayName falls back when a stored source has no name.
func libraryDisplayName(source catalog.Source) string {
	if name := strings.TrimSpace(source.DisplayName); name != "" {
		return name
	}

	return libraryKind(source.Kind).Label()
}
