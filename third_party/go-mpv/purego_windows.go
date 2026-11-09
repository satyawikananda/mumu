//go:build windows

package mpv

import (
	"errors"

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
// either a bare DLL name or an absolute path. It returns every failure instead
// of panicking.
func openLibrary(candidates []string) (uintptr, error) {
	errs := make([]error, 0, len(candidates))
	for _, candidate := range candidates {
		handle, err := windows.LoadLibrary(candidate)
		if err == nil {
			return uintptr(handle), nil
		}

		errs = append(errs, err)
	}

	return 0, errors.Join(errs...)
}

// crtnames are the C runtime DLLs that export malloc/free, tried in order.
var crtnames = []string{"ucrtbase.dll", "msvcrt.dll"}

// memLibrary loads the C runtime to resolve malloc/free, which libmpv does not
// export. A failure returns 0 and registration reports it.
func memLibrary() uintptr {
	for _, name := range crtnames {
		handle, err := windows.LoadLibrary(name)
		if err == nil {
			return uintptr(handle)
		}
	}

	return 0
}
