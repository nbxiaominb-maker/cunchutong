package handler

import (
	"encoding/json"
	"net/http"

	"bucketd/internal/meta"
	"bucketd/internal/store"
)

type DeleteHandler struct {
	store *store.DiskStore
	meta  *meta.SQLiteStore
}

func NewDeleteHandler(s *store.DiskStore, m *meta.SQLiteStore) *DeleteHandler {
	return &DeleteHandler{store: s, meta: m}
}

func (h *DeleteHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	_, err = h.meta.DecrementRefCount(id)
	if err != nil {
		http.Error(w, `{"error":"decrement failed"}`, http.StatusInternalServerError)
		return
	}

	count, err := h.meta.GetRefCount(id)
	if err != nil {
		http.Error(w, `{"error":"ref count check failed"}`, http.StatusInternalServerError)
		return
	}

	if count <= 0 {
		h.meta.DeleteFile(id)
		h.store.Delete(rec.Sha256)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"deleted":    true,
		"id":         id,
		"physical_deleted": count <= 0,
	})
}
