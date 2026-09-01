package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

func newTestStore(t *testing.T) (*events.Store, *games.Store) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return events.NewStore(conn), games.NewStore(conn)
}

func strPtr(v string) *string { return &v }

func mustCreateGame(t *testing.T, store *games.Store, name string) int64 {
	t.Helper()
	g, err := store.CreateGame(context.Background(), games.Game{Name: name})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return g.ID
}

func TestCreateAndGetEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	created, err := eventStore.CreateEvent(ctx, "Serata giochi", strPtr("Una serata di board game"),
		"2026-10-01", "20:00", []events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id")
	}

	found, err := eventStore.GetEvent(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Title != "Serata giochi" || found.EventDate != "2026-10-01" || found.StartTime != "20:00" {
		t.Fatalf("unexpected event: %+v", found)
	}

	eventGames, err := eventStore.ListEventGames(ctx, created.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(eventGames) != 1 || eventGames[0].GameID != gameID || eventGames[0].Quantity != 2 {
		t.Fatalf("unexpected event games: %+v", eventGames)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	eventStore, _ := newTestStore(t)
	_, err := eventStore.GetEvent(context.Background(), 999)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateEvent_RejectsUnknownGame(t *testing.T) {
	eventStore, _ := newTestStore(t)
	_, err := eventStore.CreateEvent(context.Background(), "Serata giochi", nil,
		"2026-10-01", "20:00", []events.EventGameInput{{GameID: 999, Quantity: 1}})
	if !errors.Is(err, events.ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

func TestListEvents_FiltersPastEventsUnlessIncluded(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateEvent(ctx, "Passato", nil, "2026-08-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create past event: %v", err)
	}
	if _, err := eventStore.CreateEvent(ctx, "Futuro", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create future event: %v", err)
	}

	futureOnly, err := eventStore.ListEvents(ctx, false, now)
	if err != nil {
		t.Fatalf("list future only: %v", err)
	}
	if len(futureOnly) != 1 || futureOnly[0].Title != "Futuro" {
		t.Fatalf("expected only the future event, got %+v", futureOnly)
	}

	all, err := eventStore.ListEvents(ctx, true, now)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}
}

func TestRemainingCapacity_DecreasesWithActiveBookingsOnly(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	eventGameID := eventGames[0].ID

	remaining, err := eventStore.RemainingCapacity(ctx, eventGameID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("expected 2, got %d", remaining)
	}

	insertBooking(t, eventStore, event.ID, eventGameID, "active")
	insertBooking(t, eventStore, event.ID, eventGameID, "cancelled")

	remaining, err = eventStore.RemainingCapacity(ctx, eventGameID)
	if err != nil {
		t.Fatalf("remaining after bookings: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 (only the active booking counts), got %d", remaining)
	}
}

func TestGetEventGame_ReturnsRow(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}

	got, err := eventStore.GetEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get event game: %v", err)
	}
	if got.EventID != event.ID || got.GameID != gameID || got.Quantity != 2 {
		t.Fatalf("unexpected event game: %+v", got)
	}
}

func TestGetEventGame_NotFound(t *testing.T) {
	eventStore, _ := newTestStore(t)
	_, err := eventStore.GetEventGame(context.Background(), 999)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateEvent_ChangesFieldsAndReplacesGames(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameA := mustCreateGame(t, gameStore, "Catan")
	gameB := mustCreateGame(t, gameStore, "Azul")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameA, Quantity: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := eventStore.UpdateEvent(ctx, event.ID, "Serata rinnovata", strPtr("Nuova descrizione"),
		"2026-10-02", "21:00", []events.EventGameInput{{GameID: gameB, Quantity: 3}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Serata rinnovata" || updated.EventDate != "2026-10-02" || updated.StartTime != "21:00" {
		t.Fatalf("unexpected updated event: %+v", updated)
	}

	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(eventGames) != 1 || eventGames[0].GameID != gameB || eventGames[0].Quantity != 3 {
		t.Fatalf("expected only game B with quantity 3, got %+v", eventGames)
	}
}

func TestUpdateEvent_RejectsQuantityBelowActiveBookings(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	insertBooking(t, eventStore, event.ID, eventGames[0].ID, "active")
	insertBooking(t, eventStore, event.ID, eventGames[0].ID, "active")

	_, err = eventStore.UpdateEvent(ctx, event.ID, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if !errors.Is(err, events.ErrQuantityBelowActiveBookings) {
		t.Fatalf("expected ErrQuantityBelowActiveBookings, got %v", err)
	}

	_, err = eventStore.UpdateEvent(ctx, event.ID, "Serata giochi", nil, "2026-10-01", "20:00", nil)
	if !errors.Is(err, events.ErrQuantityBelowActiveBookings) {
		t.Fatalf("expected ErrQuantityBelowActiveBookings when removing the game entirely, got %v", err)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("a rejected update must not have touched event_games; expected remaining 0, got %d", remaining)
	}
}

func TestDeleteEvent_CascadesToEventGamesAndBookings(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := eventStore.DeleteEvent(ctx, event.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = eventStore.GetEvent(ctx, event.ID)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteEvent_NotFound(t *testing.T) {
	eventStore, _ := newTestStore(t)
	err := eventStore.DeleteEvent(context.Background(), 999)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// insertBooking writes a booking row directly (bypassing the not-yet-written
// CreateBooking, added in a later task) so RemainingCapacity/UpdateEvent
// guard tests can set up booking state on their own.
func insertBooking(t *testing.T, store *events.Store, eventID, eventGameID int64, status string) {
	t.Helper()
	if err := store.TestInsertBooking(eventID, eventGameID, status); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}
}
