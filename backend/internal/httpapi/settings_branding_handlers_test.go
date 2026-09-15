package httpapi_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

// tinyPNG produce un PNG valido di un pixel: basta a passare la
// convalida di storage.CoverCategory (estensione + tipo sniffato).
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func uploadFile(t *testing.T, router http.Handler, cookie *http.Cookie, path, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestUploadLogo_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/logo", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUploadLogo_SavesTheFilenameAndItIsReadableBySite(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := uploadFile(t, router, cookie, "/api/settings/logo", "logo.png", tinyPNG(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	filename, _ := getSettings(t, router, cookie)["logoFilename"].(string)
	if filename == "" {
		t.Fatal("expected logoFilename to be set after upload")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/uploads/"+filename, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected the uploaded logo to be servable, got %d", getRec.Code)
	}

	site := httptest.NewRecorder()
	router.ServeHTTP(site, httptest.NewRequest(http.MethodGet, "/api/site", nil))
	var siteBody struct {
		LogoFilename string `json:"logoFilename"`
	}
	if err := json.NewDecoder(site.Body).Decode(&siteBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if siteBody.LogoFilename != filename {
		t.Fatalf("GET /api/site logoFilename = %q, atteso %q", siteBody.LogoFilename, filename)
	}
}

func TestUploadLogo_RejectsAnUnsupportedType(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := uploadFile(t, router, cookie, "/api/settings/logo", "logo.gif", []byte("GIF89a not a real gif"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadFavicon_SavesTheFilenameAndPreservesOtherSettings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	// Un titolo già salvato non deve sparire dopo l'upload della favicon:
	// l'handler deve preservare il resto della riga, non sovrascriverla.
	if rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"siteTitle":       "Ludoteca Vicolo Corto",
	}); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body.String())
	}

	rec := uploadFile(t, router, cookie, "/api/settings/favicon", "favicon.png", tinyPNG(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got := getSettings(t, router, cookie)
	if got["siteTitle"] != "Ludoteca Vicolo Corto" {
		t.Fatalf("expected the site title to survive the favicon upload, got %v", got["siteTitle"])
	}
	if got["faviconFilename"] == "" || got["faviconFilename"] == nil {
		t.Fatal("expected faviconFilename to be set after upload")
	}
}

func TestPutSettings_CannotSetLogoOrFaviconFilenameDirectly(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	// Upload a real logo first to get a legitimate filename
	rec := uploadFile(t, router, cookie, "/api/settings/logo", "logo.png", tinyPNG(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected logo upload to succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	realLogoFilename := getSettings(t, router, cookie)["logoFilename"].(string)
	if realLogoFilename == "" {
		t.Fatal("expected logoFilename to be set after upload")
	}

	// Now try to overwrite it via PUT /api/settings with attacker-supplied values
	rec = putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"logoFilename":    "attacker-supplied.png",
		"faviconFilename": "attacker-supplied.png",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the PUT's attempt to override was ignored
	got := getSettings(t, router, cookie)
	if got["logoFilename"] != realLogoFilename {
		t.Fatalf("expected logoFilename to remain %q, but PUT changed it to %q", realLogoFilename, got["logoFilename"])
	}
	if got["faviconFilename"] != nil && got["faviconFilename"] != "" {
		t.Fatalf("expected faviconFilename to remain empty, but got %q", got["faviconFilename"])
	}
}
