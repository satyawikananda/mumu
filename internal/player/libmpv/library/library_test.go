//go:build darwin

package library_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"mumu/internal/player/libmpv/library"
)

func searchFixture(t *testing.T, exists func(string) bool, open func(string) error) library.Search {
	t.Helper()

	return library.Search{
		Names:  []string{"libmpv.dylib", "libmpv.2.dylib"},
		Dirs:   []string{"/opt/homebrew/lib", "/usr/local/lib"},
		Bundle: []string{"/Applications/Mumu.app/Contents/Resources"},
		Exists: exists,
		Open:   open,
	}
}

// A packaged app prefers its bundled runtime over the dynamic-loader path.
func TestResolvePrefersBundledRuntime(t *testing.T) {
	bundleLib := "/Applications/Mumu.app/Contents/Resources/libmpv.dylib"

	var opened []string

	got, err := library.Resolve(searchFixture(t,
		func(path string) bool { return path == bundleLib },
		func(candidate string) error {
			opened = append(opened, candidate)
			if candidate == bundleLib {
				return nil
			}

			return errors.New("dlopen failed")
		},
	))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != bundleLib {
		t.Fatalf("candidate %q, want the bundled runtime", got)
	}
	if len(opened) != 1 {
		t.Fatalf("opened %v, want one attempt", opened)
	}
}

// Homebrew's prefix comes before /usr/local, and the bundle comes last.
func TestResolveInstallOrder(t *testing.T) {
	exists := func(path string) bool {
		return path == "/opt/homebrew/lib/libmpv.2.dylib" || path == "/usr/local/lib/libmpv.dylib"
	}
	open := func(candidate string) error {
		if !strings.HasPrefix(candidate, "/opt/homebrew/") {
			return errors.New("dlopen failed")
		}

		return nil
	}

	got, err := library.Resolve(searchFixture(t, exists, open))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/opt/homebrew/lib/libmpv.2.dylib" {
		t.Fatalf("candidate %q, want the Homebrew copy", got)
	}
}

// The bundled runtime is tried before Homebrew and /usr/local when present.
func TestResolveBundleBeforeInstallDirs(t *testing.T) {
	bundleLib := "/Applications/Mumu.app/Contents/Resources/libmpv.dylib"
	brew := "/opt/homebrew/lib/libmpv.dylib"

	var opened []string

	got, err := library.Resolve(searchFixture(t,
		func(path string) bool { return path == bundleLib || path == brew },
		func(candidate string) error {
			opened = append(opened, candidate)
			if candidate == bundleLib {
				return nil
			}

			return errors.New("dlopen failed")
		},
	))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != bundleLib {
		t.Fatalf("candidate %q, want the bundled copy", got)
	}
	if len(opened) == 0 || opened[0] != bundleLib {
		t.Fatalf("opened %v, want the bundle first", opened)
	}
}

// A bundle candidate rejected by the loader falls through to the next one.
func TestResolveFallsBackWhenBundleFails(t *testing.T) {
	brew := "/opt/homebrew/lib/libmpv.dylib"

	got, err := library.Resolve(library.Search{
		Names:  []string{"libmpv.dylib"},
		Dirs:   []string{"/opt/homebrew/lib"},
		Bundle: []string{"/Applications/Mumu.app/Contents/Resources"},
		Exists: func(string) bool { return true },
		Open: func(candidate string) error {
			if candidate == brew {
				return nil
			}

			return errors.New("dlopen failed")
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != brew {
		t.Fatalf("candidate %q, want the fallback copy", got)
	}
}

// Directories that hold no library are skipped without a loader call.
func TestResolveSkipsMissingFiles(t *testing.T) {
	var opened []string

	_, err := library.Resolve(searchFixture(t,
		func(string) bool { return false },
		func(candidate string) error {
			opened = append(opened, candidate)

			return errors.New("dlopen failed")
		},
	))
	if !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
	if len(opened) != 2 {
		t.Fatalf("opened %v, want only the two loader names", opened)
	}
}

// A loader failure is reported without naming user directories.
func TestResolveMissingLibraryError(t *testing.T) {
	detail := errors.New(`dlopen(/Users/someone/private/libmpv.dylib, 0x0005): image not found`)

	_, err := library.Resolve(searchFixture(t,
		func(string) bool { return true },
		func(string) error { return detail },
	))
	if !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}

	message := err.Error()
	if strings.Contains(message, "/Users") || strings.Contains(message, "private") {
		t.Fatalf("message leaks a path: %q", message)
	}
	if !strings.Contains(message, "libmpv.dylib") || !strings.Contains(message, "DYLD_LIBRARY_PATH") {
		t.Fatalf("message is not actionable: %q", message)
	}

	var notFound *library.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("error %v, want NotFoundError", err)
	}
	if len(notFound.Tried) == 0 {
		t.Fatal("NotFoundError lists no candidates")
	}
	for _, candidate := range notFound.Tried {
		if strings.Contains(candidate, "/") {
			t.Fatalf("candidate %q keeps a directory", candidate)
		}
	}
}

// A loader that cannot be injected is reported, not panicked.
func TestResolveWithoutLoader(t *testing.T) {
	_, err := library.Resolve(library.Search{})
	if !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
}

func TestCandidatesOrder(t *testing.T) {
	got := library.Candidates(
		[]string{"a.dylib"},
		[]string{"/opt/homebrew/lib", "/usr/local/lib"},
		[]string{"/app/Resources"},
	)

	want := []string{
		"a.dylib",
		"/opt/homebrew/lib/a.dylib",
		"/usr/local/lib/a.dylib",
		"/app/Resources/a.dylib",
	}
	if len(got) != len(want) {
		t.Fatalf("candidates %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A packaged app searches the bundled library directory before the Resources
// root, so an app that ships both keeps using its own runtime.
func TestCandidatesBundleLibraryDirFirst(t *testing.T) {
	got := library.Candidates(
		[]string{"libmpv.dylib"},
		[]string{"/opt/homebrew/lib"},
		[]string{"/app/Contents/Resources/libmpv", "/app/Contents/Resources"},
	)

	want := []string{
		"libmpv.dylib",
		"/opt/homebrew/lib/libmpv.dylib",
		"/app/Contents/Resources/libmpv/libmpv.dylib",
		"/app/Contents/Resources/libmpv.dylib",
	}
	if len(got) != len(want) {
		t.Fatalf("candidates %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// The default search uses the documented Homebrew prefixes in order.
func TestCandidatesDefaults(t *testing.T) {
	got := library.Candidates(nil, nil, nil)

	if got[0] != library.Names[0] {
		t.Fatalf("first candidate %q, want %q", got[0], library.Names[0])
	}
	if got[len(library.Names)] != "/opt/homebrew/lib/"+library.Names[0] {
		t.Fatalf("second location %q, want the Homebrew prefix", got[len(library.Names)])
	}
	if !contains(got, "/usr/local/lib/"+library.Names[1]) {
		t.Fatalf("candidates %v, want the /usr/local copy", got)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// An explicit library directory overrides every search path.
func TestResolveOverrideFirst(t *testing.T) {
	var opened []string

	got, err := library.Resolve(library.Search{
		Override: []string{"/runtime/lib/libmpv.dylib"},
		Names:    []string{"libmpv.dylib"},
		Dirs:     []string{"/opt/homebrew/lib"},
		Exists:   func(string) bool { return true },
		Open: func(candidate string) error {
			opened = append(opened, candidate)

			return nil
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/runtime/lib/libmpv.dylib" {
		t.Fatalf("candidate %q, want the override", got)
	}
	if len(opened) != 1 {
		t.Fatalf("opened %v, want only the override", opened)
	}
}

// MUMU_MPV_LIBRARY accepts a directory or an explicit file.
func TestFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty", value: "", want: nil},
		{name: "directory", value: "/private/tmp/mumu-runtime/lib", want: []string{
			"/private/tmp/mumu-runtime/lib/libmpv.dylib",
			"/private/tmp/mumu-runtime/lib/libmpv.2.dylib",
		}},
		{name: "missing directory expands", value: "/nope", want: []string{
			"/nope/libmpv.dylib",
			"/nope/libmpv.2.dylib",
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := library.FromEnv(func(key string) string {
				if key != library.EnvLibrary {
					t.Fatalf("read %q, want %q", key, library.EnvLibrary)
				}

				return test.value
			})

			if len(got) != len(test.want) {
				t.Fatalf("candidates %v, want %v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("candidate %d = %q, want %q", i, got[i], test.want[i])
				}
			}
		})
	}
}

// A path that exists is used as given, so a renamed dylib still works.
func TestFromEnvFile(t *testing.T) {
	file := t.TempDir() + "/libmpv.dylib"
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := library.FromEnv(func(string) string { return file })
	if len(got) != 1 || got[0] != file {
		t.Fatalf("candidates %v, want the file path", got)
	}
}
