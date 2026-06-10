package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bucketd/internal/meta"
	"bucketd/internal/store"
)

type DownloadHandler struct {
	store   *store.DiskStore
	meta    *meta.SQLiteStore
}

var inlineTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true,
	"image/webp": true, "image/svg+xml": true, "image/bmp": true,
	"image/ico": true, "image/tiff": true, "image/avif": true,
	"video/mp4": true, "video/webm": true, "video/ogg": true,
	"audio/mpeg": true, "audio/ogg": true, "audio/wav": true,
	"audio/webm": true, "audio/aac": true,
	"application/pdf": true,
	"text/plain": true, "text/html": true, "text/css": true,
	"text/csv": true, "text/xml": true,
	"application/json": true, "application/javascript": true,
}

func NewDownloadHandler(s *store.DiskStore, m *meta.SQLiteStore) *DownloadHandler {
	return &DownloadHandler{store: s, meta: m}
}

func (h *DownloadHandler) ServeFile(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, `{"error":"missing file hash"}`, http.StatusBadRequest)
		return
	}

	rec, err := h.meta.GetFileByHash(hash)
	if err != nil || rec == nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	if !rec.IsPublic {
		http.Error(w, `{"error":"file is private"}`, http.StatusForbidden)
		return
	}

	f, err := h.store.Open(hash)
	if err != nil {
		http.Error(w, `{"error":"file not found on disk"}`, http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", rec.MimeType)
	w.Header().Set("ETag", `"`+hash+`"`)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	if isInlineType(rec.MimeType) {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, sanitizeFilename(rec.Filename)))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(rec.Filename)))
	}

	http.ServeContent(w, r, filepath.Base(rec.Filename), rec.UpdatedAt, f)
}

func (h *DownloadHandler) ServeThumbnail(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, `{"error":"missing file hash"}`, http.StatusBadRequest)
		return
	}

	rec, err := h.meta.GetFileByHash(hash)
	if err != nil || rec == nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	if !strings.HasPrefix(rec.MimeType, "image/") && !strings.HasPrefix(rec.MimeType, "video/") {
		http.Error(w, `{"error":"thumbnails only available for images and videos"}`, http.StatusBadRequest)
		return
	}

	thumbPath := fmt.Sprintf("./data/thumbnails/%s/%s.jpg", hash[0:2], hash)
	http.ServeFile(w, r, thumbPath)
}

func isInlineType(mime string) bool {
	return inlineTypes[mime]
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, `"`, `'`)
	name = strings.ReplaceAll(name, "\\", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	return name
}

func formatTime(t time.Time) string {
	return t.UTC().Format(http.TimeFormat)
}
