package main

import (
	"embed"
	"net/http"
	"path/filepath"
)

const galleryPath = "/static/courtyard/datatree.html"

//go:embed web/gallery/datatree.html web/gallery/datatree.js
var galleryFiles embed.FS

func mountGallery(mux *http.ServeMux, assets string) {
	for _, name := range []string{"datatree.html", "datatree.js"} {
		mux.HandleFunc("GET /static/courtyard/"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, galleryFiles, "web/gallery/"+name)
		})
	}
	for _, name := range []string{
		"static/courtyard/archive.js",
		"static/courtyard/courtyard.css",
		"projects/games/domino-rush/vendor/three.module.js",
	} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, name))
		})
	}
}
