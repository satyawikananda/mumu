package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mumu/internal/netguard"
)

func testFetcher(t *testing.T, handler http.Handler) *GitHubFetcher {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewGitHubFetcherWithBase(
		netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second},
		server.URL,
	)
}

func TestNewGitHubFetcherDefaults(t *testing.T) {
	for _, fetcher := range []*GitHubFetcher{
		NewGitHubFetcher(netguard.Policy{}),
		NewGitHubFetcherWithBase(netguard.Policy{}, ""),
	} {
		if fetcher.releasesURL != "https://api.github.com/repos/satyawikananda/mumu/releases?per_page=30" {
			t.Fatalf("releasesURL = %q", fetcher.releasesURL)
		}
		if fetcher.client == nil {
			t.Fatal("client is nil")
		}
	}
}

func TestGitHubFetcherReleases(t *testing.T) {
	fetcher := testFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/satyawikananda/mumu/releases" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "30" {
			t.Errorf("per_page = %q, want 30", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"tag_name": "v0.2.0",
				"prerelease": false,
				"draft": false,
				"html_url": "https://evil.example/mumu",
				"published_at": "2026-01-02T15:04:05Z"
			},
			{
				"tag_name": "v0.2.1-beta.1",
				"prerelease": true,
				"draft": false,
				"html_url": "https://github.com/satyawikananda/mumu/releases/tag/v0.2.1-beta.1",
				"published_at": "2026-02-03T04:05:06Z"
			},
			{
				"tag_name": "v0.3.0",
				"prerelease": false,
				"draft": true,
				"html_url": "https://github.com/satyawikananda/mumu/releases/tag/v0.3.0",
				"published_at": "2026-03-04T05:06:07Z"
			},
			{
				"tag_name": "nightly",
				"prerelease": false,
				"draft": false,
				"html_url": "https://github.com/satyawikananda/mumu/releases/tag/nightly"
			},
			{
				"tag_name": "nightly-other",
				"prerelease": false,
				"draft": false,
				"html_url": "https://github.com/satyawikananda/other/releases/tag/nightly"
			},
			{
				"tag_name": "",
				"prerelease": false,
				"draft": false,
				"html_url": "https://github.com/satyawikananda/mumu/releases/tag/empty"
			}
		]`))
	}))

	releases, err := fetcher.Releases(t.Context())
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(releases) != 4 {
		t.Fatalf("len(releases) = %d, want 4: %+v", len(releases), releases)
	}

	stable := releases[0]
	if stable.Tag != "v0.2.0" || stable.Prerelease || stable.Draft {
		t.Fatalf("stable release = %+v", stable)
	}
	if stable.HTMLURL != "https://github.com/satyawikananda/mumu/releases/tag/v0.2.0" {
		t.Fatalf("stable URL = %q, want the constructed repository page", stable.HTMLURL)
	}
	if want := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC); !stable.PublishedAt.Equal(want) {
		t.Fatalf("PublishedAt = %v, want %v", stable.PublishedAt, want)
	}

	if beta := releases[1]; beta.Tag != "v0.2.1-beta.1" || !beta.Prerelease {
		t.Fatalf("prerelease = %+v", beta)
	}
	if draft := releases[2]; draft.Tag != "v0.3.0" || !draft.Draft {
		t.Fatalf("draft = %+v", draft)
	}
	if nightly := releases[3]; nightly.Tag != "nightly" ||
		nightly.HTMLURL != "https://github.com/satyawikananda/mumu/releases/tag/nightly" {
		t.Fatalf("non-semver release = %+v", nightly)
	}
}

func TestReleasePageURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tag     string
		htmlURL string
		want    string
		ok      bool
	}{
		{
			name:    "semver tag builds the page",
			tag:     "v1.2.3",
			htmlURL: "https://evil.example/mumu",
			want:    "https://github.com/satyawikananda/mumu/releases/tag/v1.2.3",
			ok:      true,
		},
		{
			name:    "semver tag ignores a broken html_url",
			tag:     "1.2.3-rc.1",
			htmlURL: "javascript:alert(1)",
			want:    "https://github.com/satyawikananda/mumu/releases/tag/1.2.3-rc.1",
			ok:      true,
		},
		{
			name:    "valid fallback",
			tag:     "nightly",
			htmlURL: "https://github.com/satyawikananda/mumu/releases/tag/nightly",
			want:    "https://github.com/satyawikananda/mumu/releases/tag/nightly",
			ok:      true,
		},
		{
			name:    "http fallback",
			tag:     "nightly",
			htmlURL: "http://github.com/satyawikananda/mumu/releases/tag/nightly",
			want:    "http://github.com/satyawikananda/mumu/releases/tag/nightly",
			ok:      true,
		},
		{name: "other host", tag: "nightly", htmlURL: "https://evil.example/satyawikananda/mumu/releases/tag/nightly"},
		{name: "lookalike host", tag: "nightly", htmlURL: "https://github.com.evil.example/satyawikananda/mumu/releases/tag/nightly"},
		{name: "wrong repository", tag: "nightly", htmlURL: "https://github.com/satyawikananda/other/releases/tag/nightly"},
		{name: "wrong path", tag: "nightly", htmlURL: "https://github.com/satyawikananda/mumu/issues/1"},
		{name: "credentials", tag: "nightly", htmlURL: "https://user:pass@github.com/satyawikananda/mumu/releases/tag/nightly"},
		{name: "query", tag: "nightly", htmlURL: "https://github.com/satyawikananda/mumu/releases/tag/nightly?x=1"},
		{name: "fragment", tag: "nightly", htmlURL: "https://github.com/satyawikananda/mumu/releases/tag/nightly#x"},
		{name: "port", tag: "nightly", htmlURL: "https://github.com:8443/satyawikananda/mumu/releases/tag/nightly"},
		{name: "scheme", tag: "nightly", htmlURL: "ftp://github.com/satyawikananda/mumu/releases/tag/nightly"},
		{name: "traversal", tag: "nightly", htmlURL: "https://github.com/satyawikananda/mumu/releases/../other"},
		{name: "empty", tag: "nightly", htmlURL: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := releasePageURL(tc.tag, tc.htmlURL)
			if ok != tc.ok {
				t.Fatalf("releasePageURL(%q, %q) ok = %v, want %v", tc.tag, tc.htmlURL, ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("URL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGitHubFetcherStatusErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		header  http.Header
		wantErr error
	}{
		{
			name:    "forbidden with no remaining quota",
			status:  http.StatusForbidden,
			header:  http.Header{"X-Ratelimit-Remaining": []string{"0"}},
			wantErr: ErrRateLimited,
		},
		{
			name:    "too many requests with retry after",
			status:  http.StatusTooManyRequests,
			header:  http.Header{"Retry-After": []string{"30"}},
			wantErr: ErrRateLimited,
		},
		{
			name:    "forbidden without rate limit headers",
			status:  http.StatusForbidden,
			wantErr: ErrAccess,
		},
		{
			name:    "unauthorized",
			status:  http.StatusUnauthorized,
			wantErr: ErrAccess,
		},
		{
			name:    "server error",
			status:  http.StatusInternalServerError,
			wantErr: ErrAccess,
		},
		{
			name:    "not found",
			status:  http.StatusNotFound,
			wantErr: ErrNoRelease,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := testFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, values := range tc.header {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(tc.status)
			}))

			_, err := fetcher.Releases(t.Context())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error %v, want %v", err, tc.wantErr)
			}

			// The classified error keeps the response headers reachable, so the
			// UI can surface a retry delay.
			var status *netguard.StatusError
			if !errors.As(err, &status) {
				t.Fatalf("error %v, want *netguard.StatusError", err)
			}
			if status.Code != tc.status {
				t.Fatalf("status code %d, want %d", status.Code, tc.status)
			}
			if got := status.Header.Get("Retry-After"); got != tc.header.Get("Retry-After") {
				t.Fatalf("Retry-After = %q, want %q", got, tc.header.Get("Retry-After"))
			}
		})
	}
}

func TestGitHubFetcherMalformedFeed(t *testing.T) {
	for _, body := range []string{"", "not json", `{"message":"nope"}`} {
		fetcher := testFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))

		if _, err := fetcher.Releases(t.Context()); !errors.Is(err, ErrAccess) {
			t.Fatalf("body %q: error %v, want ErrAccess", body, err)
		}
	}
}

func TestGitHubFetcherEmptyFeed(t *testing.T) {
	fetcher := testFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))

	releases, err := fetcher.Releases(t.Context())
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("releases = %+v, want none", releases)
	}

	if _, err := NewChecker(fetcher).Check(t.Context(), "0.1.0"); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("Check error %v, want ErrNoRelease", err)
	}
}

func TestGitHubFetcherBodyLimit(t *testing.T) {
	fetcher := testFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxReleaseBytes+1)))
	}))

	if _, err := fetcher.Releases(t.Context()); !errors.Is(err, ErrAccess) {
		t.Fatalf("error %v, want ErrAccess", err)
	}
}

func TestGitHubFetcherNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close()

	fetcher := NewGitHubFetcherWithBase(
		netguard.Policy{AllowPrivate: true, RequestTimeout: time.Second},
		baseURL,
	)

	if _, err := fetcher.Releases(t.Context()); !errors.Is(err, ErrNetwork) {
		t.Fatalf("error %v, want ErrNetwork", err)
	}
}

func TestGitHubFetcherTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	fetcher := NewGitHubFetcherWithBase(
		netguard.Policy{AllowPrivate: true, RequestTimeout: 20 * time.Millisecond},
		server.URL,
	)

	if _, err := fetcher.Releases(t.Context()); !errors.Is(err, ErrNetwork) {
		t.Fatalf("error %v, want ErrNetwork", err)
	}
}

func TestGitHubFetcherCanceled(t *testing.T) {
	fetcher := testFetcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := fetcher.Releases(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
}
