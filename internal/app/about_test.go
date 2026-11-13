package app

import (
	"context"
	"runtime/debug"
	"strings"
	"testing"

	"mumu/internal/ui"
	"mumu/internal/update"
)

// aboutBase reports the real build and MyGo data.
func TestAboutBase(t *testing.T) {
	a := New()
	a.appVersion = func() string { return "1.2.3" }
	a.buildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.27.3",
			Main:      debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.modified", Value: "true"},
			},
			Deps: []*debug.Module{
				{Path: "github.com/egoist/mygo", Version: "v0.9.0"},
			},
		}, true
	}

	info := a.aboutBase()
	if info.Version != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", info.Version)
	}
	if info.Commit != "0123456-dirty" {
		t.Fatalf("commit = %q, want the short dirty revision", info.Commit)
	}
	if info.GoVersion != "1.27.3" || info.MyGoVersion != "v0.9.0" {
		t.Fatalf("runtime %+v, want Go 1.27.3 and MyGo v0.9.0", info)
	}
	if info.OS == "" || info.Arch == "" {
		t.Fatalf("platform %+v, want real OS and architecture", info)
	}
	if info.License != "MIT" || info.RepositoryURL != projectRepository {
		t.Fatalf("about %+v, want MIT and the verified repository", info)
	}
}

// currentVersion prefers the packaged version, then a Go module version.
func TestCurrentVersion(t *testing.T) {
	a := New()
	a.appVersion = func() string { return "" }
	a.buildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v2.0.0"}}, true
	}
	if got := a.currentVersion(); got != "2.0.0" {
		t.Fatalf("version = %q, want 2.0.0", got)
	}

	a.buildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true
	}
	if got := a.currentVersion(); got != "" {
		t.Fatalf("development version = %q, want empty", got)
	}
}

// buildCommit tolerates a build without VCS settings.
func TestBuildCommitWithoutRevision(t *testing.T) {
	if got := buildCommit(&debug.BuildInfo{}); got != "" {
		t.Fatalf("commit = %q, want empty", got)
	}
	if got := buildCommit(nil); got != "" {
		t.Fatalf("nil commit = %q, want empty", got)
	}
}

// dependencyVersion follows a replacement module.
func TestDependencyVersionReplacement(t *testing.T) {
	build := &debug.BuildInfo{Deps: []*debug.Module{
		{
			Path:    "github.com/egoist/mygo",
			Version: "v0.8.0",
			Replace: &debug.Module{Version: "v0.9.0"},
		},
	}}
	if got := dependencyVersion(build, "github.com/egoist/mygo"); got != "v0.9.0" {
		t.Fatalf("replacement version = %q, want v0.9.0", got)
	}
}

// loadAboutCounts reports enabled cached sources, excluding a hidden community
// catalog, and distinguishes an unavailable read from zero.
func TestLoadAboutCounts(t *testing.T) {
	a, _ := discoverTestApp(t)
	seedBuiltinCatalog(t, a)
	seedUserSource(t, a)

	gen, ctx := a.about.begin(a.ctx)
	a.loadAboutCounts(ctx, gen, a.aboutBase())

	info := a.shell.Settings().About()
	if !info.CountsKnown || info.CountsLoading {
		t.Fatalf("counts %+v, want a finished real read", info)
	}
	if info.ChannelCount != 2 || info.SourceCount != 2 {
		t.Fatalf("counts %d/%d, want 2 channels from 2 sources", info.ChannelCount, info.SourceCount)
	}

	service, err := a.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetBuiltinEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	gen, ctx = a.about.begin(a.ctx)
	a.loadAboutCounts(ctx, gen, a.aboutBase())
	info = a.shell.Settings().About()
	if info.ChannelCount != 1 || info.SourceCount != 1 {
		t.Fatalf("hidden community counts %d/%d, want only the user source", info.ChannelCount, info.SourceCount)
	}
}

// fakeChecker is the manual update-check seam.
type fakeChecker struct {
	calls  int
	result update.Result
	err    error
}

func (f *fakeChecker) Check(context.Context, string) (update.Result, error) {
	f.calls++

	return f.result, f.err
}

// The manual check maps current, available and failure states without a
// launch-time request.
func TestCheckForUpdates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		result  update.Result
		err     error
		want    ui.UpdateKind
		message string
	}{
		{name: "current", current: "0.1.0", result: update.Result{Latest: "0.1.0"}, want: ui.UpdateCurrent},
		{
			name: "available", current: "0.1.0",
			result: update.Result{Latest: "0.2.0", Newer: true, URL: "https://github.com/satyawikananda/mumu/releases/tag/v0.2.0"},
			want:   ui.UpdateAvailable,
		},
		{name: "no release", current: "0.1.0", err: update.ErrNoRelease, want: ui.UpdateNoRelease, message: "No stable release"},
		{
			name: "prerelease no release", current: "0.2.0-beta.1",
			err: update.ErrNoRelease, want: ui.UpdateNoRelease,
			message: "in this build's channel",
		},
		{name: "rate limited", current: "0.1.0", err: update.ErrRateLimited, want: ui.UpdateError, message: "rate limited"},
		{name: "access", current: "0.1.0", err: update.ErrAccess, want: ui.UpdateError, message: "refused"},
		{name: "network", current: "0.1.0", err: update.ErrNetwork, want: ui.UpdateError, message: "could not reach GitHub"},
		{name: "unversioned", current: "", err: update.ErrUnversioned, want: ui.UpdateError, message: "no release version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, f := discoverTestApp(t)
			a.appVersion = func() string { return tc.current }
			fake := &fakeChecker{result: tc.result, err: tc.err}
			a.updates = fake

			a.checkForUpdates()
			waitFor(t, f, "update state", func() bool {
				state := a.shell.Settings().Update()
				return state.Kind == tc.want
			})

			if fake.calls != 1 {
				t.Fatalf("checks = %d, want one explicit check", fake.calls)
			}
			if tc.message != "" {
				state := a.shell.Settings().Update()
				if !strings.Contains(state.Message, tc.message) {
					t.Fatalf("message = %q, want %q", state.Message, tc.message)
				}
			}
		})
	}
}

// A checker is not called just by constructing or loading an app.
func TestUpdateCheckIsManualOnly(t *testing.T) {
	a := New()
	fake := &fakeChecker{}
	a.updates = fake
	a.startAbout()

	if fake.calls != 0 {
		t.Fatalf("checks = %d before the About action, want 0", fake.calls)
	}
}
