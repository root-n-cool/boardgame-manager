package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestGetSettings_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPutSettings_UpdatesLanguageAndMasksTheToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{
		"defaultLanguage": "en",
		"bggApiToken":     "abcd1234efgh",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	putReq.AddCookie(cookie)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var body struct {
		DefaultLanguage   string `json:"defaultLanguage"`
		BGGAPITokenSet    bool   `json:"bggApiTokenSet"`
		BGGAPITokenMasked string `json:"bggApiTokenMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DefaultLanguage != "en" {
		t.Fatalf("expected language 'en', got %q", body.DefaultLanguage)
	}
	if !body.BGGAPITokenSet {
		t.Fatal("expected bggApiTokenSet to be true")
	}
	if body.BGGAPITokenMasked == "abcd1234efgh" {
		t.Fatal("expected the token to be masked, not returned in clear")
	}
}

func TestPutSettings_EmptyTokenPreservesExistingToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	firstPayload, _ := json.Marshal(map[string]string{
		"defaultLanguage": "en",
		"bggApiToken":     "first-key-value",
	})
	firstReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(firstPayload))
	firstReq.AddCookie(cookie)
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on first PUT, got %d: %s", firstRec.Code, firstRec.Body.String())
	}

	// Second PUT: different defaultLanguage, bggApiToken field omitted entirely
	// (decodes to the empty string, same as explicit ""), meaning "don't change this key".
	secondPayload, _ := json.Marshal(map[string]string{
		"defaultLanguage": "it",
	})
	secondReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(secondPayload))
	secondReq.AddCookie(cookie)
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on second PUT, got %d: %s", secondRec.Code, secondRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var body struct {
		DefaultLanguage   string `json:"defaultLanguage"`
		BGGAPITokenSet    bool   `json:"bggApiTokenSet"`
		BGGAPITokenMasked string `json:"bggApiTokenMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// defaultLanguage DID change (proves the second PUT actually applied).
	if body.DefaultLanguage != "it" {
		t.Fatalf("expected language to reflect second PUT ('it'), got %q", body.DefaultLanguage)
	}
	// bggApiToken did NOT change (proves the empty field preserved the existing
	// token rather than clearing it).
	if !body.BGGAPITokenSet {
		t.Fatal("expected bggApiTokenSet to still be true after second PUT with empty token field")
	}
	expectedMasked := maskedSuffix("first-key-value")
	if body.BGGAPITokenMasked != expectedMasked {
		t.Fatalf("expected the token to still be masked from original value (suffix %q), got %q", expectedMasked, body.BGGAPITokenMasked)
	}
}

// maskedSuffix mirrors the handler's masking format (last 4 chars) so the test
// can assert the preserved key is still derived from the ORIGINAL value, not reset.
func maskedSuffix(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

func TestPutSettings_HandlesBGGToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{
		"defaultLanguage": "it",
		"bggApiToken":     "abcd1234efgh",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	putReq.AddCookie(cookie)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var body struct {
		BGGAPITokenSet    bool   `json:"bggApiTokenSet"`
		BGGAPITokenMasked string `json:"bggApiTokenMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.BGGAPITokenSet {
		t.Fatal("expected bggApiTokenSet to be true")
	}
	if body.BGGAPITokenMasked == "abcd1234efgh" {
		t.Fatal("expected bgg token to be masked, not returned in clear")
	}
}

// putSettings is the shortest way to say "save these settings" in a test.
func putSettings(t *testing.T, router http.Handler, cookie *http.Cookie, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getSettings returns the settings response as a raw map, so a test can assert
// that a field is absent — not merely empty.
func getSettings(t *testing.T, router http.Handler, cookie *http.Cookie) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return out
}

func TestPutSettings_StoresThePublicBaseURLWithoutItsTrailingSlash(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"publicBaseUrl":   "  https://giochi.example.org/  ",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Trimmed and without the trailing slash, so a caller can concatenate a path
	// without producing a double slash.
	if got := getSettings(t, router, cookie)["publicBaseUrl"]; got != "https://giochi.example.org" {
		t.Fatalf("expected the normalised base URL, got %v", got)
	}
}

func TestPutSettings_RejectsAPublicBaseURLThatIsNotAnAbsoluteHTTPURL(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	for _, invalid := range []string{"giochi.example.org", "/invito", "ftp://giochi.example.org", "https://"} {
		rec := putSettings(t, router, cookie, map[string]string{
			"defaultLanguage": "it",
			"publicBaseUrl":   invalid,
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for %q, got %d: %s", invalid, rec.Code, rec.Body.String())
		}
	}
}

// An empty value is how the admin says "not configured": the frontend then falls
// back to the address the browser is already on, which is what makes a local
// install work with no settings at all.
func TestPutSettings_EmptyPublicBaseURLClearsIt(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	if rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"publicBaseUrl":   "https://giochi.example.org",
	}); rec.Code != http.StatusOK {
		t.Fatalf("first put: %d %s", rec.Code, rec.Body.String())
	}
	if rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"publicBaseUrl":   "",
	}); rec.Code != http.StatusOK {
		t.Fatalf("second put: %d %s", rec.Code, rec.Body.String())
	}

	if got := getSettings(t, router, cookie)["publicBaseUrl"]; got != "" {
		t.Fatalf("expected the base URL to be cleared, got %v", got)
	}
}

// The enrichment feature these keys were meant for was never built, so the
// fields must be gone from the API too — not just hidden in the UI.
func TestGetSettings_NoLongerExposesTheUnusedAPIKeyFields(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	body := getSettings(t, router, cookie)
	for _, gone := range []string{"youtubeApiKeySet", "youtubeApiKeyMasked", "searchApiKeySet", "searchApiKeyMasked", "searchApiProvider"} {
		if _, present := body[gone]; present {
			t.Errorf("expected %q to be gone from the settings response", gone)
		}
	}
}

func TestPutSettings_SavesAIProviderAndMasksTheKey(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"aiBaseUrl":       "https://api.example.org/v1/",
		"aiApiKey":        "sk-secret-1234",
		"aiModel":         "gemini-flash-lite-latest",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got := getSettings(t, router, cookie)
	// Lo slash finale viene tolto: il client ci appende /chat/completions.
	if got["aiBaseUrl"] != "https://api.example.org/v1" {
		t.Fatalf("expected the base URL without a trailing slash, got %v", got["aiBaseUrl"])
	}
	if got["aiModel"] != "gemini-flash-lite-latest" {
		t.Fatalf("unexpected model: %v", got["aiModel"])
	}
	if got["aiApiKeySet"] != true || got["aiConfigured"] != true {
		t.Fatalf("expected the provider to read as configured, got %v", got)
	}
	if got["aiApiKeyMasked"] != "****1234" {
		t.Fatalf("expected a masked key, got %v", got["aiApiKeyMasked"])
	}
	if _, leaked := got["aiApiKey"]; leaked {
		t.Fatal("the API key must never be served in clear")
	}
}

func TestPutSettings_EmptyAIKeyKeepsTheStoredOne(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	if rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"aiBaseUrl":       "https://api.example.org/v1",
		"aiApiKey":        "sk-secret-1234",
		"aiModel":         "m",
	}); rec.Code != http.StatusOK {
		t.Fatalf("first put: %d %s", rec.Code, rec.Body.String())
	}

	// Salvare di nuovo senza riscrivere la chiave non deve cancellarla:
	// il campo del form torna vuoto dopo ogni salvataggio.
	rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"aiBaseUrl":       "https://api.example.org/v1",
		"aiApiKey":        "",
		"aiModel":         "m2",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got := getSettings(t, router, cookie)
	if got["aiApiKeySet"] != true || got["aiModel"] != "m2" {
		t.Fatalf("expected the key kept and the model updated, got %v", got)
	}
}

func TestPutSettings_RejectsARelativeAIBaseURL(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"aiBaseUrl":       "api.example.org",
		"aiApiKey":        "k",
		"aiModel":         "m",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on a relative base URL, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSettings_NotConfiguredByDefault(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	got := getSettings(t, router, cookie)
	if got["aiConfigured"] != false || got["aiApiKeySet"] != false {
		t.Fatalf("expected no AI provider out of the box, got %v", got)
	}
}
