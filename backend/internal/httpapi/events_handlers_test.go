package httpapi_test

import (
	"bytes"
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

type eventListItem struct {
	Title      string  `json:"title"`
	ImagePath  *string `json:"imagePath"`
	GamesCount int     `json:"gamesCount"`
}

type eventListResponse struct {
	Items    []eventListItem `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

func listEvents(t *testing.T, router http.Handler, url string, cookie *http.Cookie) eventListResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", url, rec.Code, rec.Body.String())
	}
	var body eventListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func createEventsForList(t *testing.T, server *httpapi.Server, titledDates map[string]string) {
	t.Helper()
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	for title, date := range titledDates {
		if _, err := server.Events.CreateEvent(context.Background(), title, nil, date, "20:00",
			[]events.EventGameInput{{GameID: gameID, Copies: 1}}); err != nil {
			t.Fatalf("create event %q: %v", title, err)
		}
	}
}

func TestListEvents_ReturnsOnlyUpcomingEventsUnpaged(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	createEventsForList(t, server, map[string]string{"Passato": "2020-01-01", "Futuro": "2099-01-01"})

	body := listEvents(t, router, "/api/events", nil)
	if len(body.Items) != 1 || body.Items[0].Title != "Futuro" {
		t.Fatalf("expected only the upcoming event, got %+v", body.Items)
	}
	if body.Total != 1 || body.Page != 1 || body.PageSize != 0 {
		t.Fatalf("expected an unpaged single-item response, got %+v", body)
	}
	if body.Items[0].GamesCount != 1 {
		t.Fatalf("expected the games count in the list, got %+v", body.Items[0])
	}
}

func TestListEvents_AdminSeesTheUpcomingEventsByDefaultToo(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createEventsForList(t, server, map[string]string{"Passato": "2020-01-01", "Futuro": "2099-01-01"})

	body := listEvents(t, router, "/api/events", cookie)
	if len(body.Items) != 1 || body.Items[0].Title != "Futuro" {
		t.Fatalf("expected the upcoming event, got %+v", body.Items)
	}
}

func TestListEvents_AdminPagesThroughPastEventsMostRecentFirst(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	dates := map[string]string{}
	for day := 1; day <= 14; day++ {
		dates[fmt.Sprintf("Serata %02d", day)] = fmt.Sprintf("2020-01-%02d", day)
	}
	createEventsForList(t, server, dates)

	first := listEvents(t, router, "/api/events?past=true", cookie)
	if first.Total != 14 || first.PageSize != 12 || first.Page != 1 {
		t.Fatalf("unexpected pager on the first page: %+v", first)
	}
	if len(first.Items) != 12 || first.Items[0].Title != "Serata 14" {
		t.Fatalf("expected 12 past events, most recent first, got %+v", first.Items)
	}

	second := listEvents(t, router, "/api/events?past=true&page=2", cookie)
	if second.Page != 2 || len(second.Items) != 2 || second.Items[0].Title != "Serata 02" {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestListEvents_PastArchiveIsNotServedToThePublic(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	createEventsForList(t, server, map[string]string{"Passato": "2020-01-01", "Futuro": "2099-01-01"})

	body := listEvents(t, router, "/api/events?past=true", nil)
	if len(body.Items) != 1 || body.Items[0].Title != "Futuro" {
		t.Fatalf("expected an anonymous past=true request to fall back to the upcoming events, got %+v", body.Items)
	}
}

func TestListEvents_PageBeyondTheEndFallsBackToTheLastFullOne(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	dates := map[string]string{}
	for day := 1; day <= 13; day++ {
		dates[fmt.Sprintf("Serata %02d", day)] = fmt.Sprintf("2020-01-%02d", day)
	}
	createEventsForList(t, server, dates)

	body := listEvents(t, router, "/api/events?past=true&page=9", cookie)
	if body.Page != 2 || len(body.Items) != 1 || body.Items[0].Title != "Serata 01" {
		t.Fatalf("expected a fallback to the last page, got %+v", body)
	}
}

func TestListEvents_InvalidPageFallsBackToTheFirstOne(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createEventsForList(t, server, map[string]string{"Passato": "2020-01-01"})

	for _, query := range []string{"page=0", "page=-3", "page=abc"} {
		body := listEvents(t, router, "/api/events?past=true&"+query, cookie)
		if body.Page != 1 || len(body.Items) != 1 {
			t.Fatalf("query %q: expected the first page, got %+v", query, body)
		}
	}
}

func TestGetEvent_ReturnsGamesWithRemainingCapacity(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 3}})
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
			CopyIndex int    `json:"copyIndex"`
			Seats     int    `json:"seats"`
			Remaining int    `json:"remaining"`
		} `json:"games"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Title != "Serata giochi" || len(body.Games) != 3 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Games[0].Name != "Catan" || body.Games[0].Seats != 1 || body.Games[0].Remaining != 1 {
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

func TestCreateEvent_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]any{"title": "Serata", "eventDate": "2099-01-01", "startTime": "20:00"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateEvent_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "copies": 2}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEvent_RejectsZeroCopies(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "copies": 0}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEvent_RejectsMalformedDate(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "01/01/2099", "startTime": "20:00",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEvent_RejectsDuplicateGame(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{
			{"gameId": gameID, "copies": 1},
			{"gameId": gameID, "copies": 2},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateEvent_RejectsQuantityBelowActiveBookings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := server.Events.ListEventGames(context.Background(), event.ID)
	// Entrambe le copie occupate: non è possibile scendere a una sola copia
	// senza liberarne prima una.
	for _, eg := range eventGames {
		if err := server.Events.TestInsertBooking(event.ID, eg.ID, "active"); err != nil {
			t.Fatalf("insert booking fixture: %v", err)
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "copies": 1}},
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/events/%d", event.ID), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteEvent_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/events/%d", event.ID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestListEventBookings_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events/1/bookings", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListEventBookings_ReturnsActiveBookings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := server.Events.ListEventGames(context.Background(), event.ID)
	if err := server.Events.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d/bookings", event.ID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body []struct {
		GameID   int64  `json:"gameId"`
		GameName string `json:"gameName"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].GameName != "Catan" {
		t.Fatalf("unexpected body: %+v", body)
	}
	// L'id serve alla scheda admin per linkare il gioco dalla riga.
	if body[0].GameID != gameID {
		t.Fatalf("expected the game id %d in the booking row, got %d", gameID, body[0].GameID)
	}
}

func TestCreateEvent_WithSeveralCopiesOfTheSameGame(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "copies": 2}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created event: %v", err)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/events/%d", created.ID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Games []struct {
			EventGameID int64 `json:"eventGameId"`
			GameID      int64 `json:"gameId"`
			CopyIndex   int   `json:"copyIndex"`
			Seats       int   `json:"seats"`
			Remaining   int   `json:"remaining"`
		} `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Games) != 2 {
		t.Fatalf("expected 2 copies in the payload, got %d", len(detail.Games))
	}
	if detail.Games[0].CopyIndex != 1 || detail.Games[1].CopyIndex != 2 {
		t.Fatalf("unexpected copy indexes: %+v", detail.Games)
	}
	if detail.Games[0].Seats != 1 || detail.Games[0].Remaining != 1 {
		t.Fatalf("unexpected seats/remaining: %+v", detail.Games[0])
	}
	if detail.Games[0].EventGameID == detail.Games[1].EventGameID {
		t.Fatal("expected two distinct eventGameId values")
	}
	if detail.Games[0].GameID != gameID {
		t.Fatalf("unexpected gameId %d", detail.Games[0].GameID)
	}
}
