package ui

import "github.com/egoist/mygo/ui"

// StatusKind identifies the state a StatusView reports.
type StatusKind int

const (
	// StatusEmpty is a normal empty state.
	StatusEmpty StatusKind = iota + 1
	// StatusLoading reports work without a known end.
	StatusLoading
	// StatusError reports a failure.
	StatusError
	// StatusOffline reports a missing network.
	StatusOffline
)

// Status is a reusable empty, loading, error or offline view.
type Status struct {
	Kind     StatusKind
	Title    string
	Message  string
	Action   string
	OnAction func()
}

// View draws the status centered in the available space.
func (s Status) View(c *ui.Context) {
	t := c.Theme()

	ui.Column(c).Fill().Center().Gap(12).Padding(24).Children(func() {
		switch s.Kind {
		case StatusLoading:
			ui.Spinner(c).Size(20, 20).Label("Loading")
		case StatusError:
			ui.Text(c, "!").FontSize(t.Rem(1.4)).Bold().TextColor(t.Danger).AccessHidden(true)
		case StatusOffline:
			ui.Text(c, "!").FontSize(t.Rem(1.4)).Bold().TextColor(t.Warning).AccessHidden(true)
		}

		titleColor := t.Text
		switch s.Kind {
		case StatusError:
			titleColor = t.Danger
		case StatusOffline:
			titleColor = t.Warning
		}

		ui.Text(c, s.Title).FontSize(t.Rem(1.7)).FontWeight(500).TextColor(titleColor).TextAlign(ui.Center)
		if s.Message != "" {
			ui.Text(c, s.Message).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(420)
		}
		if s.Action != "" && s.OnAction != nil {
			ui.PrimaryButton(c, s.Action).OnClick(s.OnAction)
		}
	})
}
