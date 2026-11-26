package libmpv_test

import (
	"go/build"
	"path/filepath"
	"testing"
)

func TestEnginePlatformSelection(t *testing.T) {
	for _, target := range []struct {
		os, arch string
		real     bool
	}{
		{"darwin", "arm64", true},
		{"linux", "amd64", true},
		{"windows", "amd64", true},
		{"darwin", "amd64", false},
		{"linux", "arm64", false},
		{"windows", "arm64", false},
	} {
		t.Run(target.os+"/"+target.arch, func(t *testing.T) {
			ctx := build.Default
			ctx.GOOS, ctx.GOARCH = target.os, target.arch
			for _, file := range []struct {
				path string
				want bool
			}{
				{"engine_mpv.go", target.real},
				{"engine_stub.go", !target.real},
				{"media_proxy.go", target.real},
				{"runtime.go", target.real},
				{"../../app/engine_native.go", target.real},
				{"../../app/engine_stub.go", !target.real},
				{"render_mpv.go", target.os == "darwin" && target.arch == "arm64"},
			} {
				ok, err := ctx.MatchFile(filepath.Dir(file.path), filepath.Base(file.path))
				if err != nil || ok != file.want {
					t.Errorf("%s selected=%v, error=%v; want %v", file.path, ok, err, file.want)
				}
			}
		})
	}
}
