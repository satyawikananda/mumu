//go:build unix

package mpv

// Unix uses LP64 on the supported targets: C unsigned long is pointer-sized.
type apiVersionValue = uintptr
