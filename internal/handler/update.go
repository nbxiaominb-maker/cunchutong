package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bucketd/internal/meta"
)

type UpdateHandler struct {
	meta    *meta.SQLiteStore
	baseURL string
}

func NewUpdateHandler(m *meta.SQLiteStore, baseURL string) *UpdateHandler {
	return &UpdateHandler{meta: m, baseURL: baseURL}
}

func (h *UpdateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing file id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Filename string   `json:"filename"`
		Bucket   string   `json:"bucket"`
		Tags     []string `json:"tags"`
		IsPublic *bool    `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	rec, err := h.meta.GetFileByID(id)
	if err != nil || rec == nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	filename := rec.Filename
	if req.Filename != "" {
		filename = req.Filename
	}
	bucket := rec.Bucket
	if req.Bucket != "" {
		bucket = req.Bucket
	}
	tags := rec.Tags
	if req.Tags != nil {
		tags = req.Tags
	}
	isPublic := rec.IsPublic
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	if err := h.meta.UpdateFile(id, filename, bucket, tags, isPublic); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"update failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	updated, _ := h.meta.GetFileByID(id)
	resp := fileResponse(updated, h.baseURL, false)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
