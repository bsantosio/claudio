// Package webui serves the embedded web interface.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var content embed.FS

// RegisterHandler mounts the embedded SPA assets onto the provided mux.
// GET /        → serves index.html (SPA shell)
// GET /web/*   → serves static assets (app.js, style.css, …) from the embedded FS
func RegisterHandler(mux *http.ServeMux) {
	webFS, _ := fs.Sub(content, "web")

	// Serve index.html at the SPA root.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, webFS, "index.html")
	})

	// Serve embedded static assets under /web/.
	mux.Handle("GET /web/", http.StripPrefix("/web/", http.FileServerFS(webFS)))
}
