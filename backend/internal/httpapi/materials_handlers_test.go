package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/httpapi"
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
