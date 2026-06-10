package meta

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type FileRecord struct {
	ID        string    `json:"id"`
	Sha256    string    `json:"sha256"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	Bucket    string    `json:"bucket"`
	Tags      []string  `json:"tags"`
	IsPublic  bool      `json:"is_public"`
	RefCount  int       `json:"ref_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type APIKey struct {
	Name        string
	Key         string
	Permissions []string
}

type MultipartUpload struct {
	UploadID        string
	Filename        string
	TotalSize       int64
	MimeType        string
	Bucket          string
	ChunkSize       int64
	TotalChunks     int
	ReceivedChunks  map[int]string
	Status          string
	CreatedAt       time.Time
}

type SQLiteStore struct {
	db *sql.DB
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.runMigrations(); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) runMigrations() error {
	data, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	_, err = s.db.Exec(string(data))
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) CreateFile(rec *FileRecord) error {
	tagsJSON, _ := json.Marshal(rec.Tags)
	isPublic := 0
	if rec.IsPublic {
		isPublic = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO files (id, sha256, filename, size, mime_type, bucket, tags, is_public, ref_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Sha256, rec.Filename, rec.Size, rec.MimeType, rec.Bucket,
		string(tagsJSON), isPublic, rec.RefCount, rec.CreatedAt.Format(time.RFC3339Nano), rec.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) GetFileByID(id string) (*FileRecord, error) {
	row := s.db.QueryRow(`
		SELECT id, sha256, filename, size, mime_type, bucket, tags, is_public, ref_count, created_at, updated_at
		FROM files WHERE id = ?`, id)
	return s.scanFile(row)
}

func (s *SQLiteStore) GetFileByHash(hash string) (*FileRecord, error) {
	row := s.db.QueryRow(`
		SELECT id, sha256, filename, size, mime_type, bucket, tags, is_public, ref_count, created_at, updated_at
		FROM files WHERE sha256 = ?`, hash)
	return s.scanFile(row)
}

func (s *SQLiteStore) IncrementRefCount(hash string) (int, error) {
	res, err := s.db.Exec(`UPDATE files SET ref_count = ref_count + 1, updated_at = ? WHERE sha256 = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), hash)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) DecrementRefCount(id string) (int, error) {
	res, err := s.db.Exec(`UPDATE files SET ref_count = ref_count - 1, updated_at = ? WHERE id = ? AND ref_count > 0`,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) GetRefCount(id string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT ref_count FROM files WHERE id = ?`, id).Scan(&count)
	return count, err
}

func (s *SQLiteStore) DeleteFile(id string) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) UpdateFileTags(id string, tags []string) error {
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.db.Exec(`UPDATE files SET tags = ?, updated_at = ? WHERE id = ?`,
		string(tagsJSON), time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *SQLiteStore) UpdateFile(id string, filename string, bucket string, tags []string, isPublic bool) error {
	tagsJSON, _ := json.Marshal(tags)
	pub := 0
	if isPublic {
		pub = 1
	}
	_, err := s.db.Exec(`UPDATE files SET filename = ?, bucket = ?, tags = ?, is_public = ?, updated_at = ? WHERE id = ?`,
		filename, bucket, string(tagsJSON), pub, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

type ListOptions struct {
	Bucket  string
	Tag     string
	Page    int
	PerPage int
	Sort    string
}

type ListResult struct {
	Files    []FileRecord `json:"files"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PerPage  int          `json:"per_page"`
}

func (s *SQLiteStore) ListFiles(opts ListOptions) (*ListResult, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 50
	}

	var conditions []string
	var args []any

	if opts.Bucket != "" {
		conditions = append(conditions, "bucket = ?")
		args = append(args, opts.Bucket)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM files " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	orderBy := "created_at DESC"
	if opts.Sort != "" {
		parts := strings.Split(opts.Sort, ":")
		col := parts[0]
		dir := "DESC"
		if len(parts) > 1 && strings.ToUpper(parts[1]) == "ASC" {
			dir = "ASC"
		}
		allowed := map[string]bool{"created_at": true, "filename": true, "size": true}
		if allowed[col] {
			orderBy = col + " " + dir
		}
	}

	offset := (opts.Page - 1) * opts.PerPage
	query := fmt.Sprintf(
		"SELECT id, sha256, filename, size, mime_type, bucket, tags, is_public, ref_count, created_at, updated_at FROM files %s ORDER BY %s LIMIT ? OFFSET ?",
		where, orderBy,
	)
	args = append(args, opts.PerPage, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		rec, err := s.scanFileFromRows(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *rec)
	}

	return &ListResult{
		Files:   files,
		Total:   total,
		Page:    opts.Page,
		PerPage: opts.PerPage,
	}, nil
}

func (s *SQLiteStore) ValidateAPIKey(key string) (*APIKey, error) {
	row := s.db.QueryRow(`SELECT key, name, permissions FROM api_keys WHERE key = ?`, key)
	var ak APIKey
	var perms string
	if err := row.Scan(&ak.Key, &ak.Name, &perms); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ak.Permissions = strings.Split(perms, ",")
	return &ak, nil
}

func (s *SQLiteStore) SeedAPIKeys(keys []struct {
	Name        string
	KeyHash     string
	Permissions []string
}) error {
	for _, k := range keys {
		perms := strings.Join(k.Permissions, ",")
		_, err := s.db.Exec(`
			INSERT OR IGNORE INTO api_keys (key, name, permissions) VALUES (?, ?, ?)`,
			k.KeyHash, k.Name, perms)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) GetStats() (fileCount int, storageBytes int64, err error) {
	err = s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(size), 0) FROM files").Scan(&fileCount, &storageBytes)
	return
}

func (s *SQLiteStore) scanFile(row *sql.Row) (*FileRecord, error) {
	var rec FileRecord
	var tagsJSON string
	var isPublic int
	var createdAt, updatedAt string

	err := row.Scan(
		&rec.ID, &rec.Sha256, &rec.Filename, &rec.Size, &rec.MimeType,
		&rec.Bucket, &tagsJSON, &isPublic, &rec.RefCount, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	json.Unmarshal([]byte(tagsJSON), &rec.Tags)
	rec.IsPublic = isPublic == 1
	rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	rec.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	return &rec, nil
}

func (s *SQLiteStore) scanFileFromRows(rows *sql.Rows) (*FileRecord, error) {
	var rec FileRecord
	var tagsJSON string
	var isPublic int
	var createdAt, updatedAt string

	err := rows.Scan(
		&rec.ID, &rec.Sha256, &rec.Filename, &rec.Size, &rec.MimeType,
		&rec.Bucket, &tagsJSON, &isPublic, &rec.RefCount, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(tagsJSON), &rec.Tags)
	rec.IsPublic = isPublic == 1
	rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	rec.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	return &rec, nil
}

func (s *SQLiteStore) CreateMultipartUpload(mu *MultipartUpload) error {
	chunksJSON, _ := json.Marshal(mu.ReceivedChunks)
	_, err := s.db.Exec(`
		INSERT INTO multipart_uploads (upload_id, filename, total_size, mime_type, bucket, chunk_size, total_chunks, received_chunks, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mu.UploadID, mu.Filename, mu.TotalSize, mu.MimeType, mu.Bucket,
		mu.ChunkSize, mu.TotalChunks, string(chunksJSON), mu.Status, mu.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) GetMultipartUpload(uploadID string) (*MultipartUpload, error) {
	row := s.db.QueryRow(`
		SELECT upload_id, filename, total_size, mime_type, bucket, chunk_size, total_chunks, received_chunks, status, created_at
		FROM multipart_uploads WHERE upload_id = ?`, uploadID)

	var mu MultipartUpload
	var chunksJSON string
	var createdAt string

	err := row.Scan(&mu.UploadID, &mu.Filename, &mu.TotalSize, &mu.MimeType, &mu.Bucket,
		&mu.ChunkSize, &mu.TotalChunks, &chunksJSON, &mu.Status, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	json.Unmarshal([]byte(chunksJSON), &mu.ReceivedChunks)
	mu.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)

	return &mu, nil
}

func (s *SQLiteStore) UpdateMultipartChunk(uploadID string, chunkNum int, chunkHash string) error {
	mu, err := s.GetMultipartUpload(uploadID)
	if err != nil || mu == nil {
		return fmt.Errorf("upload not found")
	}
	if mu.ReceivedChunks == nil {
		mu.ReceivedChunks = make(map[int]string)
	}
	mu.ReceivedChunks[chunkNum] = chunkHash
	chunksJSON, _ := json.Marshal(mu.ReceivedChunks)
	_, err = s.db.Exec(`UPDATE multipart_uploads SET received_chunks = ? WHERE upload_id = ?`,
		string(chunksJSON), uploadID)
	return err
}

func (s *SQLiteStore) CompleteMultipartUpload(uploadID string) error {
	_, err := s.db.Exec(`UPDATE multipart_uploads SET status = 'completed' WHERE upload_id = ?`, uploadID)
	return err
}
