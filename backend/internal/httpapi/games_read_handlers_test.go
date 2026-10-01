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

func createTestGame(t *testing.T, router http.Handler, cookie *http.Cookie, name string) int64 {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"languageCode": "it", "name": name, "nameTranslated": name})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create game setup failed: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ID
}

func TestListGames_IsPublic(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createTestGame(t, router, cookie, "Azul")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/games", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without auth, got %d", rec.Code)
	}
	var list []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 game, got %d", len(list))
	}
}

func TestGetGame_IsPublicAndIncludesLanguages(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", id), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Name      string `json:"name"`
		Languages []any  `json:"languages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul" || len(body.Languages) != 1 {
		t.Fatalf("unexpected detail: %+v", body)
	}
}

func TestGetGame_NotFoundReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/games/999", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateGame_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"owner": "Luigi"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d", id), bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUpdateGame_ChangesOwner(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"owner": "Luigi"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Owner *string `json:"owner"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Owner == nil || *body.Owner != "Luigi" {
		t.Fatalf("expected owner Luigi, got %v", body.Owner)
	}
}

func TestDeleteGame_RemovesIt(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/games/%d", id), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", id), nil))
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getRec.Code)
	}
}

func patchGame(t *testing.T, router http.Handler, cookie *http.Cookie, id int64, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func listGameNames(t *testing.T, router http.Handler, cookie *http.Cookie) map[string]map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := map[string]map[string]any{}
	for _, g := range list {
		out[g["name"].(string)] = g
	}
	return out
}

func TestListGames_HidesHiddenGamesFromVisitorsOnly(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createTestGame(t, router, cookie, "Azul")
	hiddenID := createTestGame(t, router, cookie, "Segreto")
	if rec := patchGame(t, router, cookie, hiddenID, map[string]any{"hiddenFromCatalog": true}); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}

	public := listGameNames(t, router, nil)
	if _, ok := public["Segreto"]; ok || len(public) != 1 {
		t.Fatalf("visitors must not see hidden games: %v", public)
	}
	if _, ok := public["Azul"]["hiddenFromCatalog"]; ok {
		t.Fatal("the hidden flag is admin-only")
	}

	admin := listGameNames(t, router, cookie)
	if len(admin) != 2 || admin["Segreto"]["hiddenFromCatalog"] != true {
		t.Fatalf("admins see every game with its flag: %v", admin)
	}

	// La scheda resta raggiungibile da link.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", hiddenID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("hidden game detail: %d", rec.Code)
	}
}

func TestGameType_DefaultsToFirstAndIsEditable(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	if got := listGameNames(t, router, nil)["Azul"]["gameTypeId"]; got != float64(1) {
		t.Fatalf("gameTypeId = %v, want 1", got)
	}
	if rec := patchGame(t, router, cookie, id, map[string]any{"gameTypeId": 2}); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if got := listGameNames(t, router, nil)["Azul"]["gameTypeId"]; got != float64(2) {
		t.Fatalf("gameTypeId = %v, want 2", got)
	}
	if rec := patchGame(t, router, cookie, id, map[string]any{"gameTypeId": 99}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown type: expected 400, got %d", rec.Code)
	}
}

func TestCreateGame_AcceptsGameType(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	for typeID, want := range map[int]int{2: http.StatusCreated, 99: http.StatusBadRequest} {
		payload, _ := json.Marshal(map[string]any{"languageCode": "it", "name": fmt.Sprintf("Gioco %d", typeID), "gameTypeId": typeID})
		req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("type %d: expected %d, got %d %s", typeID, want, rec.Code, rec.Body.String())
		}
	}
	if got := listGameNames(t, router, nil)["Gioco 2"]["gameTypeId"]; got != float64(2) {
		t.Fatalf("gameTypeId = %v, want 2", got)
	}
}
