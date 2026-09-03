package events_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"boardgames-manager/internal/events"
)

func TestCreateBooking_Succeeds(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
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
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
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
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
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
		[]events.EventGameInput{{GameID: gameID, Copies: 5}})
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

func TestCreateBooking_UnknownEventGameReturnsNotFound(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	_, err = eventStore.CreateBooking(ctx, event.ID, 999, "Mario Rossi", "mario@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateBooking_AllowsSamePhoneAfterCancellation(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 5}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, first.ID, first.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("expected the same phone to be able to book again after cancelling, got %v", err)
	}
}

func TestLookupBooking_FindsActiveBookingByCode(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	found, err := eventStore.LookupBooking(ctx, strings.ToLower(created.BookingCode))
	if err != nil {
		t.Fatalf("lookup with lowercase code: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected booking %d, got %d", created.ID, found.ID)
	}
}

func TestLookupBooking_WrongCodeReturnsGenericError(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("create booking: %v", err)
	}

	if _, err := eventStore.LookupBooking(ctx, "WRONGCOD"); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials for wrong code, got %v", err)
	}
}

func TestLookupBooking_CancelledBookingIsNotFound(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, created.ID, created.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.LookupBooking(ctx, created.BookingCode); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected a cancelled booking to be unreachable by lookup, got %v", err)
	}
}

func TestCancelBooking_RejectsWrongCode(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = eventStore.CancelBooking(ctx, created.ID, "WRONGCOD")
	if !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials, got %v", err)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("a rejected cancel must not free up capacity; expected 0, got %d", remaining)
	}
}

func TestListBookingsForEvent_ReturnsOnlyActiveWithGameName(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	active, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("create active booking: %v", err)
	}
	toCancel, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Luigi Verdi", "luigi@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("create booking to cancel: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, toCancel.ID, toCancel.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	list, err := eventStore.ListBookingsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 active booking, got %d", len(list))
	}
	if list[0].ID != active.ID || list[0].GameName != "Catan" {
		t.Fatalf("unexpected booking: %+v", list[0])
	}
}

func TestAdminCancelBooking_CancelsWithoutTheCodeAndFreesTheCopy(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	cancelled, err := eventStore.AdminCancelBooking(ctx, created.ID)
	if err != nil {
		t.Fatalf("admin cancel: %v", err)
	}
	if cancelled.Status != events.BookingStatusCancelled {
		t.Fatalf("expected a cancelled booking, got %q", cancelled.Status)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected the copy to be free again, got %d", remaining)
	}

	// Il partecipante non deve più poter usare il proprio codice.
	if _, err := eventStore.LookupBooking(ctx, created.BookingCode); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected the code to stop working, got %v", err)
	}
}

func TestAdminCancelBooking_DropsTheSubmittedScore(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, created.ID, created.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 82}, {Name: "Lucia", Score: 74}}); err != nil {
		t.Fatalf("submit match result: %v", err)
	}

	if _, err := eventStore.AdminCancelBooking(ctx, created.ID); err != nil {
		t.Fatalf("admin cancel: %v", err)
	}

	result, err := eventStore.GetMatchResultForBooking(ctx, created.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if result != nil {
		t.Fatalf("expected the score of a cancelled booking to be gone, got %+v", result)
	}
}

func TestAdminCancelBooking_AlreadyCancelledOrUnknownIsNotFound(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.AdminCancelBooking(ctx, created.ID); err != nil {
		t.Fatalf("first cancel: %v", err)
	}

	if _, err := eventStore.AdminCancelBooking(ctx, created.ID); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on a second cancel, got %v", err)
	}
	if _, err := eventStore.AdminCancelBooking(ctx, 9999); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown booking, got %v", err)
	}
}
