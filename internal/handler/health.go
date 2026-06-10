package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"bucketd/internal/meta"
)

var (
	startTime = time.Now()
	version   = "1.0.0"
)

type HealthHandler struct {
	meta *meta.SQLiteStore
}

func NewHealthHandler(m *meta.SQLiteStore) *HealthHandler {
	return &HealthHandler{meta: m}
}

func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	fileCount, storageBytes, _ := h.meta.GetStats()

	resp := map[string]any{
		"status":         "ok",
		"version":        version,
		"uptime_seconds": int(time.Since(startTime).Seconds()),
		"storage_bytes":  storageBytes,
		"file_count":     fileCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
