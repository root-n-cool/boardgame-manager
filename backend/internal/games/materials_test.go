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

// idsByName è il ponte fra due salvataggi: quel che i test seguenti
// verificano è che la stessa voce, salvata due volte, conservi il suo id —
// è l'id che la modale di riconsegna ha in mano quando il gioco torna.
func idsByName(t *testing.T, store *games.Store, gameID int64) map[string]int64 {
	t.Helper()
	rows, err := store.ListMaterials(context.Background(), gameID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := make(map[string]int64, len(rows))
	for _, m := range rows {
		out[m.Name] = m.ID
	}
	return out
}

func TestReplaceMaterialsKeepsIDsOnAnIdenticalSave(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()
	list := []games.MaterialInput{{Name: "tessere", Quantity: 72}, {Name: "meeple", Quantity: 40}}

	if _, err := store.ReplaceMaterials(ctx, gameID, list); err != nil {
		t.Fatalf("prima replace: %v", err)
	}
	before := idsByName(t, store, gameID)
	// Salvare senza aver cambiato niente è il gesto più comune del pannello,
	// ed è quello che prima invalidava le spunte di una modale già aperta.
	if _, err := store.ReplaceMaterials(ctx, gameID, list); err != nil {
		t.Fatalf("seconda replace: %v", err)
	}
	after := idsByName(t, store, gameID)
	for name, id := range before {
		if after[name] != id {
			t.Errorf("%q ha cambiato id: %d -> %d", name, id, after[name])
		}
	}
}

func TestReplaceMaterialsKeepsTheOtherIDsWhenOneIsRenamed(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
		{Name: "dadi", Quantity: 5},
	}); err != nil {
		t.Fatalf("prima replace: %v", err)
	}
	before := idsByName(t, store, gameID)

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple gialli", Quantity: 40},
		{Name: "dadi", Quantity: 5},
	}); err != nil {
		t.Fatalf("seconda replace: %v", err)
	}
	after := idsByName(t, store, gameID)

	if after["tessere"] != before["tessere"] || after["dadi"] != before["dadi"] {
		t.Errorf("una rinomina ha travolto le altre voci: prima %v, dopo %v", before, after)
	}
	// La voce rinominata è un'altra riga: accettato, e documentato su
	// ReplaceMaterials — al più una spunta si perde e la voce risulta non
	// verificata.
	if _, ok := after["meeple"]; ok {
		t.Errorf("il vecchio nome è ancora in tabella: %v", after)
	}
	if after["meeple gialli"] == 0 {
		t.Errorf("il nuovo nome non è stato inserito: %v", after)
	}
}

func TestReplaceMaterialsKeepsIDsWhenReordering(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
	}); err != nil {
		t.Fatalf("prima replace: %v", err)
	}
	before := idsByName(t, store, gameID)

	out, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "meeple", Quantity: 40},
		{Name: "tessere", Quantity: 72},
	})
	if err != nil {
		t.Fatalf("seconda replace: %v", err)
	}
	if len(out) != 2 || out[0].Name != "meeple" || out[0].Position != 0 || out[1].Name != "tessere" || out[1].Position != 1 {
		t.Fatalf("il riordino non è stato applicato: %+v", out)
	}
	if out[0].ID != before["meeple"] || out[1].ID != before["tessere"] {
		t.Errorf("il riordino ha rigenerato gli id: prima %v, dopo %+v", before, out)
	}
}

func TestReplaceMaterialsDeletesOnlyTheRemovedRow(t *testing.T) {
	store := newTestStore(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
		{Name: "dadi", Quantity: 5},
	}); err != nil {
		t.Fatalf("prima replace: %v", err)
	}
	before := idsByName(t, store, gameID)

	if _, err := store.ReplaceMaterials(ctx, gameID, []games.MaterialInput{
		{Name: "tessere", Quantity: 72},
		{Name: "dadi", Quantity: 5},
	}); err != nil {
		t.Fatalf("seconda replace: %v", err)
	}
	after := idsByName(t, store, gameID)

	if len(after) != 2 {
		t.Fatalf("volevo 2 voci, ho %v", after)
	}
	if after["tessere"] != before["tessere"] || after["dadi"] != before["dadi"] {
		t.Errorf("le voci rimaste hanno cambiato id: prima %v, dopo %v", before, after)
	}
}

func TestMarkMaterialsCheckedStampsTimeAndUser(t *testing.T) {
	store, conn := newTestStoreWithDB(t)
	gameID := mustGameID(t, store, "Carcassonne")
	ctx := context.Background()

	// L'utente deve esistere davvero: la colonna ha una FK, e le FK sono
	// attive (PRAGMA foreign_keys(1) in db.Open).
	res, err := conn.ExecContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES ('admin@example.com', 'x')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}

	before, err := store.GetGame(ctx, gameID)
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if before.MaterialsCheckedAt != nil || before.MaterialsCheckedBy != nil {
		t.Fatalf("un gioco nuovo non è mai stato controllato: %+v", before)
	}

	got, err := store.MarkMaterialsChecked(ctx, gameID, userID)
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if got.MaterialsCheckedAt == nil || got.MaterialsCheckedAt.IsZero() {
		t.Error("volevo una data di controllo")
	}
	if got.MaterialsCheckedBy == nil || *got.MaterialsCheckedBy != userID {
		t.Errorf("materialsCheckedBy = %v, volevo %d", got.MaterialsCheckedBy, userID)
	}

	// E deve essere durevole, non solo nel valore di ritorno.
	read, err := store.GetGame(ctx, gameID)
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if read.MaterialsCheckedAt == nil {
		t.Error("la data non è stata salvata")
	}
}

func TestMarkMaterialsCheckedOnAMissingGameIsNotFound(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.MarkMaterialsChecked(context.Background(), 9999, 1); !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("volevo ErrNotFound, ho %v", err)
	}
}
