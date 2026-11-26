//go:build darwin

package mpv

import (
	"errors"

	"github.com/ebitengine/purego"
)

// libnames are the libmpv dylib names, tried in order when Load receives no
// candidates. A bare name is resolved by the platform loader, which honours
// DYLD_LIBRARY_PATH and the default fallback paths.
var libnames = []string{"libmpv.dylib", "libmpv.2.dylib"}

// Locale categories for SetLocale (BSD/macOS values).
const (
	LCAll     = 0
	LCNumeric = 4
)

// openLibrary opens the first candidate the loader resolves. A candidate is
// either a bare library name or an absolute path. It returns every failure
// instead of panicking.
func openLibrary(candidates []string) (uintptr, error) {
	errs := make([]error, 0, len(candidates))
	for _, candidate := range candidates {
		if err := validateArchitecture(candidate); err != nil {
			errs = append(errs, err)
			continue
		}
		handle, err := purego.Dlopen(candidate, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return handle, nil
		}

		errs = append(errs, errors.New("libmpv runtime or dependency could not be loaded"))
	}

	return 0, errors.Join(errs...)
}

// memLibrary returns the handle for malloc/free; libmpv's libc dependency exposes them.
func memLibrary() uintptr {
	return libmpv
}

func closeLibrary(handle uintptr) error { return purego.Dlclose(handle) }

func closeMemory() {}
