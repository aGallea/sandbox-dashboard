// Package ui embeds the built React SPA.
//
// The Makefile's `build` target copies ui/dist into this package directory
// before running `go build`, then removes the copy. Direct `go build` without
// the Makefile will fail with "pattern dist: no matching files found".
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Assets returns the embedded SPA assets rooted at dist/.
func Assets() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
