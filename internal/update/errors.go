package update

import "errors"

// Sentinel errors classify a failed manual update check.
var (
	// ErrNoRelease reports that the feed offers no release for the current
	// channel.
	ErrNoRelease = errors.New("no matching release")
	// ErrUnversioned reports that the running build has no semantic version.
	ErrUnversioned = errors.New("current version is not a semantic version")
	// ErrAccess reports that the release feed refused or could not answer.
	ErrAccess = errors.New("release feed is not accessible")
	// ErrRateLimited reports that the release feed asked the caller to wait.
	ErrRateLimited = errors.New("release feed rate limit reached")
	// ErrNetwork reports that the release feed could not be reached.
	ErrNetwork = errors.New("release feed is unreachable")
)
