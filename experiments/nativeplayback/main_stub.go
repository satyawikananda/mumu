//go:build !darwin || !arm64 || !mumu_spike

package main

import "fmt"

func main() {
	fmt.Println("Native spike: macOS arm64, CGO_ENABLED=0 and -tags mumu_spike required; see README.md")
}
