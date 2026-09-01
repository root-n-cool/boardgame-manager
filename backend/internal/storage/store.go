package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Category struct {
	Name         string
	AllowedTypes map[string]string // MIME type -> file extension
	MaxBytes     int64
}

var ManualCategory = Category{
	Name: "manual",
	AllowedTypes: map[string]string{
		"application/pdf": ".pdf",
	},
	MaxBytes: 20 << 20, // 20 MB
}

var CoverCategory = Category{
	Name: "cover",
	AllowedTypes: map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	},
	MaxBytes: 5 << 20, // 5 MB
}

var ErrUnsupportedType = errors.New("unsupported file type")
var ErrTooLarge = errors.New("file too large")

type Store struct {
	baseDir string
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// Save reads r fully, validates its content type and size against category,
// and writes it to disk under a content-addressed filename (sha256 of the
// content + extension). Returns the filename (not a full path) to store in
// the DB. Saving identical content twice returns the same filename without
// writing a duplicate file.
func (s *Store) Save(category Category, r io.Reader) (string, error) {
	limited := io.LimitReader(r, category.MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > category.MaxBytes {
		return "", ErrTooLarge
	}

	contentType := http.DetectContentType(data)
	ext, ok := category.AllowedTypes[contentType]
	if !ok {
		return "", ErrUnsupportedType
	}

	sum := sha256.Sum256(data)
	filename := hex.EncodeToString(sum[:]) + ext

	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create uploads dir: %w", err)
	}

	fullPath := filepath.Join(s.baseDir, filename)
	if _, err := os.Stat(fullPath); err == nil {
		return filename, nil
	}

	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write upload: %w", err)
	}

	return filename, nil
}

// Open opens a previously saved file by its filename (as returned by Save).
func (s *Store) Open(filename string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.baseDir, filename))
}
