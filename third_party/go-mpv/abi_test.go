package mpv

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// Compile against the headers shipped by the pinned runtime build. This checks
// LP64 versus LLP64, alignment and offsets at the actual C ABI boundary.
func TestIntegrationABI(t *testing.T) {
	if os.Getenv("MUMU_MPV_TEST") == "" {
		t.Skip("set MUMU_MPV_TEST=1 for native ABI verification")
	}
	include := os.Getenv("MUMU_MPV_INCLUDE")
	if include == "" {
		t.Fatal("MUMU_MPV_INCLUDE must identify the pinned runtime headers")
	}
	dir := t.TempDir()
	source, executable := filepath.Join(dir, "abi.c"), filepath.Join(dir, "abi")
	compiler := "cc"
	if runtime.GOOS == "windows" {
		compiler = "gcc"
		executable += ".exe"
	}
	program := `#include <stdio.h>
#include <stddef.h>
#include <mpv/client.h>
int main(void) {
 printf("%zu %zu %zu %zu %zu %zu %zu %zu %zu %zu\n",
 sizeof(unsigned long), sizeof(mpv_event), offsetof(mpv_event, data),
 sizeof(mpv_node), offsetof(mpv_node, format), sizeof(mpv_node_list),
 offsetof(mpv_node_list, values), sizeof(mpv_byte_array),
 sizeof(mpv_event_end_file), offsetof(mpv_event_end_file, playlist_entry_id));
 return 0;
}`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), compiler, "-I", include, source, "-o", executable)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile ABI probe: %v\n%s", err, output)
	}
	output, err := exec.CommandContext(t.Context(), executable).Output()
	if err != nil {
		t.Fatal(err)
	}
	want := []uintptr{
		unsafe.Sizeof(apiVersionValue(0)), unsafe.Sizeof(event{}), unsafe.Offsetof(event{}.Data),
		unsafe.Sizeof(cNode{}), unsafe.Offsetof(cNode{}.format), unsafe.Sizeof(cNodeList{}),
		unsafe.Offsetof(cNodeList{}.values), unsafe.Sizeof(cByteArray{}),
		unsafe.Sizeof(eventEndFile{}), unsafe.Offsetof(eventEndFile{}.EntryID),
	}
	fields := strings.Fields(string(output))
	if len(fields) != len(want) {
		t.Fatalf("ABI probe output: %q", output)
	}
	for i, value := range fields {
		got, err := strconv.ParseUint(value, 10, 64)
		if err != nil || got != uint64(want[i]) {
			t.Errorf("ABI field %d: C=%s Go=%d", i, value, want[i])
		}
	}
	t.Logf("native ABI verified: %s/%s sizes and offsets=%v", runtime.GOOS, runtime.GOARCH, fields)
}
