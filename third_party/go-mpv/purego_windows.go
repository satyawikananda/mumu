//go:build windows

package mpv

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// libnames are the libmpv DLL names used by various distributions, tried in
// order when Load receives no candidates.
var libnames = []string{"libmpv-2.dll", "libmpv.dll", "mpv-2.dll", "mpv-1.dll"}

// Locale categories for SetLocale (Windows CRT values).
const (
	LCAll     = 0
	LCNumeric = 4
)

// openLibrary opens the first candidate the loader resolves. A candidate is
// an absolute DLL path. Bare names and relative paths are rejected instead
// of panicking.
func openLibrary(candidates []string) (uintptr, error) {
	errs := make([]error, 0, len(candidates))
	for _, candidate := range candidates {
		if !filepath.IsAbs(candidate) {
			errs = append(errs, errors.New("Windows runtime path must be absolute"))
			continue
		}
		if err := validateArchitecture(candidate); err != nil {
			errs = append(errs, err)
			continue
		}
		handle, err := windows.LoadLibraryEx(candidate, 0,
			windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
		if err == nil {
			return uintptr(handle), nil
		}

		errs = append(errs, errors.New("libmpv runtime or dependency could not be loaded"))
	}

	return 0, errors.Join(errs...)
}

var memoryHandle windows.Handle

// crtnames are the C runtime DLLs that export malloc/free, tried in order.
var crtnames = []string{"ucrtbase.dll", "msvcrt.dll"}

// memLibrary loads the C runtime to resolve malloc/free, which libmpv does not
// export. A failure returns 0 and registration reports it.
func memLibrary() uintptr {
	for _, name := range crtnames {
		handle, err := windows.LoadLibraryEx(name, 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
		if err == nil {
			memoryHandle = handle
			return uintptr(handle)
		}
	}

	return 0
}

func closeLibrary(handle uintptr) error {
	if err := windows.FreeLibrary(windows.Handle(handle)); err != nil {
		return fmt.Errorf("release libmpv: %w", err)
	}
	return nil
}

func closeMemory() {
	if memoryHandle != 0 {
		_ = windows.FreeLibrary(memoryHandle)
		memoryHandle = 0
	}
}
