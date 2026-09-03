package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

func createTestEvent(t *testing.T, server *httpapi.Server, gameID int64, copies int) int64 {
	t.Helper()
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2099-01-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: copies}},
	})
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

// TestCreateBooking_SoldOutOnATableReturnsATruthfulMessage guards the wording
// for a full table (seats > 1): "non ci sono più copie disponibili" is false
// there — the copy exists, its bookable seats are just all taken — and the
// message reaches the participant verbatim.
func TestCreateBooking_SoldOutOnATableReturnsATruthfulMessage(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	game, err := server.Games.CreateGame(context.Background(), games.Game{Name: "D&D", Seats: 2})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	eventID := createTestEvent(t, server, game.ID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	if err := server.Events.TestInsertBooking(eventID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert first booking fixture: %v", err)
	}
	if err := server.Events.TestInsertBooking(eventID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert second booking fixture: %v", err)
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
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Contains(body.Error, "copie") {
		t.Fatalf("expected the message not to blame the copy for a table with free seats, got %q", body.Error)
	}
	if !strings.Contains(body.Error, "posti prenotabili") {
		t.Fatalf(`expected the message to use "posti prenotabili", got %q`, body.Error)
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
	// Seats 5 su una seconda copia (copyIndex 2): valori diversi da 1 così
	// un mapper che sbagliasse campo o restasse a un valore fisso fallisce.
	game, err := server.Games.CreateGame(context.Background(), games.Game{Name: "D&D", Seats: 5})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	eventID := createTestEvent(t, server, game.ID, 2)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	tableEventGameID := eventGames[1].ID // copyIndex 2, 5 posti

	marioPayload, _ := json.Marshal(map[string]any{
		"eventGameId": tableEventGameID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	marioRec := httptest.NewRecorder()
	router.ServeHTTP(marioRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(marioPayload)))
	var mario struct {
		ID          int64  `json:"id"`
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(marioRec.Body).Decode(&mario); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	lookupPayload, _ := json.Marshal(map[string]string{"bookingCode": mario.BookingCode})
	lookupRec := httptest.NewRecorder()
	router.ServeHTTP(lookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(lookupPayload)))
	var body map[string]any
	if err := json.NewDecoder(lookupRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if v, ok := body["matchResult"]; !ok || v != nil {
		t.Fatalf("expected matchResult to be present and null, got %#v", v)
	}
	if copyIndex, _ := body["copyIndex"].(float64); copyIndex != 2 {
		t.Fatalf("expected copyIndex 2, got %#v", body["copyIndex"])
	}
	if seats, _ := body["seats"].(float64); seats != 5 {
		t.Fatalf("expected seats 5, got %#v", body["seats"])
	}
	if gameCopies, _ := body["gameCopies"].(float64); gameCopies != 2 {
		t.Fatalf("expected gameCopies 2 (event carries 2 copies of D&D), got %#v", body["gameCopies"])
	}
	if tableBookings, _ := body["tableBookings"].(float64); tableBookings != 1 {
		t.Fatalf("expected tableBookings 1 with a single person seated, got %#v", body["tableBookings"])
	}

	// Un secondo giocatore si siede allo stesso tavolo: tableBookings deve
	// riflettere entrambi, non restare fermo a 1 (una sola persona seduta
	// non distinguerebbe il conteggio da una costante).
	luigiPayload, _ := json.Marshal(map[string]any{
		"eventGameId": tableEventGameID, "participantName": "Luigi Verdi",
		"participantEmail": "luigi@example.com", "participantPhone": "3339876543",
	})
	luigiRec := httptest.NewRecorder()
	router.ServeHTTP(luigiRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(luigiPayload)))
	var luigi struct {
		ID          int64  `json:"id"`
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(luigiRec.Body).Decode(&luigi); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	luigiLookupPayload, _ := json.Marshal(map[string]string{"bookingCode": luigi.BookingCode})
	luigiLookupRec := httptest.NewRecorder()
	router.ServeHTTP(luigiLookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(luigiLookupPayload)))
	var luigiBody map[string]any
	if err := json.NewDecoder(luigiLookupRec.Body).Decode(&luigiBody); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if tableBookings, _ := luigiBody["tableBookings"].(float64); tableBookings != 2 {
		t.Fatalf("expected tableBookings 2 once a second person sits at the table, got %#v", luigiBody["tableBookings"])
	}

	// Mario invia il punteggio; Luigi, che non l'ha inserito ma siede allo
	// stesso tavolo, deve vederlo comunque nel suo lookup — è il punto
	// comportamentale dello switch da GetMatchResultForBooking (per
	// prenotazione) a GetMatchResultForEventGame (per tavolo).
	submitPayload, _ := json.Marshal(map[string]any{
		"bookingCode": mario.BookingCode,
		"players": []map[string]any{
			{"name": "Mario", "score": 42},
			{"name": "Luigi", "score": 30},
		},
	})
	submitRec := httptest.NewRecorder()
	router.ServeHTTP(submitRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", mario.ID), bytes.NewReader(submitPayload)))
	if submitRec.Code != http.StatusOK {
		t.Fatalf("submit match result: expected 200, got %d: %s", submitRec.Code, submitRec.Body.String())
	}

	sharedLookupRec := httptest.NewRecorder()
	router.ServeHTTP(sharedLookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(luigiLookupPayload)))
	var sharedBody map[string]any
	if err := json.NewDecoder(sharedLookupRec.Body).Decode(&sharedBody); err != nil {
		t.Fatalf("decode shared lookup: %v", err)
	}
	sharedResult, ok := sharedBody["matchResult"].(map[string]any)
	if !ok {
		t.Fatalf("expected a non-null matchResult in Luigi's lookup, got %#v", sharedBody["matchResult"])
	}
	players, ok := sharedResult["players"].([]any)
	if !ok || len(players) != 2 {
		t.Fatalf("expected 2 players in the shared match result, got %#v", sharedResult["players"])
	}
	first, _ := players[0].(map[string]any)
	if first["name"] != "Mario" || first["score"] != float64(42) {
		t.Fatalf("unexpected first player in the shared match result: %#v", first)
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
