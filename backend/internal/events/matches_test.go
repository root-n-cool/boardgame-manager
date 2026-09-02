package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/events"
)

func TestSubmitMatchResult_CreatesAndReturnsPlayers(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
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
	if result.BookingID != booking.ID {
		t.Fatalf("expected booking id %d, got %d", booking.ID, result.BookingID)
	}
	if len(result.Players) != 2 || result.Players[0].Name != "Mario" || result.Players[0].Score != 42 {
		t.Fatalf("unexpected players: %+v", result.Players)
	}
}

func TestSubmitMatchResult_ResubmittingReplacesPlayersWithoutDuplicating(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
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

	fetched, err := eventStore.GetMatchResultForBooking(ctx, booking.ID)
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
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
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
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
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
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
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
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
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

	found, err := eventStore.GetMatchResultForBooking(ctx, booking.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if found != nil {
		t.Fatalf("expected the match result to be deleted after cancellation, got %+v", found)
	}
}

func TestGetMatchResultForBooking_ReturnsNilWhenNoneSubmitted(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	found, err := eventStore.GetMatchResultForBooking(ctx, booking.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if found != nil {
		t.Fatalf("expected nil, got %+v", found)
	}
}
