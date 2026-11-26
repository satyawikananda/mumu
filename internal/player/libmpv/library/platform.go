package library

import (
	"path/filepath"
	"runtime"
	"strings"
)

func platformNames(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"libmpv.dylib", "libmpv.2.dylib"}
	case "windows":
		return []string{"libmpv-2.dll", "libmpv.dll", "mpv-2.dll", "mpv-1.dll"}
	case "linux":
		return []string{"libmpv.so.2", "libmpv.so"}
	default:
		return []string{}
	}
}

func platformDirs(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/opt/homebrew/lib", "/usr/local/lib"}
	case "linux":
		return []string{"/usr/local/lib", "/usr/lib", "/lib"}
	default:
		return []string{}
	}
}

// BundleDirs uses only the executable location, never the working directory.
// Non-macOS resources sit beside the executable, as in MyGo's packaged layout.
func BundleDirs(executable string) []string {
	if executable == "" {
		return nil
	}
	root := filepath.Dir(executable)
	if runtime.GOOS == "darwin" {
		root = filepath.Join(root, "..", "Resources")
	}
	return []string{filepath.Join(root, "libmpv"), root}
}

func loaderHint() string {
	switch runtime.GOOS {
	case "darwin":
		return " (or DYLD_LIBRARY_PATH)"
	case "linux":
		return " (or LD_LIBRARY_PATH)"
	default:
		return ""
	}
}

func isLibraryFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".dll") || strings.HasSuffix(base, ".dylib") ||
		strings.HasSuffix(base, ".so") || strings.Contains(base, ".so.")
}
