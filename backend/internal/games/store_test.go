package games_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
)

func newTestStore(t *testing.T) *games.Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return games.NewStore(conn)
}

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }

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
