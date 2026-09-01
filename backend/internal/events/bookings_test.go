package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/events"
)

func TestCreateBooking_Succeeds(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if booking.ID == 0 {
		t.Fatal("expected a non-zero id")
	}
	if len(booking.BookingCode) != 8 {
		t.Fatalf("expected an 8-char booking code, got %q", booking.BookingCode)
	}
	if booking.Status != events.BookingStatusActive {
		t.Fatalf("expected status active, got %q", booking.Status)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected remaining capacity 1, got %d", remaining)
	}
}

func TestCreateBooking_RejectsWhenEventAlreadyStarted(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-09-01", "10:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) // after the 10:00 start

	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrEventAlreadyStarted) {
		t.Fatalf("expected ErrEventAlreadyStarted, got %v", err)
	}
}

func TestCreateBooking_RejectsWhenSoldOut(t *testing.T) {
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

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Luigi Verdi", "luigi@example.com", "3332222222", now)
	if !errors.Is(err, events.ErrGameSoldOut) {
		t.Fatalf("expected ErrGameSoldOut, got %v", err)
	}
}

func TestCreateBooking_RejectsDuplicatePhoneForSameEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 5}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario2@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrDuplicatePhoneBooking) {
		t.Fatalf("expected ErrDuplicatePhoneBooking, got %v", err)
	}
}

// TestCreateBooking_AllowsSamePhoneAfterCancellation is intentionally not
// present in this task's test file: it exercises eventStore.CancelBooking,
// which doesn't exist until Task 4. It will be reinstated there, alongside
// the CancelBooking implementation.

func TestCreateBooking_UnknownEventGameReturnsNotFound(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	_, err = eventStore.CreateBooking(ctx, event.ID, 999, "Mario Rossi", "mario@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
