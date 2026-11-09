module mumu

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/ebitengine/purego v0.11.1
	github.com/egoist/mygo v0.8.3
	github.com/gen2brain/go-mpv v0.4.0
	golang.org/x/image v0.46.0
	golang.org/x/text v0.43.0
)

// The pinned binding opens libmpv in a package init and panics when the dylib
// is missing, which makes playback impossible in the default build and unsafe
// on machines without mpv. third_party/go-mpv is v0.4.0 with an explicit,
// retryable loader; see its README.
replace github.com/gen2brain/go-mpv => ./third_party/go-mpv

require (
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)
