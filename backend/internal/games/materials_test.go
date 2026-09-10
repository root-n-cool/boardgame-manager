package games_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"boardgames-manager/internal/games"
)

func mustGameID(t *testing.T, store *games.Store, name string) int64 {
	t.Helper()
	g, err := store.CreateGame(context.Background(), games.Game{Name: name})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return g.ID
}

func TestReplaceMaterialsStoresRowsInOrder(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")

	out, err := store.ReplaceMaterials(context.Background(), gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("volevo 2 voci, ne ho %d", len(out))
	}
	if out[0].Name != "tessere" || out[0].Quantity != 72 || out[0].Position != 0 {
		t.Errorf("prima voce inattesa: %+v", out[0])
	}
	if out[1].Name != "meeple" || out[1].Position != 1 {
		t.Errorf("seconda voce inattesa: %+v", out[1])
	}

	read, err := store.ListMaterials(context.Background(), gameID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(read) != 2 || read[0].Name != "tessere" || read[1].Name != "meeple" {
		t.Fatalf("la rilettura non rispetta l'ordine: %+v", read)
	}
}

func TestReplaceMaterialsIsATotalSubstitution(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
	}); err != nil {
		t.Fatalf("prima replace: %v", err)
	}
	// Riordino e rimozione insieme: è il caso normale dell'editor.
	out, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "meeple", Quantity: 40},
	})
	if err != nil {
		t.Fatalf("seconda replace: %v", err)
	}
	if len(out) != 1 || out[0].Name != "meeple" || out[0].Position != 0 {
		t.Fatalf("la sostituzione non ha ripulito: %+v", out)
	}
}

func TestReplaceMaterialsWithEmptyListClearsTheGame(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	out, err := store.ReplaceMaterials(ctx, gameID, nil)
	if err != nil {
		t.Fatalf("replace vuota: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("volevo una lista vuota, ho %+v", out)
	}
}

func TestReplaceMaterialsRejectsBadRows(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	cases := map[string]struct {
		in   []games.MaterialInput
		want error
	}{
		"nome vuoto":       {[]games.MaterialInput{{Name: "   ", Quantity: 4}}, games.ErrMaterialInvalid},
		"nome lunghissimo": {[]games.MaterialInput{{Name: strings.Repeat("a", games.MaxMaterialNameChars+1), Quantity: 4}}, games.ErrMaterialInvalid},
		"quantità zero":    {[]games.MaterialInput{{Name: "dadi", Quantity: 0}}, games.ErrMaterialInvalid},
		"quantità enorme":  {[]games.MaterialInput{{Name: "dadi", Quantity: games.MaxMaterialQuantity + 1}}, games.ErrMaterialInvalid},
		"duplicato": {[]games.MaterialInput{
			{Name: "Meeple", Quantity: 40}, {Name: " meeple ", Quantity: 8},
		}, games.ErrDuplicateMaterial},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := store.ReplaceMaterials(ctx, gameID, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("volevo %v, ho %v", tc.want, err)
			}
		})
	}

	tooMany := make([]games.MaterialInput, games.MaxMaterialsPerGame+1)
	for i := range tooMany {
		tooMany[i] = games.MaterialInput{Name: strings.Repeat("x", i+1), Quantity: 1}
	}
	if _, err := store.ReplaceMaterials(ctx, gameID, tooMany); !errors.Is(err, games.ErrTooManyMaterials) {
		t.Fatalf("volevo ErrTooManyMaterials, ho %v", err)
	}
}

func TestReplaceMaterialsLeavesTheOldListOnError(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "dadi", Quantity: 0},
	}); err == nil {
		t.Fatal("volevo un errore")
	}
	read, err := store.ListMaterials(ctx, gameID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(read) != 1 || read[0].Name != "tessere" {
		t.Fatalf("la lista vecchia è stata toccata: %+v", read)
	}
}

func TestDeletingAGameDeletesItsMaterials(t *testing.T) {
	store, conn := newTestStoreWithDB(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := store.DeleteGame(ctx, gameID); err != nil {
		t.Fatalf("delete game: %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_material`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("la cascata non ha portato via le voci: %d rimaste", count)
	}
}
