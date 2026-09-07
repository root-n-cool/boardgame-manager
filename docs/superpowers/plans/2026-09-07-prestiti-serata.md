# Prestiti della serata — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** registrare chi ritira e chi restituisce ogni copia di gioco
durante una serata, e permettere che un gioco stia in un evento senza
essere prenotabile.

**Architecture:** una tabella `game_loans` legata a `event_games` (la
copia della serata), dove `returned_at IS NULL` significa "fuori"; una
colonna `bookable` su `event_games`; un banco prestiti admin che legge
tutto con un solo GET e scrive con due POST. Il codice Go sta nel
package `events` esistente, accanto a `bookings.go` e `matches.go`.

**Tech Stack:** Go 1.25 + chi + modernc.org/sqlite; Vue 3
`<script setup>` + TypeScript + Vite; migrazioni SQL embeddate.

**Spec:** `docs/superpowers/specs/2026-09-07-prestiti-serata-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain Go locale è rotto. Ogni
  comando `go` va lanciato così, riusando i due volumi nominati:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Non sostituire `bgm-gomodcache` / `bgm-gocache` con volumi nuovi.
- **`npm` in locale**, nella directory `frontend/`.
- **Migrazioni forward-only.** Il file nuovo è
  `backend/internal/db/migrations/0013_loans.sql`. Non modificare mai un
  file di migrazione già esistente.
- **Nessuna dipendenza nuova**, né Go né npm.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n.
- **Design tokens** in `frontend/src/app.css`; sistema visivo in
  `DESIGN.md`.
- **`bookable` assente in una richiesta HTTP significa `true`.** Vale
  per `POST /api/events` e `PUT /api/events/{id}`.
- Commit in inglese, conventional commits (`feat:`, `fix:`, `docs:`).

---

## File Structure

**Backend, creati:**
- `backend/internal/db/migrations/0013_loans.sql` — la migrazione.
- `backend/internal/events/loans.go` — modello e store dei prestiti.
- `backend/internal/events/loans_test.go` — test dello store.
- `backend/internal/httpapi/loans_handlers.go` — i tre handler.
- `backend/internal/httpapi/loans_responses.go` — serializzazione,
  incluso l'assemblaggio della risposta del banco prestiti.
- `backend/internal/httpapi/loans_handlers_test.go` — test degli handler.

**Backend, modificati:**
- `backend/internal/events/store.go` — `Bookable` su `EventGame` e
  `EventGameInput`, scrittura e lettura della colonna, `dropCopies` che
  rispetta i prestiti aperti.
- `backend/internal/events/bookings.go` — rifiuto di prenotare una copia
  non prenotabile.
- `backend/internal/httpapi/events_handlers.go` — `bookable` nella
  richiesta di creazione/modifica evento e i nuovi 409.
- `backend/internal/httpapi/events_responses.go` — `bookable` nella
  risposta pubblica dell'evento.
- `backend/internal/httpapi/events_bookings_handlers.go` — il 409 sulla
  copia non prenotabile.
- `backend/internal/httpapi/router.go` — tre rotte protette.

**Frontend, creati:**
- `frontend/src/views/LoanDeskView.vue` — il banco prestiti.

**Frontend, modificati:**
- `frontend/src/components/EventGamesPicker.vue` — spunta "prenotabile".
- `frontend/src/views/EventAdminDetailView.vue` — `bookable` nel
  caricamento e il link al banco prestiti.
- `frontend/src/views/EventDetailView.vue` — copie non prenotabili.
- `frontend/src/router/index.ts` — la rotta del banco prestiti.
- `frontend/src/app.css` — stili del banco prestiti e della pastiglia.

**Documentazione, modificati:**
- `README.md` — la funzionalità visibile in più.
- `DESIGN.md` — i pattern visivi nuovi.

---

## Task 1: Migrazione e flag `bookable`

**Files:**
- Create: `backend/internal/db/migrations/0013_loans.sql`
- Modify: `backend/internal/events/store.go`
- Test: `backend/internal/events/store_test.go`

**Interfaces:**
- Consumes: niente (primo task).
- Produces:
  - `events.EventGame` guadagna il campo `Bookable bool`.
  - `events.EventGameInput` guadagna il campo `Bookable *bool`, dove
    `nil` significa prenotabile.
  - `events.ErrUnbookableWithActiveBookings error`.
  - la tabella `game_loans` e la colonna `event_games.bookable`.

**Perché `*bool` sull'input e `bool` sul modello letto:** ci sono 62
letterali `EventGameInput{GameID: …, Copies: …}` nei test esistenti. Con
un `bool`, il valore zero `false` li trasformerebbe tutti in giochi non
prenotabili e le prenotazioni comincerebbero a essere rifiutate. Con
`*bool`, `nil` significa prenotabile e quei letterali continuano a dire
quello che dicevano — la stessa regola del contratto HTTP. `EventGame`
invece è il modello letto dal database, dove la colonna c'è sempre.

- [ ] **Step 1: Scrivi la migrazione**

Crea `backend/internal/db/migrations/0013_loans.sql`:

```sql
-- Non tutti i giochi di una serata vanno a prenotazione: un filler come
-- Love Letter resta sul tavolo per chi arriva a mani vuote. DEFAULT 1
-- lascia identico tutto quello che c'è già in archivio.
ALTER TABLE event_games ADD COLUMN bookable INTEGER NOT NULL DEFAULT 1
    CHECK (bookable IN (0, 1));

-- Il registro di chi ha in mano cosa. Non c'è una colonna di stato: lo
-- stato è la data che manca, returned_at IS NULL significa "fuori".
-- borrower_name e borrower_phone si copiano sulla riga anche quando il
-- prestito nasce da una prenotazione, così il registro si legge per
-- intero anche se quella prenotazione viene poi annullata.
CREATE TABLE game_loans (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    event_game_id  INTEGER NOT NULL REFERENCES event_games(id) ON DELETE CASCADE,
    booking_id     INTEGER REFERENCES bookings(id) ON DELETE SET NULL,
    borrower_name  TEXT NOT NULL,
    borrower_phone TEXT NOT NULL,
    notes          TEXT,
    lent_at        TEXT NOT NULL DEFAULT (datetime('now')),
    returned_at    TEXT
);

-- Una copia sola può essere fuori una volta sola. Il vincolo sta qui e
-- non nel codice: due consegne simultanee sulla stessa copia non devono
-- poter passare entrambe.
CREATE UNIQUE INDEX idx_one_open_loan_per_copy
    ON game_loans(event_game_id) WHERE returned_at IS NULL;
```

- [ ] **Step 2: Scrivi i test che falliscono**

In `backend/internal/events/store_test.go`, in fondo al file:

```go
func TestCreateEventDefaultsToBookableCopies(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")

	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)

	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(copies) != 1 {
		t.Fatalf("copies = %d, want 1", len(copies))
	}
	if !copies[0].Bookable {
		t.Fatal("una copia creata senza dire niente deve essere prenotabile")
	}
}

func TestCreateEventStoresUnbookableCopies(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Love Letter")
	no := false

	event, err := store.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 2, Bookable: &no}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(copies) != 2 {
		t.Fatalf("copies = %d, want 2", len(copies))
	}
	for _, c := range copies {
		if c.Bookable {
			t.Fatalf("copia #%d prenotabile, doveva non esserlo", c.CopyIndex)
		}
	}
}

func TestUpdateEventTogglesBookable(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Love Letter")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	no := false

	if _, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 2, Bookable: &no}},
	}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(copies) != 2 {
		t.Fatalf("copies = %d, want 2", len(copies))
	}
	// Sia la copia che c'era prima sia quella aggiunta ora devono seguire
	// il flag: il gioco è non prenotabile, non "metà non prenotabile".
	for _, c := range copies {
		if c.Bookable {
			t.Fatalf("copia #%d prenotabile dopo lo spegnimento del flag", c.CopyIndex)
		}
	}
}

func TestUpdateEventRefusesUnbookableWithActiveBookings(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Wingspan")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if err := store.TestInsertBooking(event.ID, copies[0].ID, events.BookingStatusActive); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	no := false

	_, err = store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1, Bookable: &no}},
	})
	if !errors.Is(err, events.ErrUnbookableWithActiveBookings) {
		t.Fatalf("err = %v, want ErrUnbookableWithActiveBookings", err)
	}

	// Il rifiuto arriva prima di qualunque scrittura: la copia è ancora
	// prenotabile.
	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if !after[0].Bookable {
		t.Fatal("il flag è stato spento nonostante il rifiuto")
	}
}
```

- [ ] **Step 3: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ -run 'Bookable|Unbookable' -v
```

Atteso: FAIL in compilazione — `Bookable` non è un campo di
`EventGameInput`, `ErrUnbookableWithActiveBookings` non esiste.

- [ ] **Step 4: Aggiungi i campi e l'errore**

In `backend/internal/events/store.go`, dentro `type EventGame struct`,
dopo il campo `Seats`:

```go
	// Bookable dice se questa copia si può prenotare in anticipo. Falso è
	// il filler lasciato sul tavolo per chi arriva senza prenotazione:
	// presente nella serata, assente dal form pubblico.
	Bookable bool
```

Dentro `type EventGameInput struct`, dopo `Copies`:

```go
	// Bookable è un puntatore perché la sua assenza ha un significato:
	// nil vuol dire prenotabile. È la stessa regola del corpo HTTP, e
	// tiene in piedi ogni chiamante che non sa niente di questo campo.
	Bookable *bool
```

Sotto il blocco `var (...)` degli errori, aggiungi la riga:

```go
	ErrUnbookableWithActiveBookings = errors.New("cannot unbook a game with active bookings")
```

E in fondo al file, accanto a `isUniqueConstraintErr`:

```go
// bookableValue scioglie il puntatore: nessuna indicazione significa
// prenotabile, che è come si sono sempre comportati gli eventi.
func bookableValue(v *bool) bool {
	return v == nil || *v
}

// boolToInt traduce per SQLite, che non ha un tipo booleano.
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
```

- [ ] **Step 5: Scrivi e leggi la colonna**

In `store.go`, `insertEventGames` passa il flag a `insertCopies`:

```go
func insertEventGames(ctx context.Context, tx execQueryer, eventID int64, gamesInput []EventGameInput) error {
	for _, g := range gamesInput {
		seats, err := gameSeats(ctx, tx, g.GameID)
		if err != nil {
			return err
		}
		if err := insertCopies(ctx, tx, eventID, g.GameID, seats, bookableValue(g.Bookable), 1, g.Copies); err != nil {
			return err
		}
	}
	return nil
}
```

`insertCopies` guadagna il parametro e la colonna:

```go
// insertCopies scrive `count` copie consecutive a partire da firstIndex.
func insertCopies(ctx context.Context, tx execer, eventID, gameID int64, seats int, bookable bool, firstIndex, count int) error {
	for i := 0; i < count; i++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO event_games (event_id, game_id, copy_index, seats, bookable) VALUES (?, ?, ?, ?, ?)`,
			eventID, gameID, firstIndex+i, seats, boolToInt(bookable),
		); err != nil {
			return err
		}
	}
	return nil
}
```

`listEventGames` e `GetEventGame` leggono la colonna. In
`listEventGames` sostituisci la query e lo `Scan`:

```go
	rows, err := q.QueryContext(ctx,
		`SELECT id, event_id, game_id, copy_index, seats, bookable FROM event_games
		 WHERE event_id = ? ORDER BY game_id, copy_index`, eventID)
```

```go
	for rows.Next() {
		var eg EventGame
		var bookable int
		if err := rows.Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.CopyIndex, &eg.Seats, &bookable); err != nil {
			return nil, err
		}
		eg.Bookable = bookable == 1
		out = append(out, eg)
	}
```

In `GetEventGame`:

```go
func (s *Store) GetEventGame(ctx context.Context, id int64) (EventGame, error) {
	var eg EventGame
	var bookable int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, game_id, copy_index, seats, bookable FROM event_games WHERE id = ?`, id,
	).Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.CopyIndex, &eg.Seats, &bookable)
	if errors.Is(err, sql.ErrNoRows) {
		return EventGame{}, ErrNotFound
	}
	eg.Bookable = bookable == 1
	return eg, err
}
```

- [ ] **Step 6: Aggiorna `UpdateEvent`**

Nel passaggio di validazione (quello che costruisce `seatsByGame` e
`wanted`, subito dopo la lettura di `occupied`), sostituisci il blocco
con:

```go
	// Un passaggio a parte per validare tutti i giochi richiesti e leggere
	// i posti prenotabili: se uno non esiste, si esce prima di scrivere.
	// Lo stesso vale per il flag: spegnerlo su un gioco che qualcuno ha
	// già prenotato è un rifiuto, e deve arrivare prima delle scritture.
	seatsByGame := map[int64]int{}
	wanted := map[int64]int{}
	bookableByGame := map[int64]bool{}
	for _, g := range in.Games {
		seats, err := gameSeats(ctx, tx, g.GameID)
		if err != nil {
			return Event{}, err
		}
		seatsByGame[g.GameID] = seats
		wanted[g.GameID] = g.Copies
		bookable := bookableValue(g.Bookable)
		bookableByGame[g.GameID] = bookable
		if !bookable {
			for _, c := range copiesByGame[g.GameID] {
				if occupied[c.ID] > 0 {
					return Event{}, ErrUnbookableWithActiveBookings
				}
			}
		}
	}
```

Nel ciclo di scrittura `for _, g := range in.Games`, la chiamata a
`insertCopies` prende il flag:

```go
			if err := insertCopies(ctx, tx, id, g.GameID, seatsByGame[g.GameID], bookableByGame[g.GameID], next, g.Copies-len(copies)); err != nil {
				return Event{}, err
			}
```

E in fondo allo stesso ciclo, dopo lo `switch`, allinea il flag su tutte
le copie del gioco:

```go
		// Le copie appena inserite nascono già col flag giusto; questa
		// riga serve a quelle che c'erano prima, e farlo per tutte è più
		// semplice che tenere il conto di quali.
		if _, err := tx.ExecContext(ctx,
			`UPDATE event_games SET bookable = ? WHERE event_id = ? AND game_id = ?`,
			boolToInt(bookableByGame[g.GameID]), id, g.GameID,
		); err != nil {
			return Event{}, err
		}
```

Aggiorna anche la chiamata a `insertCopies` in `CreateEvent`, se il
compilatore la segnala: passa per `insertEventGames`, già sistemato.

- [ ] **Step 7: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./... 2>&1 | tail -25
```

Atteso: PASS in tutti i package. Se un package non compila per la firma
di `insertCopies`, è una chiamata rimasta indietro: aggiungila.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/db/migrations/0013_loans.sql \
        backend/internal/events/store.go \
        backend/internal/events/store_test.go
git commit -m "feat: let an event carry games that cannot be booked"
```

---

## Task 2: Prestiti nello store

**Files:**
- Create: `backend/internal/events/loans.go`
- Create: `backend/internal/events/loans_test.go`

**Interfaces:**
- Consumes: da Task 1, `EventGame.Bookable`, `bookableValue`,
  `boolToInt`, la tabella `game_loans`. Dal codice esistente:
  `Store.GetEventGame`, `ErrNotFound`, `isUniqueConstraintErr`,
  `queryer`, `Store.TestInsertBooking`.
- Produces:
  - `events.Loan` con campi `ID int64`, `EventGameID int64`,
    `BookingID *int64`, `BorrowerName string`, `BorrowerPhone string`,
    `Notes *string`, `LentAt time.Time`, `ReturnedAt *time.Time`.
  - `events.LoanWithGame` = `Loan` embeddato più `GameID int64`,
    `GameName string`, `CopyIndex int`.
  - `events.LoanInput` con `EventGameID int64`, `BookingID *int64`,
    `BorrowerName string`, `BorrowerPhone string`, `Notes *string`.
  - `events.ErrCopyAlreadyOut`, `events.ErrLoanAlreadyReturned`,
    `events.ErrBorrowerRequired`.
  - `(*Store).LendCopy(ctx context.Context, eventID int64, in LoanInput) (Loan, error)`
  - `(*Store).ReturnLoan(ctx context.Context, id int64, notes *string) (Loan, error)`
  - `(*Store).ListLoansForEvent(ctx context.Context, eventID int64) ([]LoanWithGame, error)`
  - `openLoanCopies(ctx context.Context, q queryer, eventID int64) (map[int64]bool, error)`
    (non esportata, per Task 3)

- [ ] **Step 1: Scrivi i test che falliscono**

Crea `backend/internal/events/loans_test.go`:

```go
package events_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/events"
)

// mustLend apre un prestito su una copia, per i test che partono da lì.
func mustLend(t *testing.T, store *events.Store, eventID, eventGameID int64, name string) events.Loan {
	t.Helper()
	loan, err := store.LendCopy(context.Background(), eventID, events.LoanInput{
		EventGameID: eventGameID, BorrowerName: name, BorrowerPhone: "3331234567",
	})
	if err != nil {
		t.Fatalf("lend to %q: %v", name, err)
	}
	return loan
}

// firstCopy è la copia su cui girano quasi tutti i test dei prestiti.
func firstCopy(t *testing.T, store *events.Store, eventID int64) events.EventGame {
	t.Helper()
	copies, err := store.ListEventGames(context.Background(), eventID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(copies) == 0 {
		t.Fatal("l'evento non ha copie")
	}
	return copies[0]
}

func TestLendCopyOpensALoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)

	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	if loan.BorrowerName != "Anna" {
		t.Errorf("borrower = %q, want Anna", loan.BorrowerName)
	}
	if loan.ReturnedAt != nil {
		t.Error("un prestito appena aperto non può essere già restituito")
	}
	if loan.LentAt.IsZero() {
		t.Error("lentAt vuoto: la data di inizio la mette il database")
	}
	if loan.BookingID != nil {
		t.Error("bookingID valorizzato su un prestito senza prenotazione")
	}
}

func TestLendCopyTrimsAndRequiresBorrower(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)

	for _, tc := range []struct{ name, borrower, phone string }{
		{"nome vuoto", "", "3331234567"},
		{"nome di spazi", "   ", "3331234567"},
		{"telefono vuoto", "Anna", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
				EventGameID: copy.ID, BorrowerName: tc.borrower, BorrowerPhone: tc.phone,
			})
			if !errors.Is(err, events.ErrBorrowerRequired) {
				t.Fatalf("err = %v, want ErrBorrowerRequired", err)
			}
		})
	}
}

func TestLendCopyRefusesASecondOpenLoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	mustLend(t, store, event.ID, copy.ID, "Anna")

	_, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copy.ID, BorrowerName: "Bruno", BorrowerPhone: "3339999999",
	})
	if !errors.Is(err, events.ErrCopyAlreadyOut) {
		t.Fatalf("err = %v, want ErrCopyAlreadyOut", err)
	}
}

func TestLendCopyRefusesACopyOfAnotherEvent(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	mine := mustCreateEvent(t, store, "Mia", "2030-01-01", "21:00", gameID)
	other := mustCreateEvent(t, store, "Altra", "2030-02-01", "21:00", gameID)
	otherCopy := firstCopy(t, store, other.ID)

	_, err := store.LendCopy(context.Background(), mine.ID, events.LoanInput{
		EventGameID: otherCopy.ID, BorrowerName: "Anna", BorrowerPhone: "3331234567",
	})
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReturnLoanClosesItAndFreesTheCopy(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")

	notes := "manca una tessera"
	returned, err := store.ReturnLoan(context.Background(), loan.ID, &notes)
	if err != nil {
		t.Fatalf("return loan: %v", err)
	}
	if returned.ReturnedAt == nil {
		t.Fatal("returnedAt vuoto dopo la restituzione")
	}
	if returned.Notes == nil || *returned.Notes != notes {
		t.Errorf("notes = %v, want %q", returned.Notes, notes)
	}

	// La copia è tornata libera: si può riprestare, e lo storico resta.
	second := mustLend(t, store, event.ID, copy.ID, "Bruno")
	if second.ID == loan.ID {
		t.Fatal("il secondo prestito ha riusato la riga del primo")
	}
	all, err := store.ListLoansForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("prestiti = %d, want 2", len(all))
	}
}

func TestReturnLoanKeepsNotesWhenNoneAreSent(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	atPickup := "scatola già ammaccata"
	loan, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copy.ID, BorrowerName: "Anna", BorrowerPhone: "3331234567",
		Notes: &atPickup,
	})
	if err != nil {
		t.Fatalf("lend: %v", err)
	}

	returned, err := store.ReturnLoan(context.Background(), loan.ID, nil)
	if err != nil {
		t.Fatalf("return loan: %v", err)
	}
	if returned.Notes == nil || *returned.Notes != atPickup {
		t.Fatalf("notes = %v, want %q: un nil non deve cancellare quello che c'era", returned.Notes, atPickup)
	}
}

func TestReturnLoanRefusesTwice(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	loan := mustLend(t, store, event.ID, copy.ID, "Anna")
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil); err != nil {
		t.Fatalf("first return: %v", err)
	}

	_, err := store.ReturnLoan(context.Background(), loan.ID, nil)
	if !errors.Is(err, events.ErrLoanAlreadyReturned) {
		t.Fatalf("err = %v, want ErrLoanAlreadyReturned", err)
	}
}

func TestReturnLoanRefusesAnUnknownID(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.ReturnLoan(context.Background(), 999, nil)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLendCopyFromABooking(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Wingspan")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	if err := store.TestInsertBooking(event.ID, copy.ID, events.BookingStatusActive); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	bookings, err := store.ListBookingsForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	bookingID := bookings[0].ID

	loan, err := store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copy.ID, BookingID: &bookingID,
		BorrowerName: "Anna", BorrowerPhone: "3331234567",
	})
	if err != nil {
		t.Fatalf("lend from booking: %v", err)
	}
	if loan.BookingID == nil || *loan.BookingID != bookingID {
		t.Fatalf("bookingID = %v, want %d", loan.BookingID, bookingID)
	}
}

func TestLendCopyRefusesABookingOnAnotherCopy(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Wingspan")
	event, err := store.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if err := store.TestInsertBooking(event.ID, copies[0].ID, events.BookingStatusActive); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	bookings, err := store.ListBookingsForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	bookingID := bookings[0].ID

	// La prenotazione è sulla copia #1: agganciarla al prestito della #2
	// renderebbe il registro una bugia.
	_, err = store.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: copies[1].ID, BookingID: &bookingID,
		BorrowerName: "Anna", BorrowerPhone: "3331234567",
	})
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListLoansForEventCarriesTheGameLabel(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	copy := firstCopy(t, store, event.ID)
	mustLend(t, store, event.ID, copy.ID, "Anna")

	loans, err := store.ListLoansForEvent(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(loans) != 1 {
		t.Fatalf("prestiti = %d, want 1", len(loans))
	}
	if loans[0].GameName != "Carcassonne" {
		t.Errorf("gameName = %q, want Carcassonne", loans[0].GameName)
	}
	if loans[0].CopyIndex != 1 {
		t.Errorf("copyIndex = %d, want 1", loans[0].CopyIndex)
	}
	if loans[0].GameID != gameID {
		t.Errorf("gameID = %d, want %d", loans[0].GameID, gameID)
	}
}

func TestListLoansForEventIgnoresOtherEvents(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	mine := mustCreateEvent(t, store, "Mia", "2030-01-01", "21:00", gameID)
	other := mustCreateEvent(t, store, "Altra", "2030-02-01", "21:00", gameID)
	mustLend(t, store, other.ID, firstCopy(t, store, other.ID).ID, "Bruno")

	loans, err := store.ListLoansForEvent(context.Background(), mine.ID)
	if err != nil {
		t.Fatalf("list loans: %v", err)
	}
	if len(loans) != 0 {
		t.Fatalf("prestiti = %d, want 0", len(loans))
	}
}

func TestDeleteEventRemovesItsLoans(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	loan := mustLend(t, store, event.ID, firstCopy(t, store, event.ID).ID, "Anna")

	if err := store.DeleteEvent(context.Background(), event.ID); err != nil {
		t.Fatalf("delete event: %v", err)
	}

	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: il prestito doveva sparire in cascata", err)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ -run 'Loan|Lend' -v
```

Atteso: FAIL in compilazione — `LendCopy`, `LoanInput` e gli errori non
esistono.

- [ ] **Step 3: Scrivi `loans.go`**

Crea `backend/internal/events/loans.go`:

```go
package events

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Loan è una copia della serata data in mano a qualcuno. Non c'è una
// colonna di stato: lo stato è la data che manca, ReturnedAt nil
// significa "fuori". Un secondo campo sarebbe una verità duplicata che
// può divergere da questa.
type Loan struct {
	ID          int64
	EventGameID int64
	// BookingID c'è solo quando il prestito nasce da una prenotazione:
	// è l'unico modo di rispondere a "chi ha prenotato e non si è
	// presentato". Nome e telefono si copiano comunque sulla riga, così
	// il registro si legge anche se la prenotazione viene annullata.
	BookingID     *int64
	BorrowerName  string
	BorrowerPhone string
	Notes         *string
	LentAt        time.Time
	ReturnedAt    *time.Time
}

// LoanWithGame è un prestito con l'etichetta del gioco già risolta: il
// banco prestiti mostra righe, non fa join per conto suo.
type LoanWithGame struct {
	Loan
	GameID    int64
	GameName  string
	CopyIndex int
}

// LoanInput è un prestito come lo apre l'organizzatore.
type LoanInput struct {
	EventGameID   int64
	BookingID     *int64
	BorrowerName  string
	BorrowerPhone string
	Notes         *string
}

var (
	ErrCopyAlreadyOut      = errors.New("copy already on loan")
	ErrLoanAlreadyReturned = errors.New("loan already returned")
	ErrBorrowerRequired    = errors.New("borrower name and phone are required")
)

// loanColumns tiene allineate le tre query che leggono un prestito: una
// colonna aggiunta qui e dimenticata in uno Scan è un panic a runtime.
const loanColumns = `l.id, l.event_game_id, l.booking_id, l.borrower_name,
	l.borrower_phone, l.notes, l.lent_at, l.returned_at`

// scanner è soddisfatto sia da *sql.Row sia da *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanLoan(sc scanner, extra ...any) (Loan, error) {
	var l Loan
	var bookingID sql.NullInt64
	var notes, returnedAt sql.NullString
	var lentAt string
	dest := append([]any{
		&l.ID, &l.EventGameID, &bookingID, &l.BorrowerName,
		&l.BorrowerPhone, &notes, &lentAt, &returnedAt,
	}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return Loan{}, err
	}
	if bookingID.Valid {
		id := bookingID.Int64
		l.BookingID = &id
	}
	// Una nota svuotata nel form arriva come stringa vuota: per chi legge
	// è la stessa cosa che non averne, e nil lo dice senza ambiguità.
	if notes.Valid && notes.String != "" {
		text := notes.String
		l.Notes = &text
	}
	// Le date le scrive SQLite con datetime('now'), che è UTC.
	l.LentAt, _ = time.Parse("2006-01-02 15:04:05", lentAt)
	if returnedAt.Valid {
		at, _ := time.Parse("2006-01-02 15:04:05", returnedAt.String)
		l.ReturnedAt = &at
	}
	return l, nil
}

// LendCopy consegna una copia della serata. La copia deve appartenere
// all'evento indicato, e la prenotazione — quando c'è — deve stare sulla
// stessa copia.
//
// Prestare una copia che ha prenotazioni attive a un nome che non è fra
// i prenotati è permesso: alle 21:30 chi non si è presentato non deve
// tenere in ostaggio la scatola. L'avviso lo dà la UI.
func (s *Store) LendCopy(ctx context.Context, eventID int64, in LoanInput) (Loan, error) {
	name := strings.TrimSpace(in.BorrowerName)
	phone := strings.TrimSpace(in.BorrowerPhone)
	if name == "" || phone == "" {
		return Loan{}, ErrBorrowerRequired
	}

	eventGame, err := s.GetEventGame(ctx, in.EventGameID)
	if err != nil {
		return Loan{}, err
	}
	if eventGame.EventID != eventID {
		return Loan{}, ErrNotFound
	}

	if in.BookingID != nil {
		var count int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM bookings
			 WHERE id = ? AND event_game_id = ? AND status = ?`,
			*in.BookingID, in.EventGameID, BookingStatusActive,
		).Scan(&count); err != nil {
			return Loan{}, err
		}
		if count == 0 {
			return Loan{}, ErrNotFound
		}
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_loans (event_game_id, booking_id, borrower_name, borrower_phone, notes)
		 VALUES (?, ?, ?, ?, ?)`,
		in.EventGameID, in.BookingID, name, phone, in.Notes,
	)
	if err != nil {
		// idx_one_open_loan_per_copy è l'unico vincolo UNIQUE su questa
		// tabella: una collisione significa che la copia è già fuori. Il
		// controllo sta nell'indice e non in una lettura preventiva, così
		// due consegne simultanee non possono passare entrambe.
		if isUniqueConstraintErr(err) {
			return Loan{}, ErrCopyAlreadyOut
		}
		return Loan{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Loan{}, err
	}
	return s.getLoanByID(ctx, id)
}

// ReturnLoan chiude un prestito. `notes` nil lascia quelle scritte alla
// consegna: la modale manda il campo solo se l'organizzatore l'ha
// toccato, e un nil non deve cancellare quello che c'era.
func (s *Store) ReturnLoan(ctx context.Context, id int64, notes *string) (Loan, error) {
	loan, err := s.getLoanByID(ctx, id)
	if err != nil {
		return Loan{}, err
	}
	if notes == nil {
		notes = loan.Notes
	}

	// `AND returned_at IS NULL` fa il lavoro del controllo: una seconda
	// restituzione non tocca nessuna riga, e lo sappiamo da RowsAffected
	// invece che da una lettura che potrebbe essere già vecchia.
	res, err := s.db.ExecContext(ctx,
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
	return s.getLoanByID(ctx, id)
}

func (s *Store) getLoanByID(ctx context.Context, id int64) (Loan, error) {
	loan, err := scanLoan(s.db.QueryRowContext(ctx,
		`SELECT `+loanColumns+` FROM game_loans l WHERE l.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Loan{}, ErrNotFound
	}
	return loan, err
}

// ListLoansForEvent è tutto il registro della serata, aperti e chiusi
// insieme, dal più recente. Chi chiama separa i due gruppi guardando
// ReturnedAt: una query invece di due, e nessun rischio che le due
// risposte arrivino da istanti diversi.
func (s *Store) ListLoansForEvent(ctx context.Context, eventID int64) ([]LoanWithGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+loanColumns+`, g.id, g.name, eg.copy_index
		 FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 WHERE eg.event_id = ?
		 ORDER BY l.lent_at DESC, l.id DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []LoanWithGame{}
	for rows.Next() {
		var lg LoanWithGame
		loan, err := scanLoan(rows, &lg.GameID, &lg.GameName, &lg.CopyIndex)
		if err != nil {
			return nil, err
		}
		lg.Loan = loan
		out = append(out, lg)
	}
	return out, rows.Err()
}

// openLoanCopies dice quali copie dell'evento sono fuori adesso. Serve
// alla modifica evento, che non deve poter togliere una copia che
// qualcuno ha in mano.
func openLoanCopies(ctx context.Context, q queryer, eventID int64) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT l.event_game_id FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 WHERE eg.event_id = ? AND l.returned_at IS NULL`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]bool{}
	for rows.Next() {
		var eventGameID int64
		if err := rows.Scan(&eventGameID); err != nil {
			return nil, err
		}
		out[eventGameID] = true
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ -v 2>&1 | tail -30
```

Atteso: PASS. Nota: `openLoanCopies` non è ancora usata — il
compilatore Go non protesta per una funzione non esportata e non usata a
livello di package, quindi non serve nessun accorgimento.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/loans.go backend/internal/events/loans_test.go
git commit -m "feat: record who takes a copy of a game and who brings it back"
```

---

## Task 3: Una copia in prestito non si può togliere dall'evento

**Files:**
- Modify: `backend/internal/events/store.go`
- Test: `backend/internal/events/loans_test.go`

**Interfaces:**
- Consumes: da Task 2, `openLoanCopies`. Da Task 1, `bookableValue`.
- Produces: `events.ErrCopyOnLoan error`; `dropCopies` con la firma
  `dropCopies(ctx context.Context, tx execer, copies []EventGame, occupied map[int64]int, onLoan map[int64]bool, count int) error`.

- [ ] **Step 1: Scrivi i test che falliscono**

`dropCopies` cerca **un numero** di copie libere partendo dalla coda,
saltando quelle bloccate: con due copie di cui la seconda in prestito e
una richiesta di scendere a una, cancella la prima e tiene la seconda.
È la semantica che la funzione ha già per le prenotazioni, ed è quella
giusta — la copia che è fisicamente fuori sopravvive e il registro resta
vero. `ErrCopyOnLoan` scatta quindi **solo quando le copie libere non
bastano**: in pratica quando la copia in prestito è l'unica e il gioco
viene tolto dall'evento. I tre test qui sotto coprono il rifiuto, il
prestito chiuso che non blocca niente, e la semantica del
restringimento.

In fondo a `backend/internal/events/loans_test.go`:

```go
func TestUpdateEventRefusesToDropACopyOnLoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	// Una copia sola, e fuori in prestito: togliere il gioco dall'evento
	// vuol dire cancellare proprio quella, e non c'è nessuna copia libera
	// con cui soddisfare la richiesta.
	mustLend(t, store, event.ID, firstCopy(t, store, event.ID).ID, "Anna")

	_, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{},
	})
	if !errors.Is(err, events.ErrCopyOnLoan) {
		t.Fatalf("err = %v, want ErrCopyOnLoan", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("copie = %d, want 1: la transazione doveva annullarsi", len(after))
	}
}

func TestUpdateEventDropsACopyOnceReturned(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event := mustCreateEvent(t, store, "Serata", "2030-01-01", "21:00", gameID)
	loan := mustLend(t, store, event.ID, firstCopy(t, store, event.ID).ID, "Anna")
	if _, err := store.ReturnLoan(context.Background(), loan.ID, nil); err != nil {
		t.Fatalf("return loan: %v", err)
	}

	// Il guard riguarda i prestiti aperti: uno chiuso non blocca niente.
	if _, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{},
	}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("copie = %d, want 0", len(after))
	}
}

func TestUpdateEventShrinkingSpareTheCopyOnLoan(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event, err := store.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	mustLend(t, store, event.ID, copies[1].ID, "Anna")

	// Scendere a una copia si può: si sacrifica quella libera, non quella
	// che qualcuno ha in mano, così il registro dei prestiti resta vero.
	if _, err := store.UpdateEvent(context.Background(), event.ID, events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	}); err != nil {
		t.Fatalf("update event: %v", err)
	}

	after, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("copie = %d, want 1", len(after))
	}
	if after[0].ID != copies[1].ID {
		t.Fatalf("copia sopravvissuta = %d, want %d: doveva restare quella in prestito", after[0].ID, copies[1].ID)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ -run 'CopyOnLoan|OnceReturned|ShrinkingSpare' -v
```

Atteso: FAIL in compilazione — `ErrCopyOnLoan` non esiste.

- [ ] **Step 3: Aggiungi l'errore**

In `store.go`, nel blocco `var (...)` degli errori:

```go
	ErrCopyOnLoan = errors.New("copy is on loan")
```

- [ ] **Step 4: Insegna a `dropCopies` a rispettare i prestiti**

Sostituisci `dropCopies` in `store.go`:

```go
// dropCopies elimina `count` copie partendo dalla più alta, saltando
// quelle con prenotazioni attive e quelle che qualcuno ha in mano. Se le
// copie libere non bastano l'operazione fallisce e la transazione del
// chiamante viene annullata: meglio un errore che una prenotazione
// cancellata a cascata sotto il naso di chi l'ha fatta, o una scatola
// che sparisce dal registro mentre è ancora fuori.
//
// Quando entrambi i motivi bloccano, vince il prestito nel messaggio:
// è quello che l'organizzatore può risolvere subito, facendosi
// restituire la scatola.
func dropCopies(ctx context.Context, tx execer, copies []EventGame, occupied map[int64]int, onLoan map[int64]bool, count int) error {
	dropped := 0
	blockedByLoan := false
	for i := len(copies) - 1; i >= 0 && dropped < count; i-- {
		if occupied[copies[i].ID] > 0 {
			continue
		}
		if onLoan[copies[i].ID] {
			blockedByLoan = true
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_games WHERE id = ?`, copies[i].ID); err != nil {
			return err
		}
		dropped++
	}
	if dropped < count {
		if blockedByLoan {
			return ErrCopyOnLoan
		}
		return ErrQuantityBelowActiveBookings
	}
	return nil
}
```

- [ ] **Step 5: Passa la mappa dei prestiti aperti in `UpdateEvent`**

In `UpdateEvent`, subito dopo la lettura di `occupied`:

```go
	onLoan, err := openLoanCopies(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}
```

E aggiorna le due chiamate a `dropCopies` (quella dei giochi spariti
dalla selezione e quella dentro lo `switch`), aggiungendo `onLoan` prima
di `count`:

```go
			if err := dropCopies(ctx, tx, copies, occupied, onLoan, len(copies)); err != nil {
```

```go
			if err := dropCopies(ctx, tx, copies, occupied, onLoan, len(copies)-g.Copies); err != nil {
```

- [ ] **Step 6: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./... 2>&1 | tail -20
```

Atteso: PASS in tutti i package.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/events/store.go backend/internal/events/loans_test.go
git commit -m "fix: refuse to drop an event copy that is out on loan"
```

---

## Task 4: API del banco prestiti

**Files:**
- Create: `backend/internal/httpapi/loans_handlers.go`
- Create: `backend/internal/httpapi/loans_responses.go`
- Create: `backend/internal/httpapi/loans_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: da Task 2, `Store.LendCopy`, `Store.ReturnLoan`,
  `Store.ListLoansForEvent`, `events.LoanInput`, `events.LoanWithGame`,
  `events.ErrCopyAlreadyOut`, `events.ErrLoanAlreadyReturned`,
  `events.ErrBorrowerRequired`. Da Task 1, `EventGame.Bookable`. Dal
  codice esistente: `parseIDParam`, `writeJSON`, `writeError`,
  `Store.ListEventGames`, `Store.ListBookingsForEvent`,
  `s.Games.GetGame`.
- Produces: le tre rotte protette
  `GET /api/events/{id}/loans`, `POST /api/events/{id}/loans`,
  `POST /api/loans/{id}/return` con la forma di risposta descritta nella
  spec.

- [ ] **Step 1: Scrivi i test che falliscono**

Crea `backend/internal/httpapi/loans_handlers_test.go`. Il file segue il
pattern dei test HTTP già in uso in questo package — `httpapi.NewRouter`
più `httptest.NewRecorder`, il cookie di sessione da
`bootstrapFirstAdmin` (in `auth_handlers_test.go`), il gioco da
`createTestGameForEvent` (in `events_handlers_test.go`) — e struct
tipizzate per decodificare, non `map[string]any`:

```go
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/httpapi"
)

// Le tre forme di risposta del banco prestiti, come struct: un typo in un
// nome di campo diventa un test rosso invece di un nil silenzioso.
type deskBookingBody struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type openLoanBody struct {
	ID            int64   `json:"id"`
	BorrowerName  string  `json:"borrowerName"`
	BorrowerPhone string  `json:"borrowerPhone"`
	LentAt        string  `json:"lentAt"`
	Notes         *string `json:"notes"`
}

type deskCopyBody struct {
	EventGameID    int64             `json:"eventGameId"`
	GameID         int64             `json:"gameId"`
	Name           string            `json:"name"`
	CopyIndex      int               `json:"copyIndex"`
	Copies         int               `json:"copies"`
	Bookable       bool              `json:"bookable"`
	Seats          int               `json:"seats"`
	ActiveBookings []deskBookingBody `json:"activeBookings"`
	OpenLoan       *openLoanBody     `json:"openLoan"`
}

type returnedLoanBody struct {
	ID           int64   `json:"id"`
	EventGameID  int64   `json:"eventGameId"`
	GameName     string  `json:"gameName"`
	CopyIndex    int     `json:"copyIndex"`
	BorrowerName string  `json:"borrowerName"`
	LentAt       string  `json:"lentAt"`
	ReturnedAt   string  `json:"returnedAt"`
	Notes        *string `json:"notes"`
}

type loanDeskBody struct {
	Copies   []deskCopyBody     `json:"copies"`
	Returned []returnedLoanBody `json:"returned"`
}

type loanBody struct {
	ID            int64   `json:"id"`
	EventGameID   int64   `json:"eventGameId"`
	BookingID     *int64  `json:"bookingId"`
	BorrowerName  string  `json:"borrowerName"`
	BorrowerPhone string  `json:"borrowerPhone"`
	Notes         *string `json:"notes"`
	LentAt        string  `json:"lentAt"`
	ReturnedAt    *string `json:"returnedAt"`
}

// loanFixture monta un evento con `copies` copie di un gioco e restituisce
// il router, il cookie admin, l'id dell'evento e le sue copie.
func loanFixture(t *testing.T, copies int) (http.Handler, *http.Cookie, int64, []events.EventGame) {
	t.Helper()
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: copies}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	return router, cookie, event.ID, eventGames
}

// doLoanRequest manda una richiesta con o senza cookie e restituisce il
// recorder, così ogni test guarda sia il codice sia il corpo.
func doLoanRequest(router http.Handler, method, path string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func readDesk(t *testing.T, router http.Handler, cookie *http.Cookie, eventID int64) loanDeskBody {
	t.Helper()
	rec := doLoanRequest(router, http.MethodGet, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET loans: %d %s", rec.Code, rec.Body.String())
	}
	var desk loanDeskBody
	if err := json.NewDecoder(rec.Body).Decode(&desk); err != nil {
		t.Fatalf("decode desk: %v", err)
	}
	return desk
}

// lend apre un prestito dall'endpoint e restituisce il prestito creato.
func lend(t *testing.T, router http.Handler, cookie *http.Cookie, eventID, eventGameID int64, name, phone, notes string) loanBody {
	t.Helper()
	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":%q,"borrowerPhone":%q,"notes":%q}`,
		eventGameID, name, phone, notes)
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST loans: %d %s", rec.Code, rec.Body.String())
	}
	var loan loanBody
	if err := json.NewDecoder(rec.Body).Decode(&loan); err != nil {
		t.Fatalf("decode loan: %v", err)
	}
	return loan
}

func TestLoanDesk_ListsEveryCopyAsAvailable(t *testing.T) {
	router, cookie, eventID, _ := loanFixture(t, 2)

	desk := readDesk(t, router, cookie, eventID)

	if len(desk.Copies) != 2 {
		t.Fatalf("copies = %d, want 2", len(desk.Copies))
	}
	first := desk.Copies[0]
	if first.OpenLoan != nil {
		t.Error("openLoan valorizzato su una copia mai prestata")
	}
	if !first.Bookable {
		t.Error("bookable = false su una copia creata senza dire niente")
	}
	if first.Copies != 2 {
		t.Errorf("copies = %d, want 2: serve alla UI per numerare le copie", first.Copies)
	}
	if first.Name != "Carcassonne" {
		t.Errorf("name = %q, want Carcassonne", first.Name)
	}
	if first.ActiveBookings == nil {
		t.Error("activeBookings = null, want []: una lista vuota non deve arrivare come null")
	}
	if desk.Returned == nil {
		t.Error("returned = null, want []")
	}
	if len(desk.Returned) != 0 {
		t.Errorf("returned = %d righe, want 0", len(desk.Returned))
	}
}

func TestLoanDesk_RequiresASession(t *testing.T) {
	router, _, eventID, eventGames := loanFixture(t, 1)

	for _, tc := range []struct{ name, method, path, body string }{
		{"lettura", http.MethodGet, fmt.Sprintf("/api/events/%d/loans", eventID), ""},
		{"consegna", http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID),
			fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"Anna","borrowerPhone":"3331234567"}`, eventGames[0].ID)},
		{"restituzione", http.MethodPost, "/api/loans/1/return", `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doLoanRequest(router, tc.method, tc.path, nil, tc.body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestLoanDesk_UnknownEventIs404(t *testing.T) {
	router, cookie, _, _ := loanFixture(t, 1)

	rec := doLoanRequest(router, http.MethodGet, "/api/events/9999/loans", cookie, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateLoan_ThenTheCopyIsOut(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)

	created := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "scatola ok")

	if created.BorrowerName != "Anna" {
		t.Errorf("borrowerName = %q, want Anna", created.BorrowerName)
	}
	if created.ReturnedAt != nil {
		t.Error("returnedAt valorizzato su un prestito appena aperto")
	}
	if created.LentAt == "" {
		t.Error("lentAt vuoto")
	}

	desk := readDesk(t, router, cookie, eventID)
	open := desk.Copies[0].OpenLoan
	if open == nil {
		t.Fatal("openLoan nil dopo la consegna")
	}
	if open.BorrowerName != "Anna" {
		t.Errorf("openLoan.borrowerName = %q, want Anna", open.BorrowerName)
	}
	if open.Notes == nil || *open.Notes != "scatola ok" {
		t.Errorf("openLoan.notes = %v, want \"scatola ok\"", open.Notes)
	}
}

func TestCreateLoan_WithoutBorrowerIs400(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"","borrowerPhone":""}`, eventGames[0].ID)

	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateLoan_OnACopyAlreadyOutIs409(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"Bruno","borrowerPhone":"3339999999"}`, eventGames[0].ID)
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateLoan_OnACopyOfAnotherEventIs404(t *testing.T) {
	router, cookie, _, eventGames := loanFixture(t, 1)
	body := fmt.Sprintf(`{"eventGameId":%d,"borrowerName":"Anna","borrowerPhone":"3331234567"}`, eventGames[0].ID)

	// L'evento 9999 non esiste: quella copia non gli appartiene.
	rec := doLoanRequest(router, http.MethodPost, "/api/events/9999/loans", cookie, body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestReturnLoan_MovesItToTheLog(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")

	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie,
		`{"notes":"manca una tessera"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	desk := readDesk(t, router, cookie, eventID)
	if desk.Copies[0].OpenLoan != nil {
		t.Error("la copia è ancora fuori dopo la restituzione")
	}
	if len(desk.Returned) != 1 {
		t.Fatalf("returned = %d righe, want 1", len(desk.Returned))
	}
	row := desk.Returned[0]
	if row.Notes == nil || *row.Notes != "manca una tessera" {
		t.Errorf("notes = %v", row.Notes)
	}
	if row.ReturnedAt == "" {
		t.Error("returnedAt vuoto nel log")
	}
	if row.GameName != "Carcassonne" {
		t.Errorf("gameName = %q, want Carcassonne", row.GameName)
	}
}

func TestReturnLoan_TwiceIs409(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	loan := lend(t, router, cookie, eventID, eventGames[0].ID, "Anna", "3331234567", "")
	path := fmt.Sprintf("/api/loans/%d/return", loan.ID)
	if rec := doLoanRequest(router, http.MethodPost, path, cookie, `{}`); rec.Code != http.StatusOK {
		t.Fatalf("first return: %d %s", rec.Code, rec.Body.String())
	}

	rec := doLoanRequest(router, http.MethodPost, path, cookie, `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestReturnLoan_UnknownIDIs404(t *testing.T) {
	router, cookie, _, _ := loanFixture(t, 1)

	rec := doLoanRequest(router, http.MethodPost, "/api/loans/9999/return", cookie, `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestLoanDesk_ShowsActiveBookingsOfACopy(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	// La prenotazione entra dall'endpoint pubblico, così nome e telefono
	// sono quelli veri.
	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
		eventGames[0].ID)
	if rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), nil, booking); rec.Code != http.StatusCreated {
		t.Fatalf("booking: %d %s", rec.Code, rec.Body.String())
	}

	desk := readDesk(t, router, cookie, eventID)

	if len(desk.Copies[0].ActiveBookings) != 1 {
		t.Fatalf("activeBookings = %d righe, want 1", len(desk.Copies[0].ActiveBookings))
	}
	row := desk.Copies[0].ActiveBookings[0]
	if row.Name != "Anna" || row.Phone != "3331234567" {
		t.Errorf("activeBookings[0] = %+v, want Anna / 3331234567", row)
	}
}

func TestCreateLoan_FromABookingLinksIt(t *testing.T) {
	router, cookie, eventID, eventGames := loanFixture(t, 1)
	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
		eventGames[0].ID)
	if rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), nil, booking); rec.Code != http.StatusCreated {
		t.Fatalf("booking: %d %s", rec.Code, rec.Body.String())
	}
	desk := readDesk(t, router, cookie, eventID)
	bookingID := desk.Copies[0].ActiveBookings[0].ID

	body := fmt.Sprintf(
		`{"eventGameId":%d,"bookingId":%d,"borrowerName":"Anna","borrowerPhone":"3331234567"}`,
		eventGames[0].ID, bookingID)
	rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/loans", eventID), cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var loan loanBody
	if err := json.NewDecoder(rec.Body).Decode(&loan); err != nil {
		t.Fatalf("decode loan: %v", err)
	}
	if loan.BookingID == nil || *loan.BookingID != bookingID {
		t.Fatalf("bookingId = %v, want %d", loan.BookingID, bookingID)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'Loan' -v 2>&1 | tail -30
```

Atteso: FAIL — le rotte non esistono, quindi 404 dove i test aspettano
200/201.

- [ ] **Step 3: Scrivi la serializzazione**

Crea `backend/internal/httpapi/loans_responses.go`:

```go
package httpapi

import (
	"context"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

// isoTime è il formato che l'app usa già per le date in uscita
// (toBookingAdminResponse): RFC3339, che il Date di JavaScript legge
// senza aiuto.
const isoTime = "2006-01-02T15:04:05Z07:00"

func toLoanResponse(l events.Loan) map[string]any {
	out := map[string]any{
		"id": l.ID, "eventGameId": l.EventGameID, "bookingId": l.BookingID,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"notes": l.Notes, "lentAt": l.LentAt.Format(isoTime),
		"returnedAt": nil,
	}
	if l.ReturnedAt != nil {
		out["returnedAt"] = l.ReturnedAt.Format(isoTime)
	}
	return out
}

// toOpenLoanResponse è il prestito come lo vede una riga di copia: il
// gioco lo sa già la riga che lo contiene, quindi qui non si ripete.
func toOpenLoanResponse(l events.LoanWithGame) map[string]any {
	return map[string]any{
		"id": l.ID, "borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "notes": l.Notes,
	}
}

// toReturnedLoanResponse è una riga del log della serata, che si legge da
// sola: il gioco ce l'ha dentro perché il log non è raggruppato per copia.
func toReturnedLoanResponse(l events.LoanWithGame) map[string]any {
	return map[string]any{
		"id": l.ID, "eventGameId": l.EventGameID, "gameId": l.GameID,
		"gameName": l.GameName, "copyIndex": l.CopyIndex,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "returnedAt": l.ReturnedAt.Format(isoTime),
		"notes": l.Notes,
	}
}

// toLoanDeskResponse è tutta la serata in una risposta: le copie con il
// loro prestito aperto (se c'è) e le prenotazioni attive che servono a
// precompilare la consegna, più il log dei prestiti chiusi. La pagina
// ricava "fuori" e "disponibili" dalla presenza di openLoan, senza un
// secondo giro.
func (s *Server) toLoanDeskResponse(ctx context.Context, eventID int64) (map[string]any, error) {
	eventGames, err := s.Events.ListEventGames(ctx, eventID)
	if err != nil {
		return nil, err
	}
	loans, err := s.Events.ListLoansForEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	bookings, err := s.Events.ListBookingsForEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	openByCopy := map[int64]events.LoanWithGame{}
	returned := []map[string]any{}
	for _, l := range loans {
		if l.ReturnedAt == nil {
			openByCopy[l.EventGameID] = l
			continue
		}
		returned = append(returned, toReturnedLoanResponse(l))
	}

	bookingsByCopy := map[int64][]map[string]any{}
	for _, b := range bookings {
		bookingsByCopy[b.EventGameID] = append(bookingsByCopy[b.EventGameID], map[string]any{
			"id": b.ID, "name": b.ParticipantName, "phone": b.ParticipantPhone,
		})
	}

	// Quante copie ha ogni gioco: la UI ne ha bisogno per decidere se
	// numerarle, perché con una copia sola "#1" è rumore.
	copiesPerGame := map[int64]int{}
	for _, eg := range eventGames {
		copiesPerGame[eg.GameID]++
	}

	// Un gioco si legge una volta anche se ha più copie, come in
	// toEventDetail.
	gameCache := map[int64]games.Game{}
	copies := make([]map[string]any, 0, len(eventGames))
	for _, eg := range eventGames {
		game, ok := gameCache[eg.GameID]
		if !ok {
			game, err = s.Games.GetGame(ctx, eg.GameID)
			if err != nil {
				return nil, err
			}
			gameCache[eg.GameID] = game
		}
		row := map[string]any{
			"eventGameId": eg.ID, "gameId": eg.GameID, "name": game.Name,
			"coverPath": game.CoverPath, "copyIndex": eg.CopyIndex,
			"copies": copiesPerGame[eg.GameID], "bookable": eg.Bookable,
			"seats": eg.Seats, "openLoan": nil,
			"activeBookings": orEmptyRows(bookingsByCopy[eg.ID]),
		}
		if l, out := openByCopy[eg.ID]; out {
			row["openLoan"] = toOpenLoanResponse(l)
		}
		copies = append(copies, row)
	}

	return map[string]any{"copies": copies, "returned": returned}, nil
}

// orEmptyRows manda [] invece di null: una lista vuota che arriva come
// null costringe ogni punto della UI a difendersi.
func orEmptyRows(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}
```

Nota sugli import: `isoTime` è una costante stringa e le date si
formattano coi metodi di `time.Time` che arrivano dallo store, quindi
questo file **non** importa `time`.

- [ ] **Step 4: Scrivi gli handler**

Crea `backend/internal/httpapi/loans_handlers.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"boardgames-manager/internal/events"
)

// listEventLoansHandler è l'unica lettura del banco prestiti.
func (s *Server) listEventLoansHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	// Un evento inesistente deve dare 404, non una serata vuota: senza
	// questo controllo la risposta sarebbe {copies: [], returned: []}, che
	// è indistinguibile da un evento senza giochi.
	if _, err := s.Events.GetEvent(r.Context(), eventID); errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	payload, err := s.toLoanDeskResponse(r.Context(), eventID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list loans")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

type createLoanRequest struct {
	EventGameID int64 `json:"eventGameId"`
	// BookingID c'è solo quando la consegna parte da una prenotazione.
	BookingID     *int64  `json:"bookingId"`
	BorrowerName  string  `json:"borrowerName"`
	BorrowerPhone string  `json:"borrowerPhone"`
	Notes         *string `json:"notes"`
}

func (s *Server) createLoanHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	var req createLoanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	loan, err := s.Events.LendCopy(r.Context(), eventID, events.LoanInput{
		EventGameID:   req.EventGameID,
		BookingID:     req.BookingID,
		BorrowerName:  req.BorrowerName,
		BorrowerPhone: req.BorrowerPhone,
		Notes:         req.Notes,
	})
	switch {
	case errors.Is(err, events.ErrBorrowerRequired):
		writeError(w, http.StatusBadRequest, "borrowerName and borrowerPhone are required")
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "copy or booking not found for this event")
	case errors.Is(err, events.ErrCopyAlreadyOut):
		writeError(w, http.StatusConflict, "questa copia è già in prestito")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create loan")
	default:
		writeJSON(w, http.StatusCreated, toLoanResponse(loan))
	}
}

type returnLoanRequest struct {
	// Notes nil lascia quelle scritte alla consegna: un corpo vuoto è il
	// caso normale, si restituisce senza avere niente da segnalare.
	Notes *string `json:"notes"`
}

func (s *Server) returnLoanHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid loan id")
		return
	}
	var req returnLoanRequest
	// Un corpo assente o vuoto è legittimo, quindi l'errore di decodifica
	// non è un errore: si prosegue con Notes nil.
	_ = json.NewDecoder(r.Body).Decode(&req)

	loan, err := s.Events.ReturnLoan(r.Context(), id, req.Notes)
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "loan not found")
	case errors.Is(err, events.ErrLoanAlreadyReturned):
		writeError(w, http.StatusConflict, "questo prestito è già stato chiuso")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not return loan")
	default:
		writeJSON(w, http.StatusOK, toLoanResponse(loan))
	}
}
```

- [ ] **Step 5: Registra le rotte**

In `backend/internal/httpapi/router.go`, dentro il gruppo `protected`,
sotto la riga di `listEventBookingsHandler`:

```go
		protected.Get("/api/events/{id}/loans", s.listEventLoansHandler)
		protected.Post("/api/events/{id}/loans", s.createLoanHandler)
		protected.Post("/api/loans/{id}/return", s.returnLoanHandler)
```

- [ ] **Step 6: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./... 2>&1 | tail -20
```

Atteso: PASS in tutti i package.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/loans_handlers.go \
        backend/internal/httpapi/loans_responses.go \
        backend/internal/httpapi/loans_handlers_test.go \
        backend/internal/httpapi/router.go
git commit -m "feat: expose the evening loan desk over the API"
```

---

## Task 5: `bookable` nell'API eventi e rifiuto di prenotare

**Files:**
- Modify: `backend/internal/httpapi/events_handlers.go`
- Modify: `backend/internal/httpapi/events_responses.go`
- Modify: `backend/internal/httpapi/events_bookings_handlers.go`
- Modify: `backend/internal/events/bookings.go`
- Test: `backend/internal/httpapi/events_handlers_test.go`
- Test: `backend/internal/events/bookings_test.go`

**Interfaces:**
- Consumes: da Task 1, `EventGameInput.Bookable *bool`,
  `EventGame.Bookable`. Da Task 2, `Store.LendCopy` e `events.LoanInput`
  (per il test del 409 sulla copia in prestito). Da Task 3,
  `events.ErrCopyOnLoan`. Da Task 4, gli helper di test `doLoanRequest` e
  `readDesk`, che vivono nello stesso package `httpapi_test`.
- Produces:
  - `eventGameRequest` guadagna `Bookable *bool \`json:"bookable"\``.
  - `events.ErrGameNotBookable error`.
  - la risposta di `GET /api/events/{id}` guadagna `bookable` per copia.
  - `PUT /api/events/{id}` risponde 409 su
    `ErrUnbookableWithActiveBookings` e su `ErrCopyOnLoan`.

- [ ] **Step 1: Scrivi i test che falliscono**

In fondo a `backend/internal/events/bookings_test.go`:

```go
func TestCreateBookingRefusesAnUnbookableCopy(t *testing.T) {
	store, gameStore := newTestStore(t)
	gameID := mustCreateGame(t, gameStore, "Love Letter")
	no := false
	event, err := store.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2030-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1, Bookable: &no}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	copies, err := store.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}

	_, err = store.CreateBooking(context.Background(), event.ID, copies[0].ID,
		"Anna", "anna@example.com", "3331234567", time.Date(2029, 12, 31, 20, 0, 0, 0, time.UTC))
	if !errors.Is(err, events.ErrGameNotBookable) {
		t.Fatalf("err = %v, want ErrGameNotBookable", err)
	}
}
```

Verifica che `errors` e `time` siano fra gli import di quel file: ci sono
già entrambi.

In fondo a `backend/internal/httpapi/events_handlers_test.go`, con gli
helper del package (`newTestServer`, `httpapi.NewRouter`,
`bootstrapFirstAdmin`, `createTestGameForEvent`) e i due helper nati in
Task 4 (`doLoanRequest`, `readDesk`), che sono nello stesso package di
test:

```go
// eventDetailGames è la lista giochi della risposta pubblica, quanto basta
// per guardare il flag.
type eventDetailGames struct {
	Games []struct {
		EventGameID int64 `json:"eventGameId"`
		Bookable    bool  `json:"bookable"`
	} `json:"games"`
}

func getEventDetailGames(t *testing.T, router http.Handler, eventID int64) eventDetailGames {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d", eventID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET event: %d %s", rec.Code, rec.Body.String())
	}
	var body eventDetailGames
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return body
}

func TestCreateEvent_BookableDefaultsToTrue(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	// Nessun campo "bookable" nel corpo: è il corpo che manderebbe un
	// client scritto prima di questa feature.
	body := fmt.Sprintf(
		`{"title":"Serata","eventDate":"2099-01-01","startTime":"21:00","games":[{"gameId":%d,"copies":1}]}`,
		gameID)
	rec := doLoanRequest(router, http.MethodPost, "/api/events", cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}

	detail := getEventDetailGames(t, router, created.ID)
	if len(detail.Games) != 1 {
		t.Fatalf("games = %d, want 1", len(detail.Games))
	}
	if !detail.Games[0].Bookable {
		t.Fatal("bookable = false: l'assenza del campo deve significare prenotabile")
	}
}

func TestCreateEvent_StoresBookableFalseAndRefusesTheBooking(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Love Letter")

	body := fmt.Sprintf(
		`{"title":"Serata","eventDate":"2099-01-01","startTime":"21:00","games":[{"gameId":%d,"copies":1,"bookable":false}]}`,
		gameID)
	rec := doLoanRequest(router, http.MethodPost, "/api/events", cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}

	detail := getEventDetailGames(t, router, created.ID)
	if detail.Games[0].Bookable {
		t.Fatal("bookable = true dopo averlo mandato false")
	}

	// E prenotarla non si può, nemmeno chiamando l'endpoint a mano.
	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
		detail.Games[0].EventGameID)
	rec = doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", created.ID), nil, booking)
	if rec.Code != http.StatusConflict {
		t.Fatalf("booking status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateEvent_UnbookableWithActiveBookingsIs409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Wingspan")
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	detail := getEventDetailGames(t, router, event.ID)
	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
		detail.Games[0].EventGameID)
	if rec := doLoanRequest(router, http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", event.ID), nil, booking); rec.Code != http.StatusCreated {
		t.Fatalf("booking: %d %s", rec.Code, rec.Body.String())
	}

	body := fmt.Sprintf(
		`{"title":"Serata","eventDate":"2099-01-01","startTime":"21:00","games":[{"gameId":%d,"copies":1,"bookable":false}]}`,
		gameID)
	rec := doLoanRequest(router, http.MethodPut, fmt.Sprintf("/api/events/%d", event.ID), cookie, body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateEvent_DroppingACopyOnLoanIs409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	// Il prestito va sull'ultima copia, quella che dropCopies sacrificherebbe.
	if _, err := server.Events.LendCopy(context.Background(), event.ID, events.LoanInput{
		EventGameID: eventGames[1].ID, BorrowerName: "Anna", BorrowerPhone: "3331234567",
	}); err != nil {
		t.Fatalf("lend: %v", err)
	}

	body := fmt.Sprintf(
		`{"title":"Serata","eventDate":"2099-01-01","startTime":"21:00","games":[{"gameId":%d,"copies":1}]}`,
		gameID)
	rec := doLoanRequest(router, http.MethodPut, fmt.Sprintf("/api/events/%d", event.ID), cookie, body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}
```

Verifica gli import di `events_handlers_test.go`: servono `context`,
`encoding/json`, `fmt`, `net/http`, `net/http/httptest`, `testing`,
`boardgames-manager/internal/events` e `boardgames-manager/internal/httpapi`.
Ci sono già tutti tranne, eventualmente, `events` — controlla e aggiungi.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/ ./internal/httpapi/ \
  -run 'Unbookable|Bookable|NotBookable' -v 2>&1 | tail -30
```

Atteso: FAIL — `ErrGameNotBookable` non esiste e `bookable` non compare
nella risposta.

- [ ] **Step 3: Rifiuta la prenotazione nello store**

In `backend/internal/events/bookings.go`, nel blocco `var (...)` degli
errori:

```go
	ErrGameNotBookable = errors.New("game is not bookable at this event")
```

E in `CreateBooking` sostituisci il blocco che conta le copie:

```go
	var eventGameCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM event_games WHERE id = ? AND event_id = ?`, eventGameID, eventID,
	).Scan(&eventGameCount); err != nil {
		return Booking{}, err
	}
	if eventGameCount == 0 {
		return Booking{}, ErrNotFound
	}
```

con:

```go
	// La stessa lettura fa due lavori: dice se la copia appartiene
	// all'evento e se è prenotabile. Un gioco lasciato sul tavolo per chi
	// arriva senza prenotazione non deve poter essere prenotato nemmeno
	// da chi chiama l'endpoint a mano.
	var bookable int
	err = s.db.QueryRowContext(ctx,
		`SELECT bookable FROM event_games WHERE id = ? AND event_id = ?`, eventGameID, eventID,
	).Scan(&bookable)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, err
	}
	if bookable == 0 {
		return Booking{}, ErrGameNotBookable
	}
```

Verifica che `database/sql` sia già fra gli import di `bookings.go`: c'è,
lo usa `getBookingByID`.

Il controllo è una lettura preventiva, non parte della `INSERT` atomica
che segue. Va bene: spegnere `bookable` mentre esistono prenotazioni
attive è già rifiutato da `ErrUnbookableWithActiveBookings` (Task 1),
quindi la finestra fra questa lettura e la scrittura non può produrre più
di una prenotazione di troppo, e solo se l'admin spegne il flag nello
stesso istante in cui qualcuno prenota una copia ancora libera.

- [ ] **Step 4: Passa `bookable` per l'API**

In `backend/internal/httpapi/events_handlers.go`, dentro
`type eventGameRequest struct`:

```go
	// Bookable assente significa prenotabile: è così che si comportava
	// l'app prima che questo campo esistesse, e un client vecchio non
	// deve cambiare comportamento.
	Bookable *bool `json:"bookable"`
```

E in `toEventGameInputs`:

```go
		out = append(out, events.EventGameInput{
			GameID: g.GameID, Copies: g.Copies, Bookable: g.Bookable,
		})
```

In `updateEventHandler`, dentro lo `switch`, prima del `case err != nil`:

```go
	case errors.Is(err, events.ErrUnbookableWithActiveBookings):
		writeError(w, http.StatusConflict, "questo gioco ha già prenotazioni: non si può togliere dalla prenotazione")
	case errors.Is(err, events.ErrCopyOnLoan):
		writeError(w, http.StatusConflict, "una copia è in prestito: farsela restituire prima di togliere le copie")
```

In `backend/internal/httpapi/events_responses.go`, `toEventGameSummary`
guadagna il parametro:

```go
func toEventGameSummary(eventGameID int64, g games.Game, copyIndex, seats, remaining int, bookable bool) map[string]any {
	return map[string]any{
		"eventGameId": eventGameID, "gameId": g.ID, "name": g.Name, "coverPath": g.CoverPath,
		"copyIndex": copyIndex, "seats": seats, "remaining": remaining, "weight": g.Weight,
		"bookable": bookable,
	}
}
```

e la sua unica chiamata, in `toEventDetail`:

```go
		gamesOut = append(gamesOut, toEventGameSummary(eg.ID, game, eg.CopyIndex, eg.Seats, remaining, eg.Bookable))
```

In `backend/internal/httpapi/events_bookings_handlers.go`, dentro lo
`switch` di `createBookingHandler`, prima del `case err != nil`:

```go
	case errors.Is(err, events.ErrGameNotBookable):
		writeError(w, http.StatusConflict, "questo gioco non è prenotabile: è a disposizione al tavolo")
```

- [ ] **Step 5: Lancia la suite e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./... 2>&1 | tail -20
```

Atteso: PASS in tutti i package.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/events/bookings.go \
        backend/internal/events/bookings_test.go \
        backend/internal/httpapi/events_handlers.go \
        backend/internal/httpapi/events_responses.go \
        backend/internal/httpapi/events_bookings_handlers.go \
        backend/internal/httpapi/events_handlers_test.go
git commit -m "feat: refuse to book a game the event only lends at the table"
```

---

## Task 6: La spunta "prenotabile" nel picker dei giochi

**Files:**
- Modify: `frontend/src/components/EventGamesPicker.vue`
- Modify: `frontend/src/views/EventAdminDetailView.vue`
- Modify: `frontend/src/app.css`

**Interfaces:**
- Consumes: da Task 5, `bookable` nella risposta di `GET /api/events/{id}`
  e nel corpo di `POST /api/events` / `PUT /api/events/{id}`.
- Produces: `SelectedGame` esportato da `EventGamesPicker.vue` diventa
  `{ gameId: number; copies: number; bookable: boolean }`.

`EventNewView.vue` non va toccato: manda `selectedGames.value` così com'è
nel corpo della richiesta, quindi il campo nuovo viaggia da sé, e il tipo
lo importa dal componente.

- [ ] **Step 1: Estendi il tipo e la logica del picker**

In `frontend/src/components/EventGamesPicker.vue`, cambia l'interfaccia
esportata:

```ts
export interface SelectedGame {
  gameId: number
  copies: number
  /**
   * Se il gioco si può prenotare in anticipo. Falso è il filler lasciato
   * sul tavolo per chi arriva senza prenotazione: sta nella serata, non
   * nel form pubblico.
   */
  bookable: boolean
}
```

`add()` nasce prenotabile, che è il caso normale:

```ts
function add(gameId: number) {
  emit('update:modelValue', [...props.modelValue, { gameId, copies: 1, bookable: true }])
}
```

E accanto a `setCopies`, il gemello per il flag:

```ts
function setBookable(gameId: number, bookable: boolean) {
  emit(
    'update:modelValue',
    props.modelValue.map((s) => (s.gameId === gameId ? { ...s, bookable } : s)),
  )
}
```

`selectedRows` va esteso per portarsi dietro il flag, perché il template
ne ha bisogno:

```ts
const selectedRows = computed(() =>
  props.modelValue
    .map((s) => ({ game: props.games.find((g) => g.id === s.gameId), copies: s.copies, bookable: s.bookable }))
    .filter(
      (row): row is { game: PickerGame; copies: number; bookable: boolean } =>
        row.game !== undefined,
    ),
)
```

- [ ] **Step 2: Aggiungi la spunta al template**

Dentro `<span class="game-select-quantity">`, subito dopo il
`<label class="game-select-copies">`:

```html
          <label class="game-select-bookable">
            <input
              type="checkbox"
              :checked="row.bookable"
              :disabled="occupiedFor(row.game.id) > 0"
              @change="setBookable(row.game.id, ($event.target as HTMLInputElement).checked)"
            />
            prenotabile
          </label>
```

E, sotto la riga, il motivo quando la spunta è bloccata — subito dopo lo
`<span v-if="capacityLabel(...)">`:

```html
          <span v-if="occupiedFor(row.game.id) > 0" class="game-select-locked">
            già prenotato: per togliere la prenotazione, annulla prima le
            prenotazioni attive
          </span>
```

Il `disabled` c'è perché il backend rifiuta lo stesso caso con un 409
(`ErrUnbookableWithActiveBookings`): meglio un campo che non si muove che
un errore dopo il salvataggio — la stessa scelta già fatta per il campo
copie.

- [ ] **Step 3: Carica il flag nella scheda admin dell'evento**

In `frontend/src/views/EventAdminDetailView.vue`, aggiungi il campo
all'interfaccia `EventGameInfo`:

```ts
  bookable: boolean
```

In `load()`, sostituisci il blocco che costruisce `copies`/`occupied` e
`selectedGames`:

```ts
  const copies: Record<number, number> = {}
  const occupied: Record<number, number> = {}
  const bookable: Record<number, boolean> = {}
  for (const g of event.games) {
    copies[g.gameId] = (copies[g.gameId] ?? 0) + 1
    if (g.seats - g.remaining > 0) {
      occupied[g.gameId] = (occupied[g.gameId] ?? 0) + 1
    }
    // Tutte le copie di un gioco condividono il flag: l'ultima che si
    // legge dice la stessa cosa della prima.
    bookable[g.gameId] = g.bookable
  }
  copiesByGame.value = copies
  occupiedCopiesByGame.value = occupied
  selectedGames.value = Object.entries(copies).map(([gameId, count]) => ({
    gameId: Number(gameId),
    copies: count,
    bookable: bookable[Number(gameId)] ?? true,
  }))
```

- [ ] **Step 4: Aggiungi gli stili**

In `frontend/src/app.css`, subito dopo la regola `.game-select-booked`:

```css
/* La spunta "prenotabile" sta sulla stessa riga del campo copie: sono le
   due cose che si decidono insieme mentre si scelgono i giochi. */
.game-select-bookable {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-family: 'Body', system-ui, sans-serif;
  font-size: 0.85rem;
  color: var(--ink-muted);
  white-space: nowrap;
}

.game-select-bookable input[type='checkbox'] {
  margin: 0;
}

.game-select-bookable:has(input:disabled) {
  opacity: 0.55;
}

.game-select-locked {
  flex-basis: 100%;
  font-family: 'Body', system-ui, sans-serif;
  font-size: 0.78rem;
  color: var(--ink-muted);
}
```

Se `--ink-muted` o `--card-line` non sono i nomi dei token in
`frontend/src/app.css`, usa quelli veri: cercali con
`grep -n "^  --" frontend/src/app.css | head -40`.

- [ ] **Step 5: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato senza errori di `vue-tsc`. Un errore su
`bookable` mancante in un letterale `SelectedGame` indica un punto non
aggiornato: sistemalo.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/EventGamesPicker.vue \
        frontend/src/views/EventAdminDetailView.vue \
        frontend/src/app.css
git commit -m "feat: choose per game whether an event takes bookings for it"
```

---

## Task 7: Il banco prestiti

**Files:**
- Create: `frontend/src/views/LoanDeskView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/views/EventAdminDetailView.vue`
- Modify: `frontend/src/app.css`

**Interfaces:**
- Consumes: da Task 4, i tre endpoint del banco prestiti con la loro
  forma di risposta. Dal codice esistente: `api` da `../api/client`,
  `ModalDialog` da `../components/ModalDialog.vue`,
  `formatEventDateTime` da `../utils/dates`.
- Produces: la rotta `admin-event-loans` su
  `/admin/events/:id/prestiti`.

- [ ] **Step 1: Scrivi la vista**

Crea `frontend/src/views/LoanDeskView.vue`:

```vue
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import ModalDialog from '../components/ModalDialog.vue'
import { formatEventDateTime } from '../utils/dates'

/** Una prenotazione attiva su una copia, come la manda il banco. */
interface CopyBooking {
  id: number
  name: string
  phone: string
}

interface OpenLoan {
  id: number
  borrowerName: string
  borrowerPhone: string
  lentAt: string
  notes: string | null
}

interface DeskCopy {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  copyIndex: number
  /** Quante copie di questo gioco ha la serata: sotto 2, "#1" è rumore. */
  copies: number
  bookable: boolean
  seats: number
  activeBookings: CopyBooking[]
  openLoan: OpenLoan | null
}

interface ReturnedLoan {
  id: number
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  borrowerName: string
  borrowerPhone: string
  lentAt: string
  returnedAt: string
  notes: string | null
}

interface LoanDesk {
  copies: DeskCopy[]
  returned: ReturnedLoan[]
}

interface EventHeader {
  title: string
  eventDate: string
  startTime: string
}

const route = useRoute()
const eventId = route.params.id as string

const desk = ref<LoanDesk>({ copies: [], returned: [] })
const eventTitle = ref('')
const eventWhen = ref('')
const error = ref('')
const loading = ref(true)

/** La copia che si sta consegnando, o null se la modale è chiusa. */
const lending = ref<DeskCopy | null>(null)
const borrowerName = ref('')
const borrowerPhone = ref('')
const lendNotes = ref('')
const lendError = ref('')
const lendSaving = ref(false)

/** Il prestito che si sta chiudendo, con la sua copia per l'etichetta. */
const returning = ref<{ copy: DeskCopy; loan: OpenLoan } | null>(null)
const returnNotes = ref('')
const returnError = ref('')
const returnSaving = ref(false)

const logOpen = ref(false)

const out = computed(() =>
  desk.value.copies.filter((c): c is DeskCopy & { openLoan: OpenLoan } => c.openLoan !== null),
)
const available = computed(() => desk.value.copies.filter((c) => c.openLoan === null))

/**
 * L'etichetta di una copia: il numero compare solo quando quel gioco ha
 * più di una copia nella serata.
 */
function copyLabel(copy: { name: string; copies: number; copyIndex: number }) {
  return copy.copies > 1 ? `${copy.name} #${copy.copyIndex}` : copy.name
}

function returnedLabel(row: ReturnedLoan) {
  // Il log non porta il conteggio delle copie, quindi lo cerca fra le
  // copie della serata; se la copia non c'è più, il numero si mostra
  // comunque perché senza di esso la riga sarebbe ambigua.
  const copy = desk.value.copies.find((c) => c.eventGameId === row.eventGameId)
  const copies = copy?.copies ?? 2
  return copies > 1 ? `${row.gameName} #${row.copyIndex}` : row.gameName
}

/** "da 25 minuti", che al tavolo è più utile di un orario. */
function since(iso: string) {
  const minutes = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000))
  if (minutes < 1) {
    return 'da poco'
  }
  if (minutes < 60) {
    return `da ${minutes} ${minutes === 1 ? 'minuto' : 'minuti'}`
  }
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest === 0
    ? `da ${hours} ${hours === 1 ? 'ora' : 'ore'}`
    : `da ${hours}h ${rest}′`
}

function clockTime(iso: string) {
  return new Date(iso).toLocaleTimeString('it-IT', { hour: '2-digit', minute: '2-digit' })
}

/**
 * Un nome che non compare fra i prenotati di una copia prenotata: si
 * consegna comunque — alle 21:30 chi non si è presentato non deve tenere
 * in ostaggio la scatola — ma l'avviso lo dice.
 */
const lendWarning = computed(() => {
  const copy = lending.value
  if (!copy || copy.activeBookings.length === 0) {
    return ''
  }
  const typed = borrowerName.value.trim().toLowerCase()
  if (typed === '') {
    return ''
  }
  if (copy.activeBookings.some((b) => b.name.trim().toLowerCase() === typed)) {
    return ''
  }
  return copy.activeBookings.length === 1
    ? `Questa copia è prenotata da ${copy.activeBookings[0].name}: la consegna a un altro nome resta registrata così.`
    : `Questa copia ha ${copy.activeBookings.length} prenotazioni: la consegna a un nome che non c'è resta registrata così.`
})

async function load() {
  const [event, loans] = await Promise.all([
    api.get<EventHeader>(`/events/${eventId}`),
    api.get<LoanDesk>(`/events/${eventId}/loans`),
  ])
  eventTitle.value = event.title
  eventWhen.value = formatEventDateTime(event.eventDate, event.startTime)
  desk.value = loans
}

function startLending(copy: DeskCopy) {
  lending.value = copy
  lendError.value = ''
  lendNotes.value = ''
  // Con una prenotazione sola non c'è niente da scegliere: si precompila.
  if (copy.activeBookings.length === 1) {
    borrowerName.value = copy.activeBookings[0].name
    borrowerPhone.value = copy.activeBookings[0].phone
  } else {
    borrowerName.value = ''
    borrowerPhone.value = ''
  }
}

function pickBooking(booking: CopyBooking) {
  borrowerName.value = booking.name
  borrowerPhone.value = booking.phone
}

/** La prenotazione da agganciare al prestito: quella col nome scelto. */
function matchedBooking(copy: DeskCopy) {
  const typed = borrowerName.value.trim().toLowerCase()
  return copy.activeBookings.find((b) => b.name.trim().toLowerCase() === typed) ?? null
}

async function submitLend() {
  const copy = lending.value
  if (!copy) {
    return
  }
  lendError.value = ''
  lendSaving.value = true
  try {
    const booking = matchedBooking(copy)
    await api.post(`/events/${eventId}/loans`, {
      eventGameId: copy.eventGameId,
      bookingId: booking ? booking.id : null,
      borrowerName: borrowerName.value,
      borrowerPhone: borrowerPhone.value,
      notes: lendNotes.value.trim() || null,
    })
    lending.value = null
    await load()
  } catch (e) {
    lendError.value = (e as Error).message
  } finally {
    lendSaving.value = false
  }
}

function startReturning(copy: DeskCopy, loan: OpenLoan) {
  returning.value = { copy, loan }
  returnNotes.value = loan.notes ?? ''
  returnError.value = ''
}

async function submitReturn() {
  const current = returning.value
  if (!current) {
    return
  }
  returnError.value = ''
  returnSaving.value = true
  try {
    await api.post(`/loans/${current.loan.id}/return`, {
      notes: returnNotes.value.trim() || null,
    })
    returning.value = null
    await load()
  } catch (e) {
    returnError.value = (e as Error).message
  } finally {
    returnSaving.value = false
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <router-link :to="`/admin/events/${eventId}`" class="back-link">&larr; Evento</router-link>

    <div class="page-head">
      <div class="page-head-text">
        <h1>Banco prestiti</h1>
        <p v-if="eventTitle" class="page-meta">{{ eventTitle }} · {{ eventWhen }}</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <template v-if="!loading && !error">
      <div class="panel-card">
        <div class="section-head">
          <h2>Fuori</h2>
          <span class="section-count">{{ out.length }}</span>
        </div>
        <p v-if="out.length === 0" class="empty-note">
          Nessun gioco è fuori: tutte le scatole sono al banco.
        </p>
        <ul v-else role="list" class="loan-list">
          <li v-for="copy in out" :key="copy.eventGameId">
            <button type="button" class="loan-row" @click="startReturning(copy, copy.openLoan)">
              <span class="loan-row-text">
                <span class="loan-row-title">{{ copyLabel(copy) }}</span>
                <span v-if="!copy.bookable" class="loan-tag">senza prenotazione</span>
                <span class="row-meta">
                  {{ copy.openLoan.borrowerName }} · {{ copy.openLoan.borrowerPhone }}
                </span>
                <span class="row-meta">
                  {{ since(copy.openLoan.lentAt) }}, dalle {{ clockTime(copy.openLoan.lentAt) }}
                </span>
                <span v-if="copy.openLoan.notes" class="loan-row-notes">
                  {{ copy.openLoan.notes }}
                </span>
              </span>
              <span class="loan-row-action">Restituito</span>
            </button>
          </li>
        </ul>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Disponibili</h2>
          <span class="section-count">{{ available.length }}</span>
        </div>
        <p v-if="available.length === 0 && desk.copies.length === 0" class="empty-note">
          Questa serata non ha ancora giochi:
          <router-link :to="`/admin/events/${eventId}`">aggiungili dalla scheda</router-link>.
        </p>
        <p v-else-if="available.length === 0" class="empty-note">
          Tutte le copie sono fuori.
        </p>
        <ul v-else role="list" class="loan-list">
          <li v-for="copy in available" :key="copy.eventGameId">
            <button type="button" class="loan-row" @click="startLending(copy)">
              <span class="loan-row-text">
                <span class="loan-row-title">{{ copyLabel(copy) }}</span>
                <span v-if="!copy.bookable" class="loan-tag">senza prenotazione</span>
                <span v-if="copy.activeBookings.length > 0" class="row-meta">
                  prenotata da
                  {{ copy.activeBookings.map((b) => b.name).join(', ') }}
                </span>
              </span>
              <span class="loan-row-action">Consegna</span>
            </button>
          </li>
        </ul>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Restituiti</h2>
          <span class="section-count">{{ desk.returned.length }}</span>
          <button
            v-if="desk.returned.length > 0"
            type="button"
            :aria-expanded="logOpen"
            @click="logOpen = !logOpen"
          >
            {{ logOpen ? 'Nascondi' : 'Mostra' }}
          </button>
        </div>
        <p v-if="desk.returned.length === 0" class="empty-note">
          Ancora nessuna restituzione in questa serata.
        </p>
        <ul v-else-if="logOpen" role="list" class="admin-list">
          <li v-for="row in desk.returned" :key="row.id">
            <div class="admin-row">
              <span class="admin-email booking-who">
                {{ returnedLabel(row) }}
                <span class="row-meta">
                  {{ row.borrowerName }} · {{ clockTime(row.lentAt) }}–{{ clockTime(row.returnedAt) }}
                </span>
                <span v-if="row.notes" class="row-meta">{{ row.notes }}</span>
              </span>
            </div>
          </li>
        </ul>
      </div>
    </template>

    <ModalDialog
      :open="lending !== null"
      :title="lending ? `Consegna: ${copyLabel(lending)}` : 'Consegna'"
      @close="lending = null"
    >
      <form v-if="lending" class="panel-form" @submit.prevent="submitLend">
        <div v-if="lending.activeBookings.length > 1" class="field-block">
          <span class="field-label">Prenotazioni su questa copia</span>
          <ul role="list" class="loan-booking-picks">
            <li v-for="b in lending.activeBookings" :key="b.id">
              <button type="button" @click="pickBooking(b)">
                {{ b.name }}
                <span class="row-meta">{{ b.phone }}</span>
              </button>
            </li>
          </ul>
        </div>

        <label>
          Nome
          <input v-model="borrowerName" required />
        </label>
        <label>
          Telefono
          <input v-model="borrowerPhone" required />
        </label>
        <label>
          <span>Note <span class="field-optional">(opzionale)</span></span>
          <textarea v-model="lendNotes"></textarea>
        </label>

        <p v-if="lendWarning" class="loan-warning">{{ lendWarning }}</p>
        <p v-if="lendError" class="error">{{ lendError }}</p>

        <div class="form-actions">
          <button type="submit" :disabled="lendSaving">
            {{ lendSaving ? 'Consegna…' : 'Consegna' }}
          </button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog
      :open="returning !== null"
      :title="returning ? `Restituzione: ${copyLabel(returning.copy)}` : 'Restituzione'"
      @close="returning = null"
    >
      <form v-if="returning" class="panel-form" @submit.prevent="submitReturn">
        <p class="loan-modal-meta">
          {{ returning.loan.borrowerName }} · {{ returning.loan.borrowerPhone }},
          {{ since(returning.loan.lentAt) }}
        </p>
        <label>
          <span>Note <span class="field-optional">(opzionale)</span></span>
          <textarea v-model="returnNotes"></textarea>
        </label>

        <p v-if="returnError" class="error">{{ returnError }}</p>

        <div class="form-actions">
          <button type="submit" :disabled="returnSaving">
            {{ returnSaving ? 'Registrazione…' : 'Restituito' }}
          </button>
        </div>
      </form>
    </ModalDialog>
  </div>
</template>
```

- [ ] **Step 2: Registra la rotta**

In `frontend/src/router/index.ts`, aggiungi l'import accanto agli altri:

```ts
import LoanDeskView from '../views/LoanDeskView.vue'
```

E la rotta fra le `children` di `/admin`, subito sotto
`admin-event-detail`:

```ts
        {
          path: 'events/:id/prestiti',
          name: 'admin-event-loans',
          component: LoanDeskView,
        },
```

- [ ] **Step 3: Metti il link nella scheda admin dell'evento**

In `frontend/src/views/EventAdminDetailView.vue`, dentro
`<div class="page-head-actions">`, come primo figlio (prima del link
"Vedi pagina pubblica"):

```html
        <router-link
          class="action-link is-compact"
          :to="{ name: 'admin-event-loans', params: { id: eventId } }"
        >
          Banco prestiti
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M4 7.5h9.5M4 12h9.5M4 16.5h6M17 9l3 3-3 3"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </router-link>
```

- [ ] **Step 4: Aggiungi gli stili**

In fondo a `frontend/src/app.css`:

```css
/* Banco prestiti: si usa in piedi, con una mano, quindi ogni riga è un
   bersaglio grande e l'azione sta a destra dove arriva il pollice. */
.loan-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.loan-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  width: 100%;
  min-height: 3.5rem;
  padding: 0.7rem 0.85rem;
  border: 1px solid var(--card-line);
  border-radius: 12px;
  background: var(--card);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.loan-row:hover,
.loan-row:focus-visible {
  border-color: var(--accent);
}

.loan-row-text {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.loan-row-title {
  font-weight: 600;
}

.loan-row-notes {
  font-family: 'Body', system-ui, sans-serif;
  font-size: 0.82rem;
  font-style: italic;
  color: var(--ink-muted);
}

.loan-row-action {
  flex-shrink: 0;
  font-family: 'Body', system-ui, sans-serif;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--accent);
  white-space: nowrap;
}

/* La pastiglia dei giochi che stanno nella serata ma non a prenotazione:
   stesso disegno di .seat-state sulla pagina pubblica, così la stessa
   informazione ha la stessa forma nei due posti in cui compare. */
.loan-tag {
  align-self: flex-start;
  padding: 0.14rem 0.45rem;
  border: 1px solid var(--card-line);
  border-radius: 999px;
  background: var(--bg);
  color: var(--ink-muted);
  font-family: 'Body', system-ui, sans-serif;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.loan-booking-picks {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.loan-booking-picks button {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  width: 100%;
  text-align: left;
}

.loan-warning {
  padding: 0.6rem 0.75rem;
  border-radius: 10px;
  background: var(--bg);
  border: 1px solid var(--card-line);
  font-family: 'Body', system-ui, sans-serif;
  font-size: 0.85rem;
  color: var(--ink-muted);
}

.loan-modal-meta {
  margin: 0 0 0.5rem;
  font-family: 'Body', system-ui, sans-serif;
  color: var(--ink-muted);
}
```

Prima di scrivere, verifica i nomi dei token con
`grep -n "^  --" frontend/src/app.css | head -40` e usa quelli veri
(`--card`, `--card-line`, `--ink-muted`, `--accent`, `--bg` sono i nomi
attesi, ma il file è la fonte).

- [ ] **Step 5: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato senza errori di `vue-tsc`.

- [ ] **Step 6: Prova a mano il ciclo completo**

```bash
docker compose up -d --build
```

Su http://localhost:8080: crea un evento con due copie di un gioco, apri
il banco prestiti dalla scheda, consegna una copia a un nome, verifica
che sparisca da "Disponibili" e compaia in "Fuori", restituiscila con una
nota, verifica che torni disponibile e che la riga compaia in
"Restituiti". Riprestala: deve funzionare.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/views/LoanDeskView.vue \
        frontend/src/router/index.ts \
        frontend/src/views/EventAdminDetailView.vue \
        frontend/src/app.css
git commit -m "feat: add the evening loan desk to the admin UI"
```

---

## Task 8: Le copie non prenotabili sulla pagina pubblica

**Files:**
- Modify: `frontend/src/views/EventDetailView.vue`
- Modify: `frontend/src/app.css`

**Interfaces:**
- Consumes: da Task 5, `bookable` per copia nella risposta di
  `GET /api/events/{id}`.
- Produces: niente per i task successivi.

- [ ] **Step 1: Estendi il tipo e la logica**

In `frontend/src/views/EventDetailView.vue`, aggiungi il campo a
`EventGameInfo`:

```ts
  bookable: boolean
```

E accanto a `seatsLabel`, la spiegazione della copia non prenotabile:

```ts
/**
 * Un gioco che la serata porta ma non mette a prenotazione: si vede —
 * "stasera c'è anche Love Letter" è informazione utile — ma non ha un
 * bottone, perché non c'è niente da prenotare.
 */
function tableOnly(g: EventGameInfo) {
  return !g.bookable
}
```

- [ ] **Step 2: Aggiorna il template**

Nel `<li>` della lista `event-games`, sostituisci le due righe di stato:

```html
            <p v-if="tableOnly(g)" class="seat-state">Senza prenotazione</p>
            <p v-else-if="isFull(g)" class="seat-state">Al completo</p>
            <p v-else-if="seatsLabel(g)">{{ seatsLabel(g) }}</p>
```

E il bottone Prenota guadagna la condizione:

```html
            <button
              v-if="!hasStarted && !isFull(g) && !tableOnly(g)"
              type="button"
              @click="startBooking(g.eventGameId)"
            >
              Prenota
            </button>
```

Sotto la lista, subito prima del `<p v-if="event.games.length === 0">`,
la nota che spiega la pastiglia — ma solo quando serve:

```html
      <p v-if="event.games.some(tableOnly)" class="table-note">
        I giochi segnati "senza prenotazione" sono a disposizione al tavolo:
        chiedili all'organizzatore quando arrivi.
      </p>
```

La classe `is-full` sul `<li>` resta legata a `isFull`: una copia non
prenotabile non è esaurita, è solo fuori dal listino, e non deve essere
sbiadita come le altre.

- [ ] **Step 3: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato senza errori.

- [ ] **Step 4: Prova a mano**

Con `docker compose up -d --build` attivo, togli la spunta "prenotabile"
a un gioco di un evento dalla scheda admin, salva, e apri la pagina
pubblica dell'evento: il gioco si vede con la pastiglia "Senza
prenotazione", senza bottone Prenota, e in fondo compare la nota.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/EventDetailView.vue frontend/src/app.css
git commit -m "feat: show table-only games on the public event page"
```

---

## Task 9: Documentazione e verifica completa

**Files:**
- Modify: `README.md`
- Modify: `DESIGN.md`

**Interfaces:**
- Consumes: tutto quello che precede.
- Produces: niente.

- [ ] **Step 1: Aggiorna il README**

In `README.md`, nella sezione che elenca cosa fa l'app (cercala con
`grep -n "prenot" README.md | head -20`), aggiungi le due funzionalità
visibili, con lo stesso tono delle voci vicine:

- durante la serata l'organizzatore registra dal **banco prestiti** chi
  ritira ogni copia e chi la restituisce, con l'orario e delle note; la
  lista dei disponibili mostra solo le scatole che non sono fuori;
- un gioco può stare in un evento **senza essere prenotabile**, per chi
  arriva senza prenotazione.

Non ci sono variabili d'ambiente nuove né passaggi di avvio diversi:
non toccare quelle sezioni.

- [ ] **Step 2: Aggiorna DESIGN.md**

In `DESIGN.md`, nella sezione dei componenti, documenta i due pattern
nuovi:

- **Riga-bersaglio del banco prestiti** (`.loan-row`): una riga intera è
  un bottone, alta almeno 3.5rem, con il testo a sinistra e il verbo
  dell'azione a destra. Serve dove si tocca in piedi, con una mano.
- **Pastiglia di stato** (`.loan-tag`, gemella di `.seat-state`): stesso
  disegno nelle due pagine in cui la stessa informazione compare, il
  banco prestiti e la pagina pubblica dell'evento.

- [ ] **Step 3: Lancia la verifica completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go vet ./... && \
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

```bash
cd frontend && npm run build
```

Atteso: `go vet` silenzioso, `go test` `ok` su tutti i package, build
frontend senza errori. Riporta l'output vero: nessuna affermazione di
"funziona" senza averlo visto.

- [ ] **Step 4: Commit**

```bash
git add README.md DESIGN.md
git commit -m "docs: document the loan desk and table-only games"
```

---

## Task 10: Pass `impeccable`

**Files:** quelli toccati nei Task 6, 7 e 8.

- [ ] **Step 1: Lancia `/impeccable` sulla superficie nuova**

Come richiede `CLAUDE.md`, è l'ultimo task del lavoro. Superfici da
passare: `frontend/src/views/LoanDeskView.vue` (la principale),
`frontend/src/components/EventGamesPicker.vue` e la lista giochi di
`frontend/src/views/EventDetailView.vue`.

Punti da guardare con attenzione, perché sono quelli che questa feature
introduce:

- il banco prestiti si usa da telefono, in piedi: bersagli grandi,
  gerarchia che mette "Fuori" prima di tutto, nessuno scorrimento
  orizzontale;
- le due modali (consegna e restituzione) hanno campi obbligatori e un
  avviso non bloccante: l'avviso deve essere annunciato ai lettori di
  schermo senza rubare il focus;
- la pastiglia "Senza prenotazione" deve leggersi come informazione, non
  come errore o come "esaurito";
- la spunta "prenotabile" disabilitata deve spiegare perché, non solo
  essere grigia.

- [ ] **Step 2: Applica quello che emerge, poi ri-verifica**

```bash
cd frontend && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add -A frontend/
git commit -m "polish: refine the loan desk after the impeccable pass"
```
