// Package library locates the libmpv runtime on disk.
//
// It decides which candidate to open and leaves loading to the caller, so the
// search order is testable with injected functions and without a real runtime.
// The adapter that owns the runtime is internal/player/libmpv.
package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// ErrNotFound reports that no candidate could be opened.
var ErrNotFound = errors.New("libmpv runtime not found")

// Names are the library file names, in preference order, for the running
// platform: Homebrew's dylib names, the ELF sonames, or the DLL names used by
// the official Windows mpv builds.
var Names = platformNames(runtime.GOOS)

// Dirs are the install locations searched after the dynamic loader's own
// search path, in order. Windows has none: its loader must not search PATH or
// the working directory, so only the executable's directory is used.
var Dirs = platformDirs(runtime.GOOS)

// EnvLibrary names a runtime chosen explicitly. Its value is a library file, or
// one or more directories that contain one, separated by os.PathListSeparator
// (':' on macOS and Linux, ';' on Windows). It exists because the go command
// drops DYLD_LIBRARY_PATH from the environment of the binaries it runs, so
// `go test` cannot use the loader's own search path.
const EnvLibrary = "MUMU_MPV_LIBRARY"

// Search describes one discovery attempt. The zero value is usable.
type Search struct {
	// Override lists candidates that win over every search path. It is meant
	// for EnvLibrary, so an explicit choice is never overridden silently.
	Override []string
	// Names and Dirs default to the package values when empty.
	Names []string
	Dirs  []string
	// Bundle lists directories resolved next to the executable. A packaged app
	// ships its own verified runtime there, so an existing bundle candidate is
	// preferred over the loader path and the installation directories.
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
	// Detail classifies the loader's reason without echoing native diagnostics,
	// which can embed paths with spaces, drive letters or UNC names.
	Detail string
}

// Error reports the failure with an actionable next step.
func (e *NotFoundError) Error() string {
	tried := e.Tried

	message := "libmpv runtime not found"
	if len(tried) > 0 {
		message += " (tried " + strings.Join(tried, ", ") + ")"
	}

	name := "libmpv"
	if len(Names) > 0 {
		name = Names[0]
	}
	if len(tried) > 0 {
		name = tried[0]
	}

	message += "; reinstall mumu, or set " + EnvLibrary +
		loaderHint() + " to the directory containing " + name

	if e.Detail != "" {
		message += ": " + e.Detail
	}

	return message
}

// Unwrap reports ErrNotFound for errors.Is.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// Resolve opens the first candidate that loads.
//
// A packaged app that carries a runtime next to its executable uses that
// bundled copy before the dynamic-loader path and the installation
// directories, so a Homebrew or system libmpv cannot shadow the build the
// release verified. A dev build without a bundle keeps the loader-first order.
// The Override always wins. On Windows every candidate must be absolute.
// It returns the candidate that loaded.
func Resolve(s Search) (string, error) {
	if s.Open == nil {
		return "", fmt.Errorf("%w: no loader given", ErrNotFound)
	}

	exists := s.Exists
	if exists == nil {
		exists = fileExists
	}

	names := s.Names
	if len(names) == 0 {
		names = Names
	}
	dirs := s.Dirs
	if len(dirs) == 0 {
		dirs = Dirs
	}

	bundle := joinCandidates(names, s.Bundle)
	rest := joinCandidates(names, dirs)
	if runtime.GOOS != "windows" {
		rest = slices.Concat(names, rest)
	}

	candidates := slices.Concat(s.Override, rest)
	if anyCandidateExists(bundle, exists) {
		candidates = slices.Concat(s.Override, bundle, rest)
	}

	details := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if runtime.GOOS == "windows" && !filepath.IsAbs(candidate) {
			return "", &NotFoundError{Detail: "Windows runtime paths must be absolute; set " + EnvLibrary}
		}
		if filepath.IsAbs(candidate) && !exists(candidate) {
			continue
		}
		if err := s.Open(candidate); err != nil {
			detail := safeDetail(err)
			if !slices.Contains(details, detail) {
				details = append(details, detail)
			}

			continue
		}

		return candidate, nil
	}

	notFound := &NotFoundError{Tried: uniqueNames(candidates)}
	notFound.Detail = strings.Join(details, "; ")

	return "", notFound
}

// anyCandidateExists reports whether any absolute candidate already exists.
func anyCandidateExists(candidates []string, exists func(string) bool) bool {
	for _, candidate := range candidates {
		if exists(candidate) {
			return true
		}
	}

	return false
}

// joinCandidates lists every name under every directory.
func joinCandidates(names, dirs []string) []string {
	candidates := make([]string, 0, len(names)*len(dirs))
	for _, dir := range dirs {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	return candidates
}

// Candidates builds the default search order: bare loader names first, then
// every installation directory, then the bundle directories. Windows omits
// bare names because its loader must not search PATH or the working directory;
// only absolute bundle candidates are returned there.
func Candidates(names, dirs, bundle []string) []string {
	if len(names) == 0 {
		names = Names
	}
	if len(dirs) == 0 {
		dirs = Dirs
	}

	candidates := make([]string, 0, len(names)*(len(dirs)+len(bundle)+1))
	if runtime.GOOS != "windows" {
		candidates = append(candidates, names...)
	}
	candidates = append(candidates, joinCandidates(names, dirs)...)
	candidates = append(candidates, joinCandidates(names, bundle)...)

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
		if fileExists(entry) || isLibraryFile(entry) {
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

// safeDetail deliberately classifies loader errors instead of echoing arbitrary
// native diagnostics. Paths can contain spaces, quotes, drive letters or UNC
// names, so token-based path replacement cannot guarantee redaction.
func safeDetail(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "architecture"), strings.Contains(message, "wrong elf"), strings.Contains(message, "bad exe"):
		return "runtime architecture does not match this application"
	case strings.Contains(message, "client api"):
		return "incompatible libmpv client API (requires major 2, minor 1 or newer)"
	case strings.Contains(message, "symbol"), strings.Contains(message, "entry point"):
		return "runtime is missing a required entry point"
	default:
		return "could not load runtime or one of its dependencies"
	}
}
