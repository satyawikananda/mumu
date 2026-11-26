package mpv

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ErrArchitecture means the runtime cannot execute in this process.
var ErrArchitecture = errors.New("libmpv runtime architecture mismatch")

// Bare Unix names are resolved and checked by dlopen. Explicit files are
// checked before loading, which also avoids executing an incompatible DLL.
func validateArchitecture(path string) error {
	if !filepath.IsAbs(path) {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: runtime file unavailable", ErrNotFound)
	}
	defer func() { _ = file.Close() }()
	valid := false
	switch runtime.GOOS {
	case "windows":
		binary, parseErr := pe.NewFile(file)
		valid = parseErr == nil && binary.Machine == pe.IMAGE_FILE_MACHINE_AMD64 && runtime.GOARCH == "amd64"
	case "linux":
		binary, parseErr := elf.NewFile(file)
		valid = parseErr == nil && binary.Class == elf.ELFCLASS64 && binary.Machine == elf.EM_X86_64 && runtime.GOARCH == "amd64"
	case "darwin":
		binary, parseErr := macho.NewFile(file)
		cpu := macho.CpuArm64
		if runtime.GOARCH == "amd64" {
			cpu = macho.CpuAmd64
		}
		if parseErr == nil {
			valid = binary.Cpu == cpu
		} else if fat, fatErr := macho.NewFatFile(file); fatErr == nil {
			for _, arch := range fat.Arches {
				if arch.Cpu == cpu {
					valid = true
				}
			}
		}
	}
	if !valid {
		return ErrArchitecture
	}
	return nil
}
