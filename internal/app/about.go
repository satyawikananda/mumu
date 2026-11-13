package app

import (
	"context"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/ui"
)

const (
	// aboutTimeout bounds the local catalog counts read.
	aboutTimeout = 8 * time.Second
	// projectRepository is the verified source of releases and updates.
	projectRepository = "https://github.com/satyawikananda/mumu"
	// projectLicense is the license in LICENSE and mygo.json.
	projectLicense = "MIT"
)

// startAbout publishes real build data and starts one local catalog count.
func (a *App) startAbout() {
	gen, ctx := a.about.begin(a.ctx)
	base := a.aboutBase()
	base.CountsLoading = true
	a.publish(func() { a.shell.Settings().SetAbout(base) })

	a.GoQuiet(func(context.Context) {
		a.loadAboutCounts(ctx, gen, base)
	})
}

// refreshAboutCounts updates the local counts after a source change. It never
// touches the network and keeps the last known values until the new read ends.
func (a *App) refreshAboutCounts() {
	gen, ctx := a.about.begin(a.ctx)
	base := a.aboutBase()
	a.GoQuiet(func(context.Context) {
		a.loadAboutCounts(ctx, gen, base)
	})
}

// loadAboutCounts fills the real channel and source counts. A failed read says
// unavailable instead of reporting zero.
func (a *App) loadAboutCounts(ctx context.Context, gen uint64, base ui.AboutInfo) {
	service, err := a.Catalog()
	if err != nil {
		base.CountsKnown = false
		base.CountsLoading = false
		a.publishAbout(gen, base)

		return
	}

	readCtx, cancel := context.WithTimeout(ctx, aboutTimeout)
	defer cancel()

	enabled, err := service.BuiltinEnabled(readCtx)
	if err != nil {
		base.CountsKnown = false
		base.CountsLoading = false
		a.publishAbout(gen, base)

		return
	}

	list, err := service.List(readCtx)
	if err != nil {
		base.CountsKnown = false
		base.CountsLoading = false
		a.publishAbout(gen, base)

		return
	}

	channels, sources := 0, 0
	for _, source := range list {
		if !source.Enabled {
			continue
		}
		if source.Kind == catalog.SourceBuiltin && !enabled {
			continue
		}
		channels += source.ItemCount
		sources++
	}

	base.ChannelCount = channels
	base.SourceCount = sources
	base.CountsKnown = true
	base.CountsLoading = false
	a.publishAbout(gen, base)
}

// publishAbout shows one about snapshot if this is still the active read.
func (a *App) publishAbout(gen uint64, info ui.AboutInfo) {
	a.publish(func() {
		if a.about.current(gen) {
			a.shell.Settings().SetAbout(info)
		}
	})
}

// aboutBase builds the build and repository facts. It performs no network
// call and never invents a version.
func (a *App) aboutBase() ui.AboutInfo {
	info := ui.AboutInfo{
		Version:       a.currentVersion(),
		GoVersion:     strings.TrimPrefix(runtime.Version(), "go"),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		License:       projectLicense,
		RepositoryURL: projectRepository,
	}

	build, ok := a.buildInfo()
	if ok && build != nil {
		if info.GoVersion == "" {
			info.GoVersion = strings.TrimPrefix(build.GoVersion, "go")
		}
		if info.Version == "" {
			info.Version = buildVersion(build.Main.Version)
		}
		info.Commit = buildCommit(build)
		info.MyGoVersion = dependencyVersion(build, "github.com/egoist/mygo")
	}

	return info
}

// currentVersion returns the installed build version, or empty when this is an
// unversioned development binary.
func (a *App) currentVersion() string {
	if version := strings.TrimSpace(a.appVersion()); version != "" {
		return version
	}

	build, ok := a.buildInfo()
	if ok && build != nil {
		return buildVersion(build.Main.Version)
	}

	return ""
}

// buildVersion normalizes a Go module version for display and comparison.
func buildVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "(devel)" {
		return ""
	}

	return strings.TrimPrefix(version, "v")
}

// buildCommit returns the short VCS revision with a dirty marker.
func buildCommit(build *debug.BuildInfo) string {
	if build == nil {
		return ""
	}

	values := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		values[setting.Key] = setting.Value
	}

	revision := strings.TrimSpace(values["vcs.revision"])
	if revision == "" {
		return ""
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if values["vcs.modified"] == "true" {
		revision += "-dirty"
	}

	return revision
}

// dependencyVersion returns one module's version from the build, including a
// replacement version when the dependency was replaced.
func dependencyVersion(build *debug.BuildInfo, path string) string {
	if build == nil {
		return ""
	}

	for _, dep := range build.Deps {
		if dep.Path != path {
			continue
		}
		if dep.Replace != nil && dep.Replace.Version != "" {
			return dep.Replace.Version
		}

		return dep.Version
	}

	return ""
}
