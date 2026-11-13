// Package update checks the public mumu release feed when the viewer asks.
//
// Checks are manual: nothing runs on launch and no result is cached here. The
// package reads release metadata only. It never downloads, installs or replaces
// an application binary.
package update
