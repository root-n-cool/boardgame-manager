package events_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"boardgames-manager/internal/events"
)

func TestSubmitMatchResult_CreatesAndReturnsPlayers(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	result, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 42}, {Name: "Luigi", Score: 30}})
	if err != nil {
		t.Fatalf("submit match result: %v", err)
	}
	if result.EventGameID != eventGames[0].ID {
		t.Fatalf("expected event game id %d, got %d", eventGames[0].ID, result.EventGameID)
	}
	if len(result.Players) != 2 || result.Players[0].Name != "Mario" || result.Players[0].Score != 42 {
		t.Fatalf("unexpected players: %+v", result.Players)
	}
}

func TestSubmitMatchResult_ResubmittingReplacesPlayersWithoutDuplicating(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	if _, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}}); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	result, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 50}, {Name: "Luigi", Score: 20}})
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if len(result.Players) != 2 {
		t.Fatalf("expected the resubmission to replace players, not accumulate them; got %+v", result.Players)
	}

	fetched, err := eventStore.GetMatchResultForEventGame(ctx, booking.EventGameID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if fetched == nil || len(fetched.Players) != 2 || fetched.Players[0].Score != 50 {
		t.Fatalf("unexpected stored match result: %+v", fetched)
	}
}

func TestSubmitMatchResult_RejectsWrongCode(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = eventStore.SubmitMatchResult(ctx, booking.ID, "WRONGCOD", []events.PlayerScore{{Name: "Mario", Score: 10}})
	if !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials, got %v", err)
	}
}

func TestSubmitMatchResult_RejectsCancelledBooking(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, booking.ID, booking.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	_, err = eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode, []events.PlayerScore{{Name: "Mario", Score: 10}})
	if !errors.Is(err, events.ErrBookingNotActive) {
		t.Fatalf("expected ErrBookingNotActive, got %v", err)
	}
}

func TestSubmitMatchResult_RejectsEmptyPlayers(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode, nil)
	if !errors.Is(err, events.ErrEmptyPlayers) {
		t.Fatalf("expected ErrEmptyPlayers, got %v", err)
	}
}

func TestCancelBooking_DeletesExistingMatchResult(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	if _, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 42}, {Name: "Luigi", Score: 30}}); err != nil {
		t.Fatalf("submit match result: %v", err)
	}

	if _, err := eventStore.CancelBooking(ctx, booking.ID, booking.BookingCode); err != nil {
		t.Fatalf("cancel booking: %v", err)
	}

	found, err := eventStore.GetMatchResultForEventGame(ctx, booking.EventGameID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if found != nil {
		t.Fatalf("expected the match result to be deleted after cancellation, got %+v", found)
	}
}

func TestGetMatchResultForEventGame_ReturnsNilWhenNoneSubmitted(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	found, err := eventStore.GetMatchResultForEventGame(ctx, booking.EventGameID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if found != nil {
		t.Fatalf("expected nil, got %+v", found)
	}
}

func TestSubmitMatchResult_IsSharedByTheWholeTable(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 3)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	second, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Luigi", "luigi@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("second booking: %v", err)
	}

	if _, err := eventStore.SubmitMatchResult(ctx, first.ID, first.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}}); err != nil {
		t.Fatalf("submit by first: %v", err)
	}

	// Il compagno di tavolo vede lo stesso risultato...
	shared, err := eventStore.GetMatchResultForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get shared: %v", err)
	}
	if shared == nil || len(shared.Players) != 1 || shared.Players[0].Name != "Mario" {
		t.Fatalf("unexpected shared result: %+v", shared)
	}

	// ...e può correggerlo: uno solo per tavolo, non uno a testa.
	if _, err := eventStore.SubmitMatchResult(ctx, second.ID, second.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}, {Name: "Luigi", Score: 12}}); err != nil {
		t.Fatalf("submit by second: %v", err)
	}
	corrected, err := eventStore.GetMatchResultForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get corrected: %v", err)
	}
	if corrected == nil || len(corrected.Players) != 2 {
		t.Fatalf("expected the table result to be replaced, got %+v", corrected)
	}

	results, err := eventStore.ListMatchResultsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result for the table, got %d", len(results))
	}
	if results[0].CopyIndex != 1 || results[0].GameName != "D&D" || len(results[0].Players) != 2 {
		t.Fatalf("unexpected admin row: %+v", results[0])
	}
}

func TestListMatchResultsForEvent_OneRowPerCopy(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	for i, eg := range eventGames {
		b, err := eventStore.CreateBooking(ctx, event.ID, eg.ID,
			fmt.Sprintf("G%d", i), fmt.Sprintf("g%d@example.com", i),
			fmt.Sprintf("33300000%02d", i), now)
		if err != nil {
			t.Fatalf("booking on copy %d: %v", i+1, err)
		}
		if _, err := eventStore.SubmitMatchResult(ctx, b.ID, b.BookingCode,
			[]events.PlayerScore{{Name: "Vincitore", Score: 40 + i}}); err != nil {
			t.Fatalf("submit on copy %d: %v", i+1, err)
		}
	}

	results, err := eventStore.ListMatchResultsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].CopyIndex != 1 || results[1].CopyIndex != 2 {
		t.Fatalf("unexpected copy indexes: %+v", results)
	}
}
