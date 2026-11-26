package mpv

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClientAPIVersion(t *testing.T) {
	for _, tt := range []struct {
		version uint32
		valid   bool
	}{
		{0, false}, {0x100ff, false}, {0x20000, false}, {0x20001, true}, {0x200ff, true}, {0x30001, false},
	} {
		if err := checkAPIVersion(uint64(tt.version)); (err == nil) != tt.valid {
			t.Errorf("version=%x: %v", tt.version, err)
		}
	}
}

// Each loader case uses a fresh process; the first successful Load is resident
// for process lifetime, just as it is in the application.
func TestIntegrationLoaderRecovery(t *testing.T) {
	if os.Getenv("MUMU_MPV_TEST") == "" {
		t.Skip("set MUMU_MPV_TEST=1 for native loader fixtures")
	}
	mode := os.Getenv("MUMU_LOADER_CHILD")
	if mode == "" {
		for _, scenario := range []string{"missing", "architecture", "api", "symbols"} {
			t.Run(scenario, func(t *testing.T) {
				cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestIntegrationLoaderRecovery$", "-test.v")
				cmd.Env = append(os.Environ(), "MUMU_LOADER_CHILD="+scenario)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("loader subprocess: %v\n%s", err, output)
				} else {
					t.Log(string(output))
				}
			})
		}
		return
	}

	path := filepath.Join(t.TempDir(), libnames[0])
	want := ErrNotFound
	switch mode {
	case "architecture":
		if err := os.WriteFile(path, []byte("not a native library"), 0o600); err != nil {
			t.Fatal(err)
		}
		want = ErrArchitecture
	case "api":
		path = compileLibrary(t, "unsigned long mpv_client_api_version(void) { return 0x10000; }", "wrong-api")
		want = ErrUnsupportedAPI
	case "symbols":
		path = compileLibrary(t, "unsigned long mpv_client_api_version(void) { return 0x20001; }", "missing-symbols")
		want = nil
	}
	for range 2 {
		err := Load([]string{path})
		if err == nil {
			t.Fatal("invalid runtime loaded")
		}
		if want != nil && !errors.Is(err, want) {
			t.Fatalf("error=%v, want %v", err, want)
		}
		if want == nil && !strings.Contains(err.Error(), "entry points") {
			t.Fatalf("missing-symbol error=%v", err)
		}
		if Loaded() || New() != nil || libmpv != 0 || create != nil {
			t.Fatal("failed registration published runtime state")
		}
	}
	// Windows refuses removing a DLL that still has an open loader reference.
	if mode != "missing" {
		if err := os.Remove(path); err != nil {
			t.Fatalf("failed runtime was not released: %v", err)
		}
	}
	candidates := runtimeCandidates(t)
	if err := Load(candidates); err != nil {
		t.Fatalf("retry with valid runtime: %v", err)
	}
	client := New()
	if client == nil {
		t.Fatal("no client after recovery")
	}
	defer client.TerminateDestroy()
	for key, value := range map[string]string{"config": "no", "vo": "null", "ao": "null"} {
		if err := client.SetOptionString(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Logf("runtime version: %s, FFmpeg: %s", client.GetPropertyString("mpv-version"), client.GetPropertyString("ffmpeg-version"))
	name, version := RuntimeInfo()
	t.Logf("HEADLESS loader recovery: OS=%s arch=%s client API=%d.%d library=%s", runtime.GOOS, runtime.GOARCH, version>>16, version&0xffff, name)
}

func runtimeCandidates(t *testing.T) []string {
	t.Helper()
	value := os.Getenv("MUMU_MPV_LIBRARY")
	if value == "" {
		t.Fatal("native loader test requires explicit MUMU_MPV_LIBRARY provenance")
	}
	candidates := []string{}
	for _, path := range filepath.SplitList(value) {
		path, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal("runtime override is unavailable")
		}
		if !info.IsDir() {
			candidates = append(candidates, path)
			continue
		}
		for _, name := range libnames {
			candidates = append(candidates, filepath.Join(path, name))
		}
	}
	return candidates
}

func compileLibrary(t *testing.T, source, name string) string {
	t.Helper()
	dir := t.TempDir()
	ext, flag, compiler := ".so", "-shared", "cc"
	if runtime.GOOS == "darwin" {
		ext, flag = ".dylib", "-dynamiclib"
	}
	if runtime.GOOS == "windows" {
		ext, compiler = ".dll", "gcc"
		source = "__declspec(dllexport) " + source
	}
	input, output := filepath.Join(dir, "fixture.c"), filepath.Join(dir, name+ext)
	if err := os.WriteFile(input, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), compiler, flag, "-o", output, input)
	if log, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile loader fixture: %v\n%s", err, log)
	}
	return output
}
