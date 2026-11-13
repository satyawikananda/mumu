package app

import (
	"context"
	"errors"
	"strings"

	"mumu/internal/netguard"
	"mumu/internal/ui"
	"mumu/internal/update"
)

// releaseChecker is the manual update-check seam used by tests.
type releaseChecker interface {
	Check(ctx context.Context, current string) (update.Result, error)
}

// newReleaseChecker builds the metadata-only GitHub checker. Constructing it
// performs no network access.
func newReleaseChecker() releaseChecker {
	return update.NewChecker(update.NewGitHubFetcher(netguard.Policy{}))
}

// checkForUpdates performs one user-requested release check. Nothing calls it
// at launch, and it never downloads or installs a release.
func (a *App) checkForUpdates() {
	if a.updates == nil {
		return
	}

	current := a.currentVersion()
	gen, ctx := a.updateCheck.begin(a.ctx)
	a.publish(func() {
		a.shell.Settings().SetUpdate(ui.UpdateState{
			Kind:    ui.UpdateChecking,
			Current: current,
		})
	})

	a.GoQuiet(func(context.Context) {
		result, err := a.updates.Check(ctx, current)
		if !a.updateCheck.current(gen) || errors.Is(err, context.Canceled) {
			return
		}

		state := updateState(current, result, err)
		a.publish(func() {
			if a.updateCheck.current(gen) {
				a.shell.Settings().SetUpdate(state)
			}
		})
	})
}

// updateState maps one checker result or failure to the explicit UI state.
func updateState(current string, result update.Result, err error) ui.UpdateState {
	if err != nil {
		state := ui.UpdateState{Kind: ui.UpdateError, Current: current}
		switch {
		case errors.Is(err, update.ErrNoRelease):
			state.Kind = ui.UpdateNoRelease
			state.Message = noReleaseMessage(current)
		case errors.Is(err, update.ErrRateLimited):
			state.Message = "GitHub's release service is rate limited. Try again later."
		case errors.Is(err, update.ErrAccess):
			state.Message = "GitHub refused the release request. Try again later."
		case errors.Is(err, update.ErrNetwork):
			state.Message = "mumu could not reach GitHub. Check your connection and try again."
		case errors.Is(err, update.ErrUnversioned):
			state.Message = "This build has no release version, so update checks are unavailable."
		default:
			state.Message = "mumu could not check for updates. Try again."
		}

		return state
	}

	if result.Newer {
		url := strings.TrimSpace(result.URL)
		if url == "" && result.Latest != "" {
			url = projectRepository + "/releases/tag/v" + result.Latest
		}

		return ui.UpdateState{
			Kind:       ui.UpdateAvailable,
			Current:    current,
			Latest:     result.Latest,
			ReleaseURL: url,
		}
	}

	return ui.UpdateState{
		Kind:    ui.UpdateCurrent,
		Current: current,
		Latest:  result.Latest,
	}
}

// noReleaseMessage names the channel the installed build checks.
func noReleaseMessage(current string) string {
	if version, err := update.ParseVersion(current); err == nil && version.IsPrerelease() {
		return "No release is published in this build's channel yet."
	}

	return "No stable release is published yet."
}
