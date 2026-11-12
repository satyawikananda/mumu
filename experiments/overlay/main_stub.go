//go:build !(darwin && arm64)

// Command overlay is the #32 feasibility spike. It needs the darwin/arm64
// native video adapter, so every other target reports that instead of building
// a second, untested composition path.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "overlay: the spike needs darwin/arm64 with the native video adapter")
	os.Exit(1)
}
