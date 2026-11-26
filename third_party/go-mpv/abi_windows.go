//go:build windows

package mpv

// Windows uses LLP64: C unsigned long remains 32 bits on x64.
type apiVersionValue = uint32
