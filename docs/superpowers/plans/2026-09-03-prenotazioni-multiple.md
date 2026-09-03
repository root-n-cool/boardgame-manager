# Prenotazioni multiple sulla stessa copia — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permettere a più persone di prenotare singolarmente posti sullo
stesso tavolo (es. D&D), con un massimale deciso dall'admin, rendendo al
tempo stesso le copie multiple dello stesso gioco distinte e numerate.

**Architecture:** Un solo modello per tutti i casi. Il gioco in catalogo
guadagna `seats` ("posti prenotabili per copia", default 1);
`event_games` smette di avere `quantity` e diventa **una riga per copia**
(`copy_index`, `seats` fotografato dal catalogo); una prenotazione occupa
un posto su una copia specifica. `match_results` passa da `booking_id` a
`event_game_id`: un risultato per tavolo, così la classifica conta una
partita una volta sola anche con sei prenotazioni.

**Tech Stack:** Go 1.25 (chi, `modernc.org/sqlite`), Vue 3 + TypeScript +
Vite, migrazioni SQL forward-only su `embed.FS`.

**Spec:** `docs/superpowers/specs/2026-09-03-prenotazioni-multiple-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto (binario
  x86_64 su Mac arm64). Definisci questa funzione nella shell, **dalla
  root del repo**, e usala in ogni step di test:

  ```bash
  gotest() {
    docker run --rm -v "$(pwd)/backend:/app" \
      -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
      -w /app golang:1.25 go test "$@"
  }
  ```

  Non sostituire i volumi nominati `bgm-gomodcache` / `bgm-gocache`:
  senza di loro ogni run riparte da zero.
- **Frontend in locale**, senza Docker: `npm run build` in `frontend/`
  (fa anche il type-check con `vue-tsc`). Non esiste una suite di test
  frontend: la verifica di un task frontend è il build che passa.
- **Migrazioni forward-only**: mai modificare un file `.sql` già
  esistente. Il nuovo file è `0008_seats_and_copies.sql`.
- **Lingua UI: italiano**, stringhe dirette nei componenti, nessun i18n.
- **Dicitura obbligatoria: "posti prenotabili"**, mai "posti" da solo, in
  ogni stringa mostrata all'utente. Nel codice la colonna e i campi Go/TS
  si chiamano `seats`.
- **Nessuna nuova dipendenza** Go o npm.
- **Commit in inglese**, conventional commits (`feat:`, `fix:`, `docs:`).
- **Niente `go build` di verifica**: `gotest` compila già tutto.

## File Structure

**Backend — modifiche**

| File | Responsabilità dopo il lavoro |
|---|---|
| `backend/internal/db/migrations/0008_seats_and_copies.sql` | *nuovo* — `games.seats` + ricostruzione di `event_games`/`bookings`/`match_results`/`match_player_scores` |
| `backend/internal/games/store.go` | `Game.Seats`, `GameUpdate.Seats`, SQL di lettura/scrittura |
| `backend/internal/events/store.go` | `EventGame` a copie, `EventGameInput.Copies`, materializzazione copie, `RemainingCapacity` sui posti |
| `backend/internal/events/bookings.go` | capienza per posti, disdetta che non azzera il tavolo, `BookingWithGame` con copia |
| `backend/internal/events/matches.go` | risultato legato alla copia |
| `backend/internal/httpapi/events_handlers.go` | payload `copies` |
| `backend/internal/httpapi/events_responses.go` | `copyIndex`/`seats` nelle risposte evento, prenotazione, risultati |
| `backend/internal/httpapi/games_handlers.go`, `games_read_handlers.go`, `games_responses.go` | `seats` in creazione/aggiornamento/lettura gioco |

**Frontend — modifiche**

| File | Responsabilità |
|---|---|
| `frontend/src/views/GameNewView.vue` | campo "Posti prenotabili per copia" in creazione manuale e da BGG |
| `frontend/src/views/GameDetailView.vue` | modifica inline dei posti prenotabili (solo admin) |
| `frontend/src/components/EventGamesPicker.vue` | `copies` invece di `quantity`, posti prenotabili ereditati, capienza totale |
| `frontend/src/views/EventNewView.vue`, `EventAdminDetailView.vue` | adattamento al picker e raggruppamento prenotazioni per copia |
| `frontend/src/views/EventDetailView.vue` | una card per copia, numerazione, posti prenotabili liberi |
| `frontend/src/views/ManageBookingView.vue` | punteggio del tavolo, condiviso |
| `frontend/src/app.css` | classi nuove per le righe raggruppate |

**Documentazione:** `README.md`, `DESIGN.md`.

---

### Task 1: Migrazione 0008 e `seats` in catalogo

**Files:**
- Create: `backend/internal/db/migrations/0008_seats_and_copies.sql`
- Modify: `backend/internal/games/store.go` (`Game`, `GameUpdate`, `CreateGame`, `GetGame`, `ListGames`, `UpdateGame`)
- Test: `backend/internal/db/migrate_test.go`, `backend/internal/games/store_test.go`

**Interfaces:**
- Consumes: niente (primo task).
- Produces:
  - `games.Game.Seats int` — posti prenotabili per copia, sempre ≥ 1.
  - `games.GameUpdate.Seats *int` — `nil` = non toccare.
  - `games.Store.CreateGame(ctx, Game) (Game, error)` — normalizza
    `Seats < 1` a `1`, così ogni chiamante esistente resta valido.
  - Schema: `games.seats`, `event_games(id, event_id, game_id, copy_index, seats)`,
    `match_results(id, event_game_id, submitted_at)`.

- [ ] **Step 1: Scrivi il test della migrazione**

In `backend/internal/db/migrate_test.go`, aggiungi in fondo:

```go
func TestMigrate_EventGamesHasCopiesAndSeats(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Due copie dello stesso gioco nello stesso evento: era vietato dal
	// vecchio UNIQUE(event_id, game_id), ora è il caso normale.
	if _, err := conn.Exec(`INSERT INTO games (name, seats) VALUES ('D&D', 5)`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO events (title, event_date, start_time) VALUES ('Serata', '2026-10-01', '20:00')`); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO event_games (event_id, game_id, copy_index, seats) VALUES (1, 1, 1, 5), (1, 1, 2, 5)`); err != nil {
		t.Fatalf("insert two copies: %v", err)
	}

	var seats int
	if err := conn.QueryRow(`SELECT seats FROM games WHERE id = 1`).Scan(&seats); err != nil {
		t.Fatalf("read game seats: %v", err)
	}
	if seats != 5 {
		t.Fatalf("expected seats 5, got %d", seats)
	}

	// Il risultato partita ora appartiene alla copia, non alla prenotazione.
	if _, err := conn.Exec(`INSERT INTO match_results (event_game_id) VALUES (1)`); err != nil {
		t.Fatalf("insert match result on event_game: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO match_results (event_game_id) VALUES (1)`); err == nil {
		t.Fatal("expected a UNIQUE violation on event_game_id")
	}
}

func TestMigrate_GamesSeatsDefaultsToOne(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO games (name) VALUES ('Catan')`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	var seats int
	if err := conn.QueryRow(`SELECT seats FROM games WHERE name = 'Catan'`).Scan(&seats); err != nil {
		t.Fatalf("read seats: %v", err)
	}
	if seats != 1 {
		t.Fatalf("expected default seats 1, got %d", seats)
	}
}
```

- [ ] **Step 2: Fai fallire il test**

```bash
gotest -run 'TestMigrate_(EventGamesHasCopiesAndSeats|GamesSeatsDefaultsToOne)' ./internal/db/ -v
```

Atteso: FAIL — `table games has no column named seats`.

- [ ] **Step 3: Scrivi la migrazione**

`backend/internal/db/migrations/0008_seats_and_copies.sql`:

```sql
-- Posti prenotabili per copia. 1 = chi prenota si prende la copia (il
-- comportamento di sempre); più di 1 = tavolo aperto, dove ogni posto si
-- prenota a sé.
ALTER TABLE games ADD COLUMN seats INTEGER NOT NULL DEFAULT 1 CHECK (seats > 0);

-- event_games passa da "una riga con quantità" a "una riga per copia":
-- il vecchio UNIQUE(event_id, game_id) va togliuto, e in SQLite un vincolo
-- di tabella non si rimuove con ALTER. Le prenotazioni e i punteggi finora
-- registrati sono dati di sviluppo, quindi le quattro tabelle della catena
-- si ricreano vuote, in ordine inverso di dipendenza per non innescare
-- cascate su tabelle ancora vive. Gli eventi e il catalogo giochi restano:
-- gli eventi esistenti vanno solo ripopolati di giochi dalla loro scheda.
DROP TABLE match_player_scores;
DROP TABLE match_results;
DROP TABLE bookings;
DROP TABLE event_games;

CREATE TABLE event_games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    game_id INTEGER NOT NULL REFERENCES games(id),
    copy_index INTEGER NOT NULL CHECK (copy_index > 0),
    seats INTEGER NOT NULL CHECK (seats > 0),
    UNIQUE(event_id, game_id, copy_index)
);

CREATE TABLE bookings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    event_game_id INTEGER NOT NULL REFERENCES event_games(id) ON DELETE CASCADE,
    participant_name TEXT NOT NULL,
    participant_email TEXT NOT NULL,
    participant_phone TEXT NOT NULL,
    booking_code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled')),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX idx_one_active_booking_per_phone_per_event
    ON bookings(event_id, participant_phone) WHERE status = 'active';

-- Il risultato è del tavolo, non della prenotazione: con sei prenotazioni
-- sullo stesso tavolo la classifica deve contare una partita, non sei.
CREATE TABLE match_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_game_id INTEGER NOT NULL UNIQUE REFERENCES event_games(id) ON DELETE CASCADE,
    submitted_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE match_player_scores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    match_result_id INTEGER NOT NULL REFERENCES match_results(id) ON DELETE CASCADE,
    player_name TEXT NOT NULL,
    score INTEGER NOT NULL
);
```

- [ ] **Step 4: Verifica che il test della migrazione passi**

```bash
gotest -run 'TestMigrate' ./internal/db/ -v
```

Atteso: PASS su tutti i test del pacchetto `db`.

- [ ] **Step 5: Scrivi il test di `seats` nello store giochi**

In `backend/internal/games/store_test.go`, in fondo:

```go
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
```

- [ ] **Step 6: Fai fallire i test**

```bash
gotest -run 'TestCreateGame_DefaultsSeatsToOne|TestCreateGame_KeepsExplicitSeats|TestUpdateGame_ChangesSeats' ./internal/games/ -v
```

Atteso: FAIL di compilazione — `unknown field Seats in struct literal`.

- [ ] **Step 7: Aggiungi `seats` allo store giochi**

In `backend/internal/games/store.go`:

```go
type Game struct {
	ID              int64
	BGGID           *string
	Name            string
	Year            *int
	MinPlayers      *int
	MaxPlayers      *int
	PlaytimeMinutes *int
	Owner           *string
	CoverPath       *string
	// Seats è quante prenotazioni distinte accetta una copia di questo
	// gioco: 1 per un gioco da tavolo normale, N per un tavolo aperto
	// (D&D, giochi di ruolo). In UI si chiama "posti prenotabili".
	Seats     int
	CreatedAt time.Time
}

type GameUpdate struct {
	Owner           *string
	Year            *int
	MinPlayers      *int
	MaxPlayers      *int
	PlaytimeMinutes *int
	Seats           *int
}
```

In `CreateGame`, prima della `INSERT`:

```go
func (s *Store) CreateGame(ctx context.Context, g Game) (Game, error) {
	// Lo zero value di un int non è un numero di posti valido: i chiamanti
	// che non se ne curano (creazione da BGG, test) ottengono il default.
	if g.Seats < 1 {
		g.Seats = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO games (bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, seats)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.BGGID, g.Name, g.Year, g.MinPlayers, g.MaxPlayers, g.PlaytimeMinutes, g.Owner, g.CoverPath, g.Seats,
	)
```

In `GetGame` e in `ListGames` aggiungi `seats` alla `SELECT` e
`&g.Seats` alla `Scan`, subito dopo `cover_path` / `&g.CoverPath`:

```go
		`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, seats, created_at
		 FROM games WHERE id = ?`, id,
	).Scan(&g.ID, &g.BGGID, &g.Name, &g.Year, &g.MinPlayers, &g.MaxPlayers, &g.PlaytimeMinutes, &g.Owner, &g.CoverPath, &g.Seats, &createdAt)
```

```go
		`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, seats, created_at
		 FROM games ORDER BY id`,
```

```go
		if err := rows.Scan(&g.ID, &g.BGGID, &g.Name, &g.Year, &g.MinPlayers, &g.MaxPlayers, &g.PlaytimeMinutes, &g.Owner, &g.CoverPath, &g.Seats, &createdAt); err != nil {
```

In `UpdateGame`, dopo il blocco `PlaytimeMinutes`:

```go
	if upd.Seats != nil {
		current.Seats = *upd.Seats
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE games SET owner = ?, year = ?, min_players = ?, max_players = ?, playtime_minutes = ?, seats = ? WHERE id = ?`,
		current.Owner, current.Year, current.MinPlayers, current.MaxPlayers, current.PlaytimeMinutes, current.Seats, id,
	)
```

- [ ] **Step 8: Verifica il pacchetto giochi**

```bash
gotest ./internal/games/ ./internal/db/
```

Atteso: `ok` per entrambi.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/db/migrations/0008_seats_and_copies.sql \
  backend/internal/db/migrate_test.go \
  backend/internal/games/store.go backend/internal/games/store_test.go
git commit -m "feat: add bookable seats per game copy and rebuild the booking chain"
```

---

### Task 2: `event_games` come righe-copia

**Files:**
- Modify: `backend/internal/events/store.go` (`EventGame`, `EventGameInput`, `insertEventGames`, `listEventGames`, `GetEventGame`, `RemainingCapacity`)
- Test: `backend/internal/events/store_test.go`

**Interfaces:**
- Consumes: `games.Game.Seats` (Task 1); schema `event_games(copy_index, seats)` (Task 1).
- Produces:
  - `events.EventGame{ID int64; EventID int64; GameID int64; CopyIndex int; Seats int}`
  - `events.EventGameInput{GameID int64; Copies int}`
  - `events.Store.ListEventGames(ctx, eventID) ([]EventGame, error)` — ordinate per `game_id`, poi `copy_index`.
  - `events.Store.RemainingCapacity(ctx, eventGameID) (int, error)` — `seats − prenotazioni attive sulla copia`.
  - helper interni: `gameSeats(ctx, queryer, gameID) (int, error)` (→ `ErrGameNotFound`), `insertCopies(ctx, execer, eventID, gameID int64, seats, firstIndex, count int) error`.

- [ ] **Step 1: Scrivi il test delle copie**

In `backend/internal/events/store_test.go`, sostituisci l'helper
`mustCreateEvent` (usa il vecchio campo `Quantity`) con:

```go
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
```

Poi aggiungi in fondo al file:

```go
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

func TestCreateEvent_RejectsUnknownGame(t *testing.T) {
	eventStore, _ := newTestStore(t)
	ctx := context.Background()

	_, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: 999, Copies: 1}})
	if !errors.Is(err, events.ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}
```

Aggiorna anche `TestCreateAndGetEvent` nello stesso file: `Quantity: 2`
diventa `Copies: 2`, e l'assert finale diventa

```go
	if len(eventGames) != 2 || eventGames[0].GameID != gameID || eventGames[1].CopyIndex != 2 {
		t.Fatalf("unexpected event games: %+v", eventGames)
	}
```

- [ ] **Step 2: Fai fallire i test**

```bash
gotest ./internal/events/ -run 'TestCreateEvent_|TestRemainingCapacity_' -v
```

Atteso: FAIL di compilazione — `unknown field Copies`, `eg.CopyIndex undefined`.

- [ ] **Step 3: Riscrivi le strutture e gli helper nello store**

In `backend/internal/events/store.go` sostituisci `EventGame` e
`EventGameInput`:

```go
// EventGame è una singola copia di un gioco dentro un evento. Due copie
// dello stesso gioco sono due righe: chi prenota sa su quale sta finendo,
// e l'organizzatore sa chi siede a quale tavolo.
type EventGame struct {
	ID        int64
	EventID   int64
	GameID    int64
	CopyIndex int
	// Seats è la fotografia dei posti prenotabili del gioco al momento in
	// cui la copia è entrata nell'evento: cambiare il catalogo dopo non
	// muove la capienza di una serata già aperta alle prenotazioni.
	Seats int
}

// EventGameInput è come l'admin descrive un gioco: quante copie ne porta.
// I posti prenotabili non si scelgono qui, si leggono dal catalogo.
type EventGameInput struct {
	GameID int64
	Copies int
}
```

Sostituisci `insertEventGames` e aggiungi i due helper:

```go
func insertEventGames(ctx context.Context, tx execQueryer, eventID int64, gamesInput []EventGameInput) error {
	for _, g := range gamesInput {
		seats, err := gameSeats(ctx, tx, g.GameID)
		if err != nil {
			return err
		}
		if err := insertCopies(ctx, tx, eventID, g.GameID, seats, 1, g.Copies); err != nil {
			return err
		}
	}
	return nil
}

// insertCopies scrive `count` copie consecutive a partire da firstIndex.
func insertCopies(ctx context.Context, tx execer, eventID, gameID int64, seats, firstIndex, count int) error {
	for i := 0; i < count; i++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO event_games (event_id, game_id, copy_index, seats) VALUES (?, ?, ?, ?)`,
			eventID, gameID, firstIndex+i, seats,
		); err != nil {
			return err
		}
	}
	return nil
}

// gameSeats fa doppio servizio: legge i posti prenotabili del gioco e, se
// il gioco non esiste, è il punto in cui l'input viene rifiutato.
func gameSeats(ctx context.Context, q queryer, gameID int64) (int, error) {
	var seats int
	err := q.QueryRowContext(ctx, `SELECT seats FROM games WHERE id = ?`, gameID).Scan(&seats)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrGameNotFound
	}
	return seats, err
}
```

- [ ] **Step 4: Aggiorna letture e capienza**

Sempre in `store.go`, sostituisci `listEventGames`, `GetEventGame` e
`RemainingCapacity`:

```go
func listEventGames(ctx context.Context, q queryer, eventID int64) ([]EventGame, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, event_id, game_id, copy_index, seats FROM event_games
		 WHERE event_id = ? ORDER BY game_id, copy_index`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventGame
	for rows.Next() {
		var eg EventGame
		if err := rows.Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.CopyIndex, &eg.Seats); err != nil {
			return nil, err
		}
		out = append(out, eg)
	}
	return out, rows.Err()
}

func (s *Store) GetEventGame(ctx context.Context, id int64) (EventGame, error) {
	var eg EventGame
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, game_id, copy_index, seats FROM event_games WHERE id = ?`, id,
	).Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.CopyIndex, &eg.Seats)
	if errors.Is(err, sql.ErrNoRows) {
		return EventGame{}, ErrNotFound
	}
	return eg, err
}

func (s *Store) RemainingCapacity(ctx context.Context, eventGameID int64) (int, error) {
	var remaining int
	err := s.db.QueryRowContext(ctx,
		`SELECT eg.seats - (
			SELECT COUNT(*) FROM bookings b WHERE b.event_game_id = eg.id AND b.status = 'active'
		 ) FROM event_games eg WHERE eg.id = ?`, eventGameID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return remaining, err
}
```

- [ ] **Step 5: Verifica che i nuovi test passino**

```bash
gotest ./internal/events/ -run 'TestCreateEvent_|TestRemainingCapacity_|TestCreateAndGetEvent' -v
```

Atteso: PASS. Il resto del pacchetto è ancora rotto (`UpdateEvent` usa
`Quantity`): è il Task 3.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/events/store.go backend/internal/events/store_test.go
git commit -m "feat: model an event game as one row per copy"
```

---

### Task 3: `UpdateEvent` che aggiunge e toglie copie

**Files:**
- Modify: `backend/internal/events/store.go` (`UpdateEvent`, rimozione di `activeBookingCountsByGame`)
- Test: `backend/internal/events/store_test.go`

**Interfaces:**
- Consumes: `EventGameInput.Copies`, `gameSeats`, `insertCopies`, `listEventGames` (Task 2).
- Produces:
  - `events.Store.UpdateEvent(ctx, id int64, title string, description *string, eventDate, startTime string, gamesInput []EventGameInput) (Event, error)` — firma invariata, semantica a copie.
  - `events.ErrQuantityBelowActiveBookings` — invariato: si alza quando le copie richieste sono meno di quelle occupate.
  - helper interno `occupiedCopies(ctx, queryer, eventID) (map[int64]int, error)` — prenotazioni attive per `event_game_id`.

- [ ] **Step 1: Scrivi i test**

In `backend/internal/events/store_test.go`, in fondo:

```go
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
```

Aggiorna anche i test di `UpdateEvent` già presenti nel file che
costruiscono `events.EventGameInput{..., Quantity: N}`: il campo diventa
`Copies: N`. Cercali con
`grep -n "Quantity" backend/internal/events/store_test.go`.

- [ ] **Step 2: Fai fallire i test**

```bash
gotest ./internal/events/ -run 'TestUpdateEvent_' -v
```

Atteso: FAIL di compilazione — `g.Quantity undefined`.

- [ ] **Step 3: Riscrivi `UpdateEvent`**

In `backend/internal/events/store.go`, sostituisci il corpo di
`UpdateEvent` dopo il controllo `affected == 0`, e rimpiazza
`activeBookingCountsByGame` con `occupiedCopies` e `dropCopies`:

```go
	existing, err := listEventGames(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}
	// Le copie arrivano già ordinate per (game_id, copy_index): raggrupparle
	// per gioco conserva quell'ordine, che è quello in cui si sacrificano
	// dalla coda.
	copiesByGame := map[int64][]EventGame{}
	for _, eg := range existing {
		copiesByGame[eg.GameID] = append(copiesByGame[eg.GameID], eg)
	}

	occupied, err := occupiedCopies(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}

	// Un passaggio a parte per validare tutti i giochi richiesti e leggere
	// i posti prenotabili: se uno non esiste, si esce prima di scrivere.
	seatsByGame := map[int64]int{}
	wanted := map[int64]int{}
	for _, g := range gamesInput {
		seats, err := gameSeats(ctx, tx, g.GameID)
		if err != nil {
			return Event{}, err
		}
		seatsByGame[g.GameID] = seats
		wanted[g.GameID] = g.Copies
	}

	// Giochi spariti dalla selezione: via tutte le loro copie, se libere.
	for gameID, copies := range copiesByGame {
		if _, stillWanted := wanted[gameID]; !stillWanted {
			if err := dropCopies(ctx, tx, copies, occupied, len(copies)); err != nil {
				return Event{}, err
			}
		}
	}

	for _, g := range gamesInput {
		copies := copiesByGame[g.GameID]
		switch {
		case g.Copies < len(copies):
			if err := dropCopies(ctx, tx, copies, occupied, len(copies)-g.Copies); err != nil {
				return Event{}, err
			}
		case g.Copies > len(copies):
			// I numeri delle copie sono etichette stabili, non posizioni:
			// le nuove partono dopo la più alta esistente, anche se in
			// mezzo c'è un buco lasciato da una copia eliminata.
			next := 1
			if len(copies) > 0 {
				next = copies[len(copies)-1].CopyIndex + 1
			}
			if err := insertCopies(ctx, tx, id, g.GameID, seatsByGame[g.GameID], next, g.Copies-len(copies)); err != nil {
				return Event{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, id)
}

// dropCopies elimina `count` copie partendo dalla più alta, saltando quelle
// con prenotazioni attive. Se le copie libere non bastano l'operazione
// fallisce e la transazione del chiamante viene annullata: meglio un errore
// che una prenotazione cancellata a cascata sotto il naso di chi l'ha fatta.
func dropCopies(ctx context.Context, tx execer, copies []EventGame, occupied map[int64]int, count int) error {
	dropped := 0
	for i := len(copies) - 1; i >= 0 && dropped < count; i-- {
		if occupied[copies[i].ID] > 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_games WHERE id = ?`, copies[i].ID); err != nil {
			return err
		}
		dropped++
	}
	if dropped < count {
		return ErrQuantityBelowActiveBookings
	}
	return nil
}

// occupiedCopies conta le prenotazioni attive di ogni copia dell'evento.
func occupiedCopies(ctx context.Context, q queryer, eventID int64) (map[int64]int, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT eg.id, COUNT(b.id) FROM event_games eg
		 LEFT JOIN bookings b ON b.event_game_id = eg.id AND b.status = 'active'
		 WHERE eg.event_id = ? GROUP BY eg.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]int{}
	for rows.Next() {
		var eventGameID int64
		var count int
		if err := rows.Scan(&eventGameID, &count); err != nil {
			return nil, err
		}
		out[eventGameID] = count
	}
	return out, rows.Err()
}
```

Cancella la vecchia funzione `activeBookingCountsByGame`: non ha più
chiamanti.

- [ ] **Step 4: Verifica**

```bash
gotest ./internal/events/ -run 'TestUpdateEvent_' -v
```

Atteso: PASS su tutti. `bookings.go` e `matches.go` compilano ancora
(usano solo `event_game_id`), ma i loro test sulla capienza vanno
aggiornati nel Task 4.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/store.go backend/internal/events/store_test.go
git commit -m "feat: add and drop event game copies on update"
```

---

### Task 4: Più prenotazioni sullo stesso tavolo

**Files:**
- Modify: `backend/internal/events/bookings.go` (`CreateBooking`, `BookingWithGame`, `ListBookingsForEvent`, nuovo `CountActiveBookingsForEventGame`)
- Test: `backend/internal/events/bookings_test.go`

**Interfaces:**
- Consumes: `event_games.seats` (Task 1), `EventGameInput.Copies` (Task 2).
- Produces:
  - `events.Store.CreateBooking(ctx, eventID, eventGameID int64, name, email, phone string, now time.Time) (Booking, error)` — firma invariata, capienza sui posti.
  - `events.BookingWithGame{Booking; GameID int64; GameName string; CopyIndex int; Seats int}`
  - `events.Store.CountActiveBookingsForEventGame(ctx, eventGameID int64) (int, error)`

- [ ] **Step 1: Scrivi i test**

In `backend/internal/events/bookings_test.go`, in fondo:

```go
func TestCreateBooking_FillsAllSeatsOfATable(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 3)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	codes := map[string]bool{}
	for i, phone := range []string{"3330000001", "3330000002", "3330000003"} {
		b, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
			fmt.Sprintf("Giocatore %d", i+1), fmt.Sprintf("p%d@example.com", i+1), phone, now)
		if err != nil {
			t.Fatalf("booking %d: %v", i+1, err)
		}
		if codes[b.BookingCode] {
			t.Fatalf("duplicate booking code %q", b.BookingCode)
		}
		codes[b.BookingCode] = true
	}

	// Quarto posto su un tavolo da tre: pieno.
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Quarto", "quarto@example.com", "3330000004", now)
	if !errors.Is(err, events.ErrGameSoldOut) {
		t.Fatalf("expected ErrGameSoldOut, got %v", err)
	}
}

func TestCreateBooking_PhoneConstraintHoldsInsideATable(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario", "mario@example.com", "3331111111", now); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario di nuovo", "mario@example.com", "3331111111", now)
	if !errors.Is(err, events.ErrDuplicatePhoneBooking) {
		t.Fatalf("expected ErrDuplicatePhoneBooking, got %v", err)
	}
}

func TestListBookingsForEvent_CarriesTheCopy(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 4)
	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[1].ID,
		"Mario", "mario@example.com", "3331111111", now); err != nil {
		t.Fatalf("booking: %v", err)
	}

	list, err := eventStore.ListBookingsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(list))
	}
	if list[0].CopyIndex != 2 || list[0].Seats != 4 || list[0].GameName != "D&D" {
		t.Fatalf("unexpected booking row: %+v", list[0])
	}
}

func TestCountActiveBookingsForEventGame(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for i, phone := range []string{"3330000001", "3330000002"} {
		if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
			fmt.Sprintf("G%d", i), fmt.Sprintf("g%d@example.com", i), phone, now); err != nil {
			t.Fatalf("booking: %v", err)
		}
	}

	count, err := eventStore.CountActiveBookingsForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}
```

Aggiorna gli `EventGameInput{..., Quantity: N}` già presenti nel file in
`Copies: N` (`grep -n "Quantity" backend/internal/events/bookings_test.go`).
Assicurati che `fmt` sia fra gli import del file.

- [ ] **Step 2: Fai fallire i test**

```bash
gotest ./internal/events/ -run 'TestCreateBooking_FillsAllSeats|TestCreateBooking_PhoneConstraintHolds|TestListBookingsForEvent_CarriesTheCopy|TestCountActiveBookingsForEventGame' -v
```

Atteso: FAIL — la terza prenotazione riceve `ErrGameSoldOut` (la
capienza legge ancora `quantity`, colonna che non esiste più) e
`list[0].CopyIndex` non compila.

- [ ] **Step 3: Capienza sui posti prenotabili**

In `backend/internal/events/bookings.go`, dentro `CreateBooking`,
sostituisci la `INSERT` atomica e il suo commento:

```go
	// Single atomic statement: the WHERE clause re-checks capacity as part of
	// the same write, so SQLite's write-lock makes this race-safe against
	// concurrent bookings for the last remaining seat — no separate
	// check-then-insert window. A collision on the (event_id, phone) unique
	// index (a duplicate booking that slipped past an earlier read) surfaces
	// here too and is mapped to ErrDuplicatePhoneBooking below.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		 SELECT ?, ?, ?, ?, ?, ?, 'active'
		 WHERE (SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active') <
		       (SELECT seats FROM event_games WHERE id = ?)`,
		eventID, eventGameID, name, email, phone, code, eventGameID, eventGameID,
	)
```

- [ ] **Step 4: Copia nelle righe di prenotazione**

Sempre in `bookings.go`, sostituisci `BookingWithGame` e
`ListBookingsForEvent`, e aggiungi il contatore:

```go
type BookingWithGame struct {
	Booking
	GameID   int64
	GameName string
	// CopyIndex e Seats servono a chi legge per tavolo: l'organizzatore
	// vuole vedere "D&D #2 — 3 di 5 posti prenotabili", non una lista piatta.
	CopyIndex int
	Seats     int
}

func (s *Store) ListBookingsForEvent(ctx context.Context, eventID int64) ([]BookingWithGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.event_id, b.event_game_id, b.participant_name, b.participant_email, b.participant_phone,
		        b.booking_code, b.status, b.created_at, g.id, g.name, eg.copy_index, eg.seats
		 FROM bookings b
		 JOIN event_games eg ON b.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 WHERE b.event_id = ? AND b.status = 'active'
		 ORDER BY eg.game_id, eg.copy_index, b.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookingWithGame
	for rows.Next() {
		var bg BookingWithGame
		var createdAt string
		if err := rows.Scan(&bg.ID, &bg.EventID, &bg.EventGameID, &bg.ParticipantName, &bg.ParticipantEmail,
			&bg.ParticipantPhone, &bg.BookingCode, &bg.Status, &createdAt, &bg.GameID, &bg.GameName,
			&bg.CopyIndex, &bg.Seats); err != nil {
			return nil, err
		}
		bg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, bg)
	}
	return out, rows.Err()
}

// CountActiveBookingsForEventGame dice quante persone siedono a un tavolo.
// La pagina pubblica se ne serve per spiegare che il punteggio è condiviso.
func (s *Store) CountActiveBookingsForEventGame(ctx context.Context, eventGameID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active'`, eventGameID,
	).Scan(&count)
	return count, err
}
```

- [ ] **Step 5: Verifica**

```bash
gotest ./internal/events/ -run 'TestCreateBooking|TestListBookingsForEvent|TestCountActiveBookings' -v
```

Atteso: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/events/bookings.go backend/internal/events/bookings_test.go
git commit -m "feat: let several people book separate seats at the same table"
```

---

### Task 5: Il punteggio è del tavolo

**Files:**
- Modify: `backend/internal/events/matches.go` (`MatchResult`, `SubmitMatchResult`, `getMatchResultByID`, `GetMatchResultForEventGame`, `ListMatchResultsForEvent`), `backend/internal/events/bookings.go` (`cancelBooking`), `backend/internal/leaderboard/leaderboard.go` (`GetLeaderboard`)
- Test: `backend/internal/events/matches_test.go`, `backend/internal/events/bookings_test.go`, `backend/internal/leaderboard/leaderboard_test.go`

**Interfaces:**
- Consumes: `match_results.event_game_id` (Task 1), `Booking.EventGameID`, `CountActiveBookingsForEventGame` (Task 4).
- Produces:
  - `events.MatchResult{ID int64; EventGameID int64; SubmittedAt time.Time; Players []PlayerScore}`
  - `events.Store.SubmitMatchResult(ctx, bookingID int64, code string, players []PlayerScore) (MatchResult, error)` — firma invariata.
  - `events.Store.GetMatchResultForEventGame(ctx, eventGameID int64) (*MatchResult, error)` — sostituisce `GetMatchResultForBooking`; `nil, nil` se non giocato.
  - `events.EventGameMatchResult{EventGameID int64; GameID int64; GameName string; CopyIndex int; Players []PlayerScore}`
  - `events.Store.ListMatchResultsForEvent(ctx, eventID int64) ([]EventGameMatchResult, error)`

- [ ] **Step 1: Scrivi i test**

In `backend/internal/events/matches_test.go`, in fondo:

```go
func TestSubmitMatchResult_IsSharedByTheWholeTable(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 3)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	second, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Luigi", "luigi@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("second booking: %v", err)
	}

	if _, err := eventStore.SubmitMatchResult(ctx, first.ID, first.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}}); err != nil {
		t.Fatalf("submit by first: %v", err)
	}

	// Il compagno di tavolo vede lo stesso risultato...
	shared, err := eventStore.GetMatchResultForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get shared: %v", err)
	}
	if shared == nil || len(shared.Players) != 1 || shared.Players[0].Name != "Mario" {
		t.Fatalf("unexpected shared result: %+v", shared)
	}

	// ...e può correggerlo: uno solo per tavolo, non uno a testa.
	if _, err := eventStore.SubmitMatchResult(ctx, second.ID, second.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}, {Name: "Luigi", Score: 12}}); err != nil {
		t.Fatalf("submit by second: %v", err)
	}
	corrected, err := eventStore.GetMatchResultForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get corrected: %v", err)
	}
	if corrected == nil || len(corrected.Players) != 2 {
		t.Fatalf("expected the table result to be replaced, got %+v", corrected)
	}

	results, err := eventStore.ListMatchResultsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result for the table, got %d", len(results))
	}
	if results[0].CopyIndex != 1 || results[0].GameName != "D&D" || len(results[0].Players) != 2 {
		t.Fatalf("unexpected admin row: %+v", results[0])
	}
}

func TestListMatchResultsForEvent_OneRowPerCopy(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Carcassonne")
	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Copies: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	for i, eg := range eventGames {
		b, err := eventStore.CreateBooking(ctx, event.ID, eg.ID,
			fmt.Sprintf("G%d", i), fmt.Sprintf("g%d@example.com", i),
			fmt.Sprintf("33300000%02d", i), now)
		if err != nil {
			t.Fatalf("booking on copy %d: %v", i+1, err)
		}
		if _, err := eventStore.SubmitMatchResult(ctx, b.ID, b.BookingCode,
			[]events.PlayerScore{{Name: "Vincitore", Score: 40 + i}}); err != nil {
			t.Fatalf("submit on copy %d: %v", i+1, err)
		}
	}

	results, err := eventStore.ListMatchResultsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].CopyIndex != 1 || results[1].CopyIndex != 2 {
		t.Fatalf("unexpected copy indexes: %+v", results)
	}
}
```

In `backend/internal/events/bookings_test.go`, in fondo:

```go
func TestCancelBooking_KeepsTheTableResultWhileSomeoneRemains(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 3)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	second, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Luigi", "luigi@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("second booking: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, first.ID, first.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Chi si sfila non porta via il punteggio degli altri.
	if _, err := eventStore.CancelBooking(ctx, first.ID, first.BookingCode); err != nil {
		t.Fatalf("cancel first: %v", err)
	}
	result, err := eventStore.GetMatchResultForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result == nil {
		t.Fatal("expected the table result to survive one cancellation")
	}

	// Svuotato il tavolo, il risultato non ha più senso e sparisce.
	if _, err := eventStore.CancelBooking(ctx, second.ID, second.BookingCode); err != nil {
		t.Fatalf("cancel second: %v", err)
	}
	result, err = eventStore.GetMatchResultForEventGame(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("get result after last cancel: %v", err)
	}
	if result != nil {
		t.Fatalf("expected the result to be gone, got %+v", result)
	}
}
```

Aggiorna anche i test già presenti che chiamano
`GetMatchResultForBooking(ctx, booking.ID)`: diventano
`GetMatchResultForEventGame(ctx, booking.EventGameID)`
(`grep -rn "GetMatchResultForBooking" backend/`).

- [ ] **Step 2: Fai fallire i test**

```bash
gotest ./internal/events/ -run 'TestSubmitMatchResult_IsShared|TestListMatchResultsForEvent_OneRowPerCopy|TestCancelBooking_KeepsTheTableResult' -v
```

Atteso: FAIL di compilazione — `GetMatchResultForEventGame undefined`.

- [ ] **Step 3: Sposta il risultato sulla copia**

In `backend/internal/events/matches.go`:

```go
type MatchResult struct {
	ID          int64
	EventGameID int64
	SubmittedAt time.Time
	Players     []PlayerScore
}
```

Sostituisci il commento e il corpo di `SubmitMatchResult` dalla
transazione in avanti:

```go
// SubmitMatchResult creates or replaces the MatchResult for the copy the
// booking sits on. A copy can only ever have one MatchResult
// (event_game_id is UNIQUE): calling this again for any booking at the
// same table replaces the previously submitted players instead of adding
// a second result — this is both "il punteggio è sempre modificabile"
// (design spec) and what keeps the leaderboard from counting one game of
// D&D once per participant.
func (s *Store) SubmitMatchResult(ctx context.Context, bookingID int64, code string, players []PlayerScore) (MatchResult, error) {
```

```go
	var matchResultID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM match_results WHERE event_game_id = ?`, b.EventGameID).Scan(&matchResultID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.ExecContext(ctx, `INSERT INTO match_results (event_game_id) VALUES (?)`, b.EventGameID)
```

In `getMatchResultByID` cambia la `SELECT` e la `Scan`:

```go
		`SELECT id, event_game_id, submitted_at FROM match_results WHERE id = ?`, id,
	).Scan(&m.ID, &m.EventGameID, &submittedAt)
```

Sostituisci `GetMatchResultForBooking` con:

```go
// GetMatchResultForEventGame returns nil (with no error) if nobody at that
// table has submitted a result yet — "not played yet" is a normal,
// expected state here, unlike ErrNotFound elsewhere in this package.
func (s *Store) GetMatchResultForEventGame(ctx context.Context, eventGameID int64) (*MatchResult, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM match_results WHERE event_game_id = ?`, eventGameID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := s.getMatchResultByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
```

Sostituisci `BookingMatchResult` e `ListMatchResultsForEvent`:

```go
type EventGameMatchResult struct {
	EventGameID int64
	GameID      int64
	GameName    string
	CopyIndex   int
	Players     []PlayerScore
}

// ListMatchResultsForEvent returns one row per copy of the event that has a
// result, with the game, its copy number and the players/scores submitted —
// read-only data for the admin event detail page. It is keyed by copy and
// not by participant because the result belongs to the table: who typed it
// in is not a fact worth storing.
func (s *Store) ListMatchResultsForEvent(ctx context.Context, eventID int64) ([]EventGameMatchResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT eg.id, g.id, g.name, eg.copy_index, mps.player_name, mps.score
		 FROM match_results mr
		 JOIN event_games eg ON mr.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 JOIN match_player_scores mps ON mps.match_result_id = mr.id
		 WHERE eg.event_id = ?
		 ORDER BY eg.game_id, eg.copy_index, mps.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventGameMatchResult
	var current *EventGameMatchResult
	for rows.Next() {
		var eventGameID, gameID int64
		var gameName, playerName string
		var copyIndex, score int
		if err := rows.Scan(&eventGameID, &gameID, &gameName, &copyIndex, &playerName, &score); err != nil {
			return nil, err
		}
		if current == nil || current.EventGameID != eventGameID {
			out = append(out, EventGameMatchResult{EventGameID: eventGameID, GameID: gameID,
				GameName: gameName, CopyIndex: copyIndex})
			current = &out[len(out)-1]
		}
		current.Players = append(current.Players, PlayerScore{Name: playerName, Score: score})
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Disdetta che non azzera il tavolo**

In `backend/internal/events/bookings.go`, dentro `cancelBooking`,
sostituisci il blocco che cancella il `match_result`:

```go
	// Un tavolo condiviso non deve poter essere azzerato da chi si sfila:
	// il risultato è di tutti quelli che ci sono ancora seduti. Solo quando
	// il tavolo si svuota il punteggio non ha più senso e va via — e questo
	// evita anche il doppio conteggio se il posto liberato viene riprenotato
	// e riscritto.
	var remainingActive int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active' AND id != ?`,
		b.EventGameID, b.ID,
	).Scan(&remainingActive); err != nil {
		return Booking{}, err
	}
	if remainingActive == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM match_results WHERE event_game_id = ?`, b.EventGameID); err != nil {
			return Booking{}, err
		}
	}
```

- [ ] **Step 5: Verifica il pacchetto eventi e la classifica**

```bash
gotest ./internal/events/ ./internal/leaderboard/
```

Atteso: `ok` per `events`. `leaderboard` fallisce: i suoi test usano
`Quantity` e la sua query passa da `mr.booking_id`. Si sistema nei due
step successivi.

- [ ] **Step 6: Scrivi il test della classifica sul tavolo**

In `backend/internal/leaderboard/leaderboard_test.go` aggiorna gli
`events.EventGameInput{..., Quantity: N}` in `Copies: N` e aggiungi in
fondo (l'helper `newTestStores` è già nel file; `fmt` va negli import):

```go
func TestGetLeaderboard_CountsATableOnce(t *testing.T) {
	eventStore, gameStore, lbStore := newTestStores(t)
	ctx := context.Background()

	game, err := gameStore.CreateGame(ctx, games.Game{Name: "D&D", Seats: 5})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: game.ID, Copies: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// Cinque prenotazioni sullo stesso tavolo, ognuna che manda lo stesso
	// risultato: la partita giocata resta una.
	for i := 0; i < 5; i++ {
		b, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
			fmt.Sprintf("G%d", i), fmt.Sprintf("g%d@example.com", i),
			fmt.Sprintf("33300000%02d", i), now)
		if err != nil {
			t.Fatalf("booking %d: %v", i, err)
		}
		if _, err := eventStore.SubmitMatchResult(ctx, b.ID, b.BookingCode,
			[]events.PlayerScore{{Name: "Mario", Score: 50}, {Name: "Luigi", Score: 30}}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	lb, err := lbStore.GetLeaderboard(ctx, game.ID)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(lb.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lb.Matches))
	}
	for _, p := range lb.Players {
		if p.GamesPlayed != 1 {
			t.Fatalf("expected 1 game for %s, got %d", p.Name, p.GamesPlayed)
		}
	}
}
```

- [ ] **Step 7: Ripunta la classifica sulla copia**

`backend/internal/leaderboard/leaderboard.go` raggiunge l'evento
passando per la prenotazione (`JOIN bookings b ON mr.booking_id = b.id`):
quella colonna non esiste più. La copia porta già sia il gioco sia
l'evento, quindi il giro dalle prenotazioni sparisce. In
`GetLeaderboard`, sostituisci la query:

```go
	rows, err := s.db.QueryContext(ctx,
		`SELECT mr.id, e.title, e.event_date, e.start_time, mps.player_name, mps.score
		 FROM match_player_scores mps
		 JOIN match_results mr ON mps.match_result_id = mr.id
		 JOIN event_games eg ON mr.event_game_id = eg.id
		 JOIN events e ON eg.event_id = e.id
		 WHERE eg.game_id = ?
		 ORDER BY mr.id, mps.id`, gameID)
```

E aggiorna il commento di `GetLeaderboard`, prima riga:

```go
// GetLeaderboard aggregates every MatchResult ever submitted for a game,
// across all events it was played at. One MatchResult is one table, so a
// game of D&D booked by six people still counts as a single match.
```

`buildLeaderboard` non cambia: raggruppa già per `match_result_id`, che
ora è per copia.

- [ ] **Step 8: Verifica**

```bash
gotest ./internal/events/ ./internal/leaderboard/
```

Atteso: `ok` per entrambi.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/events/matches.go backend/internal/events/matches_test.go \
  backend/internal/events/bookings.go backend/internal/events/bookings_test.go \
  backend/internal/leaderboard/leaderboard.go backend/internal/leaderboard/leaderboard_test.go
git commit -m "feat: make the match result belong to the table, not to one booking"
```

---

### Task 6: API eventi — `copies`, `copyIndex`, `seats`

**Files:**
- Modify: `backend/internal/httpapi/events_handlers.go` (`eventGameRequest`, `toEventGameInputs`, `decodeEventRequest`, messaggio 409), `backend/internal/httpapi/events_responses.go` (`toEventGameSummary`, `toEventDetail`, `toBookingAdminResponse`, `toBookingDetailResponse`, `toMatchResultResponse`, nuovo `toEventGameMatchResultResponse`), `backend/internal/httpapi/match_result_handlers.go` (uso di `ListMatchResultsForEvent`)
- Test: `backend/internal/httpapi/events_handlers_test.go`, `events_bookings_handlers_test.go`, `match_result_handlers_test.go`

**Interfaces:**
- Consumes: tutto lo store eventi dei Task 2–5.
- Produces (payload JSON):
  - richiesta evento: `games: [{gameId, copies}]`
  - `GET /api/events/{id}`: `games: [{eventGameId, gameId, name, coverPath, copyIndex, seats, remaining}]`
  - `GET /api/events/{id}/bookings`: `[{id, eventGameId, gameId, gameName, copyIndex, seats, participantName, participantEmail, participantPhone, createdAt}]`
  - `POST /api/bookings/lookup` e `.../cancel`: aggiunge `copyIndex`, `seats`, `gameCopies`, `tableBookings`
  - `GET /api/events/{id}/match-results`: `[{eventGameId, gameId, gameName, copyIndex, players}]`

- [ ] **Step 1: Scrivi i test**

I test dell'API non hanno helper generici: l'idioma del pacchetto è
`newTestServer` + `httpapi.NewRouter(server)` +
`bootstrapFirstAdmin(t, router, email, password)` +
`createTestGameForEvent(t, server.Games, name)` e `httptest` a mano.
Attieniti a quello.

In `backend/internal/httpapi/events_handlers_test.go`, rinomina il test
esistente `TestCreateEvent_RejectsZeroQuantity` in
`TestCreateEvent_RejectsZeroCopies` e cambia il suo payload da
`"quantity": 0` a `"copies": 0` (non aggiungerne uno nuovo: sarebbe un
doppione). Poi aggiungi in fondo:

```go
func TestCreateEvent_WithSeveralCopiesOfTheSameGame(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "copies": 2}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created event: %v", err)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/events/%d", created.ID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Games []struct {
			EventGameID int64 `json:"eventGameId"`
			GameID      int64 `json:"gameId"`
			CopyIndex   int   `json:"copyIndex"`
			Seats       int   `json:"seats"`
			Remaining   int   `json:"remaining"`
		} `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Games) != 2 {
		t.Fatalf("expected 2 copies in the payload, got %d", len(detail.Games))
	}
	if detail.Games[0].CopyIndex != 1 || detail.Games[1].CopyIndex != 2 {
		t.Fatalf("unexpected copy indexes: %+v", detail.Games)
	}
	if detail.Games[0].Seats != 1 || detail.Games[0].Remaining != 1 {
		t.Fatalf("unexpected seats/remaining: %+v", detail.Games[0])
	}
	if detail.Games[0].EventGameID == detail.Games[1].EventGameID {
		t.Fatal("expected two distinct eventGameId values")
	}
	if detail.Games[0].GameID != gameID {
		t.Fatalf("unexpected gameId %d", detail.Games[0].GameID)
	}
}
```

Assicurati che `fmt` sia negli import del file.

Poi, in tutti i file di test dell'API, sostituisci ogni `"quantity": N`
con `"copies": N` e ogni assert su `"quantity"` nelle risposte con
`"copyIndex"` / `"seats"`:

```bash
grep -rn "quantity" backend/internal/httpapi/
```

- [ ] **Step 2: Fai fallire i test**

```bash
gotest ./internal/httpapi/ -run 'TestCreateEvent_WithSeveralCopies|TestCreateEvent_RejectsZeroCopies' -v
```

Atteso: FAIL — 400 sul primo (il campo `copies` è ignorato, `quantity`
manca) o errore di compilazione se lo store è già cambiato.

- [ ] **Step 3: Aggiorna richieste e validazione**

In `backend/internal/httpapi/events_handlers.go`:

```go
type eventGameRequest struct {
	GameID int64 `json:"gameId"`
	// Copies è quante copie del gioco l'evento mette in tavola. I posti
	// prenotabili di ciascuna arrivano dal catalogo, non da qui.
	Copies int `json:"copies"`
}
```

```go
func toEventGameInputs(in []eventGameRequest) []events.EventGameInput {
	out := make([]events.EventGameInput, 0, len(in))
	for _, g := range in {
		out = append(out, events.EventGameInput{GameID: g.GameID, Copies: g.Copies})
	}
	return out
}
```

In `decodeEventRequest`, la guardia diventa:

```go
		if g.Copies < 1 {
			return eventRequest{}, false
		}
```

E il messaggio del 409 in `updateEventHandler`:

```go
	case errors.Is(err, events.ErrQuantityBelowActiveBookings):
		writeError(w, http.StatusConflict, "fewer copies than the ones with active bookings")
```

- [ ] **Step 4: Aggiorna le risposte**

In `backend/internal/httpapi/events_responses.go`:

```go
func toEventGameSummary(eventGameID int64, g games.Game, copyIndex, seats, remaining int) map[string]any {
	return map[string]any{
		"eventGameId": eventGameID, "gameId": g.ID, "name": g.Name, "coverPath": g.CoverPath,
		"copyIndex": copyIndex, "seats": seats, "remaining": remaining,
	}
}
```

```go
		gamesOut = append(gamesOut, toEventGameSummary(eg.ID, game, eg.CopyIndex, eg.Seats, remaining))
```

```go
func toBookingAdminResponse(b events.BookingWithGame) map[string]any {
	return map[string]any{
		"id": b.ID, "eventGameId": b.EventGameID, "gameId": b.GameID, "gameName": b.GameName,
		"copyIndex": b.CopyIndex, "seats": b.Seats,
		"participantName": b.ParticipantName, "participantEmail": b.ParticipantEmail,
		"participantPhone": b.ParticipantPhone,
		"createdAt":        b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
```

In `toBookingDetailResponse`, dopo `resp["gameName"] = game.Name`:

```go
	resp["copyIndex"] = eventGame.CopyIndex
	resp["seats"] = eventGame.Seats
	// Quante persone siedono a questo tavolo: la pagina pubblica lo usa per
	// dire che il punteggio è condiviso invece di far credere a ognuno di
	// avere il proprio.
	tableBookings, err := s.Events.CountActiveBookingsForEventGame(ctx, b.EventGameID)
	if err != nil {
		return nil, err
	}
	resp["tableBookings"] = tableBookings

	matchResult, err := s.Events.GetMatchResultForEventGame(ctx, b.EventGameID)
```

E in fondo al file, accanto a `toMatchResultResponse`:

```go
func toEventGameMatchResultResponse(m events.EventGameMatchResult) map[string]any {
	return map[string]any{
		"eventGameId": m.EventGameID, "gameId": m.GameID, "gameName": m.GameName,
		"copyIndex": m.CopyIndex, "players": toPlayerScores(m.Players),
	}
}
```

In `backend/internal/httpapi/match_result_handlers.go`, dentro
`listEventMatchResultsHandler`, il ciclo costruisce la mappa inline:
sostituiscilo col mapper nuovo.

```go
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, toEventGameMatchResultResponse(m))
	}
	writeJSON(w, http.StatusOK, out)
```

- [ ] **Step 5: Verifica tutto il pacchetto API**

```bash
gotest ./internal/httpapi/
```

Atteso: `ok`. Sistema i test residui che parlano ancora di `quantity`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/
git commit -m "feat: expose copies, copy index and seats over the events API"
```

---

### Task 7: API giochi — `seats`

**Files:**
- Modify: `backend/internal/httpapi/games_handlers.go` (`createGameRequest`, `createGameManually`, `createGameFromBGG`), `backend/internal/httpapi/games_read_handlers.go` (`updateGameRequest`, `updateGameHandler`), `backend/internal/httpapi/games_responses.go` (`toGameSummary`)
- Test: `backend/internal/httpapi/games_handlers_test.go` (o il file di test dei giochi già presente nel pacchetto)

**Interfaces:**
- Consumes: `games.Game.Seats`, `games.GameUpdate.Seats` (Task 1).
- Produces:
  - `POST /api/games` accetta `seats` (intero ≥ 1, opzionale, default 1); `< 1` → 400.
  - `PATCH /api/games/{id}` accetta `seats`; `< 1` → 400.
  - ogni risposta gioco porta `seats`.

- [ ] **Step 1: Scrivi i test**

In `backend/internal/httpapi/games_handlers_test.go`, in fondo (stesso
idioma dei test vicini: `newTestServer`, `httpapi.NewRouter`,
`bootstrapFirstAdmin`, `httptest`):

```go
func TestCreateGame_StoresBookableSeats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode": "it", "name": "D&D", "nameTranslated": "D&D", "seats": 5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID    int64 `json:"id"`
		Seats int   `json:"seats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Seats != 5 {
		t.Fatalf("expected seats 5, got %d", body.Seats)
	}
}

func TestCreateGame_DefaultsBookableSeatsToOne(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode": "it", "name": "Catan", "nameTranslated": "Catan",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Seats int `json:"seats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Seats != 1 {
		t.Fatalf("expected seats 1, got %d", body.Seats)
	}
}

func TestCreateGame_RejectsZeroSeats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode": "it", "name": "D&D", "nameTranslated": "D&D", "seats": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGame_ChangesBookableSeats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "D&D")

	patch := func(seats int) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]any{"seats": seats})
		req := httptest.NewRequest(http.MethodPatch,
			fmt.Sprintf("/api/games/%d", gameID), bytes.NewReader(payload))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := patch(6)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Seats int `json:"seats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Seats != 6 {
		t.Fatalf("expected seats 6, got %d", body.Seats)
	}

	if rec := patch(0); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for zero seats, got %d", rec.Code)
	}
}
```

`createTestGameForEvent` sta in `events_handlers_test.go`, stesso
pacchetto di test: si può usare da qui. Assicurati che `fmt` sia negli
import.

- [ ] **Step 2: Fai fallire i test**

```bash
gotest ./internal/httpapi/ -run 'TestCreateGame_StoresBookableSeats|TestCreateGame_DefaultsBookableSeatsToOne|TestCreateGame_RejectsZeroSeats|TestUpdateGame_ChangesBookableSeats' -v
```

Atteso: FAIL — `seats` assente dalla risposta (`nil` → panic sul type
assert) e 200 invece di 400 sui posti a zero.

- [ ] **Step 3: `seats` in creazione**

In `backend/internal/httpapi/games_handlers.go`:

```go
type createGameRequest struct {
	BGGID                 string `json:"bggId"`
	LanguageCode          string `json:"languageCode"`
	Owner                 string `json:"owner"`
	Name                  string `json:"name"`
	Year                  *int   `json:"year"`
	MinPlayers            *int   `json:"minPlayers"`
	MaxPlayers            *int   `json:"maxPlayers"`
	PlaytimeMinutes       *int   `json:"playtimeMinutes"`
	// Seats sono i posti prenotabili per copia: assente vale 1.
	Seats                 *int   `json:"seats"`
	NameTranslated        string `json:"nameTranslated"`
	DescriptionTranslated string `json:"descriptionTranslated"`
}
```

In `createGameHandler`, subito dopo il controllo su `LanguageCode`:

```go
	if req.Seats != nil && *req.Seats < 1 {
		writeError(w, http.StatusBadRequest, "seats must be at least 1")
		return
	}
```

E un helper in fondo al file, usato da entrambi i rami di creazione:

```go
// requestedSeats traduce il campo opzionale in un valore concreto: un gioco
// senza posti prenotabili dichiarati è un gioco a copia singola.
func requestedSeats(req createGameRequest) int {
	if req.Seats == nil {
		return 1
	}
	return *req.Seats
}
```

In `createGameFromBGG` e in `createGameManually`, aggiungi
`Seats: requestedSeats(req)` al literal `games.Game{...}`.

- [ ] **Step 4: `seats` in aggiornamento e in lettura**

In `backend/internal/httpapi/games_read_handlers.go`:

```go
type updateGameRequest struct {
	Owner           *string `json:"owner"`
	Year            *int    `json:"year"`
	MinPlayers      *int    `json:"minPlayers"`
	MaxPlayers      *int    `json:"maxPlayers"`
	PlaytimeMinutes *int    `json:"playtimeMinutes"`
	Seats           *int    `json:"seats"`
}
```

In `updateGameHandler`, dopo il decode:

```go
	if req.Seats != nil && *req.Seats < 1 {
		writeError(w, http.StatusBadRequest, "seats must be at least 1")
		return
	}
	game, err := s.Games.UpdateGame(r.Context(), id, games.GameUpdate{
		Owner: req.Owner, Year: req.Year, MinPlayers: req.MinPlayers,
		MaxPlayers: req.MaxPlayers, PlaytimeMinutes: req.PlaytimeMinutes,
		Seats: req.Seats,
	})
```

In `backend/internal/httpapi/games_responses.go`:

```go
func toGameSummary(g games.Game) map[string]any {
	return map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "owner": g.Owner, "coverPath": g.CoverPath,
		"seats": g.Seats,
	}
}
```

- [ ] **Step 5: Verifica la suite backend completa**

```bash
gotest ./...
```

Atteso: `ok` per tutti i pacchetti. È il primo punto del piano in cui il
backend è di nuovo interamente verde: se qualcosa fallisce, è qui che va
sistemato prima di passare al frontend.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/
git commit -m "feat: accept and return bookable seats on the games API"
```

---

### Task 8: Frontend — posti prenotabili in catalogo

**Files:**
- Modify: `frontend/src/views/GameNewView.vue`, `frontend/src/views/GameDetailView.vue`, `frontend/src/app.css`

**Interfaces:**
- Consumes: `POST /api/games` con `seats`, `PATCH /api/games/{id}` con `seats`, `seats` nella risposta gioco (Task 7).
- Produces: nessuna interfaccia per altri task; `GameDetail.seats` diventa disponibile nel tipo locale della vista.

- [ ] **Step 1: Campo in creazione gioco**

In `frontend/src/views/GameNewView.vue`, accanto agli altri `ref`
numerici manuali:

```ts
// Posti prenotabili per copia: 1 = chi prenota si prende la copia.
const seats = ref(1)
```

Nel corpo di `createManual`, aggiungi `seats: seats.value` all'oggetto
inviato. Fai lo stesso nella funzione che crea il gioco dal risultato
BGG (cerca l'altra `api.post('/games', ...)` nello stesso file) — un
tavolo si può creare anche partendo da BGG.

Nel template, dopo il campo "Max giocatori" del blocco manuale, e in
posizione equivalente nel blocco BGG:

```vue
        <label>
          Posti prenotabili per copia
          <input v-model.number="seats" type="number" min="1" />
          <span class="field-hint">
            1 = chi prenota si prende la copia e si porta i suoi.
            Più di 1 per un tavolo aperto, dove ci si iscrive uno alla volta
            (D&amp;D, giochi di ruolo, tornei).
          </span>
        </label>
```

Se `.field-hint` non esiste in `frontend/src/app.css`, aggiungila
accanto alle altre classi di form:

```css
.field-hint {
  display: block;
  margin-top: 0.35rem;
  font-size: 0.82rem;
  line-height: 1.4;
  color: var(--color-text-muted);
}
```

Verifica il nome reale del token del testo attenuato in `app.css` prima
di usarlo (`grep -n "text-muted\|--color-text" frontend/src/app.css`).

- [ ] **Step 2: Modifica inline sulla scheda gioco**

In `frontend/src/views/GameDetailView.vue`, aggiungi `seats: number` a
`interface GameDetail`, e dopo i `ref` di modifica lingua:

```ts
const editSeats = ref(1)
const seatsSaving = ref(false)
const seatsError = ref('')
```

In `load()`, dopo l'assegnazione di `game.value`:

```ts
  editSeats.value = game.value.seats
```

E una funzione accanto a `saveLanguage`:

```ts
// I posti prenotabili sono l'unico dato del gioco modificabile da qui: si
// salvano da sé, senza un "Salva" generale che non esiste in questa pagina.
async function saveSeats() {
  seatsError.value = ''
  seatsSaving.value = true
  try {
    await api.patch(`/games/${gameId}`, { seats: editSeats.value })
    await load()
  } catch (e) {
    seatsError.value = (e as Error).message
  } finally {
    seatsSaving.value = false
  }
}
```

Nel template, dentro `.game-cover-info`, subito dopo la `</dl>` dei
`gameFacts` (e prima di `<p v-if="coverError">`):

```vue
          <div v-if="auth.user" class="game-seats-edit">
            <label>
              Posti prenotabili per copia
              <input v-model.number="editSeats" type="number" min="1" />
            </label>
            <button type="button" :disabled="seatsSaving || editSeats === game.seats" @click="saveSeats">
              {{ seatsSaving ? 'Salvo…' : 'Salva' }}
            </button>
            <p class="field-hint">
              Più di 1 apre il tavolo: a un evento, ogni posto si prenota a sé
              con il suo codice.
            </p>
            <p v-if="seatsError" class="error">{{ seatsError }}</p>
          </div>
          <p v-else-if="game.seats > 1" class="row-meta">
            Tavolo da {{ game.seats }} posti prenotabili.
          </p>
```

E in `app.css`, accanto alle classi della scheda gioco:

```css
.game-seats-edit {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 0.6rem;
  margin-top: 0.9rem;
}

.game-seats-edit input {
  width: 5rem;
}

.game-seats-edit .field-hint,
.game-seats-edit .error {
  flex-basis: 100%;
  margin: 0;
}
```

- [ ] **Step 3: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato, nessun errore `vue-tsc`.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/GameNewView.vue frontend/src/views/GameDetailView.vue frontend/src/app.css
git commit -m "feat: set bookable seats per copy from the game catalogue"
```

---

### Task 9: Frontend — picker copie e scheda evento admin

**Files:**
- Modify: `frontend/src/components/EventGamesPicker.vue`, `frontend/src/views/EventNewView.vue`, `frontend/src/views/EventAdminDetailView.vue`, `frontend/src/app.css`

**Interfaces:**
- Consumes: `seats` su `GET /api/games` (Task 7); `copies` nel payload evento e `copyIndex`/`seats`/`remaining` in `GET /api/events/{id}` e `GET /api/events/{id}/bookings` (Task 6).
- Produces:
  - `PickerGame { id: number; name: string; seats: number }`
  - `SelectedGame { gameId: number; copies: number }`
  - prop `occupiedCopies?: Record<number, number>` (era `booked`).

- [ ] **Step 1: Riscrivi il picker**

In `frontend/src/components/EventGamesPicker.vue`, sostituisci tipi,
prop e funzioni dello `<script setup>`:

```ts
export interface PickerGame {
  id: number
  name: string
  /** Posti prenotabili per copia, dal catalogo. */
  seats: number
}

export interface SelectedGame {
  gameId: number
  copies: number
}

const props = defineProps<{
  games: PickerGame[]
  modelValue: SelectedGame[]
  /**
   * Copie con almeno una prenotazione attiva, per gioco (solo nella scheda
   * di un evento esistente): il backend rifiuta di scendere sotto quel
   * numero di copie, quindi qui il minimo si alza e il gioco non si può
   * togliere — meglio un campo che non scende che un 409 dopo il salvataggio.
   */
  occupiedCopies?: Record<number, number>
}>()
```

```ts
const selectedRows = computed(() =>
  props.modelValue
    .map((s) => ({ game: props.games.find((g) => g.id === s.gameId), copies: s.copies }))
    .filter((row): row is { game: PickerGame; copies: number } => row.game !== undefined),
)
```

```ts
function occupiedFor(gameId: number) {
  return props.occupiedCopies?.[gameId] ?? 0
}

function add(gameId: number) {
  emit('update:modelValue', [...props.modelValue, { gameId, copies: 1 }])
}

function remove(gameId: number) {
  emit('update:modelValue', props.modelValue.filter((s) => s.gameId !== gameId))
}

function setCopies(gameId: number, copies: number) {
  const min = Math.max(1, occupiedFor(gameId))
  emit(
    'update:modelValue',
    props.modelValue.map((s) =>
      s.gameId === gameId ? { ...s, copies: Number.isFinite(copies) ? Math.max(min, copies) : min } : s,
    ),
  )
}

/** "2 copie × 5 = 10 posti prenotabili", solo quando c'è qualcosa da spiegare. */
function capacityLabel(game: PickerGame, copies: number) {
  if (game.seats <= 1) {
    return ''
  }
  return `× ${game.seats} posti prenotabili = ${copies * game.seats} in tutto`
}
```

Nel template, la riga scelta diventa:

```vue
      <li v-for="row in selectedRows" :key="row.game.id" class="game-select-row">
        <label class="checkbox-label">
          <input
            type="checkbox"
            checked
            :disabled="occupiedFor(row.game.id) > 0"
            @change="remove(row.game.id)"
          />
          {{ row.game.name }}
        </label>
        <span class="game-select-quantity">
          <span v-if="occupiedFor(row.game.id) > 0" class="game-select-booked">
            {{ occupiedFor(row.game.id) === 1 ? '1 copia occupata' : `${occupiedFor(row.game.id)} copie occupate` }}
          </span>
          <label class="game-select-copies">
            copie
            <input
              type="number"
              :min="Math.max(1, occupiedFor(row.game.id))"
              :value="row.copies"
              @input="setCopies(row.game.id, Number(($event.target as HTMLInputElement).value))"
            />
          </label>
          <span v-if="capacityLabel(row.game, row.copies)" class="game-select-seats">
            {{ capacityLabel(row.game, row.copies) }}
          </span>
        </span>
      </li>
```

In `app.css`, accanto a `.game-select-quantity`:

```css
.game-select-seats {
  font-size: 0.82rem;
  color: var(--color-text-muted);
}
```

- [ ] **Step 2: Adegua la creazione evento**

In `frontend/src/views/EventNewView.vue` non serve altro che il tipo
aggiornato di `PickerGame`: verifica che `availableGames` sia caricato
da `GET /api/games` (che ora manda `seats`) e che nessun literal
costruisca a mano un `SelectedGame`. Se lo fa, il campo diventa
`copies`.

- [ ] **Step 3: Adegua la scheda evento admin**

In `frontend/src/views/EventAdminDetailView.vue`:

```ts
interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  copyIndex: number
  seats: number
  remaining: number
}

interface BookingAdminInfo {
  id: number
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  seats: number
  participantName: string
  participantEmail: string
  participantPhone: string
  createdAt: string
}

interface MatchResultAdminInfo {
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  players: MatchResultPlayer[]
}
```

```ts
/** Copie con almeno una prenotazione: sotto quel numero non si scende. */
const occupiedCopiesByGame = ref<Record<number, number>>({})
/** Quante copie di ogni gioco ha l'evento: serve a numerarle solo se >1. */
const copiesByGame = ref<Record<number, number>>({})
```

In `load()`, sostituisci le due righe che riempivano `selectedGames` e
`bookedByGame`:

```ts
  const copies: Record<number, number> = {}
  const occupied: Record<number, number> = {}
  for (const g of event.games) {
    copies[g.gameId] = (copies[g.gameId] ?? 0) + 1
    if (g.seats - g.remaining > 0) {
      occupied[g.gameId] = (occupied[g.gameId] ?? 0) + 1
    }
  }
  copiesByGame.value = copies
  occupiedCopiesByGame.value = occupied
  selectedGames.value = Object.entries(copies).map(([gameId, count]) => ({
    gameId: Number(gameId),
    copies: count,
  }))
```

Aggiungi, accanto agli altri `computed`:

```ts
/**
 * L'etichetta di una copia: il numero compare solo quando quel gioco ha
 * più di una copia nell'evento, così un evento normale non si riempie di
 * "#1" inutili.
 */
function copyLabel(gameId: number, gameName: string, copyIndex: number) {
  return (copiesByGame.value[gameId] ?? 1) > 1 ? `${gameName} #${copyIndex}` : gameName
}

/** Prenotazioni raggruppate per copia, nell'ordine in cui arrivano. */
const bookingsByCopy = computed(() => {
  const groups: { eventGameId: number; label: string; seats: number; rows: BookingAdminInfo[] }[] = []
  for (const b of bookings.value) {
    let group = groups.find((g) => g.eventGameId === b.eventGameId)
    if (!group) {
      group = {
        eventGameId: b.eventGameId,
        label: copyLabel(b.gameId, b.gameName, b.copyIndex),
        seats: b.seats,
        rows: [],
      }
      groups.push(group)
    }
    group.rows.push(b)
  }
  return groups
})
```

Nel template, sostituisci la lista piatta delle prenotazioni con i
gruppi (mantieni `admin-row`, `admin-pawn`, `admin-email`, `row-meta`
già esistenti):

```vue
      <div v-for="group in bookingsByCopy" :key="group.eventGameId" class="booking-copy-group">
        <h3 class="booking-copy-head">
          {{ group.label }}
          <span class="row-meta">{{ group.rows.length }} di {{ group.seats }} posti prenotabili</span>
        </h3>
        <ul role="list" class="admin-list">
          <li v-for="b in group.rows" :key="b.id">
            <div class="admin-row">
              <span class="admin-pawn" aria-hidden="true">{{ initial(b.participantName) }}</span>
              <span class="admin-email booking-who">
                {{ b.participantName }}
                <span class="row-meta">{{ b.participantEmail }} · {{ b.participantPhone }}</span>
              </span>
              <div class="admin-row-actions">
                <button type="button" @click="cancelBooking(b)">Annulla</button>
              </div>
            </div>
          </li>
        </ul>
      </div>
```

E nella lista dei risultati, la chiave e l'intestazione:

```vue
        <li v-for="m in matchResults" :key="m.eventGameId">
          <div class="admin-row">
            <span class="admin-email booking-who">
              {{ copyLabel(m.gameId, m.gameName, m.copyIndex) }}
              <span class="row-meta match-scores">
                <span v-for="(p, index) in m.players" :key="index">
                  {{ p.name }} {{ p.score }}{{ index < m.players.length - 1 ? ' · ' : '' }}
                </span>
              </span>
            </span>
            <router-link class="booking-game" :to="`/games/${m.gameId}`">Scheda</router-link>
          </div>
        </li>
```

Aggiorna il testo della conferma di annullamento, che oggi promette di
cancellare il punteggio:

```ts
  const confirmed = window.confirm(
    `Annullare la prenotazione di ${booking.participantName} per ${booking.gameName}? ` +
      'Il posto torna libero. Il punteggio del tavolo resta, a meno che non fosse la sua ultima prenotazione.',
  )
```

E in `app.css`:

```css
.booking-copy-group + .booking-copy-group {
  margin-top: 1.1rem;
}

.booking-copy-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem;
  margin: 0 0 0.5rem;
  font-size: 0.95rem;
}
```

- [ ] **Step 4: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato senza errori di tipo.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/EventGamesPicker.vue frontend/src/views/EventNewView.vue \
  frontend/src/views/EventAdminDetailView.vue frontend/src/app.css
git commit -m "feat: pick copies and group event bookings by table in the admin"
```

---

### Task 10: Frontend — pagina pubblica dell'evento

**Files:**
- Modify: `frontend/src/views/EventDetailView.vue`, `frontend/src/app.css`

**Interfaces:**
- Consumes: `games: [{eventGameId, gameId, name, coverPath, copyIndex, seats, remaining}]` da `GET /api/events/{id}` (Task 6).
- Produces: niente per altri task.

- [ ] **Step 1: Numerazione e posti prenotabili**

In `frontend/src/views/EventDetailView.vue`:

```ts
interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  copyIndex: number
  seats: number
  remaining: number
}
```

Accanto agli altri `computed`:

```ts
/** Quante copie ha ogni gioco in questo evento. */
const copiesByGame = computed(() => {
  const counts: Record<number, number> = {}
  for (const g of event.value?.games ?? []) {
    counts[g.gameId] = (counts[g.gameId] ?? 0) + 1
  }
  return counts
})

/**
 * Il numero della copia si mostra solo se ce n'è più di una: su un evento
 * con una copia per gioco, "#1" sarebbe solo rumore.
 */
function copyTitle(g: EventGameInfo) {
  return (copiesByGame.value[g.gameId] ?? 1) > 1 ? `${g.name} #${g.copyIndex}` : g.name
}

/**
 * Un tavolo aperto dice quanti posti prenotabili restano; una copia
 * singola resta con la dicitura di sempre.
 */
function availabilityLabel(g: EventGameInfo) {
  if (g.seats > 1) {
    return `Posti prenotabili liberi: ${g.remaining} di ${g.seats}`
  }
  return `Disponibilità: ${g.remaining}`
}
```

Nel template, dentro il `<li>` dei giochi:

```vue
          <router-link :to="`/games/${g.gameId}`">{{ copyTitle(g) }}</router-link>
          <p :class="{ 'is-full': g.remaining <= 0 }">{{ availabilityLabel(g) }}</p>
```

E nel titolo del form di prenotazione, che oggi mostra `selectedGame?.name`:

```vue
        <h2>Prenota: {{ selectedGame ? copyTitle(selectedGame) : '' }}</h2>
```

Stessa sostituzione nel messaggio di conferma:

```vue
          <p class="success">
            Prenotazione confermata per {{ selectedGame ? copyTitle(selectedGame) : '' }}!
          </p>
```

Quando il gioco scelto è un tavolo, spiega la condivisione del punteggio
subito sotto il codice, dove la persona sta ancora guardando lo schermo:

```vue
        <p v-if="selectedGame && selectedGame.seats > 1">
          A questo tavolo si prenota un posto a testa: il punteggio finale è
          uno per tavolo e chiunque sieda qui può inserirlo o correggerlo con
          il proprio codice.
        </p>
```

In `app.css`:

```css
.event-games .is-full {
  color: var(--color-text-muted);
}
```

- [ ] **Step 2: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/EventDetailView.vue frontend/src/app.css
git commit -m "feat: show numbered copies and free bookable seats on the public event page"
```

---

### Task 11: Frontend — punteggio del tavolo

**Files:**
- Modify: `frontend/src/views/ManageBookingView.vue`

**Interfaces:**
- Consumes: `copyIndex`, `seats`, `gameCopies`, `tableBookings`, `matchResult` da `POST /api/bookings/lookup` (Task 6 + fix round Task 11).
- Produces: niente per altri task.

- [ ] **Step 1: Dichiara il tavolo e spiega la condivisione**

In `frontend/src/views/ManageBookingView.vue`, estendi l'interfaccia:

```ts
interface BookingResult {
  id: number
  eventId: number
  eventGameId: number
  participantName: string
  bookingCode: string
  status: 'active' | 'cancelled'
  eventTitle: string
  eventDate: string
  startTime: string
  gameName: string
  copyIndex: number
  seats: number
  /** Quante copie di questo gioco porta l'evento: sotto le 2, niente "#N". */
  gameCopies: number
  /** Quante prenotazioni attive ci sono su questo tavolo, compresa la mia. */
  tableBookings: number
  matchResult: { players: PlayerScore[] } | null
}
```

Aggiungi due `computed` dopo i `ref`:

```ts
/** Un tavolo condiviso: più di un posto prenotabile e più di un prenotato. */
const isSharedTable = computed(
  () => (booking.value?.seats ?? 1) > 1 && (booking.value?.tableBookings ?? 1) > 1,
)

/**
 * Il numero della copia serve solo quando l'evento porta più copie di questo
 * gioco: con una copia sola, "#1" è rumore. `seats` non risponde a questa
 * domanda — è la capienza di una copia, non quante copie ci sono.
 */
const gameLabel = computed(() => {
  const b = booking.value
  if (!b) {
    return ''
  }
  return b.gameCopies > 1 ? `${b.gameName} #${b.copyIndex}` : b.gameName
})
```

Aggiungi `computed` all'import da `vue`.

Nel template, il titolo del riepilogo diventa `{{ gameLabel }}`, e dentro
`.booking-summary`, dopo la riga del partecipante:

```vue
          <p v-if="booking.seats > 1" class="row-meta">
            Tavolo da {{ booking.seats }} posti prenotabili · {{ booking.tableBookings }} prenotati
          </p>
```

Nel form del punteggio, subito dopo `<h2>Punteggio finale</h2>`:

```vue
          <p v-if="isSharedTable" class="row-meta">
            Il punteggio è del tavolo: lo vedono e lo possono correggere tutti
            quelli che hanno prenotato qui. Se qualcuno l'ha già inserito, qui
            sopra c'è il suo, e salvando lo sostituisci.
          </p>
```

Aggiorna anche la conferma di annullamento e il testo del bottone, che
oggi parlano di "la prenotazione per il gioco":

```ts
  if (!window.confirm(`Annullare la prenotazione per ${gameLabel.value}?`)) {
    return
  }
```

- [ ] **Step 2: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/ManageBookingView.vue
git commit -m "feat: present the match result as belonging to the table"
```

---

### Task 12: Documentazione, verifica finale e pass `impeccable`

**Files:**
- Modify: `README.md`, `DESIGN.md`

**Interfaces:**
- Consumes: tutto il lavoro precedente.
- Produces: niente codice.

- [ ] **Step 1: Aggiorna `README.md`**

Nella sezione che descrive eventi e prenotazioni, aggiungi il concetto
nuovo (adegua il wording a quello del file):

```markdown
Ogni gioco del catalogo ha un numero di **posti prenotabili per copia**:
`1` per un gioco da tavolo normale, dove chi prenota si prende la copia e
si porta i suoi; più di 1 per un tavolo aperto — una partita a D&D, un
gioco di ruolo, un torneo — dove ci si iscrive uno alla volta, ognuno con
il proprio codice, e ognuno può disdire senza far saltare la serata agli
altri.

Un evento può portare più copie dello stesso gioco: nella pagina pubblica
compaiono numerate (`Carcassonne #1`, `Carcassonne #2`) e si prenotano
separatamente. Il punteggio finale è **uno per tavolo**: lo inserisce o lo
corregge chiunque abbia prenotato lì, e la classifica conta la partita una
volta sola.
```

Se il README elenca le migrazioni o avverte su aggiornamenti che toccano
i dati, aggiungi una riga: la `0008` ricrea prenotazioni e punteggi
azzerandoli, e gli eventi già creati vanno ripopolati di giochi.

- [ ] **Step 2: Aggiorna `DESIGN.md`**

Registra i pattern nuovi nella sezione dei componenti:

```markdown
### Copie numerate

Quando un evento porta più copie dello stesso gioco, il nome prende il
suffisso `#1`, `#2`. Il numero compare **solo** se le copie sono più
d'una: su un evento con una copia per gioco sarebbe rumore. I numeri sono
etichette stabili, non posizioni: eliminando una copia di mezzo resta un
buco, perché chi ha letto "#2" al momento di prenotare deve ritrovare
"#2".

### Posti prenotabili

La dicitura è sempre "posti prenotabili", mai "posti" da solo: un "posto"
si confonderebbe con la sedia intorno al tavolo o con il numero di
giocatori del gioco. Una copia con un solo posto prenotabile non mostra
nulla — resta la dicitura di disponibilità di sempre; da due in su
compare `Posti prenotabili liberi: 3 di 5`.
```

- [ ] **Step 3: Verifica completa**

```bash
gotest ./...
cd frontend && npm run build
```

Atteso: `ok` per ogni pacchetto Go, build frontend pulito. **Non
dichiarare concluso il lavoro senza aver incollato questi due output.**

- [ ] **Step 4: Prova l'app**

```bash
docker compose up -d --build
```

Su http://localhost:8080, con un occhio al viewport mobile:

1. metti 5 posti prenotabili su un gioco dal catalogo;
2. crea un evento con quel gioco (1 copia) e un secondo gioco con 2 copie;
3. dalla pagina pubblica prenota due volte lo stesso tavolo con telefoni
   diversi, e verifica che i due `Carcassonne #1` / `#2` siano prenotabili
   separatamente;
4. inserisci un punteggio con il primo codice e rileggilo con il secondo;
5. disdici la prima prenotazione e verifica che il punteggio resti.

- [ ] **Step 5: Pass `impeccable`**

Come da CLAUDE.md, ultimo task obbligatorio: lancia `/impeccable` sulle
superfici toccate — scheda gioco, picker giochi, scheda evento admin,
pagina pubblica evento, gestione prenotazione — e applica i rilievi.

- [ ] **Step 6: Commit**

```bash
git add README.md DESIGN.md
git commit -m "docs: document bookable seats and numbered copies"
```

---

## Note per chi esegue

- **Ordine obbligato**: i Task 1–7 sono una catena (schema → store →
  API); il backend torna interamente verde solo alla fine del Task 7. I
  Task 8–11 sono indipendenti fra loro e si possono affrontare in
  qualunque ordine, purché dopo il Task 7.
- **Il `git status` di partenza ha già lavoro non committato** su eventi
  e frontend: non annullarlo e non includerlo nei commit di questo piano
  se non è tuo.
- **Gli helper di test dell'API sono quelli reali**: `newTestServer`,
  `httpapi.NewRouter(server)`,
  `bootstrapFirstAdmin(t, router, email, password)`,
  `createTestGameForEvent(t, server.Games, name)`, più `httptest` a mano.
  Non esistono `doJSON`/`decodeJSON`/`loginAsAdmin`: non inventarli.
