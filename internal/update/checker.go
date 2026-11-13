package update

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Release is one published release from the feed.
type Release struct {
	Tag         string
	Prerelease  bool
	Draft       bool
	HTMLURL     string
	PublishedAt time.Time
}

// Result is the outcome of one manual update check. Current and Latest are
// canonical semantic versions without a leading "v".
type Result struct {
	Current     string
	Latest      string
	Newer       bool
	URL         string
	Prerelease  bool
	PublishedAt time.Time
}

// Fetcher lists releases. The checker owns selection, so an implementation only
// has to read the feed; tests supply a fake.
type Fetcher interface {
	Releases(ctx context.Context) ([]Release, error)
}

// Checker selects the newest release that matches the running version channel.
type Checker struct {
	fetcher Fetcher
}

// NewChecker returns a checker reading releases through fetcher, which must not
// be nil.
func NewChecker(fetcher Fetcher) *Checker {
	return &Checker{fetcher: fetcher}
}

// Check compares current with the published releases.
//
// A stable current version sees only stable releases. A prerelease current
// version sees stable releases and prereleases in the same channel, named by
// the first prerelease identifier ("beta" for 1.0.0-beta.2). Drafts are always
// ignored and the highest semantic version wins.
func (c *Checker) Check(ctx context.Context, current string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	version, err := ParseVersion(current)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %q", ErrUnversioned, strings.TrimSpace(current))
	}
	if c == nil || c.fetcher == nil {
		return Result{}, fmt.Errorf("%w: no release feed configured", ErrAccess)
	}

	releases, err := c.fetcher.Releases(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("check updates: %w", err)
	}

	selected, found := selectRelease(releases, version)
	if !found {
		return Result{}, fmt.Errorf("%w: no release for %s", ErrNoRelease, version)
	}

	return Result{
		Current:     version.String(),
		Latest:      selected.version.String(),
		Newer:       selected.version.Compare(version) > 0,
		URL:         selected.release.HTMLURL,
		Prerelease:  selected.release.Prerelease || selected.version.IsPrerelease(),
		PublishedAt: selected.release.PublishedAt,
	}, nil
}

type candidate struct {
	release Release
	version Version
}

func selectRelease(releases []Release, current Version) (candidate, bool) {
	channel := current.Channel()

	var best candidate
	found := false

	for _, release := range releases {
		if release.Draft {
			continue
		}

		version, err := ParseVersion(release.Tag)
		if err != nil {
			continue
		}

		prerelease := release.Prerelease || version.IsPrerelease()
		if channel == "" {
			if prerelease {
				continue
			}
		} else if prerelease && version.Channel() != channel {
			continue
		}

		if !found || version.Compare(best.version) > 0 {
			best = candidate{release: release, version: version}
			found = true
		}
	}

	return best, found
}
