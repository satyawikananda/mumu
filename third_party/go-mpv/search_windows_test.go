//go:build windows

package mpv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntegrationWindowsSearch(t *testing.T) {
	if os.Getenv("MUMU_MPV_TEST") == "" {
		t.Skip("set MUMU_MPV_TEST=1 for native DLL search test")
	}
	dir, untrusted := t.TempDir(), t.TempDir()
	dependency := filepath.Join(untrusted, "mumu-fixture-dependency.dll")
	importLibrary := filepath.Join(dir, "dependency.a")
	depSource := filepath.Join(dir, "dependency.c")
	mainSource := filepath.Join(dir, "main.c")
	library := filepath.Join(dir, "fixture.dll")
	if err := os.WriteFile(depSource, []byte("__declspec(dllexport) unsigned long fixture_version(void) { return 0x20001; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainSource, []byte("__declspec(dllimport) unsigned long fixture_version(void); __declspec(dllexport) unsigned long mpv_client_api_version(void) { return fixture_version(); }"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-shared", depSource, "-o", dependency, "-Wl,--out-implib," + importLibrary},
		{"-shared", mainSource, importLibrary, "-o", library},
	} {
		if output, err := exec.CommandContext(t.Context(), "gcc", args...).CombinedOutput(); err != nil {
			t.Fatalf("DLL fixture compile: %v\n%s", err, output)
		}
	}
	t.Chdir(untrusted)
	t.Setenv("PATH", untrusted)
	for _, candidate := range []string{filepath.Base(library), filepath.Base(dependency), library} {
		handle, err := openLibrary([]string{candidate})
		if err == nil {
			_ = closeLibrary(handle)
			t.Fatalf("loaded through unsafe search: %s", filepath.Base(candidate))
		}
	}
	// The identical dependency is allowed when explicitly beside the runtime.
	contents, err := os.ReadFile(dependency)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(dependency)), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	handle, err := openLibrary([]string{library})
	if err != nil {
		t.Fatalf("explicit dependency directory: %v", err)
	}
	if err := closeLibrary(handle); err != nil {
		t.Fatal(err)
	}
}
