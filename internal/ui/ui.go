package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// GetFS returns the embedded filesystem rooted at the built React assets.
func GetFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
