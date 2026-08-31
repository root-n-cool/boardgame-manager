package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestBootstrapStatus_NeedsSetupWhenNoUsers(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.NeedsSetup {
		t.Fatal("expected needsSetup to be true with no users")
	}
}

func TestBootstrap_CreatesFirstAdminAndSetsCookie(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "supersecret1"})
	req := httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie to be set")
	}
}

func TestBootstrap_RejectedWhenUserAlreadyExists(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "supersecret1"})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload)))

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload)))

	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 on second bootstrap attempt, got %d", rec2.Code)
	}
}
