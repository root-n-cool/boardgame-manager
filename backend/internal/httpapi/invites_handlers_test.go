package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

// inviteAdmin invites an admin and returns the token of their link.
func inviteAdmin(t *testing.T, router http.Handler, cookie *http.Cookie, email string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var created struct {
		InviteToken string `json:"inviteToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if created.InviteToken == "" {
		t.Fatal("no invite token returned")
	}
	return created.InviteToken
}

func TestGetInvite_ReturnsTheInvitedEmailWithoutAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/invites/"+token, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if body.Email != "invited@example.com" {
		t.Errorf("expected invited@example.com, got %q", body.Email)
	}
}

func TestGetInvite_UnknownTokenReturnsNotFound(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/invites/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAcceptInvite_SetsThePasswordAndOpensASession(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"password": "chosenpass1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("accepting an invite must open a session")
	}

	// The freshly opened session must work on the protected routes.
	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(session)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected the new session to work, got %d", meRec.Code)
	}

	// And the chosen password must work on the next login.
	loginAndGetCookie(t, router, "invited@example.com", "chosenpass1")
}

func TestAcceptInvite_LinkWorksOnlyOnce(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"password": "chosenpass1"})
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload)))
	if first.Code != http.StatusOK {
		t.Fatalf("first accept: %d %s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	payload2, _ := json.Marshal(map[string]string{"password": "hijacked999"})
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload2)))
	if second.Code != http.StatusNotFound {
		t.Fatalf("expected 404 reusing a spent invite, got %d: %s", second.Code, second.Body.String())
	}

	// The second attempt must not have rewritten the password.
	loginAndGetCookie(t, router, "invited@example.com", "chosenpass1")
}

func TestAcceptInvite_ShortPasswordReturnsBadRequest(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"password": "corta"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
