package library

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestPlatformNames(t *testing.T) {
	for _, tt := range []struct {
		os   string
		want []string
	}{
		{"darwin", []string{"libmpv.dylib", "libmpv.2.dylib"}},
		{"linux", []string{"libmpv.so.2", "libmpv.so"}},
		{"windows", []string{"libmpv-2.dll", "libmpv.dll", "mpv-2.dll", "mpv-1.dll"}},
	} {
		if got := platformNames(tt.os); !slices.Equal(got, tt.want) {
			t.Errorf("%s names=%v, want %v", tt.os, got, tt.want)
		}
	}
}

func TestNativeSearchOrder(t *testing.T) {
	root := t.TempDir()
	override := filepath.Join(root, "override", Names[0])
	bundle := filepath.Join(root, "bundle")
	bundled := filepath.Join(bundle, Names[0])
	for _, useOverride := range []bool{true, false} {
		opened := []string{}
		search := Search{
			Bundle: []string{bundle},
			Exists: func(string) bool { return true },
			Open:   func(path string) error { opened = append(opened, path); return nil },
		}
		want := bundled
		if useOverride {
			search.Override = []string{override}
			want = override
		}
		got, err := Resolve(search)
		if err != nil || got != want || len(opened) != 1 {
			t.Fatalf("Resolve=%q, %v, opened=%v; want %q", got, err, opened, want)
		}
	}
}

func TestPathListAndBundle(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	got := FromEnv(func(string) string { return first + string(os.PathListSeparator) + second })
	want := append(joinCandidates(Names, []string{first}), joinCandidates(Names, []string{second})...)
	if !slices.Equal(got, want) {
		t.Fatalf("override=%v, want %v", got, want)
	}
	exe := filepath.Join(first, "mumu")
	dirs := BundleDirs(exe)
	root := first
	if runtime.GOOS == "darwin" {
		root = filepath.Join(first, "..", "Resources")
	}
	if !slices.Equal(dirs, []string{filepath.Join(root, "libmpv"), root}) {
		t.Fatalf("bundle=%v", dirs)
	}
}

func TestDiagnosticsDoNotExposePaths(t *testing.T) {
	for _, detail := range []string{
		`dlopen(/Users/secret person/private/libmpv.dylib): missing dependency`,
		`LoadLibrary C:\Users\secret person\libmpv-2.dll: wrong architecture`,
		`\\secret-server\private share\libmpv.dll: missing symbol`,
	} {
		_, err := Resolve(Search{Open: func(string) error { return errors.New(detail) }})
		if err == nil {
			t.Fatal("missing error")
		}
		for _, secret := range []string{"secret", "private", `C:\`, "/Users", `\\`} {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("leaked diagnostic: %s", err)
			}
		}
	}
}

func TestWindowsNeverSearchesImplicitPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows loader policy")
	}
	opened := []string{}
	_, err := Resolve(Search{Open: func(path string) error { opened = append(opened, path); return errors.New("missing") }})
	if err == nil || len(opened) != 0 {
		t.Fatalf("implicit paths opened: %v, %v", opened, err)
	}
	_, err = Resolve(Search{Override: []string{"relative/libmpv.dll"}, Open: func(string) error { t.Fatal("relative override opened"); return nil }})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative override error: %v", err)
	}
}

func TestResolveClassifiesLoaderFailures(t *testing.T) {
	for _, tt := range []struct {
		detail string
		want   string
	}{
		{"dlopen: wrong architecture", "runtime architecture"},
		{"unsupported libmpv client API version: runtime reports 1.0", "incompatible libmpv client API"},
		{"libmpv runtime is missing required entry points", "missing a required entry point"},
		{"The specified module could not be found.", "could not load runtime"},
	} {
		override := filepath.Join(t.TempDir(), "libmpv")
		_, err := Resolve(Search{
			Override: []string{override},
			Exists:   func(string) bool { return true },
			Open:     func(string) error { return errors.New(tt.detail) },
		})
		if err == nil {
			t.Fatalf("%q: missing error", tt.detail)
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%q: detail=%q, want %q", tt.detail, err, tt.want)
		}
		if strings.Contains(err.Error(), tt.detail) {
			t.Errorf("%q: raw native diagnostic echoed", tt.detail)
		}
	}
}

func TestResolveFallsBackToNextCandidate(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first-libmpv")
	second := filepath.Join(t.TempDir(), "second-libmpv")
	_, err := Resolve(Search{
		Override: []string{first, second},
		Exists:   func(string) bool { return true },
		Open: func(candidate string) error {
			if candidate == second {
				return nil
			}
			return errors.New("missing dependency")
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}
