package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"bucketd/internal/meta"
)

type ListHandler struct {
	meta    *meta.SQLiteStore
	baseURL string
}

func NewListHandler(m *meta.SQLiteStore, baseURL string) *ListHandler {
	return &ListHandler{meta: m, baseURL: baseURL}
}

func (h *ListHandler) List(w http.ResponseWriter, r *http.Request) {
	opts := meta.ListOptions{
		Bucket:  r.URL.Query().Get("bucket"),
		Tag:     r.URL.Query().Get("tag"),
		Sort:    r.URL.Query().Get("sort"),
	}

	opts.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	opts.PerPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))

	result, err := h.meta.ListFiles(opts)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"files":    result.Files,
		"total":    result.Total,
		"page":     result.Page,
		"per_page": result.PerPage,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *ListHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing file id"}`, http.StatusBadRequest)
		return
	}

	rec, err := h.meta.GetFileByID(id)
	if err != nil || rec == nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	resp := fileResponse(rec, h.baseURL, false)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
