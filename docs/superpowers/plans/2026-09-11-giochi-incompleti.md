# Giochi incompleti e log dei prestiti per gioco — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** marcare come incompleto il gioco a cui una riconsegna ha rilevato pezzi mancanti, finché un admin non chiude la segnalazione, e dare a ogni gioco un log dei suoi prestiti che mette in evidenza quelli tornati incompleti.

**Architecture:** lo stato "incompleto" non si memorizza, si deriva: due colonne su `games` registrano *quando* e *da chi* la scatola è stata dichiarata a posto, e una query dice se dopo quella data c'è stata una mancanza in `loan_material_issue`. Il log per gioco è una lettura nuova su tabelle esistenti. Tre superfici admin mostrano il marchio, una quarta è la pagina del log.

**Tech Stack:** Go 1.25 (chi, `modernc.org/sqlite`), Vue 3 `<script setup>` + TypeScript, Vite. Nessuna dipendenza nuova.

**Spec:** `docs/superpowers/specs/2026-09-11-giochi-incompleti-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto (binario x86_64 su Mac arm64). Sempre così, riusando i due volumi nominati:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  `npm` invece gira in locale, dentro `frontend/`.
- **Migrazioni forward-only.** Nessuna modifica a un `.sql` già rilasciato; il nuovo file è `0018_materials_checked.sql`.
- **Nessuna dipendenza nuova**, Go o npm.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n.
- **Il marchio non esce mai al pubblico.** `GET /api/games` e `GET /api/games/{id}` sono rotte pubbliche: i campi nuovi compaiono **solo** se la richiesta ha una sessione, e senza sessione la risposta resta identica a oggi byte per byte e la query aggregata non viene nemmeno eseguita.
- **Solo le mancanze vere segnalano.** Una riga d'esito con `returned IS NOT NULL` è una mancanza; una con `returned IS NULL` è "non verificata" e non segnala niente.
- **Design system**: token in `frontend/src/app.css`, regole in `DESIGN.md`. Il mono del progetto è `font-family: 'Data', monospace` (non esiste una variabile `--font-mono`). Niente bordo colorato sul lato delle card. Nessun controllo `disabled` senza il motivo a schermo legato con `aria-describedby`.
- **Il banco prestiti e il log si usano da telefono**: nessuno scroll orizzontale a 390px.
- **Commit in inglese**, conventional commits.

---

### Task 1: Migrazione e stato "completo" sul gioco

**Files:**
- Create: `backend/internal/db/migrations/0018_materials_checked.sql`
- Modify: `backend/internal/games/store.go` (struct `Game`, le tre query che la leggono)
- Modify: `backend/internal/games/materials.go` (aggiungi in fondo)
- Test: `backend/internal/games/materials_test.go` (aggiungi in fondo)

**Interfaces:**
- Consumes: `games.Store`, `games.ErrNotFound`, `games.Game`.
- Produces:
  - `games.Game` con due campi nuovi: `MaterialsCheckedAt *time.Time`, `MaterialsCheckedBy *int64`
  - `games.MarkMaterialsChecked(ctx context.Context, gameID, userID int64) (Game, error)`

- [ ] **Step 1: Scrivi la migrazione**

Crea `backend/internal/db/migrations/0018_materials_checked.sql`:

```sql
-- Quando un admin ha dichiarato che la scatola è di nuovo a posto, e chi.
-- NULL significa "nessuno l'ha mai fatto", che è lo stato di partenza
-- giusto per tutto il catalogo esistente: una segnalazione aperta oggi da
-- un prestito di ieri deve comparire senza bisogno di popolare niente.
ALTER TABLE games ADD COLUMN materials_checked_at TEXT;

-- SET NULL e non CASCADE: se l'utente che ha chiuso la segnalazione viene
-- cancellato, resta vero che la segnalazione è stata chiusa.
ALTER TABLE games ADD COLUMN materials_checked_by INTEGER
    REFERENCES users(id) ON DELETE SET NULL;
```

- [ ] **Step 2: Scrivi il test che fallisce**

Aggiungi in fondo a `backend/internal/games/materials_test.go`. Gli helper `newTestStore`, `newTestStoreWithDB` e `mustGameID` esistono già nel pacchetto di test — non ridichiararli.

```go
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
```

Se il file non importa già `errors`, aggiungilo.

- [ ] **Step 3: Lancia il test e verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/games/ -run MaterialsChecked -v
```
Atteso: FAIL in compilazione — `got.MaterialsCheckedAt undefined`, `store.MarkMaterialsChecked undefined`.

- [ ] **Step 4: Aggiungi i campi alla struct e alle query**

In `backend/internal/games/store.go`, dentro `type Game struct`, dopo `Seats`:

```go
	// MaterialsCheckedAt è quando un admin ha dichiarato che la scatola è
	// di nuovo completa; nil se non è mai successo. Non dice che il gioco è
	// a posto adesso — lo stato "incompleto" si deriva confrontando questa
	// data con le mancanze registrate dopo (vedi events.GamesMissingPieces).
	MaterialsCheckedAt *time.Time
	MaterialsCheckedBy *int64
```

Poi aggiorna **tutte** le query che costruiscono un `Game`: `CreateGame`, `GetGame`, `ListGames`, `UpdateGame`, `UpdateCoverPath`. Cerca con:

```bash
grep -n "SELECT id, bgg_id" backend/internal/games/store.go
```

Ogni `SELECT` guadagna `materials_checked_at, materials_checked_by` in coda, e ogni `Scan` due destinazioni. Estrai un helper invece di ripetere la conversione cinque volte:

```go
// scanCheckedColumns traduce le due colonne nullable nei campi della
// struct. Le date le scrive SQLite con datetime('now'), che è UTC — lo
// stesso formato che scanLoan legge in internal/events.
func applyCheckedColumns(g *Game, at sql.NullString, by sql.NullInt64) {
	if at.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", at.String)
		g.MaterialsCheckedAt = &t
	}
	if by.Valid {
		id := by.Int64
		g.MaterialsCheckedBy = &id
	}
}
```

- [ ] **Step 5: Scrivi `MarkMaterialsChecked`**

In fondo a `backend/internal/games/materials.go`:

```go
// MarkMaterialsChecked dichiara che la scatola è di nuovo a posto: da
// questo istante le mancanze registrate prima non segnalano più il gioco.
//
// Non cancella niente: le rilevazioni restano nel registro dei prestiti e
// nel log del gioco. Si archivia la segnalazione, non la storia — ed è il
// motivo per cui questa è una data e non un DELETE.
func (s *Store) MarkMaterialsChecked(ctx context.Context, gameID, userID int64) (Game, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE games SET materials_checked_at = datetime('now'), materials_checked_by = ?
		 WHERE id = ?`, userID, gameID)
	if err != nil {
		return Game{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Game{}, err
	}
	if affected == 0 {
		return Game{}, ErrNotFound
	}
	return s.GetGame(ctx, gameID)
}
```

- [ ] **Step 6: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```
Atteso: PASS ovunque — i test esistenti di `internal/games` e `internal/httpapi` devono passare senza modifiche.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/db/migrations/0018_materials_checked.sql \
        backend/internal/games/store.go backend/internal/games/materials.go \
        backend/internal/games/materials_test.go
git commit -m "feat: let an admin declare a game's box complete again"
```

---

### Task 2: Cosa manca a un gioco, e il suo registro dei prestiti

**Files:**
- Modify: `backend/internal/events/loans.go` (aggiungi in fondo)
- Test: `backend/internal/events/loans_test.go` (aggiungi in fondo)

**Interfaces:**
- Consumes: la colonna `games.materials_checked_at` del Task 1 (letta con SQL diretto: `internal/events` **non** importa `internal/games`); `events.Store`, `events.Loan`, `scanLoan`, `loanColumns`, `ListMaterialIssues`.
- Produces:
  - `events.MissingPiece{Name string; Expected, Returned int; Since time.Time}`
  - `events.GamesMissingPieces(ctx context.Context, gameIDs []int64) (map[int64][]MissingPiece, error)`
  - `events.LoanWithEvent{Loan; EventID int64; EventTitle, EventDate string; CopyIndex, Copies int}`
  - `events.ListLoansForGame(ctx context.Context, gameID int64) ([]LoanWithEvent, error)`

- [ ] **Step 1: Scrivi i test che falliscono**

Aggiungi in fondo a `backend/internal/events/loans_test.go`. Gli helper `newTestStoreWithConn`, `mustLend`, `firstCopy`, `mustCreateGame`, `mustCreateEvent`, `mustMaterials` esistono già.

```go
// returnWithShortage chiude un prestito dichiarando `returned` pezzi su una
// voce: è il modo più corto di produrre una mancanza registrata.
func returnWithShortage(t *testing.T, store *events.Store, loanID, materialID int64, returned int) {
	t.Helper()
	if _, err := store.ReturnLoan(context.Background(), loanID, nil,
		[]events.MaterialCheck{{MaterialID: materialID, Returned: &returned}}); err != nil {
		t.Fatalf("return con mancanza: %v", err)
	}
}

func TestGamesMissingPiecesIsEmptyWithoutShortages(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"tessere", 72})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	// Spuntata: tornata tutta, nessuna segnalazione.
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil,
		[]events.MaterialCheck{{MaterialID: ids[0], Complete: true}}); err != nil {
		t.Fatalf("return: %v", err)
	}

	got, err := store.GamesMissingPieces(context.Background(), []int64{gameID})
	if err != nil {
		t.Fatalf("missing pieces: %v", err)
	}
	if len(got[gameID]) != 0 {
		t.Fatalf("un reso pulito non segnala niente, ho %+v", got[gameID])
	}
}

func TestGamesMissingPiecesIgnoresUnverifiedRows(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	mustMaterials(t, conn, gameID, [2]any{"tessere", 72})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	// Checklist non vuota ma con la voce mai toccata: resta "non
	// verificata", e una cosa che nessuno ha guardato non è una perdita.
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil,
		[]events.MaterialCheck{}); err != nil {
		t.Fatalf("return: %v", err)
	}

	got, err := store.GamesMissingPieces(context.Background(), []int64{gameID})
	if err != nil {
		t.Fatalf("missing pieces: %v", err)
	}
	if len(got[gameID]) != 0 {
		t.Fatalf("una voce non verificata non segnala, ho %+v", got[gameID])
	}
}

func TestGamesMissingPiecesReportsAShortage(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"carte", 40}, [2]any{"dadi", 5})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")
	returnWithShortage(t, store, loan.ID, ids[0], 35)

	got, err := store.GamesMissingPieces(context.Background(), []int64{gameID})
	if err != nil {
		t.Fatalf("missing pieces: %v", err)
	}
	rows := got[gameID]
	if len(rows) != 1 {
		t.Fatalf("volevo una sola voce mancante, ho %+v", rows)
	}
	if rows[0].Name != "carte" || rows[0].Expected != 40 || rows[0].Returned != 35 {
		t.Errorf("voce inattesa: %+v", rows[0])
	}
	if rows[0].Since.IsZero() {
		t.Error("Since deve dire da quando manca")
	}
}

func TestGamesMissingPiecesKeepsTheMostRecentCount(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"carte", 40})
	copy := firstCopy(t, store, event.ID)

	loan1 := mustLend(t, store, event.ID, copy.ID, "Anna")
	returnWithShortage(t, store, loan1.ID, ids[0], 38)
	loan2 := mustLend(t, store, event.ID, copy.ID, "Bruno")
	returnWithShortage(t, store, loan2.ID, ids[0], 35)

	// Le due riconsegne cadono nello stesso secondo di datetime('now'): il
	// secondo prestito si forza indietro nel tempo così l'ordine è certo e
	// il test non dipende dalla velocità della macchina.
	if _, err := conn.Exec(
		`UPDATE game_loans SET returned_at = '2030-01-02 21:00:00' WHERE id = ?`, loan2.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if _, err := conn.Exec(
		`UPDATE game_loans SET returned_at = '2030-01-01 21:00:00' WHERE id = ?`, loan1.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	got, err := store.GamesMissingPieces(context.Background(), []int64{gameID})
	if err != nil {
		t.Fatalf("missing pieces: %v", err)
	}
	rows := got[gameID]
	if len(rows) != 1 {
		t.Fatalf("la stessa voce non deve comparire due volte: %+v", rows)
	}
	if rows[0].Returned != 35 {
		t.Errorf("returned = %d, volevo l'ultimo conteggio (35)", rows[0].Returned)
	}
}

func TestGamesMissingPiecesRespectsTheCheckedDate(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	ids := mustMaterials(t, conn, gameID, [2]any{"carte", 40})
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")
	returnWithShortage(t, store, loan.ID, ids[0], 35)

	// Dichiarata a posto DOPO la mancanza: non segnala più.
	if _, err := conn.Exec(
		`UPDATE games SET materials_checked_at = datetime('now', '+1 day') WHERE id = ?`, gameID); err != nil {
		t.Fatalf("check: %v", err)
	}
	got, _ := store.GamesMissingPieces(context.Background(), []int64{gameID})
	if len(got[gameID]) != 0 {
		t.Fatalf("dopo la risoluzione non deve segnalare: %+v", got[gameID])
	}

	// Dichiarata a posto PRIMA della mancanza: torna a segnalare.
	if _, err := conn.Exec(
		`UPDATE games SET materials_checked_at = '2000-01-01 00:00:00' WHERE id = ?`, gameID); err != nil {
		t.Fatalf("check: %v", err)
	}
	got, _ = store.GamesMissingPieces(context.Background(), []int64{gameID})
	if len(got[gameID]) != 1 {
		t.Fatalf("una mancanza successiva alla risoluzione deve segnalare: %+v", got[gameID])
	}
}

func TestGamesMissingPiecesWithNoIDs(t *testing.T) {
	store, _, _ := newTestStoreWithConn(t)
	got, err := store.GamesMissingPieces(context.Background(), nil)
	if err != nil {
		t.Fatalf("missing pieces: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("volevo una mappa vuota, ho %+v", got)
	}
}

func TestListLoansForGameSpansEvents(t *testing.T) {
	store, gameStore, conn := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	ids := mustMaterials(t, conn, gameID, [2]any{"carte", 40})
	ev1 := mustCreateEvent(t, store, "Prima serata", "2030-01-01", "21:00", gameID)
	ev2 := mustCreateEvent(t, store, "Seconda serata", "2030-02-01", "21:00", gameID)

	c1 := firstCopy(t, store, ev1.ID)
	l1 := mustLend(t, store, ev1.ID, c1.ID, "Anna")
	returnWithShortage(t, store, l1.ID, ids[0], 35)

	c2 := firstCopy(t, store, ev2.ID)
	mustLend(t, store, ev2.ID, c2.ID, "Bruno") // ancora fuori

	got, err := store.ListLoansForGame(context.Background(), gameID)
	if err != nil {
		t.Fatalf("list loans for game: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("volevo 2 prestiti, ho %d", len(got))
	}
	// Il più recente per primo.
	if got[0].BorrowerName != "Bruno" || got[0].ReturnedAt != nil {
		t.Errorf("prima riga inattesa: %+v", got[0])
	}
	if got[0].EventTitle != "Seconda serata" || got[0].EventID != ev2.ID {
		t.Errorf("la serata non è risolta: %+v", got[0])
	}
	if got[1].BorrowerName != "Anna" || got[1].ReturnedAt == nil {
		t.Errorf("seconda riga inattesa: %+v", got[1])
	}
	if got[1].Copies != 1 || got[1].CopyIndex != 1 {
		t.Errorf("copie/indice inattesi: %+v", got[1])
	}
}

func TestListLoansForGameIsEmptyForAnUnlentGame(t *testing.T) {
	store, gameStore, _ := newTestStoreWithConn(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	got, err := store.ListLoansForGame(context.Background(), gameID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("volevo zero prestiti, ho %+v", got)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ -run "MissingPieces|LoansForGame" -v
```
Atteso: FAIL in compilazione — `undefined: store.GamesMissingPieces`, `undefined: store.ListLoansForGame`.

- [ ] **Step 3: Scrivi le due letture**

In fondo a `backend/internal/events/loans.go`:

```go
// MissingPiece è una voce che a un gioco risulta mancante: quante se ne
// aspettano e quante ne sono tornate l'ultima volta che qualcuno le ha
// contate davvero.
type MissingPiece struct {
	Name     string
	Expected int
	Returned int
	// Since è il returned_at del prestito che l'ha rilevata: serve a dire
	// "dalla serata del 7 settembre" invece di un generico "manca".
	Since time.Time
}

// GamesMissingPieces torna, per un gruppo di giochi, le voci che risultano
// mancanti e non ancora risolte. Un gioco assente dalla mappa è completo.
//
// Lo stato "incompleto" non è memorizzato da nessuna parte: è questa query.
// Un flag su games sarebbe una seconda verità accanto a
// loan_material_issue, e le due possono divergere — un flag acceso dopo che
// la riga d'esito è sparita con la cancellazione di un prestito, o spento
// perché un percorso di scrittura si è dimenticato di alzarlo.
//
// Una query sola con una IN e non una per gioco: la chiama l'elenco del
// catalogo con tutti i giochi in archivio.
func (s *Store) GamesMissingPieces(ctx context.Context, gameIDs []int64) (map[int64][]MissingPiece, error) {
	out := map[int64][]MissingPiece{}
	if len(gameIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(gameIDs))
	for i, id := range gameIDs {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(gameIDs)), ",")

	// MAX(l.returned_at) con le altre colonne nude è la forma idiomatica di
	// SQLite per "la riga del massimo" (sqlite.org/lang_select.html#bareagg):
	// con un solo aggregato MIN/MAX le colonne nude vengono dalla riga
	// scelta. Serve perché la stessa voce può essere risultata corta in tre
	// serate e la scheda deve dire l'ultimo conteggio, non tre righe.
	//
	// `lmi.returned IS NOT NULL` è la regola centrale della funzione: una
	// voce contata e mancante segnala, una "non verificata" no.
	rows, err := s.db.QueryContext(ctx,
		`SELECT eg.game_id, lmi.name, lmi.expected, lmi.returned, MAX(l.returned_at)
		 FROM loan_material_issue lmi
		 JOIN game_loans l   ON l.id = lmi.loan_id
		 JOIN event_games eg ON eg.id = l.event_game_id
		 JOIN games g        ON g.id = eg.game_id
		 WHERE eg.game_id IN (`+placeholders+`)
		   AND lmi.returned IS NOT NULL
		   AND l.returned_at IS NOT NULL
		   AND (g.materials_checked_at IS NULL OR l.returned_at > g.materials_checked_at)
		 GROUP BY eg.game_id, lmi.name
		 ORDER BY eg.game_id, lmi.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var gameID int64
		var p MissingPiece
		var since string
		if err := rows.Scan(&gameID, &p.Name, &p.Expected, &p.Returned, &since); err != nil {
			return nil, err
		}
		p.Since, _ = time.Parse("2006-01-02 15:04:05", since)
		out[gameID] = append(out[gameID], p)
	}
	return out, rows.Err()
}

// LoanWithEvent è un prestito come lo mostra il log di un gioco: la copia e
// la serata risolte, perché quel log attraversa tutte le serate e una riga
// deve leggersi da sola.
type LoanWithEvent struct {
	Loan
	EventID    int64
	EventTitle string
	EventDate  string
	CopyIndex  int
	// Copies è quante copie di questo gioco aveva quella serata: con una
	// sola, "#1" è rumore e la UI lo nasconde.
	Copies int
}

// ListLoansForGame è tutto il registro di un gioco, aperti e chiusi
// insieme, dal più recente. Chi chiama separa i due gruppi guardando
// ReturnedAt, come già fa il banco prestiti: una query invece di due, e
// nessun rischio che le due risposte arrivino da istanti diversi.
func (s *Store) ListLoansForGame(ctx context.Context, gameID int64) ([]LoanWithEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+loanColumns+`, e.id, e.title, e.event_date, eg.copy_index,
		        (SELECT COUNT(*) FROM event_games x
		          WHERE x.event_id = eg.event_id AND x.game_id = eg.game_id)
		 FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 JOIN events e       ON e.id = eg.event_id
		 WHERE eg.game_id = ?
		 ORDER BY l.lent_at DESC, l.id DESC`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []LoanWithEvent{}
	for rows.Next() {
		var le LoanWithEvent
		loan, err := scanLoan(rows, &le.EventID, &le.EventTitle, &le.EventDate,
			&le.CopyIndex, &le.Copies)
		if err != nil {
			return nil, err
		}
		le.Loan = loan
		out = append(out, le)
	}
	return out, rows.Err()
}
```

Verifica il nome vero delle colonne di `events` (`title`, `event_date`) in `backend/internal/db/migrations/0004_events.sql` prima di dare per buona la query.

- [ ] **Step 4: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```
Atteso: PASS ovunque.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/loans.go backend/internal/events/loans_test.go
git commit -m "feat: tell which pieces a game is missing, and where it has been"
```

---

### Task 3: Le rotte — log del gioco, risoluzione, marchio nelle risposte

**Files:**
- Create: `backend/internal/httpapi/game_loans_handlers.go`
- Modify: `backend/internal/httpapi/games_responses.go` (`toGameSummary`, `toGameDetail`)
- Modify: `backend/internal/httpapi/games_read_handlers.go` (`listGamesHandler`, `getGameHandler`)
- Modify: `backend/internal/httpapi/loans_responses.go` (riga di copia del banco)
- Modify: `backend/internal/httpapi/router.go` (blocco `protected`)
- Test: `backend/internal/httpapi/game_loans_handlers_test.go`

**Interfaces:**
- Consumes: `events.GamesMissingPieces`, `events.ListLoansForGame`, `events.MissingPiece`, `events.LoanWithEvent`, `events.ListMaterialIssues` (Task 2); `games.MarkMaterialsChecked` (Task 1); `currentUser(r) (users.User, bool)` da `middleware_auth.go:41`; `parseIDParam`, `writeJSON`, `writeError`.
- Produces:
  - `GET /api/games/{id}/loans` → `{"loans": [...]}` (admin)
  - `POST /api/games/{id}/materials/resolve` → la scheda gioco aggiornata (admin)
  - `"incomplete": bool` in `toGameSummary`, solo con sessione
  - `"missingPieces": [{name, expected, returned, since}]` in `toGameDetail`, solo con sessione
  - `"incomplete": bool` su ogni riga di `copies` in `GET /api/events/{id}/loans`

- [ ] **Step 1: Scrivi i test che falliscono**

Crea `backend/internal/httpapi/game_loans_handlers_test.go`. Nel pacchetto di test esistono già `newTestServer`, `newTestServerWithDB`, `httpapi.NewRouter`, `bootstrapFirstAdmin(t, router, email, password)`, `createTestGameForEvent(t, server.Games, name)`, `doLoanRequest`, `lend` — **usa quelli**, non inventarne.

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

// incompleteFixture monta un gioco con una voce di materiali, una serata,
// una copia consegnata e restituita con una mancanza: il minimo perché il
// gioco risulti incompleto.
func incompleteFixture(t *testing.T) (http.Handler, *http.Cookie, int64) {
	t.Helper()
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	saved, err := server.Games.ReplaceMaterials(context.Background(), gameID,
		[]games.MaterialInput{{Name: "carte", Quantity: 40}})
	if err != nil {
		t.Fatalf("materials: %v", err)
	}
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	copies, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("copies: %v", err)
	}
	loan := lend(t, router, cookie, event.ID, copies[0].ID, "Anna", "3331234567", "")
	body := fmt.Sprintf(`{"materials":[{"materialId":%d,"complete":false,"returned":35}]}`, saved[0].ID)
	if rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body); rec.Code != http.StatusOK {
		t.Fatalf("return: %d %s", rec.Code, rec.Body.String())
	}
	return router, cookie, gameID
}

func TestGamesListMarksIncompleteOnlyForAdmins(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)

	// Con sessione: il marchio c'è ed è vero.
	rec := doLoanRequest(router, http.MethodGet, "/api/games", cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/games: %d %s", rec.Code, rec.Body.String())
	}
	var withSession []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &withSession); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, g := range withSession {
		if int64(g["id"].(float64)) == gameID {
			found = true
			if g["incomplete"] != true {
				t.Errorf("incomplete = %v, volevo true", g["incomplete"])
			}
		}
	}
	if !found {
		t.Fatal("il gioco non è nell'elenco")
	}

	// Senza sessione: il campo non deve esistere affatto. Il catalogo
	// pubblico non racconta a nessuno che a una scatola mancano pezzi.
	rec = doLoanRequest(router, http.MethodGet, "/api/games", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET pubblica: %d", rec.Code)
	}
	var anonymous []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &anonymous); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, g := range anonymous {
		if _, ok := g["incomplete"]; ok {
			t.Fatalf("la risposta pubblica non deve portare incomplete: %+v", g)
		}
	}
}

func TestGameDetailCarriesMissingPiecesForAdmins(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)
	path := fmt.Sprintf("/api/games/%d", gameID)

	rec := doLoanRequest(router, http.MethodGet, path, cookie, "")
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pieces, ok := detail["missingPieces"].([]any)
	if !ok || len(pieces) != 1 {
		t.Fatalf("missingPieces inatteso: %+v", detail["missingPieces"])
	}
	first := pieces[0].(map[string]any)
	if first["name"] != "carte" || first["expected"].(float64) != 40 || first["returned"].(float64) != 35 {
		t.Errorf("voce inattesa: %+v", first)
	}

	rec = doLoanRequest(router, http.MethodGet, path, nil, "")
	var public map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &public); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := public["missingPieces"]; ok {
		t.Fatalf("la scheda pubblica non deve portare missingPieces: %+v", public)
	}
}

func TestResolveClearsTheMark(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)
	rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/games/%d/materials/resolve", gameID), cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}

	rec = doLoanRequest(router, http.MethodGet, fmt.Sprintf("/api/games/%d", gameID), cookie, "")
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pieces, _ := detail["missingPieces"].([]any); len(pieces) != 0 {
		t.Fatalf("dopo la risoluzione non deve mancare niente: %+v", pieces)
	}
}

func TestResolveNeedsASessionAndAnExistingGame(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)

	if rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/games/%d/materials/resolve", gameID), nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("senza cookie: volevo 401, ho %d", rec.Code)
	}
	if rec := doLoanRequest(router, http.MethodPost,
		"/api/games/9999/materials/resolve", cookie, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("gioco inesistente: volevo 404, ho %d", rec.Code)
	}
}

func TestGameLoanLogShowsWhatWasMissing(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)
	path := fmt.Sprintf("/api/games/%d/loans", gameID)

	if rec := doLoanRequest(router, http.MethodGet, path, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("senza cookie: volevo 401, ho %d", rec.Code)
	}

	rec := doLoanRequest(router, http.MethodGet, path, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("log: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Loans []struct {
			EventTitle     string `json:"eventTitle"`
			EventDate      string `json:"eventDate"`
			BorrowerName   string `json:"borrowerName"`
			ReturnedAt     string `json:"returnedAt"`
			MaterialIssues []struct {
				Name     string `json:"name"`
				Expected int    `json:"expected"`
				Returned *int   `json:"returned"`
			} `json:"materialIssues"`
		} `json:"loans"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Loans) != 1 {
		t.Fatalf("volevo un prestito, ho %d", len(body.Loans))
	}
	row := body.Loans[0]
	if row.EventTitle != "Serata" || row.BorrowerName != "Anna" {
		t.Errorf("riga inattesa: %+v", row)
	}
	if len(row.MaterialIssues) != 1 || row.MaterialIssues[0].Name != "carte" ||
		row.MaterialIssues[0].Returned == nil || *row.MaterialIssues[0].Returned != 35 {
		t.Errorf("il log deve dire cosa mancava: %+v", row.MaterialIssues)
	}
}

func TestGameLoanLogOnAMissingGameIs404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	rec := doLoanRequest(router, http.MethodGet, "/api/games/9999/loans", cookie, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("volevo 404, ho %d: %s", rec.Code, rec.Body.String())
	}
}
```

Gli import di questo file sono esattamente `context`, `encoding/json`, `fmt`, `net/http`, `testing`, più `boardgames-manager/internal/events`, `.../internal/games` e `.../internal/httpapi`: non serve `net/http/httptest`, perché `doLoanRequest` costruisce già la richiesta.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run "Incomplete|MissingPieces|Resolve|GameLoanLog" -v
```
Atteso: FAIL — 404 sulle rotte nuove e campi assenti nelle risposte.

- [ ] **Step 3: Porta il marchio nelle risposte dei giochi**

In `backend/internal/httpapi/games_responses.go`, `toGameSummary` prende un parametro in più. Cambia la firma e i due chiamanti:

```go
// toGameSummary: incomplete è un *bool e non un bool perché la sua assenza
// è significativa. Le rotte dei giochi sono pubbliche, e a un visitatore
// non si racconta che a una scatola mancano pezzi: senza sessione il campo
// non esce affatto, e la risposta resta identica a quella di prima.
func toGameSummary(g games.Game, incomplete *bool) map[string]any {
	out := map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "weight": g.Weight,
		"owner": g.Owner, "coverPath": g.CoverPath, "seats": g.Seats,
		"canTranslate": g.BGGDescription != nil && *g.BGGDescription != "",
	}
	if incomplete != nil {
		out["incomplete"] = *incomplete
	}
	return out
}

// toMissingPiecesResponse è la forma che l'avviso sulla scheda legge.
// `since` esce in RFC3339 come ogni altra data dell'API.
func toMissingPiecesResponse(pieces []events.MissingPiece) []map[string]any {
	out := make([]map[string]any, 0, len(pieces))
	for _, p := range pieces {
		out = append(out, map[string]any{
			"name": p.Name, "expected": p.Expected, "returned": p.Returned,
			"since": p.Since.Format(isoTime),
		})
	}
	return out
}
```

`isoTime` è già definito in `loans_responses.go`, stesso pacchetto.

In `backend/internal/httpapi/games_read_handlers.go`, `listGamesHandler`:

```go
func (s *Server) listGamesHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Games.ListGames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list games")
		return
	}

	// La join sulle mancanze si paga solo quando serve: un visitatore non
	// vedrà mai il campo, quindi non deve nemmeno costarlo.
	var missing map[int64][]events.MissingPiece
	if _, signedIn := currentUser(r); signedIn {
		ids := make([]int64, 0, len(list))
		for _, g := range list {
			ids = append(ids, g.ID)
		}
		missing, err = s.Events.GamesMissingPieces(r.Context(), ids)
		if err != nil {
			log.Printf("games: missing pieces: %v", err)
			writeError(w, http.StatusInternalServerError, "could not list games")
			return
		}
	}

	out := make([]map[string]any, 0, len(list))
	for _, g := range list {
		var incomplete *bool
		if missing != nil {
			v := len(missing[g.ID]) > 0
			incomplete = &v
		}
		out = append(out, toGameSummary(g, incomplete))
	}
	writeJSON(w, http.StatusOK, out)
}
```

In `toGameDetail` (stesso file di `toGameSummary`), con la stessa condizione, aggiungi al risultato `"missingPieces": toMissingPiecesResponse(pieces)` quando c'è una sessione; senza sessione non aggiungere la chiave. `toGameDetail` prende già un `ctx` e ha accesso a `s`, quindi può leggere `s.Events.GamesMissingPieces(ctx, []int64{g.ID})` da sé — ma **serve la `*http.Request`** per sapere se c'è una sessione: passa un `signedIn bool` come parametro e fallo decidere al chiamante, invece di infilare la richiesta dentro una funzione di formattazione.

Aggiorna tutti i chiamanti di `toGameSummary` e `toGameDetail` (`grep -rn "toGameSummary\|toGameDetail" backend/internal/httpapi/`).

- [ ] **Step 4: Scrivi gli handler nuovi**

Crea `backend/internal/httpapi/game_loans_handlers.go`:

```go
package httpapi

import (
	"log"
	"net/http"

	"boardgames-manager/internal/events"
)

// toGameLoanResponse è una riga del log di un gioco. materialIssues ha la
// stessa forma che ha già nel banco prestiti: chi ha scritto un pezzo di UI
// per una delle due schermate riconosce l'altra.
func toGameLoanResponse(l events.LoanWithEvent, issues []events.MaterialIssue) map[string]any {
	rows := make([]map[string]any, 0, len(issues))
	for _, iss := range issues {
		row := map[string]any{"name": iss.Name, "expected": iss.Expected, "returned": nil}
		if iss.Returned != nil {
			row["returned"] = *iss.Returned
		}
		rows = append(rows, row)
	}
	out := map[string]any{
		"id": l.ID, "eventId": l.EventID, "eventTitle": l.EventTitle,
		"eventDate": l.EventDate, "copyIndex": l.CopyIndex, "copies": l.Copies,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "notes": l.Notes,
		"returnedAt": nil, "materialIssues": rows,
	}
	if l.ReturnedAt != nil {
		out["returnedAt"] = l.ReturnedAt.Format(isoTime)
	}
	return out
}

func (s *Server) listGameLoansHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	loans, err := s.Events.ListLoansForGame(r.Context(), gameID)
	if err != nil {
		log.Printf("game loans: list for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not list the loans")
		return
	}

	// Gli esiti di tutti i prestiti in una chiamata sola: il log di un
	// gioco molto prestato non deve diventare una N+1.
	ids := make([]int64, 0, len(loans))
	for _, l := range loans {
		if l.ReturnedAt != nil {
			ids = append(ids, l.ID)
		}
	}
	issues, err := s.Events.ListMaterialIssues(r.Context(), ids)
	if err != nil {
		log.Printf("game loans: issues for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not list the loans")
		return
	}

	out := make([]map[string]any, 0, len(loans))
	for _, l := range loans {
		out = append(out, toGameLoanResponse(l, issues[l.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"loans": out})
}

func (s *Server) resolveMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	user, ok := currentUser(r)
	if !ok {
		// Irraggiungibile: la rotta è nel blocco protetto. Il controllo c'è
		// perché la colonna registra CHI ha chiuso la segnalazione, e uno
		// zero lì dentro sarebbe una bugia silenziosa.
		writeError(w, http.StatusUnauthorized, "sessione richiesta")
		return
	}
	game, err := s.Games.MarkMaterialsChecked(r.Context(), gameID, user.ID)
	if err != nil {
		log.Printf("game loans: resolve for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not resolve")
		return
	}
	langs, err := s.Games.ListLanguages(r.Context(), gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	detail, err := s.toGameDetail(r.Context(), game, langs, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
```

`requireGame` esiste già in `materials_handlers.go`, stesso pacchetto. L'ultimo parametro di `toGameDetail` è il `signedIn` aggiunto allo Step 3 — adegua la chiamata alla firma vera.

- [ ] **Step 5: Registra le rotte**

In `backend/internal/httpapi/router.go`, nel blocco `protected`, sotto le rotte dei materiali:

```go
		protected.Get("/api/games/{id}/loans", s.listGameLoansHandler)
		protected.Post("/api/games/{id}/materials/resolve", s.resolveMaterialsHandler)
```

- [ ] **Step 6: Porta il marchio anche sulla riga di copia del banco**

In `backend/internal/httpapi/loans_responses.go`, dentro `toLoanDeskResponse`, prima del ciclo delle copie:

```go
	// Il banco lo mostra prima della consegna: chi dà in mano la scatola
	// deve sapere che è già incompleta, e non prendersi la colpa al rientro.
	gameIDs := make([]int64, 0, len(eventGames))
	for _, eg := range eventGames {
		gameIDs = append(gameIDs, eg.GameID)
	}
	missing, err := s.Events.GamesMissingPieces(ctx, gameIDs)
	if err != nil {
		return nil, err
	}
```

e dentro la mappa `row`, accanto a `"materials"`: `"incomplete": len(missing[eg.GameID]) > 0,`.

- [ ] **Step 7: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```
Atteso: PASS ovunque, compresi i test preesistenti delle rotte dei giochi e del banco.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/httpapi/game_loans_handlers.go \
        backend/internal/httpapi/game_loans_handlers_test.go \
        backend/internal/httpapi/games_responses.go \
        backend/internal/httpapi/games_read_handlers.go \
        backend/internal/httpapi/loans_responses.go \
        backend/internal/httpapi/router.go
git commit -m "feat: expose a game's loan log and its missing pieces"
```

---

### Task 4: Il log dei prestiti del gioco — pagina nuova

**Files:**
- Create: `frontend/src/views/GameLoansView.vue`
- Modify: `frontend/src/router/index.ts` (una rotta protetta)
- Modify: `frontend/src/views/GameAdminDetailView.vue:359` (link "Prestiti" accanto a "Classifica")
- Modify: `frontend/src/app.css` (solo ciò che non esiste già)

**Interfaces:**
- Consumes: `GET /api/games/{id}/loans` (Task 3) e `GET /api/games/{id}` per il nome del gioco nella testata.
- Produces: rotta `admin-game-loans` su `/admin/games/:id/prestiti`.

- [ ] **Step 1: Leggi il modello prima di scrivere**

Apri `frontend/src/views/GameLeaderboardView.vue` per intero: è la sottopagina-di-un-gioco già esistente e la tua deve esserne la sorella — stessa struttura di caricamento, stessa testata col nome del gioco e il link di ritorno, stesso trattamento dello stato vuoto. Guarda anche la lista "Restituiti" in `LoanDeskView.vue`: la riga di un prestito chiuso e la frase `mancano: carte 35/40` esistono già lì, e devono leggersi uguali nelle due schermate.

- [ ] **Step 2: Scrivi la vista**

Crea `frontend/src/views/GameLoansView.vue`. Requisiti, tutti obbligatori:

- Tipi: `interface MaterialIssue { name: string; expected: number; returned: number | null }` e `interface GameLoan { id: number; eventId: number; eventTitle: string; eventDate: string; copyIndex: number; copies: number; borrowerName: string; borrowerPhone: string; lentAt: string; returnedAt: string | null; notes: string | null; materialIssues: MaterialIssue[] }`.
- `onMounted` carica in parallelo il gioco e i suoi prestiti; errore → `.error` con `role="alert"`.
- Testata: nome del gioco, titolo "Prestiti", link "← Gioco" verso `admin-game-detail`.
- Una `<ul>` **spogliata esplicitamente** (`list-style: none`, padding e bordi sul `li`, non sulla `ul`): è un *Don't* scritto in `DESIGN.md` e il progetto l'ha già inciampato due volte.
- Ogni riga: `{{ eventTitle }} · {{ dataFormattata }}`, il nome di chi ha preso, gli orari in mono (`font-family: 'Data', monospace`), e `#{{ copyIndex }}` **solo** se `copies > 1`.
- Un prestito ancora aperto mostra `fuori dalle HH:MM` al posto di `HH:MM → HH:MM`.
- Una riga con `materialIssues.length` porta fondo `--danger-bg` e, sotto, la stessa frase del banco: `mancano: carte 35/40 · non verificate: dadi`. Riusa la logica di `issuesLabel` che esiste in `LoanDeskView.vue` — **spostala in `frontend/src/utils/` e importala nelle due viste** invece di copiarla: due copie di quella frase divergeranno.
- Stato vuoto: `<p class="empty-note">Questo gioco non è mai stato dato in prestito.</p>`
- Mobile-first: a 390px nessuno scroll orizzontale; il nome di chi ha preso può andare a capo.

- [ ] **Step 3: Registra la rotta**

In `frontend/src/router/index.ts`, accanto alle altre rotte admin dei giochi (riga ~73), **senza** `meta: { public: true }`:

```ts
    { path: '/admin/games/:id/prestiti', name: 'admin-game-loans', component: GameLoansView },
```

più il suo `import` in cima al file.

- [ ] **Step 4: Aggiungi il link permanente**

In `frontend/src/views/GameAdminDetailView.vue`, accanto al link "Classifica" della riga 359:

```vue
            ·
            <router-link :to="{ name: 'admin-game-loans', params: { id: game.id } }">
              Prestiti
            </router-link>
```

Presente sempre, che il gioco sia segnalato o no: è la richiesta esplicita dell'utente.

- [ ] **Step 5: Build e type-check**

```bash
cd frontend && npm run build
```
Atteso: build pulita, nessun errore di `vue-tsc`.

- [ ] **Step 6: Verifica nel browser**

```bash
docker compose up -d --build
```
Apri `http://localhost:8080/admin/games/4/prestiti` (Happy Salmon ha materiali e prestiti chiusi con mancanze nei dati di sviluppo). Controlla desktop e 390px: righe in ordine dal più recente, la riga con mancanze distinguibile a colpo d'occhio, il prestito aperto senza orario di rientro, nessuno scroll orizzontale, console pulita. Se non hai una sessione admin, dillo nel report invece di indovinare.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/views/GameLoansView.vue frontend/src/router/index.ts \
        frontend/src/views/GameAdminDetailView.vue frontend/src/utils/ frontend/src/app.css
git commit -m "feat: show every loan a game has been through"
```

---

### Task 5: Il marchio nelle tre superfici

**Files:**
- Modify: `frontend/src/views/GamesView.vue` (pastiglia sulla card)
- Modify: `frontend/src/views/GameAdminDetailView.vue` (avviso in cima + bottone di risoluzione)
- Modify: `frontend/src/views/LoanDeskView.vue` (pastiglia sulla riga di copia)
- Modify: `frontend/src/app.css` (`.state-chip`, l'avviso)
- Modify: `DESIGN.md` (documenta la pastiglia)

**Interfaces:**
- Consumes: `incomplete` in `GET /api/games` e nelle righe di `copies`; `missingPieces` in `GET /api/games/{id}`; `POST /api/games/{id}/materials/resolve` (Task 3); la rotta `admin-game-loans` (Task 4).
- Produces: nessuna, è la superficie finale.

- [ ] **Step 1: La pastiglia nel catalogo**

In `frontend/src/views/GamesView.vue`, aggiungi `incomplete?: boolean` al tipo del gioco e, dentro il `<router-link>` della card, sopra l'immagine:

```vue
            <span v-if="g.incomplete" class="state-chip is-danger">Incompleto</span>
```

In `frontend/src/app.css`, accanto a `.lang-chip` (riga ~1727):

```css
/* Pastiglia di stato: la stessa sagoma di .lang-chip, ma dice una
   condizione del gioco invece di un'etichetta neutra. Sta sopra la
   copertina e non su un bordo laterale colorato, che il sistema rifiuta
   sulle card. */
.state-chip {
  position: absolute;
  top: 0.5rem;
  left: 0.5rem;
  z-index: 1;
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.state-chip.is-danger {
  background: var(--danger-bg);
  color: var(--danger);
  border: 1px solid var(--danger);
}
```

La `<li>` della griglia (o il `<router-link>` che la riempie) ha bisogno di `position: relative` perché la pastiglia si posizioni sulla copertina: verifica se ce l'ha già prima di aggiungerlo.

- [ ] **Step 2: L'avviso sulla scheda gioco**

In `frontend/src/views/GameAdminDetailView.vue`, sopra le card (dopo la riga dei link "Classifica · Prestiti · Modifica"):

```vue
      <div v-if="game.missingPieces?.length" class="missing-notice" role="status">
        <p class="missing-notice-head">A questa scatola manca qualcosa</p>
        <p class="missing-notice-list">{{ missingLabel }}</p>
        <div class="missing-notice-actions">
          <router-link :to="{ name: 'admin-game-loans', params: { id: game.id } }">
            Vedi i prestiti
          </router-link>
          <button type="button" class="btn-secondary" :disabled="resolving" @click="resolveMaterials">
            {{ resolving ? 'Registrazione…' : 'Segna come completo' }}
          </button>
        </div>
      </div>
```

con, nello script:

```ts
/**
 * "carte 35 di 40 · segnalini pesce 4 di 6 — dalla serata del 7 settembre".
 *
 * I nomi vengono copiati sulla riga d'esito al momento della riconsegna,
 * quindi l'avviso può nominare una voce che nel catalogo non c'è più: è
 * voluto, è cosa mancava quel giorno.
 */
const missingLabel = computed(() => {
  const pieces = game.value?.missingPieces ?? []
  if (!pieces.length) {
    return ''
  }
  const parts = pieces.map((p) => `${p.name} ${p.returned} di ${p.expected}`)
  const since = new Date(pieces[0].since).toLocaleDateString('it-IT', {
    day: 'numeric',
    month: 'long',
  })
  return `${parts.join(' · ')} — dal ${since}`
})

async function resolveMaterials() {
  resolving.value = true
  try {
    game.value = await api.post(`/games/${gameId}/materials/resolve`)
    resolvedMessage.value = 'Segnalazione chiusa: la scatola risulta di nuovo completa.'
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    resolving.value = false
  }
}
```

Dopo il clic l'avviso sparisce da sé (la risposta non ha più `missingPieces`) e resta `resolvedMessage` in una `.success`. Nessuna conferma modale: il bottone sta dentro l'avviso che elenca ciò che archivia, come già deciso per "Rigenera" nelle domande suggerite.

Stile dell'avviso in `app.css`: fondo `--danger-bg`, bordo `--danger`, raggio `var(--radius)`, stesso padding di una `.panel-card`. Il titolo non è un heading — è un'etichetta, non una sezione della pagina.

- [ ] **Step 3: La pastiglia al banco prestiti**

In `frontend/src/views/LoanDeskView.vue`, aggiungi `incomplete: boolean` a `DeskCopy` e, accanto al nome del gioco nella riga della copia:

```vue
              <span v-if="copy.incomplete" class="state-chip is-danger is-inline">Incompleto</span>
```

con una variante che non è posizionata in assoluto:

```css
/* Dentro una riga di testo la pastiglia scorre nel flusso, non si
   sovrappone a niente. */
.state-chip.is-inline {
  position: static;
  display: inline-block;
  margin-left: 0.4rem;
}
```

- [ ] **Step 4: Documenta il pattern**

In `DESIGN.md`, nella sezione dei componenti accanto alla card, aggiungi la pastiglia di stato: cos'è, quando si usa (una condizione del gioco, non un'etichetta), le sue due collocazioni (sovrapposta alla copertina, o in linea in una riga), e perché non è un bordo colorato sul lato. Breve e specifico.

- [ ] **Step 5: Build e verifica nel browser**

```bash
cd frontend && npm run build && cd .. && docker compose up -d --build
```

Con un gioco incompleto nei dati di sviluppo: la pastiglia si vede in `/admin/games`, l'avviso in `/admin/games/:id` con i nomi e le quantità giuste, la pastiglia sulla riga di copia al banco prestiti, e "Segna come completo" fa sparire tutte e tre. Controlla anche 390px e la console.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/GamesView.vue frontend/src/views/GameAdminDetailView.vue \
        frontend/src/views/LoanDeskView.vue frontend/src/app.css DESIGN.md
git commit -m "feat: mark a game whose box is missing pieces"
```

---

### Task 6: Chiusura — documentazione e pass di qualità

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Suite completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
cd frontend && npm run build
```
Atteso: PASS e build pulita. Nessuna affermazione di "funziona" senza questo output nel report.

- [ ] **Step 2: Aggiorna il README**

Nella descrizione delle funzionalità, dove già si parla della checklist di riconsegna: un gioco a cui una riconsegna ha rilevato pezzi mancanti resta marcato come incompleto nel catalogo e al banco finché un admin non chiude la segnalazione, e ogni gioco ha il registro dei suoi prestiti con in evidenza quelli tornati incompleti. Poche righe, nel registro del testo che c'è.

- [ ] **Step 3: Pass `/impeccable`**

Ultimo task obbligatorio per ogni lavoro che tocca la UI:

```
/impeccable polish — la pagina GameLoansView, la pastiglia di stato nel
catalogo e al banco prestiti, e l'avviso sulla scheda gioco. Desktop e
390px.
```

Applica ciò che il pass trova.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: describe the incomplete mark and the per-game loan log"
```
