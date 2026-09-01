package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

func createTestGameForEvent(t *testing.T, gamesStore *games.Store, name string) int64 {
	t.Helper()
	g, err := gamesStore.CreateGame(context.Background(), games.Game{Name: name})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return g.ID
}

func TestListEvents_PublicSeesOnlyFutureEvents(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	if _, err := server.Events.CreateEvent(context.Background(), "Passato", nil, "2020-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create past event: %v", err)
	}
	if _, err := server.Events.CreateEvent(context.Background(), "Futuro", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create future event: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body []struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].Title != "Futuro" {
		t.Fatalf("expected only the future event for an anonymous request, got %+v", body)
	}
}

func TestListEvents_AdminSeesPastEventsToo(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	if _, err := server.Events.CreateEvent(context.Background(), "Passato", nil, "2020-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create past event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body []struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].Title != "Passato" {
		t.Fatalf("expected the past event to be visible to an admin, got %+v", body)
	}
}

func TestGetEvent_ReturnsGamesWithRemainingCapacity(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 3}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d", event.ID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Title string `json:"title"`
		Games []struct {
			Name      string `json:"name"`
			Quantity  int    `json:"quantity"`
			Remaining int    `json:"remaining"`
		} `json:"games"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Title != "Serata giochi" || len(body.Games) != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Games[0].Name != "Catan" || body.Games[0].Quantity != 3 || body.Games[0].Remaining != 3 {
		t.Fatalf("unexpected game entry: %+v", body.Games[0])
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events/999", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
