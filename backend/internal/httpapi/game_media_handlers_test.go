package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestCreateMedia_LinkSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"type": "link", "url": "https://example.com/rules.pdf", "title": "Regolamento"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateMedia_YoutubeRejectsNonYoutubeURL(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"type": "youtube", "url": "https://example.com/video"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateMedia_FileUploadSucceedsAndIsPubliclyServed(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "manuale.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write([]byte("%PDF-1.4 fake manual content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/uploads/"+body.URL, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected uploaded file to be servable without auth, got %d", getRec.Code)
	}
}

func TestGetUpload_RejectsPathTraversal(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	for _, filename := range []string{"..%2F..%2Fetc%2Fpasswd", "%2e%2e", "....%2f....%2fetc%2fpasswd"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/uploads/"+filename, nil))
		if rec.Code == http.StatusOK {
			t.Fatalf("expected path traversal attempt %q to be rejected, got 200", filename)
		}
	}
}

func TestDeleteMedia_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"type": "link", "url": "https://example.com/rules"})
	createReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), bytes.NewReader(payload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	var created struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(createRec.Body).Decode(&created)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/games/%d/languages/it/media/%d", id, created.ID), nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
