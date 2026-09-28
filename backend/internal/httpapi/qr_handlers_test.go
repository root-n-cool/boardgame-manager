package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"boardgames-manager/internal/httpapi"
)

type qrCardBody struct {
	Title                   string  `json:"title"`
	EventDate               string  `json:"eventDate"`
	EndTime                 *string `json:"endTime"`
	URL                     string  `json:"url"`
	PublicAddressConfigured bool    `json:"publicAddressConfigured"`
	SVG                     string  `json:"svg"`
}

func getQRCard(t *testing.T, router http.Handler, cookie *http.Cookie, path string) qrCardBody {
	t.Helper()
	rec := doLoanRequest(router, http.MethodGet, path, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", path, rec.Code, rec.Body.String())
	}
	var body qrCardBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestGameQR_EncodesThePublicGamePageOnThePublicAddress(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")
	putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it", "publicBaseUrl": "https://giochi.example.org",
	})
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	card := getQRCard(t, router, cookie, fmt.Sprintf("/api/games/%d/qr", gameID))
	want := fmt.Sprintf("https://giochi.example.org/games/%d", gameID)
	if card.URL != want || !card.PublicAddressConfigured || card.Title != "Catan" {
		t.Fatalf("unexpected card: url %q, configured %v, title %q", card.URL, card.PublicAddressConfigured, card.Title)
	}
	if !strings.HasPrefix(card.SVG, "<svg") || !strings.Contains(card.SVG, "<path") ||
		!strings.Contains(card.SVG, want) {
		t.Fatalf("expected an SVG labelled with %q, got %.300s", want, card.SVG)
	}
}

func TestEventQR_FallsBackToTheRequestHostAndSaysSo(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")
	eventID := createTestEvent(t, server, createTestGameForEvent(t, server.Games, "Catan"), 1)

	card := getQRCard(t, router, cookie, fmt.Sprintf("/api/events/%d/qr", eventID))
	if want := fmt.Sprintf("http://example.com/events/%d", eventID); card.URL != want {
		t.Fatalf("expected %q, got %q", want, card.URL)
	}
	if card.PublicAddressConfigured {
		t.Fatal("expected the fallback to be reported as not configured")
	}
	if card.Title != "Serata giochi" || card.EventDate != "2099-01-01" {
		t.Fatalf("expected the event's title and date, got %+v", card)
	}
}

func TestQR_UnknownTargetIs404AndAnonymousIs401(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")

	for _, path := range []string{"/api/games/999/qr", "/api/events/999/qr"} {
		if rec := doLoanRequest(router, http.MethodGet, path, cookie, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", path, rec.Code)
		}
		if rec := doLoanRequest(router, http.MethodGet, path, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s anonymous: expected 401, got %d", path, rec.Code)
		}
	}
}

func TestGamesQR_ListsEveryVisibleGameByNameWithItsCard(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "password123")
	putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it", "publicBaseUrl": "https://giochi.example.org",
	})
	catanID := createTestGameForEvent(t, server.Games, "Catan")
	azulID := createTestGameForEvent(t, server.Games, "azul")
	hiddenID := createTestGameForEvent(t, server.Games, "Segreto")
	if rec := patchGame(t, router, cookie, hiddenID, map[string]any{"hiddenFromCatalog": true}); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}

	rec := doLoanRequest(router, http.MethodGet, "/api/games/qr", cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		PublicAddressConfigured bool         `json:"publicAddressConfigured"`
		Cards                   []qrCardBody `json:"cards"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.PublicAddressConfigured {
		t.Fatal("expected the public address to be reported as configured")
	}
	if len(body.Cards) != 2 {
		t.Fatalf("expected the two visible games, got %+v", body.Cards)
	}
	for i, want := range []struct {
		title string
		id    int64
	}{{"azul", azulID}, {"Catan", catanID}} {
		url := fmt.Sprintf("https://giochi.example.org/games/%d", want.id)
		c := body.Cards[i]
		if c.Title != want.title || c.URL != url || !strings.Contains(c.SVG, url) {
			t.Errorf("card %d: expected %q at %q, got %q at %q", i, want.title, url, c.Title, c.URL)
		}
	}

	if rec := doLoanRequest(router, http.MethodGet, "/api/games/qr", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: expected 401, got %d", rec.Code)
	}
}
