package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:assets
var assets embed.FS

// FS exposes the built single page app. Build it with `npm --prefix frontend run build`.
func FS() http.FileSystem {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil
	}
	return http.FS(sub)
}
