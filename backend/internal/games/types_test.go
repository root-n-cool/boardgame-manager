package games_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/games"
)

func TestGameTypes_SeededByMigration(t *testing.T) {
	store := newTestStore(t)
	list, err := store.ListGameTypes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Slug != "GDT" || !list[0].BGGSearch || list[1].Slug != "GDR" || list[1].BGGSearch {
		t.Fatalf("seed = %+v", list)
	}
}

func TestCreateGameType_NormalizesAndAppends(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	got, err := store.CreateGameType(ctx, games.GameTypeInput{Name: " Magic: The Gathering ", Slug: " mtg ", BGGSearch: false})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Magic: The Gathering" || got.Slug != "MTG" || got.Position != 3 || got.BGGSearch {
		t.Fatalf("created = %+v", got)
	}
}

func TestCreateGameType_Validation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cases := []struct {
		in   games.GameTypeInput
		want error
	}{
		{games.GameTypeInput{Name: "", Slug: "AB"}, games.ErrGameTypeName},
		{games.GameTypeInput{Name: "X", Slug: "A"}, games.ErrGameTypeSlug},
		{games.GameTypeInput{Name: "X", Slug: "ABCDEFG"}, games.ErrGameTypeSlug},
		{games.GameTypeInput{Name: "X", Slug: "A-B"}, games.ErrGameTypeSlug},
		{games.GameTypeInput{Name: "X", Slug: "gdt"}, games.ErrSlugTaken},
	}
	for _, c := range cases {
		if _, err := store.CreateGameType(ctx, c.in); !errors.Is(err, c.want) {
			t.Errorf("%+v: err = %v, want %v", c.in, err, c.want)
		}
	}
}

func TestUpdateGameType(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	name, slug, bgg := "Giochi di ruolo", "rpg", true
	got, err := store.UpdateGameType(ctx, 2, games.GameTypeUpdate{Name: &name, Slug: &slug, BGGSearch: &bgg})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.Slug != "RPG" || !got.BGGSearch {
		t.Fatalf("updated = %+v", got)
	}
	// Tenere la propria sigla non è un conflitto.
	same := "RPG"
	if _, err := store.UpdateGameType(ctx, 2, games.GameTypeUpdate{Slug: &same}); err != nil {
		t.Fatalf("own slug: %v", err)
	}
	taken := "GDT"
	if _, err := store.UpdateGameType(ctx, 2, games.GameTypeUpdate{Slug: &taken}); !errors.Is(err, games.ErrSlugTaken) {
		t.Fatalf("taken slug: %v", err)
	}
	if _, err := store.UpdateGameType(ctx, 99, games.GameTypeUpdate{Name: &name}); !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestMoveGameType(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.MoveGameType(ctx, 2, true); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListGameTypes(ctx)
	if list[0].Slug != "GDR" || list[1].Slug != "GDT" {
		t.Fatalf("after move up = %+v", list)
	}
	// Agli estremi non succede niente e non è un errore.
	if err := store.MoveGameType(ctx, 2, true); err != nil {
		t.Fatalf("up at top: %v", err)
	}
	if err := store.MoveGameType(ctx, 1, false); err != nil {
		t.Fatalf("down at bottom: %v", err)
	}
	list, _ = store.ListGameTypes(ctx)
	if list[0].Slug != "GDR" {
		t.Fatalf("edges changed order: %+v", list)
	}
	if err := store.MoveGameType(ctx, 99, true); !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestDeleteGameType(t *testing.T) {
	store, conn := newTestStoreWithDB(t)
	ctx := context.Background()
	// SQL diretto: Game.GameTypeID arriva col Task 2.
	if _, err := conn.ExecContext(ctx, `INSERT INTO games (name, game_type_id) VALUES ('Azul', 1)`); err != nil {
		t.Fatal(err)
	}
	var inUse *games.GameTypeInUseError
	if err := store.DeleteGameType(ctx, 1); !errors.As(err, &inUse) || inUse.Count != 1 {
		t.Fatalf("in use: %v", err)
	}
	if err := store.DeleteGameType(ctx, 2); err != nil {
		t.Fatalf("empty type: %v", err)
	}
	if err := store.DeleteGameType(ctx, 2); !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("already gone: %v", err)
	}
}

func TestDefaultGameTypeID_FollowsOrder(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if id, _ := store.DefaultGameTypeID(ctx); id != 1 {
		t.Fatalf("default = %d", id)
	}
	_ = store.MoveGameType(ctx, 2, true)
	if id, _ := store.DefaultGameTypeID(ctx); id != 2 {
		t.Fatalf("default after move = %d", id)
	}
}

func TestDeleteGameType_RefusesLastOne(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.DeleteGameType(ctx, 2); err != nil {
		t.Fatal(err)
	}
	// Senza tipologie nessun gioco si potrebbe più creare.
	if err := store.DeleteGameType(ctx, 1); !errors.Is(err, games.ErrLastGameType) {
		t.Fatalf("last type: %v", err)
	}
}
