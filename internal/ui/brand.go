package ui

import (
	_ "embed"
	"log/slog"

	"github.com/egoist/mygo/ui"
)

// ProductName is the user-facing product title used by the shell, Settings
// and the native windows.
const ProductName = "Mumu TV"

//go:embed icons/tomato.png
var tomatoPNG []byte

// brandMark decodes the embedded tomato icon, or returns nil if it is unreadable.
func brandMark() *ui.Bitmap {
	mark, err := ui.DecodeBitmap(tomatoPNG)
	if err != nil {
		slog.Warn("brand mark unavailable", "error", err)
		return nil
	}

	return mark
}
