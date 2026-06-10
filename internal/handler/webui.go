package handler

import (
	"embed"
	"net/http"
)

//go:embed static/*
var staticFS embed.FS

type WebUIHandler struct{}

func NewWebUIHandler() *WebUIHandler {
	return &WebUIHandler{}
}

func (h *WebUIHandler) Serve(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
