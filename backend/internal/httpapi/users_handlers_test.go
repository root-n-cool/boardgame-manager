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

func TestCreateUser_AsAdminSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com", "password": "anotherpass1"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateUser_DuplicateEmailReturnsConflict(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "anotherpass1"})
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

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com", "password": "anotherpass1"})
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
	payload, _ := json.Marshal(map[string]string{"email": "second@example.com", "password": "anotherpass1"})
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

	payload, _ := json.Marshal(map[string]string{"email": "victim@example.com", "password": "victimpass1"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	createReq.AddCookie(adminCookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create victim: %d %s", createRec.Code, createRec.Body.String())
	}
	var victim struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&victim); err != nil {
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
