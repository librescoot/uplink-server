// Package webui serves the embedded web UI assets.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:assets
var assets embed.FS

// Handler returns an http.Handler that serves the embedded UI: index.html at
// "/", plus the /css, /js and /images subtrees.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(sub)), nil
}
