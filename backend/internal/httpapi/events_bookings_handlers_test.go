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
	"boardgames-manager/internal/httpapi"
)

func createTestEvent(t *testing.T, server *httpapi.Server, gameID int64, quantity int) int64 {
	t.Helper()
	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: quantity}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event.ID
}

func TestCreateBooking_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 2)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.BookingCode) != 8 {
		t.Fatalf("expected an 8-char booking code, got %q", body.BookingCode)
	}
}

func TestCreateBooking_SoldOutReturns409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	if err := server.Events.TestInsertBooking(eventID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLookupAndCancelBooking_FullFlow(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	createPayload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(createPayload)))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID          int64  `json:"id"`
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	lookupPayload, _ := json.Marshal(map[string]string{"email": "mario@example.com", "bookingCode": created.BookingCode})
	lookupRec := httptest.NewRecorder()
	router.ServeHTTP(lookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(lookupPayload)))
	if lookupRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", lookupRec.Code, lookupRec.Body.String())
	}
	var lookupBody struct {
		GameName   string `json:"gameName"`
		EventTitle string `json:"eventTitle"`
		EventDate  string `json:"eventDate"`
		StartTime  string `json:"startTime"`
	}
	if err := json.NewDecoder(lookupRec.Body).Decode(&lookupBody); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if lookupBody.GameName == "" || lookupBody.EventTitle == "" {
		t.Fatalf("expected non-empty gameName and eventTitle, got %+v", lookupBody)
	}

	cancelPayload, _ := json.Marshal(map[string]string{"email": "mario@example.com", "bookingCode": created.BookingCode})
	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/cancel", created.ID), bytes.NewReader(cancelPayload)))
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	var cancelled struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(cancelRec.Body).Decode(&cancelled); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected status cancelled, got %q", cancelled.Status)
	}
}

func TestLookupBooking_WrongCredentialsReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"email": "nobody@example.com", "bookingCode": "AAAAAAAA"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(payload)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
