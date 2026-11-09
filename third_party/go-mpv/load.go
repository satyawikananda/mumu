package mpv

import (
	"errors"
	"fmt"
	"sync"
)

// Client API version this binding is written against. libmpv keeps the client
// API backwards compatible inside one major version, so a newer minor is fine.
const (
	requiredAPIMajor = 2
	requiredAPIMinor = 1
)

// Runtime loading errors.
var (
	// ErrNotLoaded reports use of the binding before Load succeeded.
	ErrNotLoaded = errors.New("libmpv runtime is not loaded")
	// ErrNotFound reports a runtime that no candidate could open.
	ErrNotFound = errors.New("libmpv runtime was not found")
	// ErrUnsupportedAPI reports a runtime with an incompatible client API.
	ErrUnsupportedAPI = errors.New("unsupported libmpv client API version")
)

var (
	// loadMu serializes Load and guards loaded. Entry points are only written
	// while it is held, so every other call is safe once Load has returned nil.
	loadMu sync.Mutex
	loaded bool
)

// Load opens libmpv from the first candidate that resolves and registers its
// entry points.
//
// Candidates are tried in order; each is a bare library name, which the
// platform loader resolves (honouring DYLD_LIBRARY_PATH on macOS), or an
// absolute path. An empty list uses the platform library names.
//
// Upstream go-mpv opens the runtime in an init and panics when it is missing.
// Here a missing or incompatible runtime is a returned error, and Load can be
// retried after the caller installs one. Load is idempotent and safe to call
// from multiple goroutines.
func Load(candidates []string) error {
	loadMu.Lock()
	defer loadMu.Unlock()

	if loaded {
		return nil
	}

	tried := candidates
	if len(tried) == 0 {
		tried = libnames
	}

	handle, err := openLibrary(tried)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if err := register(handle); err != nil {
		return err
	}

	loaded = true

	return nil
}

// Loaded reports whether Load has opened a runtime.
func Loaded() bool {
	loadMu.Lock()
	defer loadMu.Unlock()

	return loaded
}

// register installs every entry point and validates the client API version. A
// partially registered runtime is left unusable so a later Load can retry with
// another one.
func register(handle uintptr) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			libmpv = 0
			err = fmt.Errorf("register libmpv entry points: %v", recovered)
		}
	}()

	libmpv = handle
	registerClientFuncs()
	registerRenderFuncs()

	if err := checkAPIVersion(); err != nil {
		libmpv = 0

		return err
	}

	return nil
}

// checkAPIVersion rejects a runtime this binding cannot call.
func checkAPIVersion() error {
	version := apiVersion()
	major := int(version >> 16)
	minor := int(version & 0xffff)

	if major != requiredAPIMajor || minor < requiredAPIMinor {
		libmpv = 0

		return fmt.Errorf(
			"%w: runtime reports %d.%d, needs %d.%d or newer",
			ErrUnsupportedAPI, major, minor, requiredAPIMajor, requiredAPIMinor,
		)
	}

	return nil
}
