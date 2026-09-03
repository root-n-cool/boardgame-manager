package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestUploadCover_ValidImageSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	buf, contentType := imageUploadBody(t, "cover.png")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/cover", id), buf)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		CoverPath *string `json:"coverPath"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.CoverPath == nil || *body.CoverPath == "" {
		t.Fatalf("expected coverPath to be set, got %v", body.CoverPath)
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", id), nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}
	var getBody struct {
		CoverPath *string `json:"coverPath"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&getBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if getBody.CoverPath == nil || *getBody.CoverPath != *body.CoverPath {
		t.Fatalf("expected GET to reflect uploaded cover, got %v", getBody.CoverPath)
	}
}

func TestUploadCover_RejectsUnsupportedType(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	buf, contentType := multipartFileBody(t, "cover.pdf", []byte("%PDF-1.4 not an image"))

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/cover", id), buf)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
