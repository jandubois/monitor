package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

// staticHandler serves the embedded React frontend.
func staticHandler() http.Handler {
	subFS, _ := fs.Sub(frontendFS, "frontend/dist")
	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists
		if _, err := fs.Stat(subFS, path[1:]); err != nil {
			// File doesn't exist, serve index.html for SPA routing
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	})
}
