package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func bootstrapFirstAdmin(t *testing.T, router http.Handler, email, password string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			return c
		}
	}
	t.Fatal("no session cookie returned by bootstrap")
	return nil
}

func TestLogin_WithValidCredentialsSetsCookie(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "supersecret1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie")
	}
}

func TestLogin_WithWrongPasswordFails(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "wrong"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMe_RequiresAuthentication(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestMe_ReturnsCurrentUserWhenAuthenticated(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogout_InvalidatesSession(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), logoutReq)

	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, meReq)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", rec.Code)
	}
}
