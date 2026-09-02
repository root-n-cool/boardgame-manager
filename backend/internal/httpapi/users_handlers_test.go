package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestListUsers_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateUser_WithOnlyAnEmailReturnsAnInviteToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		Pending     bool   `json:"pending"`
		InviteToken string `json:"inviteToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Email != "second@example.com" {
		t.Errorf("expected the invited email back, got %q", created.Email)
	}
	if !created.Pending {
		t.Error("a freshly invited admin must be reported as pending")
	}
	if len(created.InviteToken) != 64 {
		t.Errorf("expected a 64-char hex invite token, got %q", created.InviteToken)
	}
}

func TestCreateUser_MissingEmailReturnsBadRequest(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "   "})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListUsers_ReportsPendingAndActiveAdmins(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("invite second admin: %d %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var list []struct {
		Email       string  `json:"email"`
		Pending     bool    `json:"pending"`
		InviteToken *string `json:"inviteToken"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(list))
	}
	if list[0].Pending || list[0].InviteToken != nil {
		t.Errorf("the bootstrapped admin must be active with a null token, got %+v", list[0])
	}
	if !list[1].Pending || list[1].InviteToken == nil || *list[1].InviteToken == "" {
		t.Errorf("the invited admin must be pending with a token, got %+v", list[1])
	}
}

func TestCreateUser_DuplicateEmailReturnsConflict(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestDeleteUser_CannotDeleteLastRemainingUser(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	listReq := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var list []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 user, got %d", len(list))
	}

	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/users/%d", list[0].ID), nil)
	delReq.AddCookie(cookie)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when deleting last user, got %d", delRec.Code)
	}
}

// listUserIDs returns the ids currently reported by GET /api/users.
func listUserIDs(t *testing.T, router http.Handler, cookie *http.Cookie) []int64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: %d %s", rec.Code, rec.Body.String())
	}

	var list []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	ids := make([]int64, 0, len(list))
	for _, u := range list {
		ids = append(ids, u.ID)
	}
	return ids
}

func TestDeleteUser_RemovesTheUserAndReturnsOK(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create second admin: %d %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}

	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/users/%d", created.ID), nil)
	delReq.AddCookie(cookie)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting a non-last user, got %d: %s", delRec.Code, delRec.Body.String())
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(delRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if body.Status != "deleted" {
		t.Errorf("expected status \"deleted\", got %q", body.Status)
	}

	// The user must actually be gone, not just reported as deleted.
	for _, id := range listUserIDs(t, router, cookie) {
		if id == created.ID {
			t.Fatalf("user %d still listed after a successful DELETE", created.ID)
		}
	}
}

func TestDeleteUser_UnknownIDReturnsNotFound(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	// A second admin exists so the last-user guard is not what answers here.
	payload, _ := json.Marshal(map[string]string{"email": "second@example.com"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create second admin: %d %s", createRec.Code, createRec.Body.String())
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/users/424242", nil)
	delReq.AddCookie(cookie)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting an unknown user id, got %d: %s", delRec.Code, delRec.Body.String())
	}
}

// TestDeleteUser_DeletedUsersSessionIsRejected covers the ON DELETE CASCADE on
// sessions.user_id: removing an admin must immediately invalidate whatever
// session that admin was holding.
func TestDeleteUser_DeletedUsersSessionIsRejected(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	adminCookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	victimToken := inviteAdmin(t, router, adminCookie, "victim@example.com")

	acceptPayload, _ := json.Marshal(map[string]string{"password": "victimpass1"})
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, httptest.NewRequest(http.MethodPost, "/api/invites/"+victimToken, bytes.NewReader(acceptPayload)))
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept invite: %d %s", acceptRec.Code, acceptRec.Body.String())
	}

	var victim struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(acceptRec.Body).Decode(&victim); err != nil {
		t.Fatalf("decode victim: %v", err)
	}

	victimCookie := loginAndGetCookie(t, router, "victim@example.com", "victimpass1")

	// Sanity check: the victim's session works before the deletion.
	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(victimCookie)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("victim session should work before deletion, got %d", meRec.Code)
	}

	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/users/%d", victim.ID), nil)
	delReq.AddCookie(adminCookie)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete victim: %d %s", delRec.Code, delRec.Body.String())
	}

	afterReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	afterReq.AddCookie(victimCookie)
	afterRec := httptest.NewRecorder()
	router.ServeHTTP(afterRec, afterReq)
	if afterRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a deleted user's session, got %d: %s", afterRec.Code, afterRec.Body.String())
	}
}
