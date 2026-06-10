package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type DiskStore struct {
	dataDir string
	tmpDir  string
}

type StoreResult struct {
	Sha256 string
	Size   int64
	Path   string
	Exists bool
}

func NewDiskStore(dataDir, tmpDir string) (*DiskStore, error) {
	dirs := []string{
		dataDir,
		tmpDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return &DiskStore{dataDir: dataDir, tmpDir: tmpDir}, nil
}

func (s *DiskStore) Store(src io.Reader) (*StoreResult, error) {
	tmpFile, err := os.CreateTemp(s.tmpDir, "upload_*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hasher)

	size, err := io.Copy(multiWriter, src)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("write file: %w", err)
	}
	tmpFile.Close()

	hash := hex.EncodeToString(hasher.Sum(nil))
	finalPath := s.hashToPath(hash)
	finalDir := filepath.Dir(finalPath)

	if _, err := os.Stat(finalPath); err == nil {
		os.Remove(tmpPath)
		return &StoreResult{
			Sha256: hash,
			Size:   size,
			Path:   finalPath,
			Exists: true,
		}, nil
	}

	if err := os.MkdirAll(finalDir, 0755); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("create hash directory: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("move file to final location: %w", err)
	}

	return &StoreResult{
		Sha256: hash,
		Size:   size,
		Path:   finalPath,
		Exists: false,
	}, nil
}

func (s *DiskStore) StoreChunk(src io.Reader) (*StoreResult, error) {
	tmpFile, err := os.CreateTemp(s.tmpDir, "chunk_*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hasher)

	size, err := io.Copy(multiWriter, src)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("write chunk: %w", err)
	}
	tmpFile.Close()

	hash := hex.EncodeToString(hasher.Sum(nil))

	return &StoreResult{
		Sha256: hash,
		Size:   size,
		Path:   tmpPath,
		Exists: false,
	}, nil
}

func (s *DiskStore) AssembleChunks(chunkPaths []string) (*StoreResult, error) {
	tmpFile, err := os.CreateTemp(s.tmpDir, "assemble_*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hasher)

	var totalSize int64
	for _, chunkPath := range chunkPaths {
		f, err := os.Open(chunkPath)
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return nil, fmt.Errorf("open chunk %s: %w", chunkPath, err)
		}
		n, err := io.Copy(multiWriter, f)
		f.Close()
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return nil, fmt.Errorf("read chunk %s: %w", chunkPath, err)
		}
		totalSize += n
	}
	tmpFile.Close()

	hash := hex.EncodeToString(hasher.Sum(nil))
	finalPath := s.hashToPath(hash)
	finalDir := filepath.Dir(finalPath)

	if _, err := os.Stat(finalPath); err == nil {
		os.Remove(tmpPath)
		return &StoreResult{
			Sha256: hash,
			Size:   totalSize,
			Path:   finalPath,
			Exists: true,
		}, nil
	}

	if err := os.MkdirAll(finalDir, 0755); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("create hash directory: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("move assembled file: %w", err)
	}

	for _, chunkPath := range chunkPaths {
		os.Remove(chunkPath)
	}

	return &StoreResult{
		Sha256: hash,
		Size:   totalSize,
		Path:   finalPath,
		Exists: false,
	}, nil
}

func (s *DiskStore) Open(hash string) (*os.File, error) {
	path := s.hashToPath(hash)
	return os.Open(path)
}

func (s *DiskStore) Stat(hash string) (os.FileInfo, error) {
	path := s.hashToPath(hash)
	return os.Stat(path)
}

func (s *DiskStore) Delete(hash string) error {
	path := s.hashToPath(hash)
	return os.Remove(path)
}

func (s *DiskStore) Exists(hash string) bool {
	path := s.hashToPath(hash)
	_, err := os.Stat(path)
	return err == nil
}

func (s *DiskStore) hashToPath(hash string) string {
	if len(hash) < 4 {
		hash = hash + "0000"
	}
	return filepath.Join(s.dataDir, hash[0:2], hash[2:4], hash)
}
