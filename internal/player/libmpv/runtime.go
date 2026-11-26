//go:build (darwin && arm64) || ((windows || linux) && amd64)

package libmpv

import (
	"fmt"
	"log/slog"
	"os"

	mpv "github.com/gen2brain/go-mpv"

	"mumu/internal/player"
	"mumu/internal/player/libmpv/library"
)

func openRuntime() error {
	// A loaded runtime remains resident for all clients in this process.
	// Do not resolve again and misreport a different candidate on later New calls.
	if mpv.Loaded() {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: locate executable", player.ErrRuntimeUnavailable)
	}
	_, err = library.Resolve(library.Search{
		Override: library.FromEnv(os.Getenv),
		Bundle:   library.BundleDirs(executable),
		Open:     func(candidate string) error { return mpv.Load([]string{candidate}) },
	})
	if err != nil {
		return fmt.Errorf("%w: %w", player.ErrRuntimeUnavailable, err)
	}
	name, version := mpv.RuntimeInfo()
	slog.Debug(
		"libmpv runtime ready",
		"library", name,
		"client_api_major", version>>16,
		"client_api_minor", version&0xffff,
	)
	return nil
}
