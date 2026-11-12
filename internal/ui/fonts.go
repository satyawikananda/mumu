package ui

import (
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/ui"
)

// The revamp bundles two OFL families: Inter Tight for body text and Lexend
// for display. The files are static Latin faces so weight matching is stable
// across macOS text stacks.
var (
	//go:embed fonts/InterTight-400.ttf
	interTightRegular []byte
	//go:embed fonts/InterTight-500.ttf
	interTightMedium []byte
	//go:embed fonts/InterTight-600.ttf
	interTightSemiBold []byte
	//go:embed fonts/Lexend-400.ttf
	lexendRegular []byte
	//go:embed fonts/Lexend-500.ttf
	lexendMedium []byte
	//go:embed fonts/Lexend-600.ttf
	lexendSemiBold []byte
	//go:embed fonts/Lexend-700.ttf
	lexendBold []byte
)

var (
	fontsOnce sync.Once
	fontsErr  error
	fontsOK   atomic.Bool
)

// RegisterFonts registers the bundled families. Call it once from the
// composition root before opening a window. A registration failure leaves the
// app on the native system font instead of failing startup.
func RegisterFonts() error {
	fontsOnce.Do(func() {
		faces := []struct {
			name   string
			family string
			data   []byte
		}{
			{"Inter Tight regular", bodyFamily, interTightRegular},
			{"Inter Tight medium", bodyFamily, interTightMedium},
			{"Inter Tight semibold", bodyFamily, interTightSemiBold},
			{"Lexend regular", displayFamily, lexendRegular},
			{"Lexend medium", displayFamily, lexendMedium},
			{"Lexend semibold", displayFamily, lexendSemiBold},
			{"Lexend bold", displayFamily, lexendBold},
		}
		for _, face := range faces {
			if err := ui.RegisterFont(face.data, face.family); err != nil {
				fontsErr = fmt.Errorf("register %s: %w", face.name, err)
				return
			}
		}
		fontsOK.Store(true)
	})

	return fontsErr
}

// Body family and display family names.
const (
	bodyFamily    = "Inter Tight"
	displayFamily = "Lexend"
)

// bodyFont returns the registered body family, or the system font when the
// bundled fonts could not load.
func bodyFont() string {
	if fontsOK.Load() {
		return bodyFamily
	}

	return ""
}

// displayFont returns the display family with the body family as fallback.
func displayFont() string {
	if fontsOK.Load() {
		return displayFamily + ", " + bodyFamily
	}

	return ""
}
