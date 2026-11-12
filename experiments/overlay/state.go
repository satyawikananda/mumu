package main

// mode selects how the retained libmpv view meets MyGo's rendered surface.
type mode string

const (
	// modeNativeView hosts the video view as a native element, which MyGo
	// places under its surface and clips to the element box.
	modeNativeView mode = "nativeview"
	// modeBelow inserts the retained view under MyGo's surface itself. MyGo
	// keeps pointer input everywhere, because it never placed the view.
	modeBelow mode = "below"
	// modeAbove attaches the video view over the window content, the way the
	// product does today. It is the comparison case: chrome built in the window
	// stays under the video.
	modeAbove mode = "above"
)

// transparentRoot reports whether the window root must leave the video area
// unpainted. Every mode except the comparison case shows a native view under
// MyGo's surface.
func (m mode) transparentRoot() bool { return m != modeAbove }

// appState is what the view reads and what engine events write. UI thread only;
// workers publish through Window.Update.
type appState struct {
	Mode    mode
	Status  string
	Detail  string
	Playing bool
	Paused  bool
	Busy    bool
	Overlay bool
	Drawer  bool
	PiP     bool
	Volume  int
	// Clicks counts pointer hits on chrome above live video. It is the evidence
	// that input reaches the intended control.
	Clicks int
	// LastAction names the control that took the last click.
	LastAction string
	// DrawableW and DrawableH are the backing pixels the surface reports.
	DrawableW int
	DrawableH int
}

// actions are what the view asks of the app. Every one runs on the UI thread.
type actions struct {
	Play          func()
	Pause         func()
	Stop          func()
	ToggleOverlay func()
	ToggleDrawer  func()
	Fullscreen    func()
	MovePiP       func()
	Quit          func()
	Volume        func(volume int)
	// Hit records a click on a named control, so the window shows that input
	// arrived rather than only dimming a button.
	Hit func(name string)
}
