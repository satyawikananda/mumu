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

// LibraryStateKind is what the Library page shows.
type LibraryStateKind int

const (
	// LibraryLoading reports a pending source list.
	LibraryLoading LibraryStateKind = iota + 1
	// LibraryReady shows the source rows, or the empty state.
	LibraryReady
	// LibraryFailed reports that the source list could not be read.
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

// LibraryState is what the app publishes to the Library page.
type LibraryState struct {
	Kind    LibraryStateKind
	Sources []LibrarySource
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

// Library is the native Library page for playlists and custom streams. The
// app publishes state; the page dispatches intents and owns no I/O.
type Library struct {
	state    LibraryState
	list     ui.ListState
	selected int
	now      func() time.Time

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

	// OnReload re-reads the source list.
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
}

// NewLibrary returns an empty Library page.
func NewLibrary() *Library {
	l := &Library{
		state:    LibraryState{Kind: LibraryReady},
		selected: -1,
		now:      time.Now,
	}
	l.list.Selected = &l.selected
	l.list.Label = l.rowLabel

	return l
}

// SetState shows state.
func (l *Library) SetState(state LibraryState) {
	l.state = state
	if l.selected >= len(state.Sources) {
		l.selected = -1
	}
}

// State returns what the page shows, for the app and for tests.
func (l *Library) State() LibraryState { return l.state }

// SheetOpen reports whether the import sheet is open.
func (l *Library) SheetOpen() bool { return l.sheetOpen }

// Preview returns the validated draft, or nil before one succeeds.
func (l *Library) Preview() *LibraryPreview { return l.preview }

// PreviewError returns the inline validation error, if any.
func (l *Library) PreviewError() string { return l.previewErr }

// OpenAddPlaylist opens the sheet on the M3U URL tab.
func (l *Library) OpenAddPlaylist() {
	l.openSheet(LibraryPlaylistURL, 0)
}

// OpenImportFile opens the sheet on the M3U file tab.
func (l *Library) OpenImportFile() {
	l.openSheet(LibraryPlaylistURL, 1)
}

// OpenAddStream opens the custom stream sheet.
func (l *Library) OpenAddStream() {
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

// header draws the page heading and Add Playlist.
func (l *Library) header(c *ui.Context, _ *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
		pageHeading(c, "Library").Grow(1)
		ui.Button(c, "Add Playlist").OnClick(l.OpenAddPlaylist)
	})
}

// body draws the state, the section heading and the source rows.
func (l *Library) body(c *ui.Context, theme *ui.Theme) {
	switch l.state.Kind {
	case LibraryLoading:
		pageStatus(c, Status{
			Kind:    StatusLoading,
			Title:   "Loading playlists",
			Message: "Reading the sources saved on this Mac.",
		})

		return
	case LibraryFailed:
		message := l.state.Message
		if message == "" {
			message = "mumu could not read the saved playlists. Retry, or add a playlist."
		}
		pageStatus(c, Status{
			Kind:     StatusError,
			Title:    "Playlists unavailable",
			Message:  message,
			Action:   "Retry",
			OnAction: l.reload,
		})

		return
	}

	ui.Text(c, "My Playlists").FontSize(theme.Rem(1.4)).FontWeight(600)

	if l.state.Message != "" {
		l.messageBanner(c, theme)
	}

	if len(l.state.Sources) == 0 {
		l.empty(c, theme)
		return
	}

	l.listView(c, theme)
}

// messageBanner shows a page-level problem without hiding the rows.
func (l *Library) messageBanner(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 12).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Text(c, "!").TextColor(theme.Warning).AccessHidden(true)
		ui.Text(c, l.state.Message).TextColor(theme.TextMuted).Grow(1)
	})
}

// empty draws the first-run state with every import path.
func (l *Library) empty(c *ui.Context, theme *ui.Theme) {
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

// listView draws the virtualized source rows.
func (l *Library) listView(c *ui.Context, theme *ui.Theme) {
	ui.List(c, &l.list, len(l.state.Sources), func(i int) {
		l.row(c, theme, i)
	}).
		Grow(1).
		Label("Playlists").
		ItemKey(func(i int) any { return l.state.Sources[i].ID })
}

// row draws one source with its state and actions.
func (l *Library) row(c *ui.Context, theme *ui.Theme, i int) {
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
			ui.Text(c, l.rowMeta(source)).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
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

// rowMeta builds the count and last-success line.
func (l *Library) rowMeta(source LibrarySource) string {
	parts := []string{l.channelCount(source.Channels), l.relative(source.LastSuccess)}
	if source.Error != "" && source.Channels > 0 {
		parts = append(parts, "showing last known channels")
	}

	return strings.Join(parts, " · ")
}

// rowLabel names a row for assistive technology and type-select.
func (l *Library) rowLabel(i int) string {
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

// reload dispatches a list reload.
func (l *Library) reload() {
	if l.OnReload != nil {
		l.OnReload()
	}
}

// channelCount names a number of channels.
func (l *Library) channelCount(n int) string {
	if n == 1 {
		return "1 channel"
	}

	return fmt.Sprintf("%d channels", n)
}

// relative names when a source last refreshed.
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
