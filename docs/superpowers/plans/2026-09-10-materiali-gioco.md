# Materiali del gioco e checklist di riconsegna — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** dare a ogni gioco una lista di materiali (`nome` + `quantità`) e trasformarla, alla riconsegna di un prestito, in una checklist che registra ciò che non è tornato.

**Architecture:** due tabelle nuove (`game_material` nel catalogo, `loan_material_issue` sul prestito), lo store dei materiali dentro `internal/games`, la scrittura dell'esito dentro la transazione di `events.ReturnLoan`, tre rotte admin nuove più un campo opzionale su `POST /api/loans/{id}/return`, e due superfici frontend: un pannello editor nella scheda gioco admin e la checklist nella modale del banco prestiti. La proposta AI dal manuale riusa la ricerca FTS su `game_source_chunk` già in casa.

**Tech Stack:** Go 1.25 (chi, `modernc.org/sqlite`), Vue 3 `<script setup>` + TypeScript, Vite. Nessuna dipendenza nuova, né Go né npm.

**Spec:** `docs/superpowers/specs/2026-09-10-materiali-gioco-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto. Ogni `go test`/`go build` va lanciato così, riusando i due volumi nominati:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  `npm` invece gira in locale, dentro `frontend/`.
- **Migrazioni forward-only.** Nessuna modifica a un file `.sql` già rilasciato; il nuovo file è `0017_game_materials.sql` e viene applicato all'avvio dal runner custom.
- **Nessuna dipendenza nuova** (Go o npm) senza chiedere: il progetto preferisce stdlib e codice esplicito.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n.
- **Design system**: token in `frontend/src/app.css`, regole in `DESIGN.md`. In particolare: i dati numerici vanno in mono (`--font-mono`), nessuna emoji al posto di un'icona, un controllo `disabled` non resta mai senza il motivo a schermo legato con `aria-describedby`.
- **Le pagine del banco prestiti sono mobile-first**: si usano in piedi, al tavolo. Area di tocco ≥ 44px, nessuno scroll orizzontale a 390px.
- **Commit in inglese**, conventional commits, e ogni commit chiude un task.
- **Tetti condivisi** (definiti una volta in `internal/games`, riusati da `internal/ai` e dagli handler): nome ≤ 60 caratteri, quantità 1–9999, massimo 60 voci per gioco.

---

### Task 1: Migrazione e store dei materiali

**Files:**
- Create: `backend/internal/db/migrations/0017_game_materials.sql`
- Create: `backend/internal/games/materials.go`
- Test: `backend/internal/games/materials_test.go`

**Interfaces:**
- Consumes: `games.Store` (già esistente, `backend/internal/games/store.go:59`), `games.ErrNotFound`.
- Produces:
  - `games.Material{ID, GameID int64; Name string; Quantity, Position int}`
  - `games.MaterialInput{Name string; Quantity int}`
  - `games.ListMaterials(ctx context.Context, gameID int64) ([]Material, error)`
  - `games.ReplaceMaterials(ctx context.Context, gameID int64, in []MaterialInput) ([]Material, error)`
  - `games.ErrMaterialInvalid`, `games.ErrDuplicateMaterial`, `games.ErrTooManyMaterials`
  - `games.MaxMaterialNameChars = 60`, `games.MaxMaterialQuantity = 9999`, `games.MaxMaterialsPerGame = 60`

- [ ] **Step 1: Scrivi la migrazione**

Crea `backend/internal/db/migrations/0017_game_materials.sql`:

```sql
-- Il contenuto della scatola, come lo tiene l'admin nel catalogo. Sta sul
-- gioco e non sulla lingua: le tessere sono le stesse in ogni edizione, e
-- una lista per lingua vorrebbe dire tenerne due allineate a mano.
CREATE TABLE game_material (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id  INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity >= 1),
    -- L'ordine lo decide l'admin: è l'ordine in cui si controlla la
    -- scatola, e non coincide con nessun ordinamento naturale.
    position INTEGER NOT NULL,
    UNIQUE(game_id, name)
);

CREATE INDEX idx_game_material_game ON game_material(game_id, position);

-- L'esito della riconsegna, una riga solo per ciò che NON è tornato intero
-- o non è stato verificato. Un prestito pulito non ne lascia nessuna:
-- questa tabella si legge per rispondere a "cosa manca", non per
-- archiviare i controlli andati bene.
CREATE TABLE loan_material_issue (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    loan_id     INTEGER NOT NULL REFERENCES game_loans(id) ON DELETE CASCADE,

    -- SET NULL e non CASCADE: se domani l'admin cancella la voce dal
    -- catalogo, il fatto che a marzo mancassero sette tessere resta vero.
    material_id INTEGER REFERENCES game_material(id) ON DELETE SET NULL,

    -- Nome e quantità attesa sono COPIATI al momento della riconsegna, non
    -- risolti con una join: la lista del catalogo può cambiare, il registro
    -- di quella sera no.
    name        TEXT NOT NULL,
    expected    INTEGER NOT NULL,

    -- NULL = voce non verificata. Un numero = quante ne sono tornate,
    -- sempre meno di expected (una voce completa non genera riga). Lo stato
    -- vive nel nullable, come returned_at sui prestiti: una colonna di
    -- stato separata sarebbe una verità duplicata che può divergere.
    returned    INTEGER CHECK (returned IS NULL OR returned >= 0),

    UNIQUE(loan_id, material_id)
);

CREATE INDEX idx_loan_material_issue_loan ON loan_material_issue(loan_id);
```

- [ ] **Step 2: Scrivi i test che falliscono**

Crea `backend/internal/games/materials_test.go`. `newTestStoreWithDB` e `strPtr` esistono già in `backend/internal/games/store_test.go` — non ridichiararli.

```go
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
```

- [ ] **Step 3: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/games/ -run Material -v
```
Atteso: FAIL in compilazione — `undefined: games.Material`, `undefined: games.ReplaceMaterials`.

- [ ] **Step 4: Scrivi lo store**

Crea `backend/internal/games/materials.go`:

```go
package games

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// I tetti stanno qui perché lo store è l'unico punto che ogni scrittura
// attraversa, da qualunque parte arrivi: il form dell'admin, la proposta
// del modello, un test. Gli altri pacchetti li importano da qui invece di
// ricopiarne il valore.
const (
	MaxMaterialNameChars = 60
	MaxMaterialQuantity  = 9999
	MaxMaterialsPerGame  = 60
)

// Material è una voce del contenuto della scatola.
type Material struct {
	ID       int64
	GameID   int64
	Name     string
	Quantity int
	Position int
}

// MaterialInput è una voce come la manda l'editor: senza id e senza
// posizione, perché l'ordine è quello della lista che arriva.
type MaterialInput struct {
	Name     string
	Quantity int
}

var (
	ErrMaterialInvalid   = errors.New("material name or quantity is not valid")
	ErrDuplicateMaterial = errors.New("duplicate material name")
	ErrTooManyMaterials  = errors.New("too many materials for one game")
)

// ListMaterials torna le voci nell'ordine deciso dall'admin.
func (s *Store) ListMaterials(ctx context.Context, gameID int64) ([]Material, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, name, quantity, position
		 FROM game_material WHERE game_id = ? ORDER BY position, id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Material{}
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.GameID, &m.Name, &m.Quantity, &m.Position); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMaterials sostituisce l'intera lista di un gioco in una
// transazione.
//
// Sostituzione totale e non CRUD per riga: l'editor è una lista che si
// salva tutta insieme e il riordino tocca comunque quasi tutte le righe,
// quindi tre rotte costringerebbero il frontend a calcolare un diff per poi
// mandare N richieste che possono fallire a metà. Così il DB resta o tutto
// vecchio o tutto nuovo.
//
// Effetto collaterale documentato: gli id cambiano a ogni salvataggio, e
// loan_material_issue.material_id dei prestiti passati diventa NULL. Il
// registro storico sopravvive perché nome e quantità attesa sono copiati
// sulla riga di esito.
func (s *Store) ReplaceMaterials(ctx context.Context, gameID int64, in []MaterialInput) ([]Material, error) {
	clean, err := validateMaterials(in)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM game_material WHERE game_id = ?`, gameID); err != nil {
		return nil, err
	}
	for i, m := range clean {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO game_material (game_id, name, quantity, position) VALUES (?, ?, ?, ?)`,
			gameID, m.Name, m.Quantity, i,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListMaterials(ctx, gameID)
}

// validateMaterials normalizza i nomi e rifiuta ciò che non può stare in
// tabella. Il confronto per il duplicato è lowercase + trim, lo stesso
// criterio con cui la classifica riconosce due volte lo stesso giocatore.
func validateMaterials(in []MaterialInput) ([]MaterialInput, error) {
	if len(in) > MaxMaterialsPerGame {
		return nil, fmt.Errorf("%w: %d voci", ErrTooManyMaterials, len(in))
	}
	seen := make(map[string]bool, len(in))
	out := make([]MaterialInput, 0, len(in))
	for _, m := range in {
		name := strings.TrimSpace(m.Name)
		if name == "" || len([]rune(name)) > MaxMaterialNameChars {
			return nil, fmt.Errorf("%w: nome %q", ErrMaterialInvalid, m.Name)
		}
		if m.Quantity < 1 || m.Quantity > MaxMaterialQuantity {
			return nil, fmt.Errorf("%w: quantità %d per %q", ErrMaterialInvalid, m.Quantity, name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateMaterial, name)
		}
		seen[key] = true
		out = append(out, MaterialInput{Name: name, Quantity: m.Quantity})
	}
	return out, nil
}
```

Gli import di questo file sono esattamente `context`, `errors`, `fmt`, `strings`: non serve `database/sql`, perché la riconsegna legge `game_material` con la sua query, dentro `internal/events`.

- [ ] **Step 5: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/games/ -v
```
Atteso: PASS, inclusi i test preesistenti del pacchetto.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0017_game_materials.sql \
        backend/internal/games/materials.go \
        backend/internal/games/materials_test.go
git commit -m "feat: keep a box-contents list for every game"
```

---

### Task 2: Esito della riconsegna in `internal/events`

**Files:**
- Modify: `backend/internal/events/loans.go` (firma di `ReturnLoan`, riga ~148)
- Modify: `backend/internal/httpapi/loans_handlers.go:95` (adegua l'unico chiamante)
- Test: `backend/internal/events/loans_test.go` (aggiungi in fondo)

**Interfaces:**
- Consumes: `events.Store`, `events.ErrNotFound`, `events.ErrLoanAlreadyReturned`, `games.Material` (solo concettualmente: `events` legge `game_material` con SQL diretto e **non** importa `internal/games`, per non creare una dipendenza fra i due store).
- Produces:
  - `events.MaterialCheck{MaterialID int64; Complete bool; Returned *int}`
  - `events.MaterialIssue{LoanID int64; Name string; Expected int; Returned *int}`
  - `events.ReturnLoan(ctx context.Context, id int64, notes *string, checks []MaterialCheck) (Loan, error)` — **firma cambiata**, il quarto parametro è nuovo
  - `events.ListMaterialIssues(ctx context.Context, loanIDs []int64) (map[int64][]MaterialIssue, error)`
  - `events.ErrMaterialCheckInvalid`

- [ ] **Step 1: Scrivi i test che falliscono**

Aggiungi in fondo a `backend/internal/events/loans_test.go`. Gli helper `mustLend`, `firstCopy`, `newTestStore`, `mustCreateGame`, `mustCreateEvent` esistono già nel pacchetto di test.

```go
// mustMaterials scrive le voci del catalogo con SQL diretto: internal/events
// non importa internal/games, e nemmeno i suoi test devono farlo per una
// tabella di due colonne.
func mustMaterials(t *testing.T, conn *sql.DB, gameID int64, rows ...[2]any) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(rows))
	for i, r := range rows {
		res, err := conn.Exec(
			`INSERT INTO game_material (game_id, name, quantity, position) VALUES (?, ?, ?, ?)`,
			gameID, r[0], r[1], i)
		if err != nil {
			t.Fatalf("insert material %v: %v", r[0], err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestReturnLoanWithoutChecksWritesNoIssue(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	mustMaterials(t, conn, gameID, [2]any{"tessere", 72})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("return: %v", err)
	}
	issues, err := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	if len(issues[loan.ID]) != 0 {
		t.Fatalf("senza checklist non si scrive niente, ho %+v", issues[loan.ID])
	}
}

func TestReturnLoanWithEverythingCheckedWritesNoIssue(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"tessere", 72}, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	checks := []events.MaterialCheck{
		{MaterialID: ids[0], Complete: true},
		{MaterialID: ids[1], Complete: true},
	}
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, checks); err != nil {
		t.Fatalf("return: %v", err)
	}
	issues, _ := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if len(issues[loan.ID]) != 0 {
		t.Fatalf("un controllo pulito non lascia righe, ho %+v", issues[loan.ID])
	}
}

func TestReturnLoanRecordsShortagesAndUncheckedRows(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID,
		[2]any{"tessere", 72}, [2]any{"carte", 40}, [2]any{"dadi", 5})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	returned := 35
	checks := []events.MaterialCheck{
		{MaterialID: ids[0], Complete: true},                    // tutte tornate
		{MaterialID: ids[1], Complete: false, Returned: &returned}, // 35 su 40
		// ids[2] non compare: non verificata
	}
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, checks); err != nil {
		t.Fatalf("return: %v", err)
	}

	issues, err := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	got := issues[loan.ID]
	if len(got) != 2 {
		t.Fatalf("volevo 2 righe di esito, ho %+v", got)
	}
	if got[0].Name != "carte" || got[0].Expected != 40 || got[0].Returned == nil || *got[0].Returned != 35 {
		t.Errorf("riga incompleta inattesa: %+v", got[0])
	}
	if got[1].Name != "dadi" || got[1].Returned != nil {
		t.Errorf("riga non verificata inattesa: %+v", got[1])
	}
}

func TestReturnLoanTreatsExtraPiecesAsComplete(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	// 41 su 40: capita di ritrovare il pezzo di un'altra scatola. Non manca
	// niente, quindi non è un esito da registrare.
	extra := 41
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil,
		[]events.MaterialCheck{{MaterialID: ids[0], Returned: &extra}}); err != nil {
		t.Fatalf("return: %v", err)
	}
	issues, _ := store.ListMaterialIssues(context.Background(), []int64{loan.ID})
	if len(issues[loan.ID]) != 0 {
		t.Fatalf("non doveva restare niente, ho %+v", issues[loan.ID])
	}
}

func TestReturnLoanRejectsNegativeQuantity(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	negative := -1
	_, err := store.ReturnLoan(context.Background(), loan.ID, nil,
		[]events.MaterialCheck{{MaterialID: ids[0], Returned: &negative}})
	if !errors.Is(err, events.ErrMaterialCheckInvalid) {
		t.Fatalf("volevo ErrMaterialCheckInvalid, ho %v", err)
	}
	// E il prestito deve essere ancora aperto: o si chiude con il suo esito,
	// o non si chiude.
	open, err := store.ListLoansForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(open) != 1 || open[0].ReturnedAt != nil {
		t.Fatalf("il prestito non doveva chiudersi: %+v", open)
	}
}

func TestReturnLoanTwiceStillFails(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	mustMaterials(t, conn, gameID, [2]any{"meeple", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); err != nil {
		t.Fatalf("prima return: %v", err)
	}
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil, nil); !errors.Is(err, events.ErrLoanAlreadyReturned) {
		t.Fatalf("volevo ErrLoanAlreadyReturned, ho %v", err)
	}
}
```

I test usano `newTestStoreWithConn`. Se nel pacchetto di test esiste già solo `newTestStore(t) (*events.Store, *games.Store)`, aggiungi accanto ad esso, in `backend/internal/events/store_test.go`, la variante che restituisce anche la connessione — riusa il corpo dell'helper esistente invece di duplicarlo:

```go
// newTestStoreWithConn è newTestStore più la connessione grezza, per i test
// che devono scrivere tabelle di altri pacchetti (game_material) senza
// importarli.
func newTestStoreWithConn(t *testing.T) (*events.Store, *games.Store, *sql.DB) {
	t.Helper()
	// ...stesso corpo di newTestStore, restituendo anche conn
}
```

Aggiungi gli import mancanti a `loans_test.go`: `database/sql`, `errors`, `context`.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ -run Material -v
```
Atteso: FAIL in compilazione — `too many arguments in call to store.ReturnLoan`, `undefined: events.MaterialCheck`.

- [ ] **Step 3: Riscrivi `ReturnLoan` e aggiungi le letture**

In `backend/internal/events/loans.go`, sostituisci l'intero `ReturnLoan` esistente (da `// ReturnLoan chiude un prestito.` fino alla sua chiusura) con:

```go
// MaterialCheck è una voce della checklist come la manda la modale di
// riconsegna.
type MaterialCheck struct {
	MaterialID int64
	// Complete è la spunta: è tornata tutta, e Returned si ignora.
	Complete bool
	// Returned ha senso solo con Complete == false: nil significa "non
	// verificata", un numero quante ne sono tornate.
	Returned *int
}

// MaterialIssue è l'esito registrato su una voce che non è tornata intera o
// non è stata controllata. Nome e quantità attesa sono copie del momento
// della riconsegna, non join sul catalogo: la lista del gioco può cambiare,
// il registro di quella sera no.
type MaterialIssue struct {
	LoanID   int64
	Name     string
	Expected int
	// Returned nil = voce non verificata.
	Returned *int
}

// ErrMaterialCheckInvalid è una quantità resa negativa. Non è un caso da
// tollerare in silenzio: significa che la modale ha mandato spazzatura, e
// chiudere il prestito con un esito sbagliato è peggio che non chiuderlo.
var ErrMaterialCheckInvalid = errors.New("returned quantity cannot be negative")

// ReturnLoan chiude un prestito e registra l'esito del controllo dei
// materiali. `notes` nil lascia quelle scritte alla consegna: la modale
// manda il campo solo se l'organizzatore l'ha toccato, e un nil non deve
// cancellare quello che c'era.
//
// `checks` nil significa "nessuna checklist" e lascia il comportamento
// identico a prima: un gioco senza materiali, o un client vecchio, chiude un
// prestito senza scrivere nessun esito. Non è la stessa cosa di una
// checklist con tutte le voci non verificate, che invece le registra.
//
// Tutto in una transazione: prima l'UPDATE faceva una riga sola, ora sono
// una UPDATE più N INSERT, e un prestito chiuso senza il suo esito sarebbe
// peggio di un prestito rimasto aperto.
func (s *Store) ReturnLoan(ctx context.Context, id int64, notes *string, checks []MaterialCheck) (Loan, error) {
	loan, err := s.getLoanByID(ctx, id)
	if err != nil {
		return Loan{}, err
	}
	if notes == nil {
		notes = loan.Notes
	}
	for _, c := range checks {
		if c.Returned != nil && *c.Returned < 0 {
			return Loan{}, fmt.Errorf("%w: %d", ErrMaterialCheckInvalid, *c.Returned)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Loan{}, err
	}
	defer tx.Rollback()

	// `AND returned_at IS NULL` fa il lavoro del controllo: una seconda
	// restituzione non tocca nessuna riga, e lo sappiamo da RowsAffected
	// invece che da una lettura che potrebbe essere già vecchia.
	res, err := tx.ExecContext(ctx,
		`UPDATE game_loans SET returned_at = datetime('now'), notes = ?
		 WHERE id = ? AND returned_at IS NULL`, notes, id,
	)
	if err != nil {
		return Loan{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Loan{}, err
	}
	if affected == 0 {
		return Loan{}, ErrLoanAlreadyReturned
	}

	if checks != nil {
		if err := writeMaterialIssues(ctx, tx, id, checks); err != nil {
			return Loan{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Loan{}, err
	}
	return s.getLoanByID(ctx, id)
}

// writeMaterialIssues scrive una riga per ogni voce che non è tornata
// intera o non è stata verificata. Le voci le rilegge dal catalogo dentro
// la transazione invece di fidarsi di quelle mandate dal client: la modale
// può essere aperta da dieci minuti e la lista essere cambiata nel
// frattempo, e ciò che conta è il contenuto della scatola di adesso.
func writeMaterialIssues(ctx context.Context, tx *sql.Tx, loanID int64, checks []MaterialCheck) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT m.id, m.name, m.quantity
		 FROM game_material m
		 JOIN event_games eg ON eg.game_id = m.game_id
		 JOIN game_loans l ON l.event_game_id = eg.id
		 WHERE l.id = ?
		 ORDER BY m.position, m.id`, loanID)
	if err != nil {
		return err
	}
	type material struct {
		id       int64
		name     string
		quantity int
	}
	var materials []material
	for rows.Next() {
		var m material
		if err := rows.Scan(&m.id, &m.name, &m.quantity); err != nil {
			rows.Close()
			return err
		}
		materials = append(materials, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	byID := make(map[int64]MaterialCheck, len(checks))
	for _, c := range checks {
		byID[c.MaterialID] = c
	}

	for _, m := range materials {
		c, sent := byID[m.id]
		switch {
		case sent && c.Complete:
			continue // spuntata: tornata tutta
		case sent && c.Returned != nil && *c.Returned >= m.quantity:
			continue // contata e non manca niente
		}
		var returned any
		if sent && !c.Complete && c.Returned != nil {
			returned = *c.Returned
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO loan_material_issue (loan_id, material_id, name, expected, returned)
			 VALUES (?, ?, ?, ?, ?)`,
			loanID, m.id, m.name, m.quantity, returned,
		); err != nil {
			return err
		}
	}
	return nil
}

// ListMaterialIssues legge gli esiti di più prestiti in una query sola: il
// banco prestiti ne mostra una lista intera, e una query per riga sarebbe
// una N+1 su una pagina che si ricarica dopo ogni consegna.
func (s *Store) ListMaterialIssues(ctx context.Context, loanIDs []int64) (map[int64][]MaterialIssue, error) {
	out := map[int64][]MaterialIssue{}
	if len(loanIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(loanIDs)), ",")
	args := make([]any, len(loanIDs))
	for i, id := range loanIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT loan_id, name, expected, returned FROM loan_material_issue
		 WHERE loan_id IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var iss MaterialIssue
		var returned sql.NullInt64
		if err := rows.Scan(&iss.LoanID, &iss.Name, &iss.Expected, &returned); err != nil {
			return nil, err
		}
		if returned.Valid {
			v := int(returned.Int64)
			iss.Returned = &v
		}
		out[iss.LoanID] = append(out[iss.LoanID], iss)
	}
	return out, rows.Err()
}
```

`loans.go` importa già `context`, `database/sql`, `errors`, `strings`, `time`; aggiungi `fmt`.

- [ ] **Step 4: Adegua l'unico chiamante**

In `backend/internal/httpapi/loans_handlers.go:95`, la chiamata diventa:

```go
	loan, err := s.Events.ReturnLoan(r.Context(), id, req.Notes, nil)
```

Il campo `materials` della richiesta arriva nel Task 6: qui si passa `nil` solo per far compilare, e il comportamento resta identico a oggi.

- [ ] **Step 5: Lancia la suite intera e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```
Atteso: PASS ovunque. I test dei prestiti già esistenti devono passare senza modifiche oltre al quarto argomento.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/events/loans.go backend/internal/events/loans_test.go \
        backend/internal/events/store_test.go backend/internal/httpapi/loans_handlers.go
git commit -m "feat: record what did not come back with a returned game"
```

---

### Task 3: Rotte GET e PUT dei materiali

**Files:**
- Create: `backend/internal/httpapi/materials_handlers.go`
- Modify: `backend/internal/httpapi/router.go` (blocco `protected`, dopo la riga 145)
- Test: `backend/internal/httpapi/materials_handlers_test.go`

**Interfaces:**
- Consumes: `games.ListMaterials`, `games.ReplaceMaterials`, `games.Material`, `games.MaterialInput`, gli errori sentinella e i tetti del Task 1; `parseIDParam`, `writeJSON`, `writeError` (già in `backend/internal/httpapi/json.go`).
- Produces:
  - `GET /api/games/{id}/materials` → `{"materials": [{"id":1,"name":"tessere","quantity":72}]}`
  - `PUT /api/games/{id}/materials` ← `{"materials":[{"name":"tessere","quantity":72}]}` → stessa forma della GET
  - `toMaterialsResponse(ms []games.Material) map[string]any` (usata anche dal Task 5)

- [ ] **Step 1: Scrivi i test che falliscono**

Crea `backend/internal/httpapi/materials_handlers_test.go`. Guarda `backend/internal/httpapi/games_handlers_test.go` per gli helper di sessione già presenti nel pacchetto (login/cookie) e riusali con lo stesso nome, senza ridichiararli.

```go
package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetMaterialsStartsEmpty(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")

	req := httptest.NewRequest(http.MethodGet, "/api/games/"+itoa(gameID)+"/materials", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("volevo 200, ho %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Materials []map[string]any `json:"materials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Materials == nil {
		t.Fatal("volevo [] e non null: una lista vuota che arriva come null costringe la UI a difendersi")
	}
	if len(body.Materials) != 0 {
		t.Fatalf("volevo una lista vuota, ho %+v", body.Materials)
	}
}

func TestPutMaterialsSavesAndReadsBack(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")

	payload := `{"materials":[{"name":"tessere","quantity":72},{"name":"meeple","quantity":40}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/games/"+itoa(gameID)+"/materials", strings.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("volevo 200, ho %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Materials []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Quantity int    `json:"quantity"`
		} `json:"materials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Materials) != 2 || body.Materials[0].Name != "tessere" || body.Materials[1].Quantity != 40 {
		t.Fatalf("risposta inattesa: %+v", body.Materials)
	}
	if body.Materials[0].ID == 0 {
		t.Error("la risposta deve portare gli id: la checklist li rimanda indietro")
	}
}

func TestPutMaterialsRejectsBadRows(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")

	cases := map[string]string{
		"nome vuoto":    `{"materials":[{"name":"  ","quantity":4}]}`,
		"quantità zero": `{"materials":[{"name":"dadi","quantity":0}]}`,
		"duplicato":     `{"materials":[{"name":"meeple","quantity":40},{"name":"Meeple","quantity":8}]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/games/"+itoa(gameID)+"/materials", strings.NewReader(payload))
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			server.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("volevo 400, ho %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestMaterialsRoutesNeedASession(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")

	for _, tc := range []struct {
		method, body string
	}{
		{http.MethodGet, ""},
		{http.MethodPut, `{"materials":[]}`},
	} {
		req := httptest.NewRequest(tc.method, "/api/games/"+itoa(gameID)+"/materials", strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s senza cookie: volevo 401, ho %d", tc.method, rec.Code)
		}
	}
}

func TestMaterialsOnAMissingGameIs404(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	cookie := loginAsAdmin(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/games/9999/materials", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("volevo 404, ho %d: %s", rec.Code, rec.Body)
	}
}
```

Se `loginAsAdmin`, `createGameViaAPI` o `itoa` non esistono con questi nomi nel pacchetto di test, usa gli helper equivalenti che trovi in `games_handlers_test.go` e adegua le chiamate; non creare doppioni.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run Materials -v
```
Atteso: FAIL con 404 su tutte le rotte (non ancora registrate).

- [ ] **Step 3: Scrivi gli handler**

Crea `backend/internal/httpapi/materials_handlers.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"boardgames-manager/internal/games"
)

// toMaterialsResponse manda sempre una lista, mai null: una lista vuota che
// arriva come null costringe ogni punto della UI a difendersi.
func toMaterialsResponse(ms []games.Material) map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{"id": m.ID, "name": m.Name, "quantity": m.Quantity})
	}
	return map[string]any{"materials": out}
}

// requireGame risponde 404/500 e dice al chiamante se può proseguire. Le tre
// rotte dei materiali cominciano tutte così: un id inventato deve dare 404
// prima di qualunque lavoro.
func (s *Server) requireGame(w http.ResponseWriter, r *http.Request, gameID int64) (games.Game, bool) {
	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "gioco non trovato")
		return games.Game{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return games.Game{}, false
	}
	return game, true
}

func (s *Server) listMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	ms, err := s.Games.ListMaterials(r.Context(), gameID)
	if err != nil {
		log.Printf("materials: list for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the materials")
		return
	}
	writeJSON(w, http.StatusOK, toMaterialsResponse(ms))
}

type materialsRequest struct {
	Materials []struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	} `json:"materials"`
}

func (s *Server) putMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	var req materialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo della richiesta non valido")
		return
	}
	in := make([]games.MaterialInput, 0, len(req.Materials))
	for _, m := range req.Materials {
		in = append(in, games.MaterialInput{Name: m.Name, Quantity: m.Quantity})
	}

	ms, err := s.Games.ReplaceMaterials(r.Context(), gameID, in)
	switch {
	case errors.Is(err, games.ErrMaterialInvalid):
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"ogni voce vuole un nome (fino a %d caratteri) e una quantità da 1 a %d",
			games.MaxMaterialNameChars, games.MaxMaterialQuantity))
	case errors.Is(err, games.ErrDuplicateMaterial):
		writeError(w, http.StatusBadRequest, "due voci con lo stesso nome: unisci le righe")
	case errors.Is(err, games.ErrTooManyMaterials):
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"al massimo %d voci per gioco", games.MaxMaterialsPerGame))
	case err != nil:
		log.Printf("materials: save for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not save the materials")
	default:
		writeJSON(w, http.StatusOK, toMaterialsResponse(ms))
	}
}
```

- [ ] **Step 4: Registra le rotte**

In `backend/internal/httpapi/router.go`, dentro il blocco `protected`, subito dopo la riga delle `suggested-questions`:

```go
		protected.Get("/api/games/{id}/materials", s.listMaterialsHandler)
		protected.Put("/api/games/{id}/materials", s.putMaterialsHandler)
```

- [ ] **Step 5: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -v
```
Atteso: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/materials_handlers.go \
        backend/internal/httpapi/materials_handlers_test.go \
        backend/internal/httpapi/router.go
git commit -m "feat: expose the box-contents list over the admin API"
```

---

### Task 4: Proposta dei materiali dal modello (`internal/ai`)

**Files:**
- Create: `backend/internal/ai/materials.go`
- Test: `backend/internal/ai/materials_internal_test.go` (il parser è privato)

**Interfaces:**
- Consumes: `ai.HTTPClient` e i suoi helper interni (`chat`, `configured`, `ErrNotConfigured`) — leggi `backend/internal/ai/suggest.go` e `client.go` prima di scrivere, e riusa lo stesso meccanismo di chiamata; `games.MaxMaterialNameChars`, `games.MaxMaterialQuantity`, `games.MaxMaterialsPerGame`.
- Produces:
  - `ai.SuggestedMaterial{Name string; Quantity int}`
  - `ai.MaterialLister` interface con `ListMaterials(ctx context.Context, gameName string, passages []string) ([]SuggestedMaterial, error)`
  - `(*ai.HTTPClient).ListMaterials` che la implementa
  - `ai.ErrMaterialsRejected`

- [ ] **Step 1: Scrivi i test del parser**

Crea `backend/internal/ai/materials_internal_test.go`:

```go
package ai

import "testing"

func TestParseMaterialsReadsCleanLines(t *testing.T) {
	got := parseMaterials("tessere\t72\nmeeple\t40\n")
	if len(got) != 2 {
		t.Fatalf("volevo 2 voci, ho %+v", got)
	}
	if got[0].Name != "tessere" || got[0].Quantity != 72 {
		t.Errorf("prima voce inattesa: %+v", got[0])
	}
	if got[1].Name != "meeple" || got[1].Quantity != 40 {
		t.Errorf("seconda voce inattesa: %+v", got[1])
	}
}

func TestParseMaterialsSurvivesSloppyFormatting(t *testing.T) {
	raw := "Ecco il contenuto:\n" +
		"1. tessere paesaggio: 72\n" +
		"- meeple — 40\n" +
		"* dadi 5\n" +
		"plancia punteggio  1\n"
	got := parseMaterials(raw)
	want := map[string]int{
		"tessere paesaggio": 72,
		"meeple":            40,
		"dadi":              5,
		"plancia punteggio": 1,
	}
	if len(got) != len(want) {
		t.Fatalf("volevo %d voci, ho %+v", len(want), got)
	}
	for _, m := range got {
		if want[m.Name] != m.Quantity {
			t.Errorf("voce inattesa: %+v", m)
		}
	}
}

func TestParseMaterialsDropsWhatItCannotUse(t *testing.T) {
	raw := "Contenuto della scatola\n" + // nessun numero: non è una voce
		"regolamento\n" + // idem
		"carte 40\n" +
		"tessere 0\n" + // sotto il minimo
		"segnalini 99999\n" + // sopra il massimo
		"Carte 12\n" // duplicato di "carte", vince il primo
	got := parseMaterials(raw)
	if len(got) != 1 || got[0].Name != "carte" || got[0].Quantity != 40 {
		t.Fatalf("volevo solo carte 40, ho %+v", got)
	}
}

func TestParseMaterialsStopsAtTheCap(t *testing.T) {
	raw := ""
	for i := 0; i < 200; i++ {
		raw += "pezzo" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + " 2\n"
	}
	if got := parseMaterials(raw); len(got) > 60 {
		t.Fatalf("il tetto non è stato applicato: %d voci", len(got))
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -run ParseMaterials -v
```
Atteso: FAIL — `undefined: parseMaterials`.

- [ ] **Step 3: Scrivi il pacchetto**

Crea `backend/internal/ai/materials.go`. **Prima di scriverlo apri `backend/internal/ai/suggest.go`** e copia da lì la meccanica della chiamata (costruzione del contesto con timeout, invocazione del client, gestione di `ErrNotConfigured`): questo file deve essere il suo gemello, non una variante.

```go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"boardgames-manager/internal/games"
)

// materialsTimeout sta fra suggestTimeout e segmentTimeout: l'input è
// qualche passaggio di manuale e l'output può essere trenta righe, quindi
// più di tre domande e molto meno di un manuale intero.
const materialsTimeout = 45 * time.Second

// ErrMaterialsRejected dice che nella risposta non c'era nemmeno una riga
// utilizzabile. È distinto da un guasto di rete perché il pannello admin
// deve poter dire "non l'ho trovato nel manuale" invece di "riprova".
var ErrMaterialsRejected = errors.New("ai materials rejected: no usable line in the response")

// SuggestedMaterial è una voce proposta dal modello. Non è ancora una
// games.Material: non ha id, non ha posizione, e soprattutto non è salvata —
// l'admin la conferma prima.
type SuggestedMaterial struct {
	Name     string
	Quantity int
}

// MaterialLister è l'astrazione che serve all'handler; HTTPClient la
// implementa, i test iniettano un finto. Stesso schema di QuestionSuggester.
type MaterialLister interface {
	ListMaterials(ctx context.Context, gameName string, passages []string) ([]SuggestedMaterial, error)
}

// materialsSystemPrompt chiede righe e vieta tutto il resto. Non è una
// garanzia — la mitigazione vera è parseMaterials — ma è ciò che rende lo
// scarto raro invece che normale.
var materialsSystemPrompt = "Ricevi il nome di un gioco da tavolo e alcuni passaggi del suo regolamento. " +
	"Estrai il CONTENUTO DELLA SCATOLA: l'elenco dei pezzi fisici con la loro quantità.\n" +
	"Regole assolute:\n" +
	"1. Una voce per riga, nella forma NOME<TAB>QUANTITÀ. Nessuna numerazione, nessun elenco puntato, nessun preambolo, nessun commento.\n" +
	"2. La quantità è un numero intero. Se il regolamento non la dice, salta la voce.\n" +
	"3. Nomi in italiano, al plurale, come li direbbe un giocatore al tavolo: \"tessere\", \"meeple\", \"carte\".\n" +
	fmt.Sprintf("4. Al massimo %d voci, ogni nome sotto i %d caratteri.\n", games.MaxMaterialsPerGame, games.MaxMaterialNameChars) +
	"5. Una riga per tipo di pezzo: \"meeple\t40\", non una riga per colore, a meno che il regolamento non li elenchi già separati.\n" +
	"6. IGNORA tutto ciò che non è un pezzo dentro la scatola: regole, autori, crediti, ringraziamenti, siti web."

// ListMaterials chiede al modello il contenuto della scatola partendo da
// passaggi del manuale già indicizzato. Non salva niente: la proposta la
// conferma l'admin.
func (c *HTTPClient) ListMaterials(ctx context.Context, gameName string, passages []string) ([]SuggestedMaterial, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	if len(passages) == 0 {
		// Nessun passaggio da leggere: il chiamante deve dirlo all'admin
		// ("indicizza prima un manuale"), non ricevere una lista inventata.
		return nil, ErrMaterialsRejected
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Gioco: %s\n\nPassaggi dal regolamento:\n", gameName)
	for _, p := range passages {
		fmt.Fprintf(&b, "---\n%s\n", p)
	}

	payload, err := json.Marshal(chatRequest{
		Model: c.Model,
		// temperature 0 come SuggestQuestions: il contenuto di una scatola
		// è un fatto, non deve cambiare a ogni generazione.
		Temperature:     0,
		ReasoningEffort: reasoningEffortNone,
		Messages: []chatMessage{
			{Role: "system", Content: materialsSystemPrompt},
			{Role: "user", Content: b.String()},
		},
	})
	if err != nil {
		return nil, err
	}

	raw, err := c.postChat(ctx, payload, materialsTimeout)
	if err != nil {
		return nil, err
	}
	out := parseMaterials(raw)
	if len(out) == 0 {
		return nil, ErrMaterialsRejected
	}
	return out, nil
}

// trailingNumber trova l'ultimo intero della riga: è lì che sta la quantità
// sia in "tessere 72" sia in "tessere paesaggio: 72", e cercare il primo
// numero inciamperebbe su "1." di una numerazione.
var trailingNumber = regexp.MustCompile(`(\d+)\s*$`)

// leadingBullet è la numerazione o il puntino che il modello aggiunge anche
// quando gli si dice di non farlo.
var leadingBullet = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)])\s*`)

// parseMaterials è la mitigazione vera contro una risposta fuori formato:
// tiene solo le righe che portano un nome e un intero nei limiti dello
// store, e scarta in silenzio tutto il resto. Se non resta niente, chi
// chiama lo racconta con ErrMaterialsRejected.
func parseMaterials(raw string) []SuggestedMaterial {
	seen := map[string]bool{}
	out := []SuggestedMaterial{}
	for _, line := range strings.Split(raw, "\n") {
		line = leadingBullet.ReplaceAllString(strings.TrimSpace(line), "")
		if line == "" {
			continue
		}
		m := trailingNumber.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		quantity, err := strconv.Atoi(m[1])
		if err != nil || quantity < 1 || quantity > games.MaxMaterialQuantity {
			continue
		}
		name := strings.TrimSpace(line[:len(line)-len(m[0])])
		name = strings.TrimRight(name, " \t:—-–")
		name = strings.TrimSpace(name)
		if name == "" || len([]rune(name)) > games.MaxMaterialNameChars {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, SuggestedMaterial{Name: name, Quantity: quantity})
		if len(out) == games.MaxMaterialsPerGame {
			break
		}
	}
	return out
}
```

**Nota per chi implementa:** `chatRequest`, `chatMessage`, `reasoningEffortNone` e `postChat` sono già nel pacchetto (`client.go` e `ask.go`) e li usa anche `SuggestQuestions`: la costruzione della richiesta qui sopra è la stessa, riusala così com'è invece di inventarne una variante. `postChat` applica già il timeout che riceve, quindi non serve un `context.WithTimeout` in più.

- [ ] **Step 4: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -v
```
Atteso: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/materials.go backend/internal/ai/materials_internal_test.go
git commit -m "feat: read the box contents out of an indexed rulebook"
```

---

### Task 5: Rotta di proposta `POST /api/games/{id}/materials/suggest`

**Files:**
- Modify: `backend/internal/httpapi/materials_handlers.go` (aggiungi in fondo)
- Modify: `backend/internal/httpapi/router.go` (blocco `protected`)
- Modify: `backend/internal/httpapi/ai_fake_test.go` (aggiungi il finto)
- Test: `backend/internal/httpapi/materials_handlers_test.go` (aggiungi in fondo)

**Interfaces:**
- Consumes: `ai.MaterialLister`, `ai.SuggestedMaterial`, `ai.ErrMaterialsRejected`, `ai.ErrNotConfigured` (Task 4); `s.Manuals.Search`, `s.Games.ListLanguages`; il campo `Server.Suggester` come modello per l'iniezione.
- Produces:
  - Campo `Server.MaterialLister ai.MaterialLister` (nil in produzione, valorizzato nei test)
  - `POST /api/games/{id}/materials/suggest` → `{"materials":[{"name":"tessere","quantity":72}]}`, **senza scrivere nel DB**

- [ ] **Step 1: Scrivi i test che falliscono**

Aggiungi a `backend/internal/httpapi/ai_fake_test.go`:

```go
// fakeMaterialLister sta al posto del provider per la proposta dei
// materiali, come fakeTranslator per le traduzioni.
type fakeMaterialLister struct {
	out          []ai.SuggestedMaterial
	err          error
	calls        int
	lastGame     string
	lastPassages []string
}

func (f *fakeMaterialLister) ListMaterials(ctx context.Context, gameName string, passages []string) ([]ai.SuggestedMaterial, error) {
	f.calls++
	f.lastGame = gameName
	f.lastPassages = passages
	return f.out, f.err
}
```

Aggiungi l'import di `boardgames-manager/internal/ai` al file se manca.

Aggiungi in fondo a `backend/internal/httpapi/materials_handlers_test.go`:

```go
func TestSuggestMaterialsNeedsAnIndexedManual(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.MaterialLister = &fakeMaterialLister{
		out: []ai.SuggestedMaterial{{Name: "tessere", Quantity: 72}},
	}
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")

	req := httptest.NewRequest(http.MethodPost, "/api/games/"+itoa(gameID)+"/materials/suggest", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	// Nessuna fonte indicizzata: 422 con il consiglio giusto, come fa la
	// rigenerazione delle domande suggerite.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("volevo 422, ho %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "indicizza") {
		t.Errorf("il messaggio deve dire cosa fare, ho %s", rec.Body)
	}
}

func TestSuggestMaterialsProposesWithoutSaving(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	lister := &fakeMaterialLister{out: []ai.SuggestedMaterial{
		{Name: "tessere", Quantity: 72},
		{Name: "meeple", Quantity: 40},
	}}
	server.MaterialLister = lister
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")
	indexOneChunk(t, conn, gameID, "Contenuto della scatola: 72 tessere e 40 meeple.")

	req := httptest.NewRequest(http.MethodPost, "/api/games/"+itoa(gameID)+"/materials/suggest", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("volevo 200, ho %d: %s", rec.Code, rec.Body)
	}
	if lister.calls != 1 {
		t.Fatalf("volevo una chiamata al modello, ne ho %d", lister.calls)
	}
	if len(lister.lastPassages) == 0 {
		t.Error("al modello devono arrivare i passaggi del manuale")
	}
	var body struct {
		Materials []map[string]any `json:"materials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Materials) != 2 {
		t.Fatalf("volevo 2 voci proposte, ho %+v", body.Materials)
	}

	// La proposta non tocca il DB: è l'admin a confermare.
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM game_material`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("la proposta ha salvato %d voci: non deve salvare niente", count)
	}
}

func TestSuggestMaterialsTellsWhenTheModelIsUseless(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.MaterialLister = &fakeMaterialLister{err: ai.ErrMaterialsRejected}
	cookie := loginAsAdmin(t, server)
	gameID := createGameViaAPI(t, server, cookie, "Carcassonne")
	indexOneChunk(t, conn, gameID, "Contenuto della scatola: tante cose.")

	req := httptest.NewRequest(http.MethodPost, "/api/games/"+itoa(gameID)+"/materials/suggest", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("volevo 422, ho %d: %s", rec.Code, rec.Body)
	}
}
```

Serve l'helper `indexOneChunk`. Guarda come i test esistenti di `ask_handler_test.go` popolano `game_source_chunk`: se c'è già un helper con quel ruolo riusalo e cancella questo passaggio; altrimenti aggiungilo a `materials_handlers_test.go`:

```go
// indexOneChunk mette una fonte indicizzata sul gioco, il minimo perché la
// ricerca FTS trovi qualcosa.
func indexOneChunk(t *testing.T, conn *sql.DB, gameID int64, text string) {
	t.Helper()
	store := manuals.NewStore(conn)
	err := store.ReplaceSource(context.Background(), gameID, nil, []manuals.SourceChunk{{
		ReferenceType: "faq", Reference: "https://example.test/faq",
		Heading: "Contenuto della scatola", Seq: 0, Text: text,
	}})
	if err != nil {
		t.Fatalf("index chunk: %v", err)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run SuggestMaterials -v
```
Atteso: FAIL in compilazione — `server.MaterialLister undefined`.

- [ ] **Step 3: Aggiungi il campo iniettabile**

In `backend/internal/httpapi/router.go`, accanto a `Suggester` (riga ~64):

```go
	// MaterialLister, quando è valorizzato, è il generatore della proposta
	// di materiali dal manuale. Stesso schema di AI/Vision/Segmenter/
	// Suggester: nil in produzione, un finto nei test.
	MaterialLister ai.MaterialLister
```

- [ ] **Step 4: Scrivi l'handler**

Aggiungi in fondo a `backend/internal/httpapi/materials_handlers.go` (e aggiungi gli import `boardgames-manager/internal/ai` e `strings` se mancano):

```go
// materialKeywords sono le parole con cui i regolamenti italiani intitolano
// l'elenco dei pezzi. Una query per parola, come vuole Manuals.Search: con
// un OR unico una parola comune sommergerebbe una rara.
var materialKeywords = []string{"contenuto", "componenti", "materiale", "materiali", "scatola"}

// maxMaterialPassages tiene il prompt corto: l'elenco dei pezzi sta in una
// pagina, e sei passaggi la coprono con margine.
const maxMaterialPassages = 6

// materialLister restituisce il generatore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di suggester() in questions_handlers.go, e per la stessa
// ragione: cambiare modello non deve richiedere un riavvio.
func (s *Server) materialLister(ctx context.Context) ai.MaterialLister {
	if s.MaterialLister != nil {
		return s.MaterialLister
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("materials: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// baseLanguageCode è la lingua da preferire nella ricerca sul manuale: la
// stessa che la chat usa come preferenza.
func (s *Server) baseLanguageCode(ctx context.Context, gameID int64) string {
	langs, err := s.Games.ListLanguages(ctx, gameID)
	if err != nil {
		return ""
	}
	for _, l := range langs {
		if l.IsBaseLanguage {
			return l.LanguageCode
		}
	}
	return ""
}

func (s *Server) suggestMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	game, ok := s.requireGame(w, r, gameID)
	if !ok {
		return
	}

	hits, _, err := s.Manuals.Search(r.Context(), gameID,
		s.baseLanguageCode(r.Context(), gameID), materialKeywords)
	if err != nil {
		log.Printf("materials: search for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not search the manual")
		return
	}
	if len(hits) == 0 {
		// Stesso trattamento di errNoHeadings per le domande suggerite:
		// l'admin deve leggere cosa fare, non "riprova".
		writeError(w, http.StatusUnprocessableEntity,
			"Nel manuale indicizzato non ho trovato l'elenco dei componenti: indicizza un documento nella sezione Chatbot, o scrivi le voci a mano.")
		return
	}
	if len(hits) > maxMaterialPassages {
		hits = hits[:maxMaterialPassages]
	}
	passages := make([]string, 0, len(hits))
	for _, h := range hits {
		passages = append(passages, h.Text)
	}

	out, err := s.materialLister(r.Context()).ListMaterials(r.Context(), game.Name, passages)
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		writeError(w, http.StatusUnprocessableEntity,
			"Nessun provider AI configurato: controlla le impostazioni.")
		return
	case errors.Is(err, ai.ErrMaterialsRejected):
		writeError(w, http.StatusUnprocessableEntity,
			"Nel manuale non ho trovato un elenco di componenti leggibile: scrivi le voci a mano.")
		return
	case err != nil:
		log.Printf("materials: suggest for game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway, "Il provider AI non ha risposto: riprova.")
		return
	}

	// La proposta non si salva: la conferma è dell'admin, come per ogni
	// altro risultato automatico dell'app.
	rows := make([]map[string]any, 0, len(out))
	for _, m := range out {
		rows = append(rows, map[string]any{"name": m.Name, "quantity": m.Quantity})
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": rows})
}
```

- [ ] **Step 5: Registra la rotta**

In `backend/internal/httpapi/router.go`, nel blocco `protected`, sotto le due rotte del Task 3:

```go
		protected.Post("/api/games/{id}/materials/suggest", s.suggestMaterialsHandler)
```

- [ ] **Step 6: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```
Atteso: PASS ovunque.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/materials_handlers.go \
        backend/internal/httpapi/materials_handlers_test.go \
        backend/internal/httpapi/ai_fake_test.go \
        backend/internal/httpapi/router.go
git commit -m "feat: propose a box-contents list from the indexed manual"
```

---

### Task 6: La checklist attraversa l'API dei prestiti

**Files:**
- Modify: `backend/internal/httpapi/loans_handlers.go` (`returnLoanRequest` e `returnLoanHandler`)
- Modify: `backend/internal/httpapi/loans_responses.go` (`toLoanDeskResponse`, `toReturnedLoanResponse`)
- Test: `backend/internal/httpapi/loans_handlers_test.go` (aggiungi in fondo)

**Interfaces:**
- Consumes: `events.MaterialCheck`, `events.MaterialIssue`, `events.ListMaterialIssues`, `events.ErrMaterialCheckInvalid` (Task 2); `games.ListMaterials` (Task 1).
- Produces:
  - `POST /api/loans/{id}/return` accetta `{"notes":…, "materials":[{"materialId":7,"complete":true},{"materialId":8,"complete":false,"returned":35},{"materialId":9,"complete":false,"returned":null}]}`
  - ogni riga di `copies` in `GET /api/events/{id}/loans` porta `"materials": [{"id","name","quantity"}]`
  - ogni riga di `returned` porta `"materialIssues": [{"name","expected","returned"}]` con `returned: null` per le non verificate

- [ ] **Step 1: Scrivi i test che falliscono**

Prima estendi le struct di risposta in cima a `backend/internal/httpapi/loans_handlers_test.go` — sono lì apposta perché un typo diventi un test rosso:

```go
type materialBody struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type materialIssueBody struct {
	Name     string `json:"name"`
	Expected int    `json:"expected"`
	Returned *int   `json:"returned"`
}
```

Aggiungi `Materials []materialBody \`json:"materials"\`` a `deskCopyBody` e `MaterialIssues []materialIssueBody \`json:"materialIssues"\`` a `returnedLoanBody`.

Poi aggiungi in fondo al file l'helper e i quattro test:

```go
// loanFixtureWithMaterials è loanFixture più le voci del catalogo sul
// gioco della serata, e restituisce i loro id nell'ordine in cui sono
// state scritte. Non riusa loanFixture perché ha bisogno del *Server per
// scrivere i materiali, che loanFixture non restituisce.
func loanFixtureWithMaterials(t *testing.T, rows ...games.MaterialInput) (http.Handler, *http.Cookie, int64, []events.EventGame, []int64) {
	t.Helper()
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	saved, err := server.Games.ReplaceMaterials(context.Background(), gameID, rows)
	if err != nil {
		t.Fatalf("replace materials: %v", err)
	}
	ids := make([]int64, 0, len(saved))
	for _, m := range saved {
		ids = append(ids, m.ID)
	}

	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	return router, cookie, event.ID, eventGames, ids
}

func TestLoanDesk_CarriesTheGameMaterials(t *testing.T) {
	router, cookie, eventID, _, ids := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
		games.MaterialInput{Name: "meeple", Quantity: 40},
	)

	desk := readDesk(t, router, cookie, eventID)
	if len(desk.Copies) != 1 {
		t.Fatalf("copies = %d, want 1", len(desk.Copies))
	}
	got := desk.Copies[0].Materials
	if len(got) != 2 {
		t.Fatalf("materials = %d righe, want 2: %+v", len(got), got)
	}
	if got[0].ID != ids[0] || got[0].Name != "tessere" || got[0].Quantity != 72 {
		t.Errorf("prima voce = %+v", got[0])
	}
	if got[1].Name != "meeple" || got[1].Quantity != 40 {
		t.Errorf("seconda voce = %+v", got[1])
	}
}

func TestReturnLoan_WithChecklistRecordsWhatIsMissing(t *testing.T) {
	router, cookie, eventID, eventGames, ids := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
		games.MaterialInput{Name: "carte", Quantity: 40},
		games.MaterialInput{Name: "dadi", Quantity: 5},
	)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	// tessere spuntate, carte contate 35 su 40, dadi mai toccati.
	body := fmt.Sprintf(
		`{"materials":[{"materialId":%d,"complete":true},{"materialId":%d,"complete":false,"returned":35}]}`,
		ids[0], ids[1])
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	desk := readDesk(t, router, cookie, eventID)
	if len(desk.Returned) != 1 {
		t.Fatalf("returned = %d righe, want 1", len(desk.Returned))
	}
	issues := desk.Returned[0].MaterialIssues
	if len(issues) != 2 {
		t.Fatalf("materialIssues = %d righe, want 2 (carte e dadi): %+v", len(issues), issues)
	}
	if issues[0].Name != "carte" || issues[0].Expected != 40 ||
		issues[0].Returned == nil || *issues[0].Returned != 35 {
		t.Errorf("riga incompleta = %+v", issues[0])
	}
	if issues[1].Name != "dadi" || issues[1].Returned != nil {
		t.Errorf("riga non verificata = %+v", issues[1])
	}
	for _, iss := range issues {
		if iss.Name == "tessere" {
			t.Error("una voce spuntata non deve lasciare un esito")
		}
	}
}

func TestReturnLoan_WithoutMaterialsFieldStillWorks(t *testing.T) {
	router, cookie, eventID, eventGames, _ := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
	)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	// Il corpo di ieri: nessun campo materials. È il contratto che tiene in
	// piedi i giochi senza materiali e i client non aggiornati.
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	desk := readDesk(t, router, cookie, eventID)
	if len(desk.Returned) != 1 {
		t.Fatalf("returned = %d righe, want 1", len(desk.Returned))
	}
	if len(desk.Returned[0].MaterialIssues) != 0 {
		t.Fatalf("senza checklist non si scrive niente: %+v", desk.Returned[0].MaterialIssues)
	}
}

func TestReturnLoan_RejectsANegativeReturnedQuantity(t *testing.T) {
	router, cookie, eventID, eventGames, ids := loanFixtureWithMaterials(t,
		games.MaterialInput{Name: "tessere", Quantity: 72},
	)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	body := fmt.Sprintf(`{"materials":[{"materialId":%d,"complete":false,"returned":-3}]}`, ids[0])
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	// O si chiude con il suo esito, o non si chiude: la copia deve essere
	// ancora fuori.
	desk := readDesk(t, router, cookie, eventID)
	if desk.Copies[0].OpenLoan == nil {
		t.Error("il prestito si è chiuso nonostante l'errore")
	}
}
```

Aggiungi `boardgames-manager/internal/games` agli import del file se manca.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run "LoanDesk|ReturnLoan" -v
```
Atteso: FAIL — `materials` assente nella risposta, `materialIssues` assente.

- [ ] **Step 3: Estendi la richiesta di restituzione**

In `backend/internal/httpapi/loans_handlers.go`, sostituisci `returnLoanRequest` e la chiamata dentro `returnLoanHandler`:

```go
type returnLoanRequest struct {
	// Notes nil lascia quelle scritte alla consegna: un corpo vuoto è il
	// caso normale, si restituisce senza avere niente da segnalare.
	Notes *string `json:"notes"`
	// Materials assente (nil) significa "nessuna checklist": un gioco senza
	// materiali, o un client vecchio, chiude il prestito come prima. Una
	// lista presente ma con voci mancanti è un'altra cosa — quelle voci
	// risultano non verificate.
	Materials []returnMaterialCheck `json:"materials"`
}

type returnMaterialCheck struct {
	MaterialID int64 `json:"materialId"`
	Complete   bool  `json:"complete"`
	// Returned nil = non verificata. Ha senso solo con Complete false.
	Returned *int `json:"returned"`
}
```

e dentro l'handler, prima della chiamata:

```go
	var checks []events.MaterialCheck
	if req.Materials != nil {
		checks = make([]events.MaterialCheck, 0, len(req.Materials))
		for _, m := range req.Materials {
			checks = append(checks, events.MaterialCheck{
				MaterialID: m.MaterialID, Complete: m.Complete, Returned: m.Returned,
			})
		}
	}

	loan, err := s.Events.ReturnLoan(r.Context(), id, req.Notes, checks)
```

Aggiungi il caso d'errore nello `switch`, prima di `case err != nil`:

```go
	case errors.Is(err, events.ErrMaterialCheckInvalid):
		writeError(w, http.StatusBadRequest, "una quantità restituita non può essere negativa")
```

- [ ] **Step 4: Porta materiali ed esiti nella risposta del banco**

In `backend/internal/httpapi/loans_responses.go`:

1. `toReturnedLoanResponse` prende un secondo parametro e lo aggiunge alla mappa:

```go
// toReturnedLoanResponse è una riga del log della serata, che si legge da
// sola: il gioco ce l'ha dentro perché il log non è raggruppato per copia.
// issues è ciò che non è tornato intero — quasi sempre vuoto, ed è il punto:
// una riga con qualcosa dentro va guardata.
func toReturnedLoanResponse(l events.LoanWithGame, issues []events.MaterialIssue) map[string]any {
	rows := make([]map[string]any, 0, len(issues))
	for _, iss := range issues {
		row := map[string]any{"name": iss.Name, "expected": iss.Expected, "returned": nil}
		if iss.Returned != nil {
			row["returned"] = *iss.Returned
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"id": l.ID, "eventGameId": l.EventGameID, "gameId": l.GameID,
		"gameName": l.GameName, "copyIndex": l.CopyIndex,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "returnedAt": l.ReturnedAt.Format(isoTime),
		"notes": l.Notes, "materialIssues": rows,
	}
}
```

2. In `toLoanDeskResponse`, raccogli prima gli id dei prestiti chiusi, leggi gli esiti in una query sola e poi costruisci le righe:

```go
	closedIDs := []int64{}
	closed := []events.LoanWithGame{}
	for _, l := range loans {
		if l.ReturnedAt == nil {
			openByCopy[l.EventGameID] = l
			continue
		}
		closed = append(closed, l)
		closedIDs = append(closedIDs, l.ID)
	}
	issuesByLoan, err := s.Events.ListMaterialIssues(ctx, closedIDs)
	if err != nil {
		return nil, err
	}
	for _, l := range closed {
		returned = append(returned, toReturnedLoanResponse(l, issuesByLoan[l.ID]))
	}
```

(rimuovi il vecchio `returned = append(...)` dal primo ciclo).

3. Nella costruzione di `row`, aggiungi i materiali del gioco, letti una volta per gioco come già si fa con `gameCache`:

```go
	// I materiali si leggono una volta per gioco, non una per copia: una
	// serata con quattro copie di Carcassonne farebbe quattro query uguali.
	materialsCache := map[int64][]map[string]any{}
```

e dentro il ciclo delle copie, prima di comporre `row`:

```go
		materials, cached := materialsCache[eg.GameID]
		if !cached {
			ms, err := s.Games.ListMaterials(ctx, eg.GameID)
			if err != nil {
				return nil, err
			}
			materials = make([]map[string]any, 0, len(ms))
			for _, m := range ms {
				materials = append(materials, map[string]any{
					"id": m.ID, "name": m.Name, "quantity": m.Quantity,
				})
			}
			materialsCache[eg.GameID] = materials
		}
```

aggiungendo poi `"materials": materials,` alla mappa `row`.

- [ ] **Step 5: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```
Atteso: PASS ovunque.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/loans_handlers.go \
        backend/internal/httpapi/loans_responses.go \
        backend/internal/httpapi/loans_handlers_test.go
git commit -m "feat: carry the material checklist through the loan API"
```

---

### Task 7: Editor dei materiali nella scheda gioco admin

**Files:**
- Create: `frontend/src/components/GameMaterialsPanel.vue`
- Modify: `frontend/src/views/GameAdminDetailView.vue` (import + uso, accanto a `SuggestedQuestionsPanel` alla riga ~664)
- Modify: `frontend/src/app.css` (solo se serve una classe nuova: prima cerca di comporre con `.panel-card`, `.admin-list`, `.btn-secondary`, `.btn-danger`, `.btn-with-icon` esistenti)

**Interfaces:**
- Consumes: `api.get/put/post` da `frontend/src/api/client.ts`; le rotte dei Task 3 e 5.
- Produces: componente `GameMaterialsPanel` con props `{ gameId: number; aiConfigured: boolean }`.

- [ ] **Step 1: Leggi il pattern prima di scrivere**

Apri `frontend/src/components/SuggestedQuestionsPanel.vue` per intero. È il modello da seguire: struttura del `<form>`, testata con l'azione di rigenerazione, gestione di `loading`/`saving`/`error`, il confronto con lo stato salvato invece di un flag, e il bottone spento con la nota legata via `aria-describedby`.

- [ ] **Step 2: Scrivi il componente**

Crea `frontend/src/components/GameMaterialsPanel.vue`. Requisiti, tutti obbligatori:

- Stato: `rows: { name: string; quantity: string }[]`, più `loading`, `saving`, `suggesting`, `error`, `proposal` (booleano: la lista attuale viene da una proposta non ancora salvata).
- `onMounted` → `api.get<{materials: {id:number;name:string;quantity:number}[]}>('/games/' + props.gameId + '/materials')`; le quantità vivono come stringa nel form e si convertono al salvataggio (un `<input type="number">` legato a un numero rende impossibile svuotare il campo mentre si digita).
- Una `<ol>` di righe: `<input>` nome (`maxlength="60"`), `<input inputmode="numeric">` quantità, bottone `↑`, bottone `↓`, bottone rimuovi. Prima riga senza `↑`, ultima senza `↓`; ogni bottone icona ha un `aria-label` esplicito ("Sposta tessere in su", "Rimuovi tessere").
- "Aggiungi voce": aggiunge una riga vuota e le mette il focus (`nextTick` + `ref` sull'ultimo input).
- Un solo submit "Salva materiali" → `api.put('/games/' + props.gameId + '/materials', { materials })`, con `quantity: Number(...)`. Errore dal server → messaggio in `.error` con `role="alert"`.
- Stato vuoto: `<p class="empty-note">Serve alla riconsegna: ogni voce diventa una casella da spuntare quando il gioco torna.</p>`
- "Genera dal manuale" nella testata del pannello, `:disabled="busy || !props.aiConfigured"`, con `aria-describedby` che punta alla nota "Nessun provider AI configurato: controlla le impostazioni." quando è spento. Al click: `api.post('/games/' + props.gameId + '/materials/suggest')` → sostituisce `rows` con la proposta, imposta `proposal = true` e mostra `<p class="empty-note">Proposta dal manuale: correggi quel che serve, poi salva.</p>`. Un errore 422 mostra il messaggio del server così com'è: dice già cosa fare.
- `busy = saving || suggesting`, e ogni bottone si spegne finché una delle due richieste è in volo — due scritture concorrenti sullo stesso stato arriverebbero in ordine imprevedibile.
- Commento in testa al file che spiega perché la lista si salva tutta insieme e perché "Genera" sta nella testata e non accanto a "Salva" (è l'azione che *riempie* il pannello, non l'alternativa al salvataggio).

- [ ] **Step 3: Montalo nella scheda gioco**

In `frontend/src/views/GameAdminDetailView.vue`, importa il componente accanto agli altri e mettilo in una `<section>` dopo `SuggestedQuestionsPanel`:

```vue
          <GameMaterialsPanel :game-id="game.id" :ai-configured="aiConfigured" />
```

- [ ] **Step 4: Build e type-check**

```bash
cd frontend && npm run build
```
Atteso: build pulita, nessun errore di `vue-tsc`.

- [ ] **Step 5: Verifica nel browser**

```bash
docker compose up -d --build
```
Poi, con Claude in Chrome su `http://localhost:8080/admin/games/<id>`: aggiungi tre voci, riordinale, salva, ricarica la pagina e verifica che tornino nell'ordine giusto. Prova un duplicato e controlla che il messaggio d'errore arrivi a schermo. Leggi la console: nessun errore.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/GameMaterialsPanel.vue \
        frontend/src/views/GameAdminDetailView.vue frontend/src/app.css
git commit -m "feat: edit a game's box contents from the catalogue"
```

---

### Task 8: Checklist nella modale di riconsegna

**Files:**
- Modify: `frontend/src/views/LoanDeskView.vue` (tipi ~37-52, stato ~79-82, `startReturning` ~214, `submitReturn` ~220, modale ~419-442, lista dei resi ~345)
- Modify: `frontend/src/app.css` (classi della checklist)

**Interfaces:**
- Consumes: i campi `materials` e `materialIssues` della risposta del banco (Task 6).
- Produces: nessuna, è la superficie finale.

- [ ] **Step 1: Estendi i tipi**

In `frontend/src/views/LoanDeskView.vue`:

```ts
interface Material {
  id: number
  name: string
  quantity: number
}

interface MaterialIssue {
  name: string
  expected: number
  /** null = voce non verificata al momento della riconsegna. */
  returned: number | null
}
```

Aggiungi `materials: Material[]` a `DeskCopy` e `materialIssues: MaterialIssue[]` a `ReturnedLoan`.

- [ ] **Step 2: Aggiungi lo stato della checklist**

```ts
/**
 * Una riga della checklist. `returned` resta una stringa: un input numerico
 * legato a un numero non si può svuotare mentre si digita, e "campo vuoto"
 * è esattamente uno dei tre stati che dobbiamo poter rappresentare.
 */
type MaterialCheckRow = { material: Material; complete: boolean; returned: string }

const materialChecks = ref<MaterialCheckRow[]>([])

/** Verificata = spuntata, oppure con una quantità scritta. */
const verifiedCount = computed(
  () => materialChecks.value.filter((r) => r.complete || r.returned.trim() !== '').length,
)

const shortageCount = computed(
  () =>
    materialChecks.value.filter(
      (r) => !r.complete && r.returned.trim() !== '' && Number(r.returned) < r.material.quantity,
    ).length,
)

const uncheckedCount = computed(
  () => materialChecks.value.filter((r) => !r.complete && r.returned.trim() === '').length,
)
```

- [ ] **Step 3: Riempi e svuota la checklist con la modale**

In `startReturning`, dopo `returnError.value = ''`:

```ts
  // Tutte da spuntare: il senso della checklist è forzare il controllo voce
  // per voce, e partire da "tutto a posto" lo annullerebbe.
  materialChecks.value = copy.materials.map((material) => ({
    material,
    complete: false,
    returned: '',
  }))
```

- [ ] **Step 4: Manda la checklist**

In `submitReturn`, sostituisci il corpo della `api.post`:

```ts
    await api.post(`/loans/${current.loan.id}/return`, {
      notes: returnNotes.value.trim() || null,
      // Campo assente quando il gioco non ha materiali: il backend
      // distingue "nessuna checklist" da "checklist con voci non
      // verificate", e mandare [] direbbe la seconda cosa.
      materials: materialChecks.value.length
        ? materialChecks.value.map((r) => ({
            materialId: r.material.id,
            complete: r.complete,
            returned: r.complete || r.returned.trim() === '' ? null : Number(r.returned),
          }))
        : undefined,
    })
```

- [ ] **Step 5: Scrivi il markup della sezione**

Nella modale di restituzione, fra `.loan-modal-meta` e il campo Note:

```vue
        <fieldset v-if="materialChecks.length" class="material-check">
          <legend>
            Materiali
            <span class="material-check-progress">
              {{ verifiedCount }} di {{ materialChecks.length }} verificate
            </span>
          </legend>
          <ul class="material-check-list">
            <li v-for="row in materialChecks" :key="row.material.id">
              <span class="material-check-name">{{ row.material.name }}</span>
              <span class="material-check-expected">{{ row.material.quantity }}</span>
              <input
                v-model="row.returned"
                class="material-check-input"
                type="text"
                inputmode="numeric"
                :disabled="row.complete"
                :placeholder="row.complete ? '—' : ''"
                :aria-label="`${row.material.name}: quantità tornata`"
              />
              <input
                v-model="row.complete"
                type="checkbox"
                class="material-check-box"
                :aria-label="`${row.material.name}: tutte tornate`"
              />
            </li>
          </ul>
          <p v-if="shortageCount || uncheckedCount" class="empty-note">
            <template v-if="shortageCount">
              {{ shortageCount }} {{ shortageCount === 1 ? 'voce incompleta' : 'voci incomplete' }}
            </template>
            <template v-if="shortageCount && uncheckedCount">, </template>
            <template v-if="uncheckedCount">
              {{ uncheckedCount }} non {{ uncheckedCount === 1 ? 'verificata' : 'verificate' }}
            </template>
          </p>
        </fieldset>
```

Il bottone "Restituito" resta abilitato in ogni caso: la riga qui sopra informa, non blocca.

- [ ] **Step 6: Stila la riga per il pollice**

In `frontend/src/app.css`, accanto alle altre classi del banco prestiti:

```css
/* La riga della checklist si legge e si tocca in piedi, al tavolo: nome
   elastico, quantità attesa in mono perché è un dato, campo stretto quanto
   basta a quattro cifre, casella con l'area di tocco piena. */
.material-check-list li {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.35rem 0;
}

.material-check-name {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

.material-check-expected {
  font-family: var(--font-mono);
  color: var(--ink-muted);
  font-variant-numeric: tabular-nums;
}

.material-check-input {
  width: 4.5rem;
  flex: none;
  font-family: var(--font-mono);
  text-align: right;
}

.material-check-box {
  flex: none;
  width: 1.35rem;
  height: 1.35rem;
  /* L'area di tocco arriva a 44px senza allargare il disegno della
     casella: il margine negativo la fa crescere sotto il dito, non sotto
     l'occhio. */
  margin: -0.6rem;
  padding: 0.6rem;
  box-sizing: content-box;
}
```

Controlla i nomi dei token contro `frontend/src/app.css` prima di usarli (`--font-mono`, `--ink-muted`): se nel progetto si chiamano diversamente, usa quelli veri.

- [ ] **Step 7: Mostra gli esiti nel registro dei resi**

Nella `<li>` della lista `desk.returned`, sotto la riga del nome:

```vue
              <p v-if="row.materialIssues.length" class="row-meta material-issues">
                {{ issuesLabel(row.materialIssues) }}
              </p>
```

con:

```ts
/**
 * "mancano: carte 35/40 · non verificate: dadi" — il problema si legge dalla
 * lista, senza aprire niente. Un prestito pulito non ha esiti e non stampa
 * nessuna riga.
 */
function issuesLabel(issues: MaterialIssue[]) {
  const missing = issues
    .filter((i) => i.returned !== null)
    .map((i) => `${i.name} ${i.returned}/${i.expected}`)
  const unchecked = issues.filter((i) => i.returned === null).map((i) => i.name)
  const parts: string[] = []
  if (missing.length) {
    parts.push(`mancano: ${missing.join(', ')}`)
  }
  if (unchecked.length) {
    parts.push(`non verificate: ${unchecked.join(', ')}`)
  }
  return parts.join(' · ')
}
```

- [ ] **Step 8: Build e type-check**

```bash
cd frontend && npm run build
```
Atteso: build pulita.

- [ ] **Step 9: Verifica nel browser, mobile prima**

```bash
docker compose up -d --build
```
Con Claude in Chrome, a 390px di larghezza, su `/events/<id>/loans` (banco prestiti): consegna una copia di un gioco con materiali, apri la riconsegna e verifica che
(a) le righe partano tutte vuote e non spuntate,
(b) la spunta disabiliti l'input,
(c) il contatore in testata salga,
(d) il riepilogo compaia solo quando serve,
(e) non ci sia scroll orizzontale,
(f) dopo il salvataggio la riga nel registro mostri l'esito.
Ripeti con un gioco senza materiali: la sezione non deve comparire. Leggi la console: nessun errore.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/views/LoanDeskView.vue frontend/src/app.css
git commit -m "feat: tick off every piece when a game comes back"
```

---

### Task 9: Chiusura — documentazione e pass di qualità

**Files:**
- Modify: `README.md` (sezione delle funzionalità visibili)
- Modify: `DESIGN.md` (se la checklist ha introdotto un pattern nuovo)

- [ ] **Step 1: Suite completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
cd frontend && npm run build
```
Atteso: PASS e build pulita. Nessuna affermazione di "funziona" senza questo output.

- [ ] **Step 2: Aggiorna la documentazione**

`README.md`: una riga nella descrizione delle funzionalità — il catalogo tiene il contenuto della scatola, e la riconsegna di un prestito lo fa spuntare voce per voce, registrando cosa manca.

`DESIGN.md`: se la riga della checklist è un pattern nuovo (riga con dato in mono + input stretto + casella con area di tocco allargata), documentala fra i componenti; se invece riusa pattern esistenti, non aggiungere niente.

- [ ] **Step 3: Pass `/impeccable`**

Come da CLAUDE.md, ultimo task obbligatorio per ogni lavoro che tocca la UI:

```
/impeccable polish — il pannello Materiali in GameAdminDetailView e la
sezione Materiali della modale di riconsegna in LoanDeskView, viewport
mobile 390px e desktop.
```

Applica ciò che il pass trova.

- [ ] **Step 4: Commit**

```bash
git add README.md DESIGN.md
git commit -m "docs: describe the box-contents list and the return checklist"
```
