package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
)

func TestGetMaterialsStartsEmpty(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d/materials", gameID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("volevo 200, ho %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Materials []map[string]any `json:"materials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Materials == nil {
		t.Fatal("volevo [] e non null: una lista vuota che arriva come null costringe la UI a difendersi")
	}
	if len(body.Materials) != 0 {
		t.Fatalf("volevo una lista vuota, ho %+v", body.Materials)
	}
}

func TestPutMaterialsSavesAndReadsBack(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	payload := `{"materials":[{"name":"tessere","quantity":72},{"name":"meeple","quantity":40}]}`
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/games/%d/materials", gameID), strings.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("volevo 200, ho %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Materials []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Quantity int    `json:"quantity"`
		} `json:"materials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Materials) != 2 || body.Materials[0].Name != "tessere" || body.Materials[1].Quantity != 40 {
		t.Fatalf("risposta inattesa: %+v", body.Materials)
	}
	if body.Materials[0].ID == 0 {
		t.Error("la risposta deve portare gli id: la checklist li rimanda indietro")
	}

	// Rileggendo con una GET si vede lo stesso stato salvato dalla PUT.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d/materials", gameID), nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET dopo la PUT: volevo 200, ho %d: %s", getRec.Code, getRec.Body)
	}
	var getBody struct {
		Materials []struct {
			Name     string `json:"name"`
			Quantity int    `json:"quantity"`
		} `json:"materials"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getBody); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(getBody.Materials) != 2 || getBody.Materials[1].Name != "meeple" {
		t.Fatalf("la GET non rispecchia la PUT: %+v", getBody.Materials)
	}
}

func TestPutMaterialsRejectsBadRows(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	cases := map[string]string{
		"nome vuoto":    `{"materials":[{"name":"  ","quantity":4}]}`,
		"quantità zero": `{"materials":[{"name":"dadi","quantity":0}]}`,
		"duplicato":     `{"materials":[{"name":"meeple","quantity":40},{"name":"Meeple","quantity":8}]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/games/%d/materials", gameID), strings.NewReader(payload))
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("volevo 400, ho %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestMaterialsRoutesNeedASession(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	for _, tc := range []struct {
		method, body string
	}{
		{http.MethodGet, ""},
		{http.MethodPut, `{"materials":[]}`},
	} {
		req := httptest.NewRequest(tc.method, fmt.Sprintf("/api/games/%d/materials", gameID), strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s senza cookie: volevo 401, ho %d", tc.method, rec.Code)
		}
	}
}

func TestMaterialsOnAMissingGameIs404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodGet, "/api/games/9999/materials", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("volevo 404, ho %d: %s", rec.Code, rec.Body)
	}
}

// indexOneChunk mette una fonte indicizzata sul gioco, il minimo perché la
// ricerca FTS trovi qualcosa.
func indexOneChunk(t *testing.T, conn *sql.DB, gameID int64, text string) {
	t.Helper()
	store := manuals.NewStore(conn)
	err := store.ReplaceSource(context.Background(), gameID, nil, []manuals.SourceChunk{{
		ReferenceType: "faq", Reference: "https://example.test/faq",
		Heading: "Contenuto della scatola", Seq: 0, Text: text,
	}})
	if err != nil {
		t.Fatalf("index chunk: %v", err)
	}
}

func TestSuggestMaterialsNeedsAnIndexedManual(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.MaterialLister = &fakeMaterialLister{
		out: []ai.SuggestedMaterial{{Name: "tessere", Quantity: 72}},
	}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/materials/suggest", gameID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Nessuna fonte indicizzata: 422 con il consiglio giusto, come fa la
	// rigenerazione delle domande suggerite.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("volevo 422, ho %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "indicizza") {
		t.Errorf("il messaggio deve dire cosa fare, ho %s", rec.Body)
	}
}

func TestSuggestMaterialsProposesWithoutSaving(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	lister := &fakeMaterialLister{out: []ai.SuggestedMaterial{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
	}}
	server.MaterialLister = lister
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")
	indexOneChunk(t, conn, gameID, "Contenuto della scatola: 72 tessere e 40 meeple.")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/materials/suggest", gameID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("volevo 200, ho %d: %s", rec.Code, rec.Body)
	}
	if lister.calls != 1 {
		t.Fatalf("volevo una chiamata al modello, ne ho %d", lister.calls)
	}
	if len(lister.lastPassages) == 0 {
		t.Error("al modello devono arrivare i passaggi del manuale")
	}
	var body struct {
		Materials []map[string]any `json:"materials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Materials) != 2 {
		t.Fatalf("volevo 2 voci proposte, ho %+v", body.Materials)
	}

	// La proposta non tocca il DB: è l'admin a confermare.
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM game_material`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("la proposta ha salvato %d voci: non deve salvare niente", count)
	}
}

func TestSuggestMaterialsTellsWhenTheModelIsUseless(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.MaterialLister = &fakeMaterialLister{err: ai.ErrMaterialsRejected}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")
	indexOneChunk(t, conn, gameID, "Contenuto della scatola: tante cose.")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/materials/suggest", gameID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("volevo 422, ho %d: %s", rec.Code, rec.Body)
	}
}
