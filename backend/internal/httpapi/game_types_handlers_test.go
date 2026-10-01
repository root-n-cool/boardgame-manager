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

func gameTypesRequest(t *testing.T, router http.Handler, cookie *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func listGameTypes(t *testing.T, router http.Handler, cookie *http.Cookie) []map[string]any {
	t.Helper()
	rec := gameTypesRequest(t, router, cookie, http.MethodGet, "/api/game-types", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGameTypes_PublicListHidesCounts(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	public := listGameTypes(t, router, nil)
	if len(public) != 2 || public[0]["slug"] != "GDT" || public[0]["bggSearch"] != true {
		t.Fatalf("public = %v", public)
	}
	if _, ok := public[0]["gameCount"]; ok {
		t.Fatal("gameCount is admin-only")
	}
	admin := listGameTypes(t, router, cookie)
	if admin[0]["gameCount"] != float64(0) {
		t.Fatalf("admin = %v", admin)
	}
}

func TestGameTypes_WritesNeedAdmin(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/game-types"},
		{http.MethodPatch, "/api/game-types/1"},
		{http.MethodPost, "/api/game-types/1/move"},
		{http.MethodDelete, "/api/game-types/1"},
	} {
		if rec := gameTypesRequest(t, router, nil, c.method, c.path, map[string]any{}); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", c.method, c.path, rec.Code)
		}
	}
}

func TestGameTypes_CreateUpdateMove(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := gameTypesRequest(t, router, cookie, http.MethodPost, "/api/game-types",
		map[string]any{"name": "Magic: The Gathering", "slug": "mtg", "bggSearch": false})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	json.NewDecoder(rec.Body).Decode(&created)
	if created["slug"] != "MTG" || created["position"] != float64(3) {
		t.Fatalf("created = %v", created)
	}
	id := int64(created["id"].(float64))

	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, "/api/game-types",
		map[string]any{"name": "Doppione", "slug": "gdt"}); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate slug: %d", rec.Code)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, "/api/game-types",
		map[string]any{"name": "", "slug": "XY"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name: %d", rec.Code)
	}

	if rec := gameTypesRequest(t, router, cookie, http.MethodPatch, fmt.Sprintf("/api/game-types/%d", id),
		map[string]any{"name": "Magic"}); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodPatch, "/api/game-types/99",
		map[string]any{"name": "X"}); rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing: %d", rec.Code)
	}

	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, fmt.Sprintf("/api/game-types/%d/move", id),
		map[string]any{"direction": "up"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body.String())
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, fmt.Sprintf("/api/game-types/%d/move", id),
		map[string]any{"direction": "sideways"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad direction: %d", rec.Code)
	}
	list := listGameTypes(t, router, cookie)
	if list[1]["name"] != "Magic" {
		t.Fatalf("order after move = %v", list)
	}
}

func TestGameTypes_DeleteBlockedWhenInUse(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createTestGame(t, router, cookie, "Azul") // prende GDT (id 1)

	rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("in use: %d", rec.Code)
	}
	var body map[string]any
	json.NewDecoder(rec.Body).Decode(&body)
	if body["games"] != float64(1) {
		t.Fatalf("body = %v", body)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/2", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("empty: %d", rec.Code)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/2", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("gone: %d", rec.Code)
	}
}

func TestGameTypes_DeleteLastIsConflict(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	if rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/2", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("first: %d", rec.Code)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/1", nil); rec.Code != http.StatusConflict {
		t.Fatalf("last: %d %s", rec.Code, rec.Body.String())
	}
}
