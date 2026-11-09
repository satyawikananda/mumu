package ui

import (
	_ "embed"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed icons/watch.svg
var watchSVG []byte

//go:embed icons/discover.svg
var discoverSVG []byte

//go:embed icons/library.svg
var librarySVG []byte

//go:embed icons/settings.svg
var settingsSVG []byte

// iconSet holds the shell's parsed icons.
type iconSet struct {
	watch    *ui.SVG
	discover *ui.SVG
	library  *ui.SVG
	settings *ui.SVG
}

// icons parses the embedded SVG icons once.
var icons = sync.OnceValue(func() iconSet {
	return iconSet{
		watch:    ui.MustParseSVG(watchSVG),
		discover: ui.MustParseSVG(discoverSVG),
		library:  ui.MustParseSVG(librarySVG),
		settings: ui.MustParseSVG(settingsSVG),
	}
})
