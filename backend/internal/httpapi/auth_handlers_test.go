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

func loginAndGetCookie(t *testing.T, router http.Handler, email, password string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			return c
		}
	}
	t.Fatal("no session cookie returned by login")
	return nil
}

// assertSessionCookieHardened pins down the headline security property of this
// phase: the session cookie is not readable from JavaScript and is not sent on
// cross-site requests. Nothing else in the suite would notice if these
// attributes were dropped.
func assertSessionCookieHardened(t *testing.T, c *http.Cookie) {
	t.Helper()
	if c.Name != "session_token" {
		t.Fatalf("expected the session_token cookie, got %q", c.Name)
	}
	if c.Value == "" {
		t.Error("session cookie has an empty value")
	}
	if !c.HttpOnly {
		t.Error("session cookie must be HttpOnly so page scripts cannot read it")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("session cookie Path = %q, want \"/\"", c.Path)
	}
}

func TestBootstrap_SessionCookieIsHardened(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	assertSessionCookieHardened(t, bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1"))
}

func TestLogin_SessionCookieIsHardened(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	assertSessionCookieHardened(t, loginAndGetCookie(t, router, "admin@example.com", "supersecret1"))
}

func TestLogout_ClearingCookieKeepsHttpOnlyAndPath(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, logoutReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			cleared = c
		}
	}
	if cleared == nil {
		t.Fatal("logout did not send a session_token cookie to clear")
	}
	// The clearing cookie has to match the original's Path/HttpOnly or the
	// browser keeps the real one alongside it.
	if cleared.Value != "" {
		t.Errorf("expected an empty cookie value, got %q", cleared.Value)
	}
	if cleared.Path != "/" {
		t.Errorf("clearing cookie Path = %q, want \"/\"", cleared.Path)
	}
	if !cleared.HttpOnly {
		t.Error("clearing cookie should still be HttpOnly")
	}
	if cleared.MaxAge >= 0 {
		t.Errorf("clearing cookie MaxAge = %d, want a negative value", cleared.MaxAge)
	}
}

// TestLogout_ReturnsServerErrorWhenSessionDeleteFails exercises the error
// branch of logoutHandler. A BEFORE DELETE trigger is the one seam that fails
// the session DELETE while leaving the SELECT that requireAuth needs working —
// closing the connection instead would make requireAuth answer 401 first.
func TestLogout_ReturnsServerErrorWhenSessionDeleteFails(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	if _, err := conn.Exec(`CREATE TRIGGER block_session_delete BEFORE DELETE ON sessions
		BEGIN SELECT RAISE(ABORT, 'forced delete failure'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, logoutReq)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the session delete fails, got %d: %s", rec.Code, rec.Body.String())
	}
	// A failed logout must not tell the client the cookie is gone.
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			t.Errorf("expected no session cookie mutation on the error path, got %#v", c)
		}
	}
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
