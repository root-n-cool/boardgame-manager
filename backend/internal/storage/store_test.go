package storage_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"boardgames-manager/internal/storage"
)

func TestSave_ValidPDFIsStoredAndReturnsContentAddressedName(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 fake pdf content for testing"
	name, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("expected .pdf extension, got %q", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("saved content mismatch")
	}
}

func TestSave_RejectsWrongType(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	_, err := store.Save(storage.ManualCategory, strings.NewReader("plain text, not a pdf"))
	if err != storage.ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType, got %v", err)
	}
}

func TestSave_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	tiny := storage.Category{
		Name:         "tiny",
		AllowedTypes: map[string]string{"application/pdf": ".pdf"},
		MaxBytes:     10,
	}
	_, err := store.Save(tiny, strings.NewReader("%PDF-1.4 this is definitely more than ten bytes"))
	if err != storage.ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestSave_DeduplicatesIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 identical content"
	name1, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	name2, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if name1 != name2 {
		t.Fatalf("expected same content-addressed name, got %q and %q", name1, name2)
	}
}

func TestOpen_ReturnsSavedContent(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 open me"
	name, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	f, err := store.Open(name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch")
	}
}
