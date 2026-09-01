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

func TestPutSettings_UpdatesLanguageAndMasksKeys(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{
		"defaultLanguage":   "en",
		"youtubeApiKey":     "abcd1234efgh",
		"searchApiKey":      "search-secret-key",
		"searchApiProvider": "google",
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
		DefaultLanguage     string `json:"defaultLanguage"`
		YouTubeAPIKeySet    bool   `json:"youtubeApiKeySet"`
		YouTubeAPIKeyMasked string `json:"youtubeApiKeyMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DefaultLanguage != "en" {
		t.Fatalf("expected language 'en', got %q", body.DefaultLanguage)
	}
	if !body.YouTubeAPIKeySet {
		t.Fatal("expected youtubeApiKeySet to be true")
	}
	if body.YouTubeAPIKeyMasked == "abcd1234efgh" {
		t.Fatal("expected youtube key to be masked, not returned in clear")
	}
}

func TestPutSettings_EmptyKeyPreservesExistingKey(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	firstPayload, _ := json.Marshal(map[string]string{
		"defaultLanguage": "en",
		"youtubeApiKey":   "first-key-value",
	})
	firstReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(firstPayload))
	firstReq.AddCookie(cookie)
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on first PUT, got %d: %s", firstRec.Code, firstRec.Body.String())
	}

	// Second PUT: different defaultLanguage, youtubeApiKey field omitted entirely
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
		DefaultLanguage     string `json:"defaultLanguage"`
		YouTubeAPIKeySet    bool   `json:"youtubeApiKeySet"`
		YouTubeAPIKeyMasked string `json:"youtubeApiKeyMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// defaultLanguage DID change (proves the second PUT actually applied).
	if body.DefaultLanguage != "it" {
		t.Fatalf("expected language to reflect second PUT ('it'), got %q", body.DefaultLanguage)
	}
	// youtubeApiKey did NOT change (proves the empty field preserved the existing key
	// rather than clearing it).
	if !body.YouTubeAPIKeySet {
		t.Fatal("expected youtubeApiKeySet to still be true after second PUT with empty key field")
	}
	expectedMasked := maskedSuffix("first-key-value")
	if body.YouTubeAPIKeyMasked != expectedMasked {
		t.Fatalf("expected youtube key to still be masked from original value (suffix %q), got %q", expectedMasked, body.YouTubeAPIKeyMasked)
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
