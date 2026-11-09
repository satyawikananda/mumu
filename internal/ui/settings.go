package ui

import "github.com/egoist/mygo/ui"

// Settings is the Settings page's catalog controls. The app publishes the
// stored setting; the page dispatches intents and owns no I/O.
type Settings struct {
	community    bool
	communityErr string

	// OnCommunity asks the app to show or hide the community catalog.
	OnCommunity func(enabled bool)
	// OnManagePlaylists sends the viewer to the Library.
	OnManagePlaylists func()
}

// NewSettings returns the default catalog settings: the community list is on,
// matching the service's default.
func NewSettings() *Settings {
	return &Settings{community: true}
}

// SetCommunity shows the stored setting and clears a previous failure.
func (s *Settings) SetCommunity(enabled bool) {
	s.community = enabled
	s.communityErr = ""
}

// SetCommunityError shows an inline failure under the switch.
func (s *Settings) SetCommunityError(message string) {
	s.communityErr = message
}

// Community returns the setting the page shows.
func (s *Settings) Community() bool { return s.community }

// CommunityError returns the inline failure, empty for none.
func (s *Settings) CommunityError() string { return s.communityErr }

// Sources draws the community catalog setting, its attribution and the
// playlists shortcut.
func (s *Settings) Sources(c *ui.Context, theme *ui.Theme) {
	ui.Text(c, "Sources").FontSize(theme.Rem(1.4)).FontWeight(600)

	ui.Column(c).Gap(8).Children(func() {
		enabled := s.community
		choice := ui.Checkbox(c, &enabled, "Show community catalog")
		if choice.Changed() {
			// Optimistic: the app reverts when the store refuses.
			s.community = enabled
			s.communityErr = ""
			if s.OnCommunity != nil {
				s.OnCommunity(enabled)
			}
		}

		if s.communityErr != "" {
			ui.Text(c, s.communityErr).FontSize(theme.Rem(0.9)).TextColor(theme.Danger)
		}

		ui.Text(c, attributionText).
			FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).MaxWidth(620)

		ui.Button(c, "Manage playlists").OnClick(func() {
			if s.OnManagePlaylists != nil {
				s.OnManagePlaylists()
			}
		})
	})
}

// attributionText credits the community catalog without implying rights.
const attributionText = "Channel data from iptv-org (CC0). Streams belong to their owners. Not verified by mumu."
