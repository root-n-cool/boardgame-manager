package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/httpapi"
)

func createTestEvent(t *testing.T, server *httpapi.Server, gameID int64, copies int) int64 {
	t.Helper()
	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: copies}})
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

	lookupPayload, _ := json.Marshal(map[string]string{"bookingCode": created.BookingCode})
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

	cancelPayload, _ := json.Marshal(map[string]string{"bookingCode": created.BookingCode})
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

func TestLookupBooking_WrongCodeReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"bookingCode": "AAAAAAAA"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(payload)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLookupBooking_IncludesNullMatchResultWhenNoneSubmitted(t *testing.T) {
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
	var created struct {
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	lookupPayload, _ := json.Marshal(map[string]string{"bookingCode": created.BookingCode})
	lookupRec := httptest.NewRecorder()
	router.ServeHTTP(lookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(lookupPayload)))
	var body map[string]any
	if err := json.NewDecoder(lookupRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if v, ok := body["matchResult"]; !ok || v != nil {
		t.Fatalf("expected matchResult to be present and null, got %#v", v)
	}
}

func TestAdminCancelBooking_RemovesItFromTheEventBookings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 2)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking, err := server.Events.CreateBooking(context.Background(), eventID, eventGames[0].ID,
		"Mario Rossi", "mario@example.com", "3331234567", time.Now())
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bookings/%d", booking.ID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d/bookings", eventID), nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected the cancelled booking to be gone, got %+v", list)
	}
}

func TestAdminCancelBooking_UnknownOrAlreadyCancelledReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking, err := server.Events.CreateBooking(context.Background(), eventID, eventGames[0].ID,
		"Mario Rossi", "mario@example.com", "3331234567", time.Now())
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := server.Events.AdminCancelBooking(context.Background(), booking.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	for _, path := range []string{fmt.Sprintf("/api/bookings/%d", booking.ID), "/api/bookings/9999"} {
		req := httptest.NewRequest(http.MethodDelete, path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE %s: expected 404, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestAdminCancelBooking_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking, err := server.Events.CreateBooking(context.Background(), eventID, eventGames[0].ID,
		"Mario Rossi", "mario@example.com", "3331234567", time.Now())
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bookings/%d", booking.ID), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	// La prenotazione deve essere ancora attiva.
	found, err := server.Events.LookupBooking(context.Background(), booking.BookingCode)
	if err != nil || found.Status != events.BookingStatusActive {
		t.Fatalf("expected the booking to stay active, got %+v (err %v)", found, err)
	}
}
