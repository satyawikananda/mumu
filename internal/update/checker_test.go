package update

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"
)

type fakeFetcher struct {
	releases []Release
	err      error
	calls    int
}

func (f *fakeFetcher) Releases(context.Context) ([]Release, error) {
	f.calls++

	return f.releases, f.err
}

var publishedAt = time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)

func fakeRelease(tag string, prerelease, draft bool) Release {
	return Release{
		Tag:         tag,
		Prerelease:  prerelease,
		Draft:       draft,
		HTMLURL:     "https://github.com/satyawikananda/mumu/releases/tag/" + url.PathEscape(tag),
		PublishedAt: publishedAt,
	}
}

func TestCheckerStable(t *testing.T) {
	fetcher := &fakeFetcher{releases: []Release{
		fakeRelease("v0.1.0", false, false),
		fakeRelease("v0.2.0-beta.1", true, false),
		fakeRelease("v0.2.0", false, false),
		fakeRelease("v0.9.0", false, true),
	}}

	result, err := NewChecker(fetcher).Check(t.Context(), "v0.1.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if result.Current != "0.1.0" || result.Latest != "0.2.0" || !result.Newer {
		t.Fatalf("result = %+v, want 0.1.0 -> 0.2.0 newer", result)
	}
	if result.Prerelease {
		t.Fatalf("result = %+v, want a stable release", result)
	}
	if result.URL != "https://github.com/satyawikananda/mumu/releases/tag/v0.2.0" {
		t.Fatalf("URL = %q", result.URL)
	}
	if !result.PublishedAt.Equal(publishedAt) {
		t.Fatalf("PublishedAt = %v, want %v", result.PublishedAt, publishedAt)
	}
}

func TestCheckerStableIgnoresPrereleases(t *testing.T) {
	fetcher := &fakeFetcher{releases: []Release{
		fakeRelease("v0.1.0", false, false),
		fakeRelease("v0.3.0-beta.1", true, false),
		// A prerelease tag is still a prerelease even when the feed omits the
		// GitHub flag.
		fakeRelease("v0.4.0-rc.1", false, false),
		fakeRelease("nightly", false, false),
	}}

	result, err := NewChecker(fetcher).Check(t.Context(), "0.1.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Latest != "0.1.0" || result.Newer {
		t.Fatalf("result = %+v, want no newer stable release", result)
	}

	onlyPrereleases := &fakeFetcher{releases: []Release{
		fakeRelease("v0.3.0-beta.1", true, false),
	}}
	if _, err := NewChecker(onlyPrereleases).Check(t.Context(), "0.1.0"); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("error %v, want ErrNoRelease", err)
	}
}

func TestCheckerPrereleaseChannel(t *testing.T) {
	fetcher := &fakeFetcher{releases: []Release{
		fakeRelease("v0.1.0", false, false),
		fakeRelease("v0.2.0-beta.2", true, false),
		fakeRelease("v0.3.0-rc.1", true, false),
	}}

	result, err := NewChecker(fetcher).Check(t.Context(), "v0.2.0-beta.1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Current != "0.2.0-beta.1" || result.Latest != "0.2.0-beta.2" || !result.Newer {
		t.Fatalf("result = %+v, want beta.1 -> beta.2", result)
	}
	if !result.Prerelease {
		t.Fatalf("result = %+v, want a prerelease", result)
	}
}

func TestCheckerPrereleaseAcceptsStable(t *testing.T) {
	fetcher := &fakeFetcher{releases: []Release{
		fakeRelease("v0.2.0", false, false),
		fakeRelease("v0.2.0-beta.9", true, false),
	}}

	result, err := NewChecker(fetcher).Check(t.Context(), "v0.2.0-beta.1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Latest != "0.2.0" || !result.Newer || result.Prerelease {
		t.Fatalf("result = %+v, want the stable release", result)
	}
}

func TestCheckerPrereleaseRejectsOtherChannels(t *testing.T) {
	fetcher := &fakeFetcher{releases: []Release{
		fakeRelease("v9.0.0-rc.1", true, false),
	}}

	if _, err := NewChecker(fetcher).Check(t.Context(), "v0.2.0-beta.1"); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("error %v, want ErrNoRelease", err)
	}
}

func TestCheckerNoNewerRelease(t *testing.T) {
	fetcher := &fakeFetcher{releases: []Release{
		fakeRelease("v0.1.0", false, false),
		fakeRelease("v0.2.0", false, false),
	}}

	result, err := NewChecker(fetcher).Check(t.Context(), "0.2.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Newer {
		t.Fatalf("result = %+v, want Newer false for an equal release", result)
	}

	result, err = NewChecker(fetcher).Check(t.Context(), "1.0.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Newer {
		t.Fatalf("result = %+v, want Newer false for a newer running build", result)
	}
}

func TestCheckerNoRelease(t *testing.T) {
	for _, releases := range [][]Release{
		nil,
		{},
		{fakeRelease("v0.2.0", false, true)},
		{fakeRelease("nightly", false, false)},
	} {
		fetcher := &fakeFetcher{releases: releases}
		if _, err := NewChecker(fetcher).Check(t.Context(), "0.1.0"); !errors.Is(err, ErrNoRelease) {
			t.Fatalf("releases %+v: error %v, want ErrNoRelease", releases, err)
		}
	}
}

func TestCheckerRejectsUnversionedCurrent(t *testing.T) {
	for _, current := range []string{"", "  ", "dev", "1.2", "nightly"} {
		fetcher := &fakeFetcher{releases: []Release{fakeRelease("v0.2.0", false, false)}}
		if _, err := NewChecker(fetcher).Check(t.Context(), current); !errors.Is(err, ErrUnversioned) {
			t.Fatalf("current %q: error %v, want ErrUnversioned", current, err)
		}
		if fetcher.calls != 0 {
			t.Fatalf("current %q: fetcher called %d times, want 0", current, fetcher.calls)
		}
	}
}

func TestCheckerPropagatesFetcherError(t *testing.T) {
	fetcher := &fakeFetcher{err: fmt.Errorf("%w: 429", ErrRateLimited)}

	_, err := NewChecker(fetcher).Check(t.Context(), "0.1.0")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("error %v, want ErrRateLimited", err)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetcher called %d times, want 1", fetcher.calls)
	}
}

func TestCheckerWithoutFetcher(t *testing.T) {
	var checker *Checker
	if _, err := checker.Check(t.Context(), "0.1.0"); !errors.Is(err, ErrAccess) {
		t.Fatalf("error %v, want ErrAccess", err)
	}

	if _, err := NewChecker(nil).Check(t.Context(), "0.1.0"); !errors.Is(err, ErrAccess) {
		t.Fatalf("error %v, want ErrAccess", err)
	}
}
