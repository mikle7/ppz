package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var assetsFS embed.FS

// assetsHandler serves files from the embedded assets directory at /assets/<name>.
// One handler covers any future static assets we drop into internal/server/assets/.
func assetsHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		// Build-time issue (no `assets/` dir under embed root) — fall back to
		// 404 so the server doesn't fail to start.
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
