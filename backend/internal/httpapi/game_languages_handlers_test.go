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

func TestCreateLanguage_PrefillsFromBaseLanguage(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code           string `json:"code"`
		IsBaseLanguage bool   `json:"isBaseLanguage"`
		Name           string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "en" || body.IsBaseLanguage {
		t.Fatalf("unexpected new language: %+v", body)
	}
	if body.Name != "Azul" {
		t.Fatalf("expected new language prefilled with base name 'Azul', got %q", body.Name)
	}
}

func TestCreateLanguage_PrefillsFromEditedBaseLanguage(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	// Edit the base language BEFORE adding a new one, so game.Name and the
	// base language's saved Name diverge — this is what actually proves the
	// prefill reads the base language's row, not the static game name.
	editPayload, _ := json.Marshal(map[string]string{"name": "Azul (edited)", "description": "Edited description."})
	editReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d/languages/it", id), bytes.NewReader(editPayload))
	editReq.AddCookie(cookie)
	editRec := httptest.NewRecorder()
	router.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected 200 editing base language, got %d: %s", editRec.Code, editRec.Body.String())
	}

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul (edited)" {
		t.Fatalf("expected new language prefilled with the EDITED base name 'Azul (edited)', got %q — this would incorrectly pass if the handler used game.Name directly instead of reading the base language's saved row", body.Name)
	}
	if body.Description == nil || *body.Description != "Edited description." {
		t.Fatalf("expected new language prefilled with the EDITED base description, got %v", body.Description)
	}
}

func TestCreateLanguage_DuplicateCodeReturns409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected first add to succeed with 201, got %d: %s", rec.Code, rec.Body.String())
	}

	dupeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	dupeReq.AddCookie(cookie)
	dupeRec := httptest.NewRecorder()
	router.ServeHTTP(dupeRec, dupeReq)
	if dupeRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate language code, got %d: %s", dupeRec.Code, dupeRec.Body.String())
	}
}

func TestCreateLanguage_NormalizesCaseAndWhitespaceBeforeDedup(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected first add to succeed with 201, got %d: %s", rec.Code, rec.Body.String())
	}

	dupePayload, _ := json.Marshal(map[string]string{"languageCode": " EN "})
	dupeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(dupePayload))
	dupeReq.AddCookie(cookie)
	dupeRec := httptest.NewRecorder()
	router.ServeHTTP(dupeRec, dupeReq)
	if dupeRec.Code != http.StatusConflict {
		t.Fatalf("expected ' EN ' to collide with normalized existing 'en' and return 409, got %d: %s", dupeRec.Code, dupeRec.Body.String())
	}
}

func TestCreateLanguage_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUpdateLanguage_ChangesNameAndDescription(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"name": "Azul (IT)", "description": "Un gioco di piastrelle."})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d/languages/it", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul (IT)" || body.Description == nil || *body.Description != "Un gioco di piastrelle." {
		t.Fatalf("unexpected updated language: %+v", body)
	}
}

func TestCreateLanguage_TranslatesFromTheRawBGGDescription(t *testing.T) {
	tr := &fakeTranslator{out: "Ein Spiel über Vögel."}
	server, _ := newTestServerWithTranslator(t, tr)
	router, cookie := setupWingspanFromBGG(t, server)

	rec := postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating the game, got %d: %s", rec.Code, rec.Body.String())
	}
	// Creare il gioco da BGG traduce già la lingua base: azzera il
	// contatore prima di provare la traduzione della nuova lingua.
	tr.calls = 0

	payload, _ := json.Marshal(map[string]string{"languageCode": "de"})
	req := httptest.NewRequest(http.MethodPost, "/api/games/1/languages", bytes.NewReader(payload))
	req.AddCookie(cookie)
	langRec := httptest.NewRecorder()
	router.ServeHTTP(langRec, req)
	if langRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", langRec.Code, langRec.Body.String())
	}
	if tr.calls != 1 {
		t.Fatalf("expected exactly one translation call for the new language, got %d", tr.calls)
	}
	if tr.lastText != "A worker placement game about birds." || tr.lastLang != "de" {
		t.Fatalf("expected the raw BGG text translated into de, got %q / %q", tr.lastText, tr.lastLang)
	}
	var got map[string]any
	json.Unmarshal(langRec.Body.Bytes(), &got)
	if got["description"] != "Ein Spiel über Vögel." {
		t.Fatalf("expected the translated description, got %v", got["description"])
	}
}

func TestCreateLanguage_ManualGameFallsBackToTheBaseLanguage(t *testing.T) {
	tr := &fakeTranslator{out: "NON DEVE COMPARIRE"}
	server, _ := newTestServerWithTranslator(t, tr)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Gioco fatto in casa")

	// La descrizione base è scritta dall'admin, non tradotta.
	editPayload, _ := json.Marshal(map[string]string{"name": "Gioco fatto in casa", "description": "Un gioco di carte fatto in casa"})
	editReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d/languages/it", id), bytes.NewReader(editPayload))
	editReq.AddCookie(cookie)
	editRec := httptest.NewRecorder()
	router.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected 200 editing base language, got %d: %s", editRec.Code, editRec.Body.String())
	}

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.calls != 0 {
		t.Fatalf("a manual game has no BGG original to translate, got %d calls", tr.calls)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	// Senza originale BGG resta il ripiego di sempre: si copia la lingua
	// base come punto di partenza per una traduzione a mano.
	if got["description"] != "Un gioco di carte fatto in casa" {
		t.Fatalf("expected the base language text as the fallback, got %v", got["description"])
	}
}

func TestUpdateLanguage_NotFoundReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"name": "Nope"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d/languages/de", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// postLanguage aggiunge una lingua a un gioco. body è il JSON grezzo, così i
// test possono mandare anche una sorgente inesistente o un campo assente.
func postLanguage(router http.Handler, cookie *http.Cookie, gameID int64, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", gameID), bytes.NewReader([]byte(body)))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// wingspanWithItalianBase crea Wingspan da BGG con lingua base italiana e
// rimette a zero il contatore del traduttore, che quella creazione ha già
// usato una volta per la lingua base.
func wingspanWithItalianBase(t *testing.T, tr *fakeTranslator) (http.Handler, *http.Cookie) {
	t.Helper()
	server, _ := newTestServerWithTranslator(t, tr)
	router, cookie := setupWingspanFromBGG(t, server)
	if rec := postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`); rec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201 creating the game, got %d: %s", rec.Code, rec.Body.String())
	}
	tr.calls, tr.lastText, tr.lastLang = 0, "", ""
	return router, cookie
}

func TestCreateLanguage_SourceBGGTranslatesTheOriginal(t *testing.T) {
	tr := &fakeTranslator{out: "Ein Spiel über Vögel."}
	router, cookie := wingspanWithItalianBase(t, tr)

	rec := postLanguage(router, cookie, 1, `{"languageCode":"de","source":"bgg"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.lastText != "A worker placement game about birds." || tr.lastLang != "de" {
		t.Fatalf("expected the raw BGG text translated into de, got %q / %q", tr.lastText, tr.lastLang)
	}
}

func TestCreateLanguage_SourceExistingLanguageTranslatesThatText(t *testing.T) {
	tr := &fakeTranslator{out: "Ein Spiel über Vögel."}
	router, cookie := wingspanWithItalianBase(t, tr)

	// L'admin corregge a mano l'italiano: da lì in poi è una sorgente
	// migliore dell'inglese di BGG, ed è il motivo per cui si può sceglierla.
	editPayload, _ := json.Marshal(map[string]string{"name": "Wingspan", "description": "Un gioco corretto a mano."})
	editReq := httptest.NewRequest(http.MethodPatch, "/api/games/1/languages/it", bytes.NewReader(editPayload))
	editReq.AddCookie(cookie)
	editRec := httptest.NewRecorder()
	router.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("setup: expected 200 editing the base language, got %d", editRec.Code)
	}

	rec := postLanguage(router, cookie, 1, `{"languageCode":"de","source":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.lastText != "Un gioco corretto a mano." {
		t.Fatalf("expected the hand-corrected Italian as the source, got %q", tr.lastText)
	}
}

func TestCreateLanguage_UnknownSourceIsNotFound(t *testing.T) {
	tr := &fakeTranslator{out: "irrilevante"}
	router, cookie := wingspanWithItalianBase(t, tr)

	rec := postLanguage(router, cookie, 1, `{"languageCode":"de","source":"fr"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a source language the game does not have, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.calls != 0 {
		t.Fatalf("expected no translation call for an unknown source, got %d", tr.calls)
	}
}

func TestCreateLanguage_EmptySourceKeepsTheOldBehaviour(t *testing.T) {
	tr := &fakeTranslator{out: "Ein Spiel über Vögel."}
	router, cookie := wingspanWithItalianBase(t, tr)

	rec := postLanguage(router, cookie, 1, `{"languageCode":"de"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	// Sorgente assente: si continua a partire dall'originale BGG, come prima
	// che la scelta esistesse.
	if tr.lastText != "A worker placement game about birds." {
		t.Fatalf("expected the BGG original as the default source, got %q", tr.lastText)
	}
}

func TestCreateLanguage_SourceWithoutAICopiesItVerbatim(t *testing.T) {
	server := newTestServer(t)
	router, cookie := setupWingspanFromBGG(t, server)
	if rec := postGame(router, cookie, `{"bggId":"266192","languageCode":"it"}`); rec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d", rec.Code)
	}

	// Senza AI la lingua base ha ricevuto il testo BGG invariato: la si
	// modifica, altrimenti il test passerebbe anche leggendo la sorgente
	// sbagliata, perché i due testi coinciderebbero.
	editPayload, _ := json.Marshal(map[string]string{"name": "Wingspan", "description": "Testo italiano scritto a mano."})
	editReq := httptest.NewRequest(http.MethodPatch, "/api/games/1/languages/it", bytes.NewReader(editPayload))
	editReq.AddCookie(cookie)
	editRec := httptest.NewRecorder()
	router.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("setup: expected 200 editing the base language, got %d", editRec.Code)
	}

	rec := postLanguage(router, cookie, 1, `{"languageCode":"de","source":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Description *string `json:"description"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	// Senza provider il select è un "copia da": arriva il testo della lingua
	// scelta, invariato.
	if body.Description == nil || *body.Description != "Testo italiano scritto a mano." {
		t.Fatalf("expected the chosen source copied verbatim, got %v", body.Description)
	}
}
