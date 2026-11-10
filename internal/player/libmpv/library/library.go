// Package library locates the libmpv runtime on disk.
//
// It decides which candidate to open and leaves loading to the caller, so the
// search order is testable with injected functions and without a real dylib.
// The adapter that owns the runtime is internal/player/libmpv.
package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrNotFound reports that no candidate could be opened.
var ErrNotFound = errors.New("libmpv runtime not found")

// Names are the library file names, in preference order, as distributed by
// Homebrew and the official mpv builds.
var Names = []string{"libmpv.dylib", "libmpv.2.dylib"}

// Dirs are the install locations searched after the dynamic loader's own
// search path, in order.
var Dirs = []string{"/opt/homebrew/lib", "/usr/local/lib"}

// EnvLibrary names a runtime chosen explicitly. Its value is a libmpv file, or
// one or more ':'-separated directories that contain one. It exists because
// the go command drops DYLD_LIBRARY_PATH from the environment of the binaries
// it runs, so `go test` cannot use the loader's own search path.
const EnvLibrary = "MUMU_MPV_LIBRARY"

// Search describes one discovery attempt. The zero value is usable.
type Search struct {
	// Override lists candidates that win over every search path. It is meant
	// for EnvLibrary, so an explicit choice is never overridden silently.
	Override []string
	// Names and Dirs default to the package values when empty.
	Names []string
	Dirs  []string
	// Bundle lists directories resolved next to the executable. They are
	// searched last, and only when they exist.
	Bundle []string
	// Exists reports whether path names a file. Absolute candidates are
	// skipped when it returns false; nil uses os.Stat.
	Exists func(path string) bool
	// Open opens one candidate. It returns nil on success and must not panic.
	Open func(candidate string) error
}

// NotFoundError reports a search that opened nothing. It never contains
// absolute paths, because a bundled candidate can live under the user's home.
type NotFoundError struct {
	// Tried lists the library names in search order, without directories.
	Tried []string
	// Detail is the loader's reason with absolute paths reduced to base names.
	Detail string
}

// Error reports the failure with an actionable next step.
func (e *NotFoundError) Error() string {
	tried := e.Tried

	message := "libmpv runtime not found"
	if len(tried) > 0 {
		message += " (tried " + strings.Join(tried, ", ") + ")"
	}

	name := Names[0]
	if len(tried) > 0 {
		name = tried[0]
	}

	message += "; reinstall mumu, or set " + EnvLibrary +
		" (or DYLD_LIBRARY_PATH) to the directory containing " + name

	if e.Detail != "" {
		message += ": " + e.Detail
	}

	return message
}

// Unwrap reports ErrNotFound for errors.Is.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// Resolve opens the first candidate that loads, in this order: the dynamic
// loader's own search path, every installation directory, then the bundle
// directories. It returns the candidate that loaded.
func Resolve(s Search) (string, error) {
	if s.Open == nil {
		return "", fmt.Errorf("%w: no loader given", ErrNotFound)
	}

	exists := s.Exists
	if exists == nil {
		exists = fileExists
	}

	candidates := slices.Concat(s.Override, Candidates(s.Names, s.Dirs, s.Bundle))

	var detail error
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) && !exists(candidate) {
			continue
		}
		if err := s.Open(candidate); err != nil {
			detail = err

			continue
		}

		return candidate, nil
	}

	notFound := &NotFoundError{Tried: uniqueNames(candidates)}
	if detail != nil {
		notFound.Detail = compactPaths(detail.Error())
	}

	return "", notFound
}

// Candidates builds the search order: bare loader names first, then every
// directory, then the bundle directories.
func Candidates(names, dirs, bundle []string) []string {
	if len(names) == 0 {
		names = Names
	}
	if len(dirs) == 0 {
		dirs = Dirs
	}

	candidates := make([]string, 0, len(names)*(len(dirs)+len(bundle)+1))
	candidates = append(candidates, names...)
	for _, dir := range append(append([]string{}, dirs...), bundle...) {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	return candidates
}

// FromEnv returns the candidates named by EnvLibrary. Each entry is a libmpv
// file, or a directory that holds one under any of the known names.
func FromEnv(getenv func(string) string) []string {
	if getenv == nil {
		return nil
	}

	value := strings.TrimSpace(getenv(EnvLibrary))
	if value == "" {
		return nil
	}

	var candidates []string
	for _, entry := range strings.Split(value, string(os.PathListSeparator)) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if fileExists(entry) {
			candidates = append(candidates, entry)

			continue
		}
		for _, name := range Names {
			candidates = append(candidates, filepath.Join(entry, name))
		}
	}

	return candidates
}

// fileExists reports whether path names a file.
func fileExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Mode().IsRegular()
}

// uniqueNames returns the base names of candidates, deduplicated, in order.
func uniqueNames(candidates []string) []string {
	names := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))

	for _, candidate := range candidates {
		name := filepath.Base(candidate)
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}

	return names
}

// pathBoundaries are the characters that can precede the first slash of a
// path inside a loader message, such as the "(" in "dlopen(/opt/lib".
const pathBoundaries = "('\"=`["

// compactPaths rewrites absolute paths in a loader message to their base names,
// so the reason can be shown without exposing where the app was launched from.
func compactPaths(message string) string {
	fields := strings.Fields(message)

	for i, field := range fields {
		start := pathStart(field)
		if start < 0 {
			continue
		}

		rest := field[start:]
		trimmed := strings.TrimRight(rest, ",:;)\"'")
		if trimmed == "" || trimmed == "/" {
			continue
		}
		fields[i] = field[:start] + filepath.Base(trimmed) + rest[len(trimmed):]
	}

	return strings.Join(fields, " ")
}

// pathStart returns the index where an absolute path begins in field, or -1.
func pathStart(field string) int {
	for i := range len(field) {
		if field[i] != '/' {
			continue
		}
		if i == 0 || strings.ContainsRune(pathBoundaries, rune(field[i-1])) {
			return i
		}

		return -1
	}

	return -1
}
