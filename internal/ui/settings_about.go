package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// shortcuts draws the read-only command reference from the one registry.
func (s *Settings) shortcuts(c *ui.Context, theme *ui.Theme, panelWidth float32) {
	s.panelHead(c, theme, "Shortcuts",
		"Every key mumu listens to. Fixed in this build, and they follow your layout when the system remaps keys.")

	shortcutReference(c, theme, panelWidth)

	ui.Text(c, "Remapping isn't in this build. The same keys work whether you're on the home feed or inside a stream.").
		Font(bodyFont()).FontSize(textSize(theme, 12)).TextColor(theme.TextMuted).
		MaxWidth(720)
}

// aboutPanel draws the real build, license and update information.
func (s *Settings) aboutPanel(c *ui.Context, theme *ui.Theme) {
	s.aboutHero(c, theme)
	s.aboutActions(c, theme)

	settingsGroup(c, theme, "ABOUT", func() {
		s.valueRow(c, theme, "settings-about-version", "Version",
			s.versionLabel(true), nil, nil)
		s.valueRow(c, theme, "settings-about-catalog", "Channel catalog",
			s.catalogLabel(), nil, nil)
		s.valueRow(c, theme, "settings-about-license", "License",
			s.licenseLabel(), nil, nil)
		s.valueRow(c, theme, "settings-about-repository", "Repository",
			repositoryLabel(s.about.RepositoryURL), icons().external,
			func() {
				if s.about.RepositoryURL != "" {
					c.OpenURL(s.about.RepositoryURL)
				}
			})
	})

	settingsGroup(c, theme, "CREDITS", func() {
		s.valueRow(c, theme, "settings-credits-channel-data", "Channel data",
			"iptv-org (CC0)", nil, nil)
		s.valueRow(c, theme, "settings-credits-icons", "Icons",
			"Lucide (ISC)", nil, nil)
		s.valueRow(c, theme, "settings-credits-fonts", "Fonts",
			"Lexend · Inter Tight (OFL 1.1)", nil, nil)
		s.valueRow(c, theme, "settings-credits-toolkit", "UI toolkit",
			myGoLabel(s.about.MyGoVersion), nil, nil)
	})

	settingsGroup(c, theme, "SYSTEM", func() {
		s.valueRow(c, theme, "settings-system-runtime", "Runtime",
			runtimeLabel(s.about), nil, nil)
		s.valueRow(c, theme, "settings-system-display", "Display",
			displayLabel(c), nil, nil)
	})

	settingsError(c, theme, s.aboutErr)
}

// aboutHero draws the product identity and actual build line.
func (s *Settings) aboutHero(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Children(func() {
		if s.mark != nil {
			ui.Image(c, s.mark).Size(64, 64).Fit(ui.Contain).AccessHidden(true)
		}
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, "mumu").Font(displayFont()).FontSize(textSize(theme, 41)).
				FontWeight(700).TextColor(theme.Text).Role(ui.RoleHeading).Level(1)
			ui.Text(c, "A native desktop TV player for live channels you choose.").
				Font(bodyFont()).FontSize(textSize(theme, 13)).TextColor(theme.TextMuted)
			ui.Text(c, s.versionLabel(true)).Font(bodyFont()).
				FontSize(textSize(theme, 12)).TextColor(theme.TextMuted)
		})
	})
}

// aboutActions draws the manual update check and clipboard action.
func (s *Settings) aboutActions(c *ui.Context, theme *ui.Theme) {
	checking := s.update.Kind == UpdateChecking
	checkLabel := "Check for updates"
	switch s.update.Kind {
	case UpdateChecking:
		checkLabel = "Checking…"
	case UpdateAvailable, UpdateCurrent, UpdateNoRelease:
		checkLabel = "Check again"
	case UpdateError:
		checkLabel = "Try again"
	}

	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
		check := settingsActionButton(c, theme, "settings-check-updates",
			icons().refresh, checkLabel, checking || s.OnCheckUpdates == nil, false)
		if check.Clicked() {
			s.update = UpdateState{Kind: UpdateChecking, Current: s.update.Current}
			s.copied = false
			if s.OnCheckUpdates != nil {
				s.OnCheckUpdates()
			}
		}

		copyButton := settingsActionButton(c, theme, "settings-copy-info",
			icons().copy, "Copy system info", false, false)
		if copyButton.Clicked() {
			info := s.SystemInfo(displayLabel(c))
			if info != "" {
				c.Services().WriteClipboard(info)
				s.copied = true
			}
		}

		if s.update.Kind == UpdateAvailable && s.update.ReleaseURL != "" {
			open := settingsActionButton(c, theme, "settings-open-release",
				icons().external, "Open release page", false, true)
			if open.Clicked() {
				c.OpenURL(s.update.ReleaseURL)
			}
		}
	})

	status := s.updateStatus()
	if status != "" {
		ui.Text(c, status).Font(bodyFont()).FontSize(textSize(theme, 12)).
			TextColor(updateTone(theme, s.update.Kind)).MaxWidth(720)
	}
	if s.copied {
		ui.Text(c, "System info copied to the clipboard.").Font(bodyFont()).
			FontSize(textSize(theme, 12)).TextColor(theme.Success)
	}
}

// settingsActionButton draws one compact native action button.
func settingsActionButton(
	c *ui.Context,
	theme *ui.Theme,
	key string,
	icon *ui.SVG,
	label string,
	disabled, primary bool,
) ui.Element {
	border := theme.Border
	background := theme.Surface
	foreground := theme.Text
	if primary {
		border = theme.Accent
		background = theme.Accent
		foreground = theme.AccentText
	}

	button := ui.ButtonBase(c.Key(key)).Height(34).Padding(8, 12).Radius(8).
		Gap(7).Border(1, border).Background(background).FocusRingOffset(-2).
		Label(label).Disabled(disabled)
	if disabled {
		button = button.Opacity(0.55)
	} else if button.Hovered() {
		if primary {
			button = button.Background(theme.AccentHover)
		} else {
			button = button.Background(theme.SurfaceHover)
		}
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(14, 14).TextColor(foreground).AccessHidden(true)
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 12)).
			FontWeight(600).TextColor(foreground).SingleLine()
	})

	return button
}

// valueRow draws one About label and value, optionally as an action.
func (s *Settings) valueRow(
	c *ui.Context,
	theme *ui.Theme,
	key, label, value string,
	icon *ui.SVG,
	action func(),
) {
	if action == nil {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).
			Padding(10, 14).Children(func() {
			ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 13)).
				TextColor(theme.Text).Grow(1).SingleLine()
			ui.Text(c, value).Font(bodyFont()).FontSize(textSize(theme, 12)).
				TextColor(theme.TextMuted).SingleLine()
		})

		return
	}

	row := ui.ButtonBase(c.Key(key)).FillWidth().AlignItems(ui.Center).Gap(16).
		Padding(10, 14).FocusRingOffset(-2).Label(label + ": " + value)
	if row.Hovered() {
		row = row.Background(theme.SurfaceHover)
	}
	row.Children(func() {
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 13)).
			TextColor(theme.Text).Grow(1).SingleLine()
		ui.Text(c, value).Font(bodyFont()).FontSize(textSize(theme, 12)).
			TextColor(theme.TextMuted).SingleLine()
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14).TextColor(theme.TextMuted).
				AccessHidden(true)
		}
	})
	if row.Clicked() {
		action()
	}
}

// SystemInfo returns the redacted diagnostics copied to the clipboard. It
// contains build, runtime, display and catalog data only: no stream URL,
// credential, local path, favorite or watch history.
func (s *Settings) SystemInfo(display string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mumu %s\n", s.versionLabel(true))
	fmt.Fprintf(&b, "Runtime %s\n", runtimeLabel(s.about))
	fmt.Fprintf(&b, "Display %s\n", display)
	fmt.Fprintf(&b, "Catalog %s\n", s.catalogLabel())

	return b.String()
}

// updateStatus names the manual check outcome without inventing a time.
func (s *Settings) updateStatus() string {
	switch s.update.Kind {
	case UpdateChecking:
		return "Checking GitHub releases…"
	case UpdateCurrent:
		current := s.update.Current
		if current == "" {
			current = s.versionLabel(false)
		}

		return "Up to date · " + current
	case UpdateAvailable:
		if s.update.Latest == "" {
			return "A newer release is available."
		}

		return "mumu " + s.update.Latest + " is available."
	case UpdateNoRelease:
		if s.update.Message != "" {
			return s.update.Message
		}

		return "No stable release is published yet."
	case UpdateError:
		if s.update.Message != "" {
			return s.update.Message
		}

		return "mumu could not check for updates. Try again."
	default:
		return ""
	}
}

// updateTone colors the status without making color the only signal.
func updateTone(theme *ui.Theme, kind UpdateKind) ui.Color {
	switch kind {
	case UpdateCurrent:
		return theme.Success
	case UpdateAvailable:
		return theme.Text
	case UpdateError:
		return theme.Danger
	default:
		return theme.TextMuted
	}
}

// catalogLabel formats the real cached catalog counts.
func (s *Settings) catalogLabel() string {
	switch {
	case s.about.CountsLoading:
		return "Loading catalog…"
	case !s.about.CountsKnown:
		return "Catalog unavailable"
	default:
		return fmt.Sprintf("%s · %s",
			settingsCount(s.about.ChannelCount, "channel"),
			settingsCount(s.about.SourceCount, "source"))
	}
}

// licenseLabel names the project license.
func (s *Settings) licenseLabel() string {
	if license := strings.TrimSpace(s.about.License); license != "" {
		return license
	}

	return "MIT"
}

// settingsCount formats a count with grouped thousands.
func settingsCount(n int, noun string) string {
	digits := fmt.Sprintf("%d", n)
	if n >= 1000 {
		var b strings.Builder
		start := len(digits) % 3
		if start == 0 {
			start = 3
		}
		b.WriteString(digits[:start])
		for i := start; i < len(digits); i += 3 {
			b.WriteByte(',')
			b.WriteString(digits[i : i+3])
		}
		digits = b.String()
	}
	if n == 1 {
		return "1 " + noun
	}

	return digits + " " + noun + "s"
}

// runtimeLabel formats the actual Go, MyGo, OS and architecture data.
func runtimeLabel(info AboutInfo) string {
	parts := []string{}
	if info.GoVersion != "" {
		parts = append(parts, "Go "+info.GoVersion)
	}
	platform := strings.Trim(strings.Join([]string{info.OS, info.Arch}, "/"), "/")
	if platform != "" {
		parts = append(parts, platform)
	}
	if len(parts) == 0 {
		return "Unavailable"
	}

	return strings.Join(parts, " · ")
}

// myGoLabel names the pinned UI toolkit version from the build.
func myGoLabel(version string) string {
	if strings.TrimSpace(version) == "" {
		return "Unavailable"
	}

	return "MyGo " + strings.TrimPrefix(version, "v")
}

// repositoryLabel shortens the verified repository URL for display.
func repositoryLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "Unavailable"
	}

	return strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
}

// displayLabel reports the current window's logical size and text scale. The
// public MyGo API does not expose a backing scale factor, so none is invented.
func displayLabel(c *ui.Context) string {
	width, height := c.Size()
	scale := c.Preferences().TextScale
	if scale <= 0 {
		scale = 1
	}

	return fmt.Sprintf("%.0f × %.0f · text %.2f×", width, height, scale)
}
