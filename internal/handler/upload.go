package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bucketd/internal/meta"
	"bucketd/internal/store"

	"github.com/oklog/ulid/v2"
)

type UploadHandler struct {
	store    *store.DiskStore
	meta     *meta.SQLiteStore
	maxSize  int64
	baseURL  string
}

func NewUploadHandler(s *store.DiskStore, m *meta.SQLiteStore, maxSize int64, baseURL string) *UploadHandler {
	return &UploadHandler{store: s, meta: m, maxSize: maxSize, baseURL: baseURL}
}

func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxSize); err != nil {
		http.Error(w, `{"error":"file too large or invalid form"}`, http.StatusRequestEntityTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing file field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	if header.Size > h.maxSize {
		http.Error(w, `{"error":"file exceeds maximum size"}`, http.StatusRequestEntityTooLarge)
		return
	}

	bucket := r.FormValue("bucket")
	if bucket == "" {
		bucket = "default"
	}
	isPublic := r.FormValue("is_public") != "false"
	tagsStr := r.FormValue("tags")
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	result, err := h.store.Store(file)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"storage failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	mimeType := detectMIME(header, file)

	id := ulid.Make().String()
	now := time.Now().UTC()

	if result.Exists {
		existing, _ := h.meta.GetFileByHash(result.Sha256)
		if existing != nil {
			h.meta.IncrementRefCount(result.Sha256)
			resp := fileResponse(existing, h.baseURL, true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
			return
		}
	}

	rec := &meta.FileRecord{
		ID:        id,
		Sha256:    result.Sha256,
		Filename:  header.Filename,
		Size:      result.Size,
		MimeType:  mimeType,
		Bucket:    bucket,
		Tags:      tags,
		IsPublic:  isPublic,
		RefCount:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.meta.CreateFile(rec); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"metadata failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	resp := fileResponse(rec, h.baseURL, false)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *UploadHandler) MultipartInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Mime     string `json:"mime"`
		Bucket   string `json:"bucket"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Filename == "" {
		http.Error(w, `{"error":"filename required"}`, http.StatusBadRequest)
		return
	}
	if req.Bucket == "" {
		req.Bucket = "default"
	}

	chunkSize := int64(8 << 20) // 8MB
	totalChunks := int((req.Size + chunkSize - 1) / chunkSize)
	if totalChunks == 0 {
		totalChunks = 1
	}

	uploadID := ulid.Make().String()
	mu := &meta.MultipartUpload{
		UploadID:       uploadID,
		Filename:       req.Filename,
		TotalSize:      req.Size,
		MimeType:       req.Mime,
		Bucket:         req.Bucket,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
		ReceivedChunks: make(map[int]string),
		Status:         "pending",
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.meta.CreateMultipartUpload(mu); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"upload_id":    uploadID,
		"chunk_size":   chunkSize,
		"total_chunks": totalChunks,
	})
}

func (h *UploadHandler) MultipartChunk(w http.ResponseWriter, r *http.Request) {
	uploadID := r.PathValue("uploadID")
	chunkStr := r.PathValue("chunkNumber")

	chunkNum, err := strconv.Atoi(chunkStr)
	if err != nil {
		http.Error(w, `{"error":"invalid chunk number"}`, http.StatusBadRequest)
		return
	}

	mu, err := h.meta.GetMultipartUpload(uploadID)
	if err != nil || mu == nil {
		http.Error(w, `{"error":"upload not found"}`, http.StatusNotFound)
		return
	}
	if mu.Status != "pending" {
		http.Error(w, `{"error":"upload already completed or aborted"}`, http.StatusBadRequest)
		return
	}

	result, err := h.store.StoreChunk(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"chunk storage failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	if err := h.meta.UpdateMultipartChunk(uploadID, chunkNum, result.Sha256); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"chunk metadata failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"chunk_number": chunkNum,
		"sha256":       result.Sha256,
		"received":     true,
		"temp_path":    result.Path,
	})
}

func (h *UploadHandler) MultipartComplete(w http.ResponseWriter, r *http.Request) {
	uploadID := r.PathValue("uploadID")

	mu, err := h.meta.GetMultipartUpload(uploadID)
	if err != nil || mu == nil {
		http.Error(w, `{"error":"upload not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Chunks []struct {
			Number  int    `json:"number"`
			Sha256  string `json:"sha256"`
			TempPath string `json:"temp_path"`
		} `json:"chunks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	var chunkPaths []string
	for _, c := range req.Chunks {
		chunkPaths = append(chunkPaths, c.TempPath)
	}

	result, err := h.store.AssembleChunks(chunkPaths)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"assembly failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	mimeType := mu.MimeType
	if mimeType == "" {
		ext := filepath.Ext(mu.Filename)
		mimeType = mime.TypeByExtension(ext)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	id := ulid.Make().String()
	now := time.Now().UTC()

	if result.Exists {
		existing, _ := h.meta.GetFileByHash(result.Sha256)
		if existing != nil {
			h.meta.IncrementRefCount(result.Sha256)
			resp := fileResponse(existing, h.baseURL, true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
			h.meta.CompleteMultipartUpload(uploadID)
			return
		}
	}

	rec := &meta.FileRecord{
		ID:        id,
		Sha256:    result.Sha256,
		Filename:  mu.Filename,
		Size:      result.Size,
		MimeType:  mimeType,
		Bucket:    mu.Bucket,
		IsPublic:  true,
		RefCount:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.meta.CreateFile(rec); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"metadata failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	h.meta.CompleteMultipartUpload(uploadID)

	resp := fileResponse(rec, h.baseURL, false)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

type FileResponse struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	Filename      string    `json:"filename"`
	Size          int64     `json:"size"`
	Mime          string    `json:"mime"`
	Sha256        string    `json:"sha256"`
	Bucket        string    `json:"bucket"`
	Tags          []string  `json:"tags"`
	IsPublic      bool      `json:"is_public"`
	Deduplicated  bool      `json:"deduplicated"`
	CreatedAt     time.Time `json:"created_at"`
}

func fileResponse(rec *meta.FileRecord, baseURL string, dedup bool) FileResponse {
	url := fmt.Sprintf("%s/f/%s", baseURL, rec.Sha256)
	return FileResponse{
		ID:           rec.ID,
		URL:          url,
		Filename:     rec.Filename,
		Size:         rec.Size,
		Mime:         rec.MimeType,
		Sha256:       rec.Sha256,
		Bucket:       rec.Bucket,
		Tags:         rec.Tags,
		IsPublic:     rec.IsPublic,
		Deduplicated: dedup,
		CreatedAt:    rec.CreatedAt,
	}
}

func detectMIME(header *multipart.FileHeader, file io.ReadSeeker) string {
	ct := header.Header.Get("Content-Type")
	if ct != "" && ct != "application/octet-stream" {
		return ct
	}
	if ext := filepath.Ext(header.Filename); ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
	if file != nil {
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		file.Seek(0, io.SeekStart)
		if n > 0 {
			return http.DetectContentType(buf[:n])
		}
	}
	return "application/octet-stream"
}
