//go:build unix && !darwin

package mpv

import (
	"errors"

	"github.com/ebitengine/purego"
)

// libnames are the libmpv shared object names, tried in order when Load
// receives no candidates.
var libnames = []string{"libmpv.so.2", "libmpv.so"}

// Locale categories for SetLocale (glibc/musl values).
const (
	LCAll     = 6
	LCNumeric = 1
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
