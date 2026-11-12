package ui

import (
	_ "embed"
	"log/slog"

	"github.com/egoist/mygo/ui"
)

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
