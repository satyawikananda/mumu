package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mumu/internal/netguard"
)

const (
	defaultGitHubAPI      = "https://api.github.com"
	mumuReleasesPath      = "/repos/satyawikananda/mumu/releases"
	mumuReleasesQuery     = "?per_page=30"
	mumuReleasePagePrefix = "https://github.com/satyawikananda/mumu/releases/"
	mumuReleasesPrefix    = "/satyawikananda/mumu/releases/"
	releaseAccept         = "application/vnd.github+json"
	maxReleaseBytes       = 512 << 10
	releaseRequestTimeout = 8 * time.Second
)

// GitHubFetcher reads the public mumu release feed from the GitHub Releases
// API. It never downloads a release asset.
type GitHubFetcher struct {
	client      *netguard.Client
	releasesURL string
}

type releasePayload struct {
	TagName     string    `json:"tag_name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

// NewGitHubFetcher returns a fetcher for the public mumu repository.
func NewGitHubFetcher(policy netguard.Policy) *GitHubFetcher {
	return NewGitHubFetcherWithBase(policy, defaultGitHubAPI)
}

// NewGitHubFetcherWithBase returns a fetcher reading from baseURL. The
// production build uses NewGitHubFetcher; tests use this to avoid the network.
func NewGitHubFetcherWithBase(policy netguard.Policy, baseURL string) *GitHubFetcher {
	if policy.RequestTimeout <= 0 {
		policy.RequestTimeout = releaseRequestTimeout
	}

	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultGitHubAPI
	}

	return &GitHubFetcher{
		client:      netguard.New(policy),
		releasesURL: base + mumuReleasesPath + mumuReleasesQuery,
	}
}

// Releases returns every entry in the feed in published order. Drafts are kept
// and flagged; the checker ignores them.
func (f *GitHubFetcher) Releases(ctx context.Context) ([]Release, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	response, err := f.client.Fetch(ctx, netguard.Request{
		URL:      f.releasesURL,
		MaxBytes: maxReleaseBytes,
		Accept:   releaseAccept,
	})
	if err != nil {
		return nil, classifyReleaseError(ctx, err)
	}
	if len(response.Body) == 0 {
		return nil, fmt.Errorf("%w: empty release feed", ErrAccess)
	}

	var payload []releasePayload
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, fmt.Errorf("%w: malformed release feed: %w", ErrAccess, err)
	}

	releases := make([]Release, 0, len(payload))
	for _, item := range payload {
		tag := strings.TrimSpace(item.TagName)
		if tag == "" {
			continue
		}

		page, ok := releasePageURL(tag, item.HTMLURL)
		if !ok {
			continue
		}

		releases = append(releases, Release{
			Tag:         tag,
			Prerelease:  item.Prerelease,
			Draft:       item.Draft,
			HTMLURL:     page,
			PublishedAt: item.PublishedAt,
		})
	}

	return releases, nil
}

func classifyReleaseError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	var status *netguard.StatusError
	if errors.As(err, &status) {
		return classifyStatus(status)
	}
	if errors.Is(err, netguard.ErrResponseTooLarge) {
		return fmt.Errorf("%w: release feed is too large", ErrAccess)
	}

	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

func classifyStatus(status *netguard.StatusError) error {
	switch status.Code {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %w", ErrNoRelease, status)
	case http.StatusForbidden, http.StatusTooManyRequests:
		if rateLimited(status.Header) {
			return fmt.Errorf("%w: %w", ErrRateLimited, status)
		}

		return fmt.Errorf("%w: %w", ErrAccess, status)
	default:
		return fmt.Errorf("%w: %w", ErrAccess, status)
	}
}

func rateLimited(header http.Header) bool {
	if strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0" {
		return true
	}

	return strings.TrimSpace(header.Get("Retry-After")) != ""
}

// releasePageURL prefers a page built from a semantic tag and otherwise
// accepts only a release URL inside the mumu repository on github.com.
func releasePageURL(tag, htmlURL string) (string, bool) {
	if _, err := ParseVersion(tag); err == nil {
		return mumuReleasePagePrefix + "tag/" + url.PathEscape(tag), true
	}

	parsed, err := url.Parse(strings.TrimSpace(htmlURL))
	if err != nil || parsed.Opaque != "" || parsed.User != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	parsed.Scheme = scheme
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return "", false
	}
	if !strings.EqualFold(strings.TrimSuffix(parsed.Hostname(), "."), "github.com") {
		return "", false
	}
	if !strings.HasPrefix(parsed.Path, mumuReleasesPrefix) {
		return "", false
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}

	return parsed.String(), true
}
