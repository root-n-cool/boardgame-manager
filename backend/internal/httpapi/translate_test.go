package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

// postTranslate POSTs to the retranslation endpoint for one game/language.
func postTranslate(router http.Handler, cookie *http.Cookie, gameID int64, lang string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/%s/translate", gameID, lang), bytes.NewReader(nil))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestTranslateLanguage_OverwritesTheDescription(t *testing.T) {
	tr := &fakeTranslator{out: "Descrizione tradotta di nuovo."}
	server, _ := newTestServerWithTranslator(t, tr)
	router, cookie := setupWingspanFromBGG(t, server)

	rec := postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating the game, got %d: %s", rec.Code, rec.Body.String())
	}

	// Correggi a mano la descrizione con una PATCH, così il test prova che
	// il bottone di ritraduzione sovrascrive davvero (non è un no-op).
	patchPayload, _ := json.Marshal(map[string]string{"name": "Wingspan", "description": "Correzione manuale."})
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/games/1/languages/it", bytes.NewReader(patchPayload))
	patchReq.AddCookie(cookie)
	patchRec := httptest.NewRecorder()
	router.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 patching the language, got %d: %s", patchRec.Code, patchRec.Body.String())
	}

	rec = postTranslate(router, cookie, 1, "it")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["description"] != "Descrizione tradotta di nuovo." {
		t.Fatalf("expected the retranslated text, got %v", got["description"])
	}
	// Si traduce sempre dal grezzo BGG, mai dal testo già in scheda:
	// altrimenti si ritradurrebbe una traduzione o una correzione manuale.
	if tr.lastText != "A worker placement game about birds." {
		t.Fatalf("expected the raw BGG description as the source, got %q", tr.lastText)
	}
}

func TestTranslateLanguage_WithoutAIReturns409(t *testing.T) {
	server := newTestServer(t)
	router, cookie := setupWingspanFromBGG(t, server)

	rec := postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating the game, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = postTranslate(router, cookie, 1, "it")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 without an AI provider, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTranslateLanguage_WithoutARawDescriptionReturns409(t *testing.T) {
	tr := &fakeTranslator{out: "irrilevante"}
	server, _ := newTestServerWithTranslator(t, tr)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	// Gioco creato a mano: nessun bgg_description.
	createTestGame(t, router, cookie, "Gioco fatto in casa")

	rec := postTranslate(router, cookie, 1, "it")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 with nothing to translate from, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.calls != 0 {
		t.Fatalf("expected no provider call, got %d", tr.calls)
	}
}

func TestTranslateLanguage_UnknownGameOrLanguage(t *testing.T) {
	tr := &fakeTranslator{out: "irrilevante"}
	server, _ := newTestServerWithTranslator(t, tr)
	router, cookie := setupWingspanFromBGG(t, server)

	rec := postTranslate(router, cookie, 999, "it")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on an unknown game, got %d", rec.Code)
	}

	rec = postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating the game, got %d: %s", rec.Code, rec.Body.String())
	}

	// Il gioco esiste ma non ha la lingua francese.
	rec = postTranslate(router, cookie, 1, "fr")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on a language the game does not have, got %d", rec.Code)
	}
}

func TestTranslateLanguage_ProviderFailureReturns502(t *testing.T) {
	tr := &fakeTranslator{err: errors.New("provider down")}
	server, _ := newTestServerWithTranslator(t, tr)
	router, cookie := setupWingspanFromBGG(t, server)

	rec := postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating the game, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = postTranslate(router, cookie, 1, "it")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when the provider fails on an explicit request, got %d", rec.Code)
	}
}
