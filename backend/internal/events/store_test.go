package events_test

import (
	"context"
	"errors"
	"fmt"
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

func mustCreateEvent(t *testing.T, store *events.Store, title, date, startTime string, gameIDs ...int64) events.Event {
	t.Helper()
	input := make([]events.EventGameInput, 0, len(gameIDs))
	for _, id := range gameIDs {
		input = append(input, events.EventGameInput{GameID: id, Copies: 1})
	}
	e, err := store.CreateEvent(context.Background(), title, nil, date, startTime, input)
	if err != nil {
		t.Fatalf("create event %q: %v", title, err)
	}
	return e
}

// mustCreateGameWithSeats crea un gioco tavolo, con più di un posto
// prenotabile per copia.
func mustCreateGameWithSeats(t *testing.T, store *games.Store, name string, seats int) int64 {
	t.Helper()
	g, err := store.CreateGame(context.Background(), games.Game{Name: name, Seats: seats})
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
		"2026-10-01", "20:00", []events.EventGameInput{{GameID: gameID, Copies: 2}})
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
	if len(eventGames) != 2 || eventGames[0].GameID != gameID || eventGames[1].CopyIndex != 2 {
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

func TestListEvents_UpcomingOnly_SortedByNearestFirst(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	mustCreateEvent(t, eventStore, "Passato", "2026-08-01", "20:00", gameID)
	mustCreateEvent(t, eventStore, "Lontano", "2026-11-01", "20:00", gameID)
	mustCreateEvent(t, eventStore, "Imminente", "2026-10-01", "20:00", gameID)

	list, total, err := eventStore.ListEvents(ctx, events.ListEventsParams{Now: now})
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 upcoming events, got total %d", total)
	}
	if len(list) != 2 || list[0].Title != "Imminente" || list[1].Title != "Lontano" {
		t.Fatalf("expected the nearest event first, got %+v", list)
	}
}

func TestListEvents_Past_SortedByMostRecentFirst(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	mustCreateEvent(t, eventStore, "Vecchio", "2026-06-01", "20:00", gameID)
	mustCreateEvent(t, eventStore, "Recente", "2026-08-20", "20:00", gameID)
	mustCreateEvent(t, eventStore, "Futuro", "2026-10-01", "20:00", gameID)

	list, total, err := eventStore.ListEvents(ctx, events.ListEventsParams{Past: true, Now: now})
	if err != nil {
		t.Fatalf("list past: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 past events, got total %d", total)
	}
	if len(list) != 2 || list[0].Title != "Recente" || list[1].Title != "Vecchio" {
		t.Fatalf("expected the most recent past event first, got %+v", list)
	}
}

// An event that started earlier today is already past: the cut is on date and
// time together, not on the date alone.
func TestListEvents_SplitsOnDateAndTime(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	now := time.Date(2026, 9, 1, 21, 30, 0, 0, time.UTC)

	mustCreateEvent(t, eventStore, "Iniziato", "2026-09-01", "20:00", gameID)
	mustCreateEvent(t, eventStore, "Stasera", "2026-09-01", "22:00", gameID)

	upcoming, _, err := eventStore.ListEvents(ctx, events.ListEventsParams{Now: now})
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}
	if len(upcoming) != 1 || upcoming[0].Title != "Stasera" {
		t.Fatalf("expected only the event still to start, got %+v", upcoming)
	}

	past, _, err := eventStore.ListEvents(ctx, events.ListEventsParams{Past: true, Now: now})
	if err != nil {
		t.Fatalf("list past: %v", err)
	}
	if len(past) != 1 || past[0].Title != "Iniziato" {
		t.Fatalf("expected the already started event among the past ones, got %+v", past)
	}
}

func TestListEvents_PaginatesPastEventsAndKeepsTheFullTotal(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	for day := 1; day <= 5; day++ {
		mustCreateEvent(t, eventStore, fmt.Sprintf("Serata %d", day),
			fmt.Sprintf("2026-08-%02d", day), "20:00", gameID)
	}

	page1, total, err := eventStore.ListEvents(ctx, events.ListEventsParams{Past: true, Now: now, Limit: 2})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected the total to count every past event, got %d", total)
	}
	if len(page1) != 2 || page1[0].Title != "Serata 5" || page1[1].Title != "Serata 4" {
		t.Fatalf("unexpected first page: %+v", page1)
	}

	page3, _, err := eventStore.ListEvents(ctx, events.ListEventsParams{Past: true, Now: now, Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("list third page: %v", err)
	}
	if len(page3) != 1 || page3[0].Title != "Serata 1" {
		t.Fatalf("unexpected last page: %+v", page3)
	}
}

func TestListEvents_ReportsHowManyGamesEachEventHas(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	catan := mustCreateGame(t, gameStore, "Catan")
	carcassonne := mustCreateGame(t, gameStore, "Carcassonne")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// Carcassonne has 2 copies here: event_games therefore holds 3 rows for
	// this event (1 + 2), but GamesCount must still read 2 — it counts
	// distinct games on the table, not copies. This is the exact regression
	// the copy-rows model introduces: dropping DISTINCT from the query
	// would report 3.
	if _, err := eventStore.CreateEvent(ctx, "Due giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: catan, Copies: 1}, {GameID: carcassonne, Copies: 2}}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := eventStore.CreateEvent(ctx, "Senza giochi", nil, "2026-10-02", "20:00", nil); err != nil {
		t.Fatalf("create empty event: %v", err)
	}

	list, _, err := eventStore.ListEvents(ctx, events.ListEventsParams{Now: now})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].GamesCount != 2 || list[1].GamesCount != 0 {
		t.Fatalf("unexpected games counts: %+v", list)
	}
}

func TestUpdateImagePath_StoresAndReturnsTheNewPath(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event := mustCreateEvent(t, eventStore, "Serata giochi", "2026-10-01", "20:00", gameID)

	if event.ImagePath != nil {
		t.Fatalf("expected a new event to have no image, got %v", *event.ImagePath)
	}

	updated, err := eventStore.UpdateImagePath(ctx, event.ID, "abc123.jpg")
	if err != nil {
		t.Fatalf("update image path: %v", err)
	}
	if updated.ImagePath == nil || *updated.ImagePath != "abc123.jpg" {
		t.Fatalf("unexpected image path: %+v", updated.ImagePath)
	}

	reloaded, err := eventStore.GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reloaded.ImagePath == nil || *reloaded.ImagePath != "abc123.jpg" {
		t.Fatalf("image path did not persist: %+v", reloaded.ImagePath)
	}
}

func TestUpdateImagePath_NotFound(t *testing.T) {
	eventStore, _ := newTestStore(t)
	_, err := eventStore.UpdateImagePath(context.Background(), 999, "abc123.jpg")
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRemainingCapacity_DecreasesWithActiveBookingsOnly(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "Catan", 2)

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
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
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
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
	if got.EventID != event.ID || got.GameID != gameID || got.CopyIndex != 1 {
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
		[]events.EventGameInput{{GameID: gameA, Copies: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := eventStore.UpdateEvent(ctx, event.ID, "Serata rinnovata", strPtr("Nuova descrizione"),
		"2026-10-02", "21:00", []events.EventGameInput{{GameID: gameB, Copies: 3}})
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
	if len(eventGames) != 3 {
		t.Fatalf("expected 3 copies of game B, got %+v", eventGames)
	}
	for _, eg := range eventGames {
		if eg.GameID != gameB {
			t.Fatalf("expected only game B, got %+v", eventGames)
		}
	}
}

func TestUpdateEvent_RejectsQuantityBelowActiveBookings(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	insertBooking(t, eventStore, event.ID, eventGames[0].ID, "active")
	insertBooking(t, eventStore, event.ID, eventGames[1].ID, "active")

	_, err = eventStore.UpdateEvent(ctx, event.ID, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
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
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
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

func TestCreateEvent_MaterialisesOneRowPerCopy(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")

	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 3}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(eventGames) != 3 {
		t.Fatalf("expected 3 copies, got %d", len(eventGames))
	}
	for i, eg := range eventGames {
		if eg.GameID != gameID {
			t.Fatalf("copy %d: unexpected game %d", i, eg.GameID)
		}
		if eg.CopyIndex != i+1 {
			t.Fatalf("expected copy_index %d, got %d", i+1, eg.CopyIndex)
		}
		if eg.Seats != 1 {
			t.Fatalf("expected 1 seat per copy, got %d", eg.Seats)
		}
	}
}

func TestCreateEvent_CopiesSeatsFromCatalogue(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)

	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(eventGames) != 1 || eventGames[0].Seats != 5 {
		t.Fatalf("expected one copy with 5 seats, got %+v", eventGames)
	}

	// I posti sono una fotografia: cambiare il catalogo dopo non muove
	// la capienza di un evento già aperto alle prenotazioni.
	seats := 7
	if _, err := gameStore.UpdateGame(ctx, gameID, games.GameUpdate{Seats: &seats}); err != nil {
		t.Fatalf("update game seats: %v", err)
	}
	eventGames, err = eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list again: %v", err)
	}
	if eventGames[0].Seats != 5 {
		t.Fatalf("expected snapshot of 5 seats, got %d", eventGames[0].Seats)
	}
}

func TestRemainingCapacity_CountsSeatsOnTheCopy(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)

	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 5 {
		t.Fatalf("expected 5 free seats, got %d", remaining)
	}

	if err := eventStore.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	remaining, err = eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining after booking: %v", err)
	}
	if remaining != 4 {
		t.Fatalf("expected 4 free seats, got %d", remaining)
	}
}

// TestActiveBookingCountsByEventGame_GroupsOccupancyForTheWholeEvent covers
// the method the public event page uses to fetch occupancy for every copy in
// one query instead of RemainingCapacity called once per copy.
func TestActiveBookingCountsByEventGame_GroupsOccupancyForTheWholeEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if err := eventStore.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking on copy 1: %v", err)
	}
	if err := eventStore.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert second booking on copy 1: %v", err)
	}
	// A cancelled booking must not count towards occupancy.
	if err := eventStore.TestInsertBooking(event.ID, eventGames[1].ID, "cancelled"); err != nil {
		t.Fatalf("insert cancelled booking on copy 2: %v", err)
	}

	counts, err := eventStore.ActiveBookingCountsByEventGame(ctx, event.ID)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts[eventGames[0].ID] != 2 {
		t.Fatalf("expected 2 active bookings on copy 1, got %d", counts[eventGames[0].ID])
	}
	if counts[eventGames[1].ID] != 0 {
		t.Fatalf("expected 0 active bookings on copy 2, got %d", counts[eventGames[1].ID])
	}
}

func TestCreateEvent_RejectsUnknownGame(t *testing.T) {
	eventStore, _ := newTestStore(t)
	ctx := context.Background()

	_, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: 999, Copies: 1}})
	if !errors.Is(err, events.ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

func TestUpdateEvent_AddsCopiesKeepingExistingIDs(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)

	before, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}

	if _, err := eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 3}}); err != nil {
		t.Fatalf("update: %v", err)
	}

	after, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("expected 3 copies, got %d", len(after))
	}
	// La copia già esistente non si ricrea: cancellarla porterebbe via le
	// sue prenotazioni a cascata.
	if after[0].ID != before[0].ID {
		t.Fatalf("expected copy #1 to keep id %d, got %d", before[0].ID, after[0].ID)
	}
	if after[1].CopyIndex != 2 || after[2].CopyIndex != 3 {
		t.Fatalf("unexpected copy indexes: %+v", after)
	}
}

func TestUpdateEvent_DropsOnlyFreeCopies(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")

	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 3}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Occupata la copia #3, quella più alta: scendendo a 2 copie deve
	// sopravvivere lei e cadere la #2, che è libera.
	if err := eventStore.TestInsertBooking(event.ID, eventGames[2].ID, "active"); err != nil {
		t.Fatalf("insert booking: %v", err)
	}

	if _, err := eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}}); err != nil {
		t.Fatalf("update: %v", err)
	}

	after, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected 2 copies, got %d", len(after))
	}
	if after[0].ID != eventGames[0].ID || after[1].ID != eventGames[2].ID {
		t.Fatalf("expected copies #1 and #3 to survive, got %+v", after)
	}
}

// TestUpdateEvent_CopyNumbersAreStableLabelsNeverRenumbered chains a drop and
// an add: copy numbers are stable labels, not positions — dropping a middle
// copy leaves a permanent gap, and a later addition must take the next number
// after the highest surviving one, never reuse the gap. Existing tests create
// a gap (TestUpdateEvent_DropsOnlyFreeCopies) or add copies
// (TestUpdateEvent_AddsCopiesKeepingExistingIDs) but never chain the two, so
// nothing pins that the "next" index is computed from the highest copy_index
// rather than from len(copies) (which a gap would make wrong).
func TestUpdateEvent_CopyNumbersAreStableLabelsNeverRenumbered(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")

	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 3}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Occupa la #3, la più alta: scendendo a 2 copie sopravvive lei e cade
	// la #2, libera, lasciando un buco.
	if err := eventStore.TestInsertBooking(event.ID, eventGames[2].ID, "active"); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	if _, err := eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}}); err != nil {
		t.Fatalf("drop middle copy: %v", err)
	}
	afterDrop, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list after drop: %v", err)
	}
	if len(afterDrop) != 2 || afterDrop[0].CopyIndex != 1 || afterDrop[1].CopyIndex != 3 {
		t.Fatalf("expected copies #1 and #3 with a gap at #2, got %+v", afterDrop)
	}

	// Ora si aggiunge una copia: deve prendere il numero dopo la più alta
	// esistente (4), non riempire il buco lasciato dalla #2.
	if _, err := eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 3}}); err != nil {
		t.Fatalf("add copy after the gap: %v", err)
	}
	afterAdd, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list after add: %v", err)
	}
	if len(afterAdd) != 3 {
		t.Fatalf("expected 3 copies, got %d", len(afterAdd))
	}
	indexes := []int{afterAdd[0].CopyIndex, afterAdd[1].CopyIndex, afterAdd[2].CopyIndex}
	if indexes[0] != 1 || indexes[1] != 3 || indexes[2] != 4 {
		t.Fatalf("expected copy numbers 1, 3, 4 (gap at 2 preserved), got %v", indexes)
	}
}

func TestUpdateEvent_RejectsFewerCopiesThanOccupied(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")

	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, eg := range eventGames {
		if err := eventStore.TestInsertBooking(event.ID, eg.ID, "active"); err != nil {
			t.Fatalf("insert booking: %v", err)
		}
	}

	_, err = eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 1}})
	if !errors.Is(err, events.ErrQuantityBelowActiveBookings) {
		t.Fatalf("expected ErrQuantityBelowActiveBookings, got %v", err)
	}

	// Niente è stato scritto: entrambe le copie sono ancora lì.
	after, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected the update to be rolled back, got %d copies", len(after))
	}
}

func TestUpdateEvent_RemovesGameWithoutBookings(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	catan := mustCreateGame(t, gameStore, "Catan")
	carcassonne := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", catan, carcassonne)

	if _, err := eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: catan, Copies: 1}}); err != nil {
		t.Fatalf("update: %v", err)
	}

	after, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(after) != 1 || after[0].GameID != catan {
		t.Fatalf("expected only Catan to remain, got %+v", after)
	}
}

func TestUpdateEvent_NewCopiesTakeCurrentCatalogueSeats(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)

	seats := 7
	if _, err := gameStore.UpdateGame(ctx, gameID, games.GameUpdate{Seats: &seats}); err != nil {
		t.Fatalf("update game: %v", err)
	}
	if _, err := eventStore.UpdateEvent(ctx, event.ID, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	after, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// La copia vecchia conserva la sua fotografia, la nuova nasce con i
	// posti prenotabili di adesso.
	if after[0].Seats != 5 || after[1].Seats != 7 {
		t.Fatalf("expected seats 5 then 7, got %+v", after)
	}
}
