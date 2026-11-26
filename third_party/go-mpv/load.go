package mpv

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
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
	loadMu     sync.Mutex
	loaded     bool
	loadedName string
)

// Load opens libmpv from the first candidate that resolves and registers its
// entry points.
//
// Candidates are tried in order. Each is either an absolute library file or,
// on macOS and Linux, a bare library name that the platform loader resolves
// (honouring DYLD_LIBRARY_PATH / LD_LIBRARY_PATH). Windows accepts only
// absolute paths, as its loader must not search PATH or the working directory;
// its platform names are therefore rejected when passed without a directory.
// An empty list uses the platform library names.
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
	if len(tried) == 0 {
		return fmt.Errorf("%w: no library names for this platform", ErrNotFound)
	}

	failures := make([]error, 0, len(tried))
	for _, candidate := range tried {
		handle, err := openLibrary([]string{candidate})
		if err != nil {
			failures = append(failures, fmt.Errorf("%w: %w", ErrNotFound, err))
			continue
		}
		if err := register(handle); err != nil {
			closeMemory()
			closeErr := closeLibrary(handle)
			failures = append(failures, errors.Join(err, closeErr))
			continue
		}
		loaded = true
		loadedName = filepath.Base(candidate)
		return nil
	}

	return errors.Join(failures...)
}

// Loaded reports whether Load has opened a runtime.
func Loaded() bool {
	loadMu.Lock()
	defer loadMu.Unlock()

	return loaded
}

// RuntimeInfo returns the successfully loaded file name (never its directory)
// and client API version. Before Load succeeds both values are zero.
func RuntimeInfo() (string, uint64) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if !loaded {
		return "", 0
	}
	return loadedName, uint64(apiVersion())
}

// register installs every entry point and validates the client API version. A
// partially registered runtime is left unusable so a later Load can retry with
// another one.
func register(handle uintptr) (err error) {
	// Register into temporary typed slots. Nothing callable is published until
	// every symbol and the API version have passed validation.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("libmpv runtime is missing required entry points")
		}
		if err != nil {
			libmpv = 0
		}
	}()
	libmpv = handle
	var version func() apiVersionValue
	purego.RegisterLibFunc(&version, handle, "mpv_client_api_version")
	if err := checkAPIVersion(uint64(version())); err != nil {
		return err
	}
	commits := []func(){}
	stage := func(destination any, library uintptr, name string) {
		target := reflect.ValueOf(destination).Elem()
		slot := reflect.New(target.Type())
		purego.RegisterLibFunc(slot.Interface(), library, name)
		commits = append(commits, func() { target.Set(slot.Elem()) })
	}
	registerClientFuncs(stage)
	registerRenderFuncs(stage)
	for _, commit := range commits {
		commit()
	}
	cAlloc = func(size int) unsafe.Pointer { return memAlloc(uintptr(size)) }
	cFree = memFree
	return nil
}

// checkAPIVersion rejects a runtime this binding cannot call.
func checkAPIVersion(version uint64) error {
	major := int(version >> 16)
	minor := int(version & 0xffff)

	if major != requiredAPIMajor || minor < requiredAPIMinor {
		return fmt.Errorf(
			"%w: runtime reports %d.%d, needs %d.%d or newer",
			ErrUnsupportedAPI, major, minor, requiredAPIMajor, requiredAPIMinor,
		)
	}

	return nil
}
