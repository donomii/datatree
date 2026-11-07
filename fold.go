package main

import (
	_ "embed"
	"net/http"
)

//go:embed web/fold.js
var foldScript []byte

func mountFoldControl(mux *http.ServeMux) {
	mux.HandleFunc("GET /static/fold.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(foldScript)
	})
}
