package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// LibraryKind is the stored kind of a user source.
type LibraryKind int

const (
	// LibraryPlaylistURL is a remote M3U playlist.
	LibraryPlaylistURL LibraryKind = iota + 1
	// LibraryPlaylistFile is an M3U playlist chosen from this Mac.
	LibraryPlaylistFile
	// LibraryStream is a single live stream URL.
	LibraryStream
)

// Label returns the short kind shown in a source row.
func (k LibraryKind) Label() string {
	switch k {
	case LibraryPlaylistURL:
		return "M3U URL"
	case LibraryPlaylistFile:
		return "M3U file"
	case LibraryStream:
		return "Stream"
	default:
		return "Source"
	}
}

// LibrarySection is the tab the Library page shows.
type LibrarySection int

const (
	// LibraryFavorites shows the user's favorite channels.
	LibraryFavorites LibrarySection = iota
	// LibraryRecent shows channels that genuinely played.
	LibraryRecent
	// LibraryPlaylists shows the user's playlists and custom streams.
	LibraryPlaylists
)

// LibraryStateKind is what the Library page shows.
type LibraryStateKind int

const (
	// LibraryLoading reports a pending library read.
	LibraryLoading LibraryStateKind = iota + 1
	// LibraryReady shows the sections, or their empty states.
	LibraryReady
	// LibraryFailed reports that the library could not be read.
	LibraryFailed
)

// LibrarySource is one row of My Playlists.
type LibrarySource struct {
	ID          string
	Name        string
	Kind        LibraryKind
	Enabled     bool
	Channels    int
	LastSuccess time.Time
	Error       string
	// Busy disables this row's actions while its work runs.
	Busy bool
}

// LibraryChannel is one favorite or recently watched row.
//
// Key is the stable source-scoped identity shared with Discover and Watch.
// SourceName keeps two channels with the same display name distinguishable.
type LibraryChannel struct {
	Key           string
	Name          string
	SourceName    string
	Meta          string
	Live          bool
	Favorite      bool
	Playable      bool
	SourceEnabled bool
	PlayedAt      time.Time
}

// LibraryState is what the app publishes to the Library page.
type LibraryState struct {
	Kind      LibraryStateKind
	Favorites []LibraryChannel
	Recent    []LibraryChannel
	Sources   []LibrarySource
	// Message is an inline banner for the whole page.
	Message string
}

// LibraryPreview is a credential-free summary of a validated source.
type LibraryPreview struct {
	Kind         LibraryKind
	Name         string
	Location     string
	Channels     int
	Samples      []string
	Warnings     []string
	WarningCount int
	Skipped      int
	// SkippedReasons counts skipped entries by reason, most frequent first.
	SkippedReasons []SkippedReason
	// SkippedExamples names up to three skipped entries without a URL.
	SkippedExamples []string
	Truncated       bool
	// NeedsPrivate reports streams on private or local addresses, which the
	// sheet can allow explicitly for this source only.
	NeedsPrivate bool
	// Schemes lists the distinct stream schemes the preview found.
	Schemes []string
}

// SkippedReason is one reason a playlist entry was skipped.
type SkippedReason struct {
	Label string
	Count int
}

// LibraryRequest identifies what the page asks the app to validate or import.
type LibraryRequest struct {
	Kind LibraryKind
	URL  string
	Path string
	Name string
	// AllowPrivate carries the sheet's explicit consent for this source.
	AllowPrivate bool
}

// libraryList is one virtualized section: its list state and selection.
type libraryList struct {
	state    ui.ListState
	selected int
}

// clamp keeps the selection inside n rows.
func (l *libraryList) clamp(n int) {
	if l.selected >= n {
		l.selected = -1
	}
}

// current returns the selected index, or -1.
func (l *libraryList) current(n int) int {
	if l.selected < 0 || l.selected >= n {
		return -1
	}

	return l.selected
}

// Library is the native Library page for favorites, recent viewing and user
// sources. The app publishes state; the page dispatches intents and owns no I/O.
type Library struct {
	state   LibraryState
	section int
	query   string

	favs    libraryList
	recent  libraryList
	sources libraryList

	favoriteRows []LibraryChannel
	recentRows   []LibraryChannel

	now func() time.Time

	sheetOpen     bool
	sheetKind     LibraryKind
	sheetNotified bool
	tab           int
	url           string
	path          string
	name          string
	preview       *LibraryPreview
	previewErr    string
	previewBusy   bool
	importBusy    bool
	importErr     string
	pickBusy      bool
	allowPrivate  bool

	confirmOpen bool
	confirmID   string
	confirmName string

	// OnReload re-reads the library.
	OnReload func()
	// OnRefresh re-reads one source's channels.
	OnRefresh func(id string)
	// OnToggle enables or disables one source.
	OnToggle func(id string, enabled bool)
	// OnRemove deletes a confirmed source.
	OnRemove func(id string)
	// OnPickFile opens the native file dialog.
	OnPickFile func()
	// OnPreview validates a draft without storing it.
	OnPreview func(req LibraryRequest)
	// OnImport stores the validated draft.
	OnImport func(req LibraryRequest)
	// OnSheetClose cancels pending preview work.
	OnSheetClose func()
	// OnPlay opens one favorite or recent channel.
	OnPlay func(key string)
	// OnToggleFavorite adds or removes one channel from favorites.
	OnToggleFavorite func(key string)
	// OnBrowse sends the viewer to Discover.
	OnBrowse func()
}

// NewLibrary returns an empty Library page on the Favorites tab.
func NewLibrary() *Library {
	l := &Library{
		state:   LibraryState{Kind: LibraryReady},
		section: int(LibraryFavorites),
		now:     time.Now,
	}
	l.favs.selected, l.recent.selected, l.sources.selected = -1, -1, -1
	l.favs.state.Selected = &l.favs.selected
	l.recent.state.Selected = &l.recent.selected
	l.sources.state.Selected = &l.sources.selected
	l.favs.state.Label = l.favoriteLabel
	l.recent.state.Label = l.recentLabel
	l.sources.state.Label = l.sourceLabel

	return l
}

// SetState shows state.
func (l *Library) SetState(state LibraryState) {
	l.state = state
	l.refreshRows()
}

// State returns what the page shows, for the app and for tests.
func (l *Library) State() LibraryState { return l.state }

// Section returns the visible tab.
func (l *Library) Section() LibrarySection { return LibrarySection(l.section) }

// ShowFavorites opens the Favorites tab.
func (l *Library) ShowFavorites() { l.section = int(LibraryFavorites) }

// ShowPlaylists opens the Playlists tab.
func (l *Library) ShowPlaylists() { l.section = int(LibraryPlaylists) }

// SheetOpen reports whether the import sheet is open.
func (l *Library) SheetOpen() bool { return l.sheetOpen }

// Preview returns the validated draft, or nil before one succeeds.
func (l *Library) Preview() *LibraryPreview { return l.preview }

// PreviewError returns the inline validation error, if any.
func (l *Library) PreviewError() string { return l.previewErr }

// OpenAddPlaylist opens the sheet on the M3U URL tab.
func (l *Library) OpenAddPlaylist() {
	l.ShowPlaylists()
	l.openSheet(LibraryPlaylistURL, 0)
}

// OpenImportFile opens the sheet on the M3U file tab.
func (l *Library) OpenImportFile() {
	l.ShowPlaylists()
	l.openSheet(LibraryPlaylistURL, 1)
}

// OpenAddStream opens the custom stream sheet.
func (l *Library) OpenAddStream() {
	l.ShowPlaylists()
	l.openSheet(LibraryStream, 0)
}

// SetFilePath shows the file chosen in the native dialog; an empty path
// only clears the picker's busy state.
func (l *Library) SetFilePath(path string) {
	l.pickBusy = false
	if path == "" {
		return
	}
	l.path = path
	l.resetPreview()
}

// SetFilePathError reports a file dialog that could not open.
func (l *Library) SetFilePathError(message string) {
	l.pickBusy = false
	l.previewErr = message
}

// SetPreview shows the validated draft, or an inline error.
func (l *Library) SetPreview(preview *LibraryPreview, errText string) {
	l.previewBusy = false
	l.preview = preview
	l.previewErr = errText
	l.importErr = ""
	// Consent is per draft and starts off for every new preview.
	l.allowPrivate = false
}

// SetImportError reports a failed import and lets the viewer retry.
func (l *Library) SetImportError(message string) {
	l.importBusy = false
	l.importErr = message
}

// ImportFinished closes the sheet after the source was saved.
func (l *Library) ImportFinished() {
	l.importBusy = false
	l.closeSheet()
}

// View draws the page on the MyGo main thread.
func (l *Library) View(c *ui.Context, theme *ui.Theme) {
	l.header(c, theme)
	l.body(c, theme)
	l.sheet(c, theme)
	l.confirm(c, theme)
}

// body draws the state, the tabs and the visible section.
func (l *Library) body(c *ui.Context, theme *ui.Theme) {
	switch l.state.Kind {
	case LibraryLoading:
		pageStatus(c, Status{
			Kind:    StatusLoading,
			Title:   "Loading library",
			Message: "Reading favorites, recent channels and playlists on this Mac.",
		})

		return
	case LibraryFailed:
		message := l.state.Message
		if message == "" {
			message = "mumu could not read the saved library. Retry, or add a playlist."
		}
		pageStatus(c, Status{
			Kind:     StatusError,
			Title:    "Library unavailable",
			Message:  message,
			Action:   "Retry",
			OnAction: l.reload,
		})

		return
	}

	l.tabs(c)

	switch LibrarySection(l.section) {
	case LibraryRecent:
		l.channelSection(c, theme, l.recentRows, &l.recent, recentSection)
	case LibraryPlaylists:
		l.playlists(c, theme)
	default:
		l.channelSection(c, theme, l.favoriteRows, &l.favs, favoritesSection)
	}
}

// header draws the page heading and Add Playlist.
func (l *Library) header(c *ui.Context, _ *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
		pageHeading(c, "Library").Grow(1)
		ui.Button(c, "Add Playlist").OnClick(l.OpenAddPlaylist)
	})
}

// tabs draws the three Library sections.
func (l *Library) tabs(c *ui.Context) {
	section := l.section
	if ui.Tabs(c, &section, "Favorites", "Recently watched", "Playlists").Changed() {
		l.section = section
	}
}

// sectionMode names the copy a channel section shows.
type sectionMode int

const (
	favoritesSection sectionMode = iota
	recentSection
)

// channelSection draws the search field, the banner and the channel rows.
func (l *Library) channelSection(c *ui.Context, theme *ui.Theme, rows []LibraryChannel, list *libraryList, mode sectionMode) {
	label := "Search favorites"
	if mode == recentSection {
		label = "Search recently watched"
	}
	if ui.SearchField(c, &l.query).Label(label).Changed() {
		l.refreshRows()
	}

	if l.state.Message != "" {
		l.messageBanner(c, theme)
	}

	if len(rows) == 0 {
		l.emptyChannels(c, mode)

		return
	}

	listElement := ui.List(c, &list.state, len(rows), func(i int) {
		l.channelRow(c, theme, rows[i], mode)
	}).
		Grow(1).
		Label(l.sectionTitle(mode)).
		ItemKey(func(i int) any { return rows[i].Key })

	if listElement.Submitted() {
		if index := list.current(len(rows)); index >= 0 {
			l.play(rows[index])
		}
	}
}

// emptyChannels draws the first-run state for one channel section.
func (l *Library) emptyChannels(c *ui.Context, mode sectionMode) {
	if l.query != "" {
		pageStatus(c, Status{
			Kind:    StatusEmpty,
			Title:   "No matches",
			Message: "No saved channel matches your search.",
			Action:  "Clear search",
			OnAction: func() {
				l.query = ""
				l.refreshRows()
			},
		})

		return
	}

	title, message := "No favorites yet", "Choose a channel in Discover or Watch, then Add to Favorites."
	if mode == recentSection {
		title, message = "Nothing watched yet", "Channels appear here after playback really starts."
	}

	pageStatus(c, Status{
		Kind:     StatusEmpty,
		Title:    title,
		Message:  message,
		Action:   "Browse Discover",
		OnAction: l.browse,
	})
}

// channelRow draws one favorite or recent channel.
func (l *Library) channelRow(c *ui.Context, theme *ui.Theme, channel LibraryChannel, _ sectionMode) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 10).Children(func() {
		ui.Avatar(c, channel.Name, nil)

		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, channel.Name).FontSize(theme.Rem(1.08)).FontWeight(500).SingleLine()
			if meta := l.channelMeta(channel); meta != "" {
				ui.Text(c, meta).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
			}
			if reason := l.unavailableReason(channel); reason != "" {
				ui.Text(c, reason).FontSize(theme.Rem(0.85)).TextColor(theme.Warning)
			}
		})

		if channel.Live {
			ui.Text(c, "LIVE").FontSize(theme.Rem(0.8)).FontWeight(600).
				TextColor(theme.TextMuted).Padding(2, 7).Radius(4).
				Background(theme.Surface).Border(1, theme.Border)
		}

		label := "Add to Favorites"
		if channel.Favorite {
			label = "Remove from Favorites"
		}
		ui.Button(c, label).AccessSelected(channel.Favorite).OnClick(func() {
			l.toggleFavorite(channel.Key)
		})
	})
}

// playlists draws the message banner, the source rows and Add actions.
func (l *Library) playlists(c *ui.Context, theme *ui.Theme) {
	if l.state.Message != "" {
		l.messageBanner(c, theme)
	}

	if len(l.state.Sources) == 0 {
		l.emptyPlaylists(c, theme)

		return
	}

	ui.List(c, &l.sources.state, len(l.state.Sources), func(i int) {
		l.sourceRow(c, theme, i)
	}).
		Grow(1).
		Label("Playlists").
		ItemKey(func(i int) any { return l.state.Sources[i].ID })

	ui.Row(c).Gap(8).Wrap().Children(func() {
		ui.Button(c, "Add M3U URL").OnClick(l.OpenAddPlaylist)
		ui.Button(c, "Import M3U file").OnClick(l.OpenImportFile)
		ui.Button(c, "Add Stream").OnClick(l.OpenAddStream)
	})
}

// emptyPlaylists draws the first-run state with every import path.
func (l *Library) emptyPlaylists(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Padding(24).Children(func() {
		ui.Text(c, "No playlists yet").FontSize(theme.Rem(1.7)).FontWeight(500).TextAlign(ui.Center)
		ui.Text(c, "Add an M3U playlist from a URL or a file, or add a single stream.").
			TextColor(theme.TextMuted).TextAlign(ui.Center).MaxWidth(420)

		ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Children(func() {
			ui.PrimaryButton(c, "Add Playlist").OnClick(l.OpenAddPlaylist)
			ui.Button(c, "Import M3U file").OnClick(l.OpenImportFile)
			ui.Button(c, "Add Stream").OnClick(l.OpenAddStream)
		})
	})
}

// messageBanner shows a page-level problem without hiding the rows.
func (l *Library) messageBanner(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 12).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Text(c, "!").TextColor(theme.Warning).AccessHidden(true)
		ui.Text(c, l.state.Message).TextColor(theme.TextMuted).Grow(1)
	})
}

// sourceRow draws one source with its state and actions.
func (l *Library) sourceRow(c *ui.Context, theme *ui.Theme, i int) {
	source := l.state.Sources[i]

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(10, 12).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				name := ui.Text(c, source.Name).FontSize(theme.Rem(1.08)).FontWeight(500).SingleLine()
				if !source.Enabled {
					name.TextColor(theme.TextMuted)
				}
				ui.Text(c, source.Kind.Label()).FontSize(theme.Rem(0.8)).TextColor(theme.TextMuted)
				if !source.Enabled {
					ui.Text(c, "Disabled").FontSize(theme.Rem(0.8)).TextColor(theme.TextMuted)
				}
			})
			ui.Text(c, l.sourceMeta(source)).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
			if source.Error != "" {
				ui.Text(c, source.Error).FontSize(theme.Rem(0.85)).TextColor(theme.Danger)
			}
		})

		if source.Busy {
			ui.Spinner(c).Size(16, 16).Label("Working on " + source.Name)
		}

		refresh := ui.Button(c, "Refresh").OnClick(func() {
			if l.OnRefresh != nil {
				l.OnRefresh(source.ID)
			}
		})
		if source.Busy {
			refresh.Disabled(true)
		}

		remove := ui.Button(c, "Remove…").OnClick(func() {
			l.confirmID, l.confirmName = source.ID, source.Name
			l.confirmOpen = true
		})
		if source.Busy {
			remove.Disabled(true)
		}

		enabled := source.Enabled
		toggle := ui.Switch(c, &enabled).Label("Enable " + source.Name)
		if source.Busy {
			toggle = toggle.Disabled(true)
		}
		if toggle.Changed() && l.OnToggle != nil {
			l.OnToggle(source.ID, enabled)
		}
	})
}

// sourceMeta builds the count and last-success line.
func (l *Library) sourceMeta(source LibrarySource) string {
	parts := []string{l.channelCount(source.Channels), l.relative(source.LastSuccess)}
	if source.Error != "" && source.Channels > 0 {
		parts = append(parts, "showing last known channels")
	}

	return strings.Join(parts, " · ")
}

// refreshRows re-filters both channel sections and clamps their selection.
func (l *Library) refreshRows() {
	l.favoriteRows = l.filterChannels(l.state.Favorites)
	l.recentRows = l.filterChannels(l.state.Recent)
	l.favs.clamp(len(l.favoriteRows))
	l.recent.clamp(len(l.recentRows))
	l.sources.clamp(len(l.state.Sources))
}

// filterChannels narrows a section by the search query.
func (l *Library) filterChannels(channels []LibraryChannel) []LibraryChannel {
	query := strings.ToLower(strings.TrimSpace(l.query))
	if query == "" {
		return channels
	}

	rows := make([]LibraryChannel, 0, len(channels))
	for _, channel := range channels {
		if strings.Contains(strings.ToLower(channel.Name), query) ||
			strings.Contains(strings.ToLower(channel.SourceName), query) {
			rows = append(rows, channel)
		}
	}

	return rows
}

// sectionTitle names a channel section for assistive technology.
func (l *Library) sectionTitle(mode sectionMode) string {
	if mode == recentSection {
		return "Recently watched"
	}

	return "Favorites"
}

// favoriteLabel names a favorite row for assistive technology.
func (l *Library) favoriteLabel(i int) string {
	return l.channelLabel(l.favoriteRows, i)
}

// recentLabel names a recent row for assistive technology.
func (l *Library) recentLabel(i int) string {
	return l.channelLabel(l.recentRows, i)
}

// channelLabel builds a channel row's accessible name.
func (l *Library) channelLabel(rows []LibraryChannel, i int) string {
	if i < 0 || i >= len(rows) {
		return ""
	}

	parts := []string{rows[i].Name}
	if meta := l.channelMeta(rows[i]); meta != "" {
		parts = append(parts, meta)
	}
	if reason := l.unavailableReason(rows[i]); reason != "" {
		parts = append(parts, reason)
	}

	return strings.Join(parts, ", ")
}

// sourceLabel names a source row for assistive technology and type-select.
func (l *Library) sourceLabel(i int) string {
	if i < 0 || i >= len(l.state.Sources) {
		return ""
	}

	source := l.state.Sources[i]
	state := "enabled"
	if !source.Enabled {
		state = "disabled"
	}

	return strings.Join([]string{
		source.Name,
		source.Kind.Label(),
		state,
		l.channelCount(source.Channels),
	}, ", ")
}

// channelMeta builds "Category · Source · watched 2h ago" from a row.
func (l *Library) channelMeta(channel LibraryChannel) string {
	parts := make([]string, 0, 3)
	if meta := strings.TrimSpace(channel.Meta); meta != "" {
		parts = append(parts, meta)
	}
	if source := strings.TrimSpace(channel.SourceName); source != "" {
		parts = append(parts, source)
	}
	if !channel.PlayedAt.IsZero() {
		parts = append(parts, "watched "+l.relative(channel.PlayedAt))
	}

	return strings.Join(parts, " · ")
}

// unavailableReason explains why a row cannot play, empty when it can.
func (l *Library) unavailableReason(channel LibraryChannel) string {
	switch {
	case !channel.SourceEnabled:
		return "Source disabled. Enable it in Playlists."
	case !channel.Playable:
		return "No supported stream."
	default:
		return ""
	}
}

// toggleFavorite dispatches the favorite intent.
func (l *Library) toggleFavorite(key string) {
	if l.OnToggleFavorite != nil {
		l.OnToggleFavorite(key)
	}
}

// play dispatches the open intent.
func (l *Library) play(channel LibraryChannel) {
	if l.OnPlay != nil {
		l.OnPlay(channel.Key)
	}
}

// browse dispatches the Discover intent.
func (l *Library) browse() {
	if l.OnBrowse != nil {
		l.OnBrowse()
	}
}

// reload dispatches a library reload.
func (l *Library) reload() {
	if l.OnReload != nil {
		l.OnReload()
	}
}

// sheet draws the import sheet while it is open.
func (l *Library) sheet(c *ui.Context, theme *ui.Theme) {
	if !l.sheetOpen {
		return
	}

	panel := ui.DialogBase(c, &l.sheetOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(520).MaxWidthPercent(92).Padding(20).Gap(12).Radius(12).
			Background(theme.Background).Border(1, theme.Border).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.35)).Label(l.sheetTitle())
		ui.Scroll(c).MaxHeight(460).Gap(12).Children(func() {
			l.sheetBody(c, theme)
		})
	})
	if panel.Valid() && panel.Dismissed() {
		l.closeSheet()
	}
}

// sheetTitle names the open sheet.
func (l *Library) sheetTitle() string {
	if l.sheetKind == LibraryStream {
		return "Add Stream"
	}

	return "Add Playlist"
}

// sheetBody builds the fields, preview and actions of the sheet.
func (l *Library) sheetBody(c *ui.Context, theme *ui.Theme) {
	ui.Text(c, l.sheetTitle()).FontSize(theme.Rem(1.4)).FontWeight(600)

	if l.sheetKind == LibraryStream {
		ui.Field(c, "Stream URL", func() {
			if ui.TextInput(c, &l.url).Placeholder("https://example.com/live.m3u8").
				AutoFocus().Changed() {
				l.resetPreview()
			}
		})
	} else {
		if ui.Segmented(c, &l.tab, "M3U URL", "M3U file").Label("Source type").Changed() {
			l.resetPreview()
		}

		if l.tab == 0 {
			ui.Field(c, "Playlist URL", func() {
				if ui.TextInput(c, &l.url).Placeholder("https://example.com/list.m3u").
					AutoFocus().Changed() {
					l.resetPreview()
				}
			})
		} else {
			ui.Field(c, "Playlist file", func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &l.path).ReadOnly(true).Placeholder("No file chosen")
					choose := ui.Button(c, "Choose File…").OnClick(func() {
						if l.pickBusy {
							return
						}
						l.pickBusy = true
						if l.OnPickFile != nil {
							l.OnPickFile()
						}
					})
					if l.pickBusy {
						choose.Disabled(true)
					}
				})
			})
		}
	}

	ui.Field(c, "Display name", func() {
		if ui.TextInput(c, &l.name).Placeholder("Optional").Changed() {
			l.resetPreview()
		}
	})

	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		validate := ui.Button(c, "Validate and Preview").OnClick(l.validate)
		if l.previewBusy || l.importBusy {
			validate.Disabled(true)
		}
		if l.previewBusy {
			ui.Spinner(c).Size(14, 14).Label("Validating")
			ui.Text(c, "Checking the source…").TextColor(theme.TextMuted)
		} else if l.pickBusy {
			ui.Text(c, "Waiting for the file dialog…").TextColor(theme.TextMuted)
		}
	})

	if l.previewErr != "" {
		ui.Text(c, l.previewErr).TextColor(theme.Danger)
	}
	if l.preview != nil {
		l.previewPanel(c, theme)
		l.privateConsent(c, theme)
	}
	if l.importErr != "" {
		ui.Text(c, l.importErr).TextColor(theme.Danger)
	}

	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		ui.Button(c, "Cancel").OnClick(l.closeSheet)

		importButton := ui.PrimaryButton(c, "Import").OnClick(l.importDraft)
		if l.preview == nil || l.previewBusy || l.importBusy {
			importButton.Disabled(true)
		}
	})
}

// previewPanel summarizes a validated draft.
func (l *Library) previewPanel(c *ui.Context, theme *ui.Theme) {
	preview := l.preview
	kind := "M3U playlist"
	if preview.Kind == LibraryStream {
		kind = "Single stream"
	}

	ui.Column(c).Gap(3).Padding(10, 12).Background(theme.Surface).Radius(6).
		Border(1, theme.Border).Children(func() {
		ui.Textf(c, "%s · %s", kind, l.channelCount(preview.Channels)).FontWeight(500)
		if preview.Name != "" {
			ui.Text(c, preview.Name).TextColor(theme.TextMuted)
		}
		if preview.Location != "" {
			ui.Text(c, preview.Location).FontSize(theme.Rem(0.85)).
				TextColor(theme.TextMuted).SingleLine()
		}
		if len(preview.Samples) > 0 {
			ui.Text(c, "Sample: "+strings.Join(preview.Samples, ", ")).
				FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted)
		}
		if preview.Skipped > 0 {
			ui.Textf(c, "Skipped entries · %d", preview.Skipped).FontWeight(500)
			for _, reason := range preview.SkippedReasons {
				ui.Textf(c, "%d %s", reason.Count, reason.Label).
					FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted)
			}
			for i, example := range preview.SkippedExamples {
				if i == 3 {
					break
				}
				if example == "" {
					continue
				}
				ui.Text(c, "e.g. "+example).
					FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
			}
		}
		for _, warning := range preview.Warnings {
			ui.Text(c, warning).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted)
		}
		if more := preview.WarningCount - len(preview.Warnings); more > 0 {
			ui.Textf(c, "and %d more warnings", more).
				FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted)
		}
		if preview.Truncated {
			ui.Text(c, "The list was truncated at the channel limit.").
				FontSize(theme.Rem(0.85)).TextColor(theme.Warning)
		}
	})
}

// privateConsent asks for explicit permission before storing private hosts.
func (l *Library) privateConsent(c *ui.Context, theme *ui.Theme) {
	if l.preview == nil || !l.preview.NeedsPrivate {
		return
	}

	ui.Column(c).Gap(4).Padding(8, 10).Background(theme.Surface).Radius(6).
		Border(1, theme.Border).Children(func() {
		ui.Checkbox(c, &l.allowPrivate, "Allow private and LAN addresses for this source")
		ui.Text(c, "Enable this only for a source you trust. mumu keeps the choice on this Mac.").
			FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted)
		if schemes := strings.Join(l.preview.Schemes, ", "); schemes != "" {
			ui.Textf(c, "Stream schemes found · %s", schemes).
				FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted)
		}
	})
}

// confirm draws the remove confirmation, naming the source.
func (l *Library) confirm(c *ui.Context, _ *ui.Theme) {
	if !l.confirmOpen {
		return
	}

	message := fmt.Sprintf(
		"Removing “%s” also removes its channels, favorites and recent entries from this Mac.",
		l.confirmName,
	)
	switch ui.AlertDialog(c, &l.confirmOpen, "Remove “"+l.confirmName+"”?", message, "Cancel", "Remove") {
	case 1:
		id := l.confirmID
		l.confirmID, l.confirmName = "", ""
		if l.OnRemove != nil {
			l.OnRemove(id)
		}
	case 0, ui.AlertDismissed:
		l.confirmID, l.confirmName = "", ""
	}
}

// openSheet resets the draft and opens the sheet.
func (l *Library) openSheet(kind LibraryKind, tab int) {
	if l.sheetOpen {
		l.closeSheet()
	}

	l.sheetKind = kind
	l.tab = tab
	l.clearDraft()
	l.sheetOpen = true
	l.sheetNotified = false
}

// closeSheet resets the draft and tells the app to cancel pending work once.
// DialogBase closes the sheet itself on Escape, before this runs.
func (l *Library) closeSheet() {
	if !l.sheetOpen && l.sheetNotified {
		return
	}

	l.sheetOpen = false
	l.clearDraft()

	if l.sheetNotified {
		return
	}
	l.sheetNotified = true

	if l.OnSheetClose != nil {
		l.OnSheetClose()
	}
}

// clearDraft resets the sheet's fields and transient work flags.
func (l *Library) clearDraft() {
	l.url, l.path, l.name = "", "", ""
	l.preview = nil
	l.previewErr = ""
	l.importErr = ""
	l.previewBusy = false
	l.importBusy = false
	l.pickBusy = false
	l.allowPrivate = false
}

// resetPreview drops a preview that no longer matches the draft.
func (l *Library) resetPreview() {
	l.preview = nil
	l.previewErr = ""
	l.importErr = ""
	l.allowPrivate = false
}

// validate dispatches the draft for validation.
func (l *Library) validate() {
	if l.previewBusy || l.importBusy {
		return
	}
	l.preview = nil
	l.previewErr = ""
	l.importErr = ""
	l.previewBusy = true
	if l.OnPreview != nil {
		l.OnPreview(l.request())
	}
}

// importDraft dispatches the validated draft for saving.
func (l *Library) importDraft() {
	if l.preview == nil || l.previewBusy || l.importBusy {
		return
	}
	l.importBusy = true
	l.importErr = ""
	if l.OnImport != nil {
		l.OnImport(l.request())
	}
}

// request returns the draft the sheet describes.
func (l *Library) request() LibraryRequest {
	switch {
	case l.sheetKind == LibraryStream:
		return LibraryRequest{
			Kind:         LibraryStream,
			URL:          strings.TrimSpace(l.url),
			Name:         strings.TrimSpace(l.name),
			AllowPrivate: l.allowPrivate,
		}
	case l.tab == 1:
		return LibraryRequest{
			Kind:         LibraryPlaylistFile,
			Path:         strings.TrimSpace(l.path),
			Name:         strings.TrimSpace(l.name),
			AllowPrivate: l.allowPrivate,
		}
	default:
		return LibraryRequest{
			Kind:         LibraryPlaylistURL,
			URL:          strings.TrimSpace(l.url),
			Name:         strings.TrimSpace(l.name),
			AllowPrivate: l.allowPrivate,
		}
	}
}

// channelCount names a number of channels.
func (l *Library) channelCount(n int) string {
	if n == 1 {
		return "1 channel"
	}

	return fmt.Sprintf("%d channels", n)
}

// relative names when something happened.
func (l *Library) relative(at time.Time) string {
	if at.IsZero() {
		return "not refreshed yet"
	}

	now := l.now()
	if now.Before(at) {
		now = at
	}
	age := now.Sub(at)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	default:
		return "on " + at.Format("2 Jan 2006")
	}
}
