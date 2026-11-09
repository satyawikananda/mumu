//go:build darwin && arm64 && mumu_spike

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type spike struct {
	main, pip         *mygo.Window
	dispatcher        *mygo.Window
	surface           *videoSurface
	presentation      presentation
	status            snapshot
	volume            float64
	commands          chan intent
	cancel            context.CancelFunc
	done              chan struct{}
	closing, released bool
	theme             *ui.Theme
}

func (a *spike) send(cmd intent) {
	select {
	case a.commands <- cmd:
	default:
		log.Print("intent queue full")
	}
}

func (a *spike) view(c *ui.Context, mini bool) {
	c.SetTheme(a.theme)
	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			if !mini && a.presentation == pipPresentation {
				ui.Text(c, "Playing in Picture-in-Picture")
			}
		})
		height := float32(138)
		if mini {
			height = 78
		}
		ui.Column(c).Height(height).FillWidth().Padding(10).Gap(8).Children(func() {
			if !mini {
				ui.Text(c, "mumu · native playback spike").Bold().TextColor(ui.RGB(255, 77, 77))
				ui.Text(c, a.status.state+" · "+a.status.detail).FontSize(12)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if mini {
					ui.Button(c, "Return").OnClick(func() { a.move(mainPresentation) })
					ui.Button(c, "Close PiP").OnClick(a.closePiP)
					label := a.status.state
					if label == "unavailable" {
						label = "error"
					}
					ui.Text(c, label).FontSize(11)
				} else {
					canPlay := a.status.state == "idle" || a.status.state == "stopped" || a.status.state == "error"
					ui.PrimaryButton(c, "Play").Disabled(!canPlay).OnClick(func() { a.send(intent{kind: "play"}) })
					ui.Button(c, "PiP").Disabled(a.presentation != mainPresentation).OnClick(func() { a.move(pipPresentation) })
					ui.Button(c, "Fullscreen").OnClick(func() { a.main.ToggleFullScreen() })
					ui.Button(c, "Quit").OnClick(a.shutdown)
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Volume").FontSize(12)
				ui.Slider(c, &a.volume, 0, 100).Width(140).Label("Volume").OnChange(func() { a.send(intent{kind: "volume", volume: a.volume}) })
			})
		})
	})
}

func (a *spike) newMain() {
	a.main = mygo.NewWindow(mygo.WindowOptions{
		Title: "mumu · M0 native spike", Width: 960, Height: 678, MinWidth: 540, MinHeight: 400,
		UseContentSize: true, BackgroundColor: "#151517", Content: ui.View(func(c *ui.Context) { a.view(c, false) }),
	})
	a.main.OnResize(func() {
		if a.surface != nil && a.presentation == mainPresentation {
			a.surface.resize(a.main.NativeHandle(), 138)
		}
	})
	a.main.OnClose(func(e *mygo.CloseEvent) {
		if a.released {
			return
		}
		if a.presentation == pipPresentation {
			return
		}
		e.PreventDefault()
		a.shutdown()
	})
	a.main.OnClosed(func() { a.main = nil; log.Print("main-closed") })
}

func (a *spike) move(target presentation) {
	if a.closing || a.surface == nil {
		return
	}
	start := time.Now()
	err := transfer(&a.presentation, target, func() error {
		if target == mainPresentation {
			if a.main == nil {
				a.newMain()
			}
			a.main.Restore()
			a.main.Show()
			if err := a.surface.attach(a.main.NativeHandle(), 138); err != nil {
				return err
			}
			if a.pip != nil {
				a.pip.Hide()
			}
			return nil
		}
		if a.pip == nil {
			a.pip = mygo.NewWindow(mygo.WindowOptions{
				Title: "mumu · PiP", Width: 480, Height: 348, MinWidth: 320, MinHeight: 258,
				UseContentSize: true, AlwaysOnTop: true, Panel: true,
				BackgroundColor: "#09090B", Content: ui.View(func(c *ui.Context) { a.view(c, true) }),
			})
			a.pip.OnMove(func() { log.Printf("pip-moved bounds=%+v", a.pip.Bounds()) })
			a.pip.OnResize(func() {
				if a.presentation == pipPresentation {
					r := a.pip.ContentBounds()
					height := r.Width*9/16 + 78
					if r.Height != height {
						r.Height = height
						a.pip.SetContentBounds(r)
					}
					a.surface.resize(a.pip.NativeHandle(), 78)
					log.Printf("pip-resized content=%+v", a.pip.ContentBounds())
				}
			})
			a.pip.OnClose(func(e *mygo.CloseEvent) {
				if a.released {
					return
				}
				e.PreventDefault()
				a.closePiP()
			})
		}
		a.pip.Show()
		return a.surface.attach(a.pip.NativeHandle(), 78)
	})
	log.Printf("transfer target=%s state=%s attachment_ms=%.3f err=%v", target, a.presentation, float64(time.Since(start).Microseconds())/1000, err)
	if a.pip != nil {
		log.Printf("pip-window visible=%t always_on_top=%t", a.pip.IsVisible(), a.pip.IsAlwaysOnTop())
	}
	if a.main != nil {
		a.main.Invalidate()
	}
}

func (a *spike) closePiP() {
	if a.main != nil {
		a.move(mainPresentation)
		return
	}
	a.shutdown()
}

func (a *spike) shutdown() {
	if a.closing {
		return
	}
	a.closing = true
	a.presentation = stopped
	a.cancel()
	go func() {
		<-a.done
		mygo.RunOnMain(func() {
			a.surface.close()
			a.released = true
			log.Print("teardown-complete")
			mygo.App.Quit()
		})
	}()
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	source := flag.String("source", "", "Explicitly authorized stream URL or local HLS file")
	exercise := flag.Bool("exercise", false, "Run timed native lifecycle exercise; explicitly starts playback")
	duration := flag.Duration("duration", 0, "Automatically quit after duration (0 disables)")
	flag.Parse()
	if *source == "" {
		fmt.Fprintln(os.Stderr, "Specify -source with an authorized HLS sample; see spike README")
		os.Exit(2)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &spike{presentation: mainPresentation, volume: 30, commands: make(chan intent, 16), cancel: cancel, done: make(chan struct{}), theme: ui.DarkTheme()}
	a.theme.Background, a.theme.Surface = ui.RGB(21, 21, 23), ui.RGB(38, 38, 41)
	a.theme.Accent, a.theme.AccentHover = ui.RGB(217, 54, 54), ui.RGB(201, 44, 44)
	a.theme.Text, a.theme.TextMuted = ui.RGB(245, 245, 246), ui.RGB(184, 184, 191)
	mygo.App.WhenReady(func() {
		a.newMain()
		a.dispatcher = a.main
		var err error
		a.surface, err = newVideoSurface()
		if err == nil {
			err = a.surface.attach(a.main.NativeHandle(), 138)
		}
		if err != nil {
			log.Printf("native-surface-error: %v", err)
			if a.surface != nil {
				a.surface.close()
				a.surface = nil
			}
			a.released = true
			mygo.App.Quit()
			return
		}
		mygo.App.Focus()
		a.main.Focus()
		go func() {
			defer close(a.done)
			runSession(ctx, a.surface, *source, a.commands, func(s snapshot) {
				a.mainUpdate(func() { a.status = s })
			})
			a.mainUpdate(func() {
				if !a.closing {
					a.status.state = "unavailable"
				}
			})
		}()
		if *exercise {
			go a.exercise(ctx)
		}
		if *duration > 0 {
			go func() {
				select {
				case <-ctx.Done():
				case <-time.After(*duration):
					mygo.RunOnMain(a.shutdown)
				}
			}()
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !a.released && a.surface != nil {
			e.PreventDefault()
			a.shutdown()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *spike) mainUpdate(fn func()) {
	// Window.Update posts without waiting; resolve live windows on the UI thread.
	a.dispatcher.Update(func() {
		fn()
		if a.main != nil {
			a.main.Invalidate()
		}
		if a.pip != nil {
			a.pip.Invalidate()
		}
	})
}

func (a *spike) exercise(ctx context.Context) {
	steps := []struct {
		after time.Duration
		name  string
		run   func()
	}{
		{after: 2 * time.Second, name: "play", run: func() { a.send(intent{kind: "play"}) }},
		{after: 5 * time.Second, name: "resize", run: func() { a.main.SetSize(800, 588) }},
		{after: 8 * time.Second, name: "pip", run: func() { a.move(pipPresentation) }},
		{after: 9 * time.Second, name: "volume-zero", run: func() { a.volume = 0; a.send(intent{kind: "volume", volume: 0}) }},
		{after: 10 * time.Second, name: "volume-restore", run: func() { a.volume = 30; a.send(intent{kind: "volume", volume: 30}) }},
		{after: 11 * time.Second, name: "minimize-main", run: func() { a.main.Minimize() }},
		{after: 12 * time.Second, name: "close-pip-with-main", run: a.closePiP},
		{after: 14 * time.Second, name: "pip-for-hide", run: func() { a.move(pipPresentation) }},
		{after: 15 * time.Second, name: "hide-main", run: func() { a.main.Hide() }},
		{after: 16 * time.Second, name: "return", run: func() { a.move(mainPresentation) }},
		{after: 17 * time.Second, name: "fullscreen", run: func() { a.main.SetFullScreen(true) }},
		{after: 20 * time.Second, name: "exit-fullscreen", run: func() { a.main.SetFullScreen(false) }},
		{after: 23 * time.Second, name: "pip-again", run: func() { a.move(pipPresentation) }},
		{after: 24 * time.Second, name: "resize-pip", run: func() { a.pip.SetSize(640, 500) }},
		{after: 25 * time.Second, name: "move-pip", run: func() { r := a.pip.Bounds(); r.X += 30; r.Y += 30; a.pip.SetBounds(r) }},
		{after: 26 * time.Second, name: "close-main", run: func() { a.main.Close() }},
		{after: 27 * time.Second, name: "return-recreates-main", run: func() { a.move(mainPresentation) }},
		{after: 28 * time.Second, name: "pip-final", run: func() { a.move(pipPresentation) }},
		{after: 29 * time.Second, name: "close-main-again", run: func() { a.main.Close() }},
		{after: 30 * time.Second, name: "close-pip-without-main", run: a.closePiP},
	}
	start := time.Now()
	for _, step := range steps {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(start.Add(step.after))):
			mygo.RunOnMain(func() { log.Printf("exercise=%s", step.name); step.run() })
		}
	}
}
