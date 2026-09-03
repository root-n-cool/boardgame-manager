package games_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
)

func newTestStore(t *testing.T) *games.Store {
	t.Helper()
	store, _ := newTestStoreWithDB(t)
	return store
}

// newTestStoreWithDB also hands back the raw connection, for tests that need
// to set up state (e.g. events/event_games rows) directly via SQL without
// importing the events package.
func newTestStoreWithDB(t *testing.T) (*games.Store, *sql.DB) {
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
	return games.NewStore(conn), conn
}

func strPtr(v string) *string     { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestCreateAndGetGame(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{
		Name: "Catan", Year: intPtr(1995), MinPlayers: intPtr(3), MaxPlayers: intPtr(4),
		PlaytimeMinutes: intPtr(90), Owner: strPtr("Mario"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id")
	}

	found, err := store.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Name != "Catan" || *found.Year != 1995 || *found.Owner != "Mario" {
		t.Fatalf("unexpected game: %+v", found)
	}
}

func TestGetGame_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetGame(context.Background(), 999)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListGames_ReturnsAllCreated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.CreateGame(ctx, games.Game{Name: "Azul"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.CreateGame(ctx, games.Game{Name: "Wingspan"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := store.ListGames(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 games, got %d", len(list))
	}
}

func TestUpdateGame_ChangesOnlyProvidedFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{Name: "Azul", Owner: strPtr("Mario"), Year: intPtr(2017)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := store.UpdateGame(ctx, created.ID, games.GameUpdate{Owner: strPtr("Luigi")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if *updated.Owner != "Luigi" {
		t.Fatalf("expected owner Luigi, got %v", updated.Owner)
	}
	if *updated.Year != 2017 {
		t.Fatalf("expected year to stay 2017, got %v", updated.Year)
	}
}

func TestDeleteGame_RemovesIt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{Name: "Azul"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.DeleteGame(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = store.GetGame(ctx, created.ID)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteGame_UsedByEventReturnsErrGameInUse(t *testing.T) {
	store, conn := newTestStoreWithDB(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{Name: "Azul"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := conn.ExecContext(ctx,
		`INSERT INTO events (title, event_date, start_time) VALUES ('t', '2099-01-01', '20:00')`)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	eventID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("event id: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO event_games (event_id, game_id, copy_index, seats) VALUES (?, ?, 1, 1)`, eventID, created.ID); err != nil {
		t.Fatalf("insert event_games: %v", err)
	}

	err = store.DeleteGame(ctx, created.ID)
	if !errors.Is(err, games.ErrGameInUse) {
		t.Fatalf("expected ErrGameInUse, got %v", err)
	}
}

func TestDeleteGame_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.DeleteGame(context.Background(), 999)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateLanguageAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	lang, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true,
		Name: "Catan", Description: strPtr("Un gioco di insediamento."),
	})
	if err != nil {
		t.Fatalf("create language: %v", err)
	}
	if !lang.IsBaseLanguage {
		t.Fatal("expected is_base_language to be true")
	}

	list, err := store.ListLanguages(ctx, game.ID)
	if err != nil {
		t.Fatalf("list languages: %v", err)
	}
	if len(list) != 1 || list[0].LanguageCode != "it" {
		t.Fatalf("unexpected languages: %+v", list)
	}
}

func TestGetLanguage_NotFound(t *testing.T) {
	store := newTestStore(t)
	game, err := store.CreateGame(context.Background(), games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	_, err = store.GetLanguage(context.Background(), game.ID, "en")
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateLanguage_DuplicateReturnsErrDuplicateLanguage(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	if _, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "en", IsBaseLanguage: true, Name: "Catan",
	}); err != nil {
		t.Fatalf("create language: %v", err)
	}

	_, err = store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "en", IsBaseLanguage: false, Name: "Catan",
	})
	if !errors.Is(err, games.ErrDuplicateLanguage) {
		t.Fatalf("expected ErrDuplicateLanguage, got %v", err)
	}
}

func TestUpdateLanguage_ChangesNameAndDescription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	if _, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true, Name: "Catan",
	}); err != nil {
		t.Fatalf("create language: %v", err)
	}

	updated, err := store.UpdateLanguage(ctx, game.ID, "it", "I Coloni di Catan", strPtr("Descrizione aggiornata."))
	if err != nil {
		t.Fatalf("update language: %v", err)
	}
	if updated.Name != "I Coloni di Catan" || *updated.Description != "Descrizione aggiornata." {
		t.Fatalf("unexpected updated language: %+v", updated)
	}
}

func TestCreateGame_DefaultsSeatsToOne(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if game.Seats != 1 {
		t.Fatalf("expected seats 1, got %d", game.Seats)
	}
}

func TestCreateGame_KeepsExplicitSeats(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "D&D", Seats: 5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if game.Seats != 5 {
		t.Fatalf("expected seats 5, got %d", game.Seats)
	}

	found, err := store.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Seats != 5 {
		t.Fatalf("expected persisted seats 5, got %d", found.Seats)
	}
}

func TestUpdateGame_ChangesSeats(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "D&D"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	seats := 7
	updated, err := store.UpdateGame(ctx, game.ID, games.GameUpdate{Seats: &seats})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Seats != 7 {
		t.Fatalf("expected seats 7, got %d", updated.Seats)
	}

	// Un update che non menziona i posti non li tocca.
	owner := "Danilo"
	untouched, err := store.UpdateGame(ctx, game.ID, games.GameUpdate{Owner: &owner})
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if untouched.Seats != 7 {
		t.Fatalf("expected seats to stay 7, got %d", untouched.Seats)
	}
}

func TestGameWeight_RoundTripsAndUpdates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{Name: "Catan", Weight: floatPtr(2.2809)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Weight == nil || *created.Weight != 2.2809 {
		t.Fatalf("unexpected weight after create: %v", created.Weight)
	}

	updated, err := store.UpdateGame(ctx, created.ID, games.GameUpdate{Weight: floatPtr(3.5)})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Weight == nil || *updated.Weight != 3.5 {
		t.Fatalf("unexpected weight after update: %v", updated.Weight)
	}
}

func TestGameWeight_DefaultsToUnknown(t *testing.T) {
	store := newTestStore(t)
	created, err := store.CreateGame(context.Background(), games.Game{Name: "Azul"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Weight != nil {
		t.Fatalf("expected no weight, got %v", *created.Weight)
	}
}

func TestCreateGame_PersistsBGGDescription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	raw := "A worker placement game about birds."
	created, err := store.CreateGame(ctx, games.Game{Name: "Wingspan", BGGDescription: &raw})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.BGGDescription == nil || *created.BGGDescription != raw {
		t.Fatalf("expected the raw BGG description to survive the round trip, got %v", created.BGGDescription)
	}

	fetched, err := store.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.BGGDescription == nil || *fetched.BGGDescription != raw {
		t.Fatalf("expected GetGame to return the raw description, got %v", fetched.BGGDescription)
	}
}

func TestCreateGame_WithoutBGGDescription(t *testing.T) {
	store := newTestStore(t)
	created, err := store.CreateGame(context.Background(), games.Game{Name: "Gioco a mano"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.BGGDescription != nil {
		t.Fatalf("expected no raw description for a manual game, got %q", *created.BGGDescription)
	}
}
