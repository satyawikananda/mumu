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

// brandRow draws the tomato mark beside the wordmark.
func brandRow(c *ui.Context, mark *ui.Bitmap) {
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		drawBrandMark(c, mark)
		ui.Text(c, "Mumu").FontSize(c.Theme().Rem(1.5)).FontWeight(600)
	})
}

// drawBrandMark draws the tomato mark at 24 pt, when it decoded.
func drawBrandMark(c *ui.Context, mark *ui.Bitmap) {
	if mark == nil {
		return
	}

	ui.Image(c, mark).Size(24, 24)
}
