package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

type testBooking struct {
	ID          int64  `json:"id"`
	BookingCode string `json:"bookingCode"`
}

func createTestBooking(t *testing.T, router http.Handler, eventID, eventGameID int64) testBooking {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGameID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create booking: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body testBooking
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestSubmitMatchResult_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	payload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players": []map[string]any{
			{"name": "Mario", "score": 42},
			{"name": "Luigi", "score": 30},
		},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Players []struct {
			Name  string `json:"name"`
			Score int    `json:"score"`
		} `json:"players"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Players) != 2 || body.Players[0].Name != "Mario" || body.Players[0].Score != 42 {
		t.Fatalf("unexpected players: %+v", body.Players)
	}
}

func TestSubmitMatchResult_WrongCodeReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	payload, _ := json.Marshal(map[string]any{
		"bookingCode": "WRONGCOD",
		"players":     []map[string]any{{"name": "Mario", "score": 42}},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(payload)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSubmitMatchResult_CancelledBookingReturns409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	cancelPayload, _ := json.Marshal(map[string]string{"bookingCode": booking.BookingCode})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/cancel", booking.ID), bytes.NewReader(cancelPayload)))

	payload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 42}},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(payload)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitMatchResult_ResubmitReplacesPlayers(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	firstPayload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 10}},
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(firstPayload)))

	secondPayload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 50}, {"name": "Luigi", "score": 20}},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(secondPayload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Players []struct {
			Name string `json:"name"`
		} `json:"players"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Players) != 2 {
		t.Fatalf("expected 2 players after resubmission, got %d", len(body.Players))
	}
}
