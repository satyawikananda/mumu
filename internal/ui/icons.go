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

//go:embed icons/house.svg
var houseSVG []byte

//go:embed icons/search.svg
var searchSVG []byte

//go:embed icons/user.svg
var userSVG []byte

//go:embed icons/arrow-left.svg
var arrowLeftSVG []byte

//go:embed icons/plus.svg
var plusSVG []byte

//go:embed icons/globe.svg
var globeSVG []byte

//go:embed icons/tv.svg
var tvSVG []byte

//go:embed icons/heart.svg
var heartSVG []byte

//go:embed icons/volume-2.svg
var volumeSVG []byte

//go:embed icons/volume-x.svg
var muteSVG []byte

//go:embed icons/play.svg
var playSVG []byte

//go:embed icons/pause.svg
var pauseSVG []byte

//go:embed icons/picture-in-picture-2.svg
var pipSVG []byte

//go:embed icons/maximize.svg
var fullscreenSVG []byte

//go:embed icons/chevron-down.svg
var chevronDownSVG []byte

//go:embed icons/loader-circle.svg
var loaderSVG []byte

//go:embed icons/check.svg
var checkSVG []byte

//go:embed icons/x.svg
var xSVG []byte

//go:embed icons/list-video.svg
var listVideoSVG []byte

//go:embed icons/folder-open.svg
var folderOpenSVG []byte

//go:embed icons/clock.svg
var clockSVG []byte

//go:embed icons/signal.svg
var signalSVG []byte

//go:embed icons/wifi-off.svg
var offlineSVG []byte

//go:embed icons/triangle-alert.svg
var alertSVG []byte

//go:embed icons/refresh-cw.svg
var refreshSVG []byte

//go:embed icons/panel-left.svg
var panelLeftSVG []byte

//go:embed icons/monitor-play.svg
var monitorPlaySVG []byte

//go:embed icons/compass.svg
var compassSVG []byte

//go:embed icons/circle-dot.svg
var liveDotSVG []byte

//go:embed icons/rotate-ccw.svg
var rotateLeftSVG []byte

//go:embed icons/copy.svg
var copySVG []byte

//go:embed icons/external-link.svg
var externalLinkSVG []byte

//go:embed icons/trash-2.svg
var trashSVG []byte

//go:embed icons/eye.svg
var eyeSVG []byte

//go:embed icons/eye-off.svg
var eyeOffSVG []byte

//go:embed icons/sliders-horizontal.svg
var slidersSVG []byte

// iconSet holds the parsed Lucide icons used by the native UI.
type iconSet struct {
	watch      *ui.SVG
	discover   *ui.SVG
	library    *ui.SVG
	settings   *ui.SVG
	home       *ui.SVG
	search     *ui.SVG
	user       *ui.SVG
	arrowLeft  *ui.SVG
	plus       *ui.SVG
	globe      *ui.SVG
	tv         *ui.SVG
	heart      *ui.SVG
	volume     *ui.SVG
	mute       *ui.SVG
	play       *ui.SVG
	pause      *ui.SVG
	pip        *ui.SVG
	fullscreen *ui.SVG
	chevron    *ui.SVG
	loader     *ui.SVG
	check      *ui.SVG
	close      *ui.SVG
	listVideo  *ui.SVG
	folder     *ui.SVG
	clock      *ui.SVG
	signal     *ui.SVG
	offline    *ui.SVG
	alert      *ui.SVG
	refresh    *ui.SVG
	panelLeft  *ui.SVG
	monitor    *ui.SVG
	compass    *ui.SVG
	liveDot    *ui.SVG
	rotateLeft *ui.SVG
	copy       *ui.SVG
	external   *ui.SVG
	trash      *ui.SVG
	eye        *ui.SVG
	eyeOff     *ui.SVG
	sliders    *ui.SVG
}

// icons parses the embedded SVG icons once.
var icons = sync.OnceValue(func() iconSet {
	return iconSet{
		watch:      ui.MustParseSVG(watchSVG),
		discover:   ui.MustParseSVG(discoverSVG),
		library:    ui.MustParseSVG(librarySVG),
		settings:   ui.MustParseSVG(settingsSVG),
		home:       ui.MustParseSVG(houseSVG),
		search:     ui.MustParseSVG(searchSVG),
		user:       ui.MustParseSVG(userSVG),
		arrowLeft:  ui.MustParseSVG(arrowLeftSVG),
		plus:       ui.MustParseSVG(plusSVG),
		globe:      ui.MustParseSVG(globeSVG),
		tv:         ui.MustParseSVG(tvSVG),
		heart:      ui.MustParseSVG(heartSVG),
		volume:     ui.MustParseSVG(volumeSVG),
		mute:       ui.MustParseSVG(muteSVG),
		play:       ui.MustParseSVG(playSVG),
		pause:      ui.MustParseSVG(pauseSVG),
		pip:        ui.MustParseSVG(pipSVG),
		fullscreen: ui.MustParseSVG(fullscreenSVG),
		chevron:    ui.MustParseSVG(chevronDownSVG),
		loader:     ui.MustParseSVG(loaderSVG),
		check:      ui.MustParseSVG(checkSVG),
		close:      ui.MustParseSVG(xSVG),
		listVideo:  ui.MustParseSVG(listVideoSVG),
		folder:     ui.MustParseSVG(folderOpenSVG),
		clock:      ui.MustParseSVG(clockSVG),
		signal:     ui.MustParseSVG(signalSVG),
		offline:    ui.MustParseSVG(offlineSVG),
		alert:      ui.MustParseSVG(alertSVG),
		refresh:    ui.MustParseSVG(refreshSVG),
		panelLeft:  ui.MustParseSVG(panelLeftSVG),
		monitor:    ui.MustParseSVG(monitorPlaySVG),
		compass:    ui.MustParseSVG(compassSVG),
		liveDot:    ui.MustParseSVG(liveDotSVG),
		rotateLeft: ui.MustParseSVG(rotateLeftSVG),
		copy:       ui.MustParseSVG(copySVG),
		external:   ui.MustParseSVG(externalLinkSVG),
		trash:      ui.MustParseSVG(trashSVG),
		eye:        ui.MustParseSVG(eyeSVG),
		eyeOff:     ui.MustParseSVG(eyeOffSVG),
		sliders:    ui.MustParseSVG(slidersSVG),
	}
})
