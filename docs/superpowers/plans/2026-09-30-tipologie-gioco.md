# Tipologie di gioco gestite da admin — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sostituire le tipologie fisse (`board`/`rpg`) con una tabella `game_types` gestita dall'admin e dividere ogni lista di giochi in tab per tipologia.

**Architecture:** Nuova tabella `game_types` + colonna `games.game_type_id` (migrazione 0027, che converte `kind`). CRUD delle tipologie nel package `games` (file `types.go`) esposto su `/api/game-types`. Nel frontend uno store Pinia `gameTypes` sostituisce `utils/gameKinds.ts`; un componente `GameTypeTabs` sostituisce il filtro "Tipo" e compare in cinque liste; una pagina admin `/admin/game-types` gestisce l'elenco.

**Tech Stack:** Go 1.25 + chi + SQLite (modernc), Vue 3 `<script setup>` + TS + Pinia.

**Spec:** `docs/superpowers/specs/2026-09-30-tipologie-gioco-design.md`

## Global Constraints

- Comandi Go **solo** in Docker: `docker run --rm -v "$(pwd)/backend:/app" -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./...` (da root repo). Di seguito abbreviato `GOTEST <args>` = quel comando con `go test <args>` al posto di `go test ./...`.
- Frontend: `npm run build` in `frontend/` (fa anche `vue-tsc`).
- Migrazioni forward-only: nessuna modifica a `0001`–`0026`.
- Nessuna nuova dipendenza Go o npm.
- UI in italiano, stringhe dirette nei componenti; messaggi d'errore API in italiano come gli altri recenti.
- JSON in camelCase (`gameTypeId`, `bggSearch`, `gameCount`).
- Commit solo se Furt lo chiede: gli step "Commit" preparano il messaggio ma si eseguono solo con consenso esplicito. Stile `feat:`/`fix:` in inglese, trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Prenotazioni, punteggi e classifiche non cambiano.

## Review Focus

1. **DB esistente con giochi `rpg`** → dopo la migrazione sono GDR, gli altri GDT; nessun gioco senza tipologia. Test in Task 1.
2. **Slug con minuscole/spazi (" mtg ")** → normalizzato a `MTG`; duplicato case-insensitive (`gdt`) rifiutato con 409. Test in Task 1 e Task 3.
3. **Eliminazione della tipologia che è la prima in ordine (default)** mentre è vuota → consentita; il default diventa la successiva e un gioco creato senza tipologia la prende. Test in Task 2.
4. **Frecce su/giù agli estremi** (su sulla prima, giù sull'ultima) → nessun errore, nessun cambio. Test in Task 1.
5. **Tab selezionata che sparisce** (filtro per nome o lista ricaricata che non contiene più quella tipologia) → la tab torna a "Tutti", mai lista vuota senza tab. Gestito nel componente in Task 5 (watch), verificato a mano in Task 7.

---

### Task 1: Migrazione e store delle tipologie

**Files:**
- Create: `backend/internal/db/migrations/0027_game_types.sql`
- Create: `backend/internal/games/types.go`
- Test: `backend/internal/games/types_test.go`

**Interfaces:**
- Produces (package `games`, metodi su `*Store`):
  - `type GameType struct { ID int64; Name string; Slug string; BGGSearch bool; Position int; GameCount int }`
  - `type GameTypeInput struct { Name string; Slug string; BGGSearch bool }`
  - `type GameTypeUpdate struct { Name *string; Slug *string; BGGSearch *bool }`
  - `var ErrGameTypeName, ErrGameTypeSlug, ErrSlugTaken error`
  - `type GameTypeInUseError struct{ Count int }` (implementa `error`)
  - `ListGameTypes(ctx) ([]GameType, error)` — ordinato per `position, id`, con `GameCount`
  - `GetGameType(ctx, id int64) (GameType, error)` — `ErrNotFound` se manca
  - `CreateGameType(ctx, GameTypeInput) (GameType, error)`
  - `UpdateGameType(ctx, id int64, GameTypeUpdate) (GameType, error)`
  - `MoveGameType(ctx, id int64, up bool) error`
  - `DeleteGameType(ctx, id int64) error` — `*GameTypeInUseError` se ha giochi
  - `DefaultGameTypeID(ctx) (int64, error)`
  - `GameTypeExists(ctx, id int64) (bool, error)`

- [ ] **Step 1: Scrivi la migrazione**

`backend/internal/db/migrations/0027_game_types.sql`:

```sql
-- Le tipologie di gioco escono dal codice e diventano una tabella che
-- l'admin gestisce. games.kind resta (le migrazioni vanno solo avanti) ma
-- da qui in poi nessuno la legge: la tipologia è games.game_type_id.
-- Niente REFERENCES su game_type_id: con foreign_keys attivo SQLite non
-- accetta un ADD COLUMN con foreign key e default non NULL. L'integrità
-- la tiene lo store (blocco dell'eliminazione, validazione dell'id).
CREATE TABLE game_types (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  slug       TEXT    NOT NULL UNIQUE,
  bgg_search INTEGER NOT NULL DEFAULT 1,
  position   INTEGER NOT NULL,
  created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO game_types (id, name, slug, bgg_search, position) VALUES
  (1, 'Gioco da tavolo', 'GDT', 1, 1),
  (2, 'Gioco di ruolo',  'GDR', 0, 2);

ALTER TABLE games ADD COLUMN game_type_id INTEGER NOT NULL DEFAULT 1;
UPDATE games SET game_type_id = 2 WHERE kind = 'rpg';
```

- [ ] **Step 2: Scrivi i test che falliscono**

`backend/internal/games/types_test.go` (riusa `newTestStore`/`newTestStoreWithDB` di `store_test.go`):

```go
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

func TestGameTypes_MigrationConvertsKind(t *testing.T) {
	store, conn := newTestStoreWithDB(t)
	ctx := context.Background()
	// Simula un gioco scritto prima della 0027: kind='rpg', game_type_id al default.
	if _, err := conn.ExecContext(ctx, `INSERT INTO games (name, kind, game_type_id) VALUES ('D&D', 'rpg', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE games SET game_type_id = 2 WHERE kind = 'rpg'`); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListGameTypes(ctx)
	if list[1].GameCount != 1 || list[0].GameCount != 0 {
		t.Fatalf("counts = %+v", list)
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
```

- [ ] **Step 3: Esegui i test, verifica che falliscano**

Run: `GOTEST ./internal/games/ -run 'GameType'`
Expected: FAIL in compilazione (`store.ListGameTypes undefined`).

- [ ] **Step 4: Implementa `types.go`**

```go
package games

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// GameType è una tipologia di gioco (GDT, GDR, MTG...). La gestisce
// l'admin: nome, sigla, se la creazione di un gioco passa dalla ricerca
// BGG, e la posizione che decide l'ordine delle tab.
type GameType struct {
	ID        int64
	Name      string
	Slug      string
	BGGSearch bool
	Position  int
	// GameCount è quanti giochi del catalogo hanno questa tipologia.
	GameCount int
}

type GameTypeInput struct {
	Name      string
	Slug      string
	BGGSearch bool
}

type GameTypeUpdate struct {
	Name      *string
	Slug      *string
	BGGSearch *bool
}

var (
	ErrGameTypeName = errors.New("il nome della tipologia è obbligatorio")
	ErrGameTypeSlug = errors.New("la sigla deve avere da 2 a 6 lettere o cifre")
	ErrSlugTaken    = errors.New("sigla già usata da un'altra tipologia")
)

// GameTypeInUseError blocca l'eliminazione di una tipologia che ha
// ancora giochi: prima si spostano, poi si elimina.
type GameTypeInUseError struct{ Count int }

func (e *GameTypeInUseError) Error() string {
	return fmt.Sprintf("la tipologia ha ancora %d giochi", e.Count)
}

var slugPattern = regexp.MustCompile(`^[A-Z0-9]{2,6}$`)

func normalizeGameType(name, slug string) (string, string, error) {
	name = strings.TrimSpace(name)
	slug = strings.ToUpper(strings.TrimSpace(slug))
	if name == "" {
		return "", "", ErrGameTypeName
	}
	if !slugPattern.MatchString(slug) {
		return "", "", ErrGameTypeSlug
	}
	return name, slug, nil
}

const gameTypeColumns = `t.id, t.name, t.slug, t.bgg_search, t.position,
	(SELECT COUNT(*) FROM games g WHERE g.game_type_id = t.id)`

func scanGameType(row interface{ Scan(...any) error }) (GameType, error) {
	var gt GameType
	err := row.Scan(&gt.ID, &gt.Name, &gt.Slug, &gt.BGGSearch, &gt.Position, &gt.GameCount)
	return gt, err
}

func (s *Store) ListGameTypes(ctx context.Context) ([]GameType, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+gameTypeColumns+` FROM game_types t ORDER BY t.position, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameType{}
	for rows.Next() {
		gt, err := scanGameType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, gt)
	}
	return out, rows.Err()
}

func (s *Store) GetGameType(ctx context.Context, id int64) (GameType, error) {
	gt, err := scanGameType(s.db.QueryRowContext(ctx, `SELECT `+gameTypeColumns+` FROM game_types t WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return GameType{}, ErrNotFound
	}
	return gt, err
}

// slugTaken confronta già normalizzato (maiuscolo): lo slug salvato lo è
// sempre, quindi basta l'uguaglianza. excludeID è la tipologia che si sta
// modificando, che può tenere la propria sigla.
func (s *Store) slugTaken(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_types WHERE slug = ? AND id <> ?`, slug, excludeID).Scan(&n)
	return n > 0, err
}

func (s *Store) CreateGameType(ctx context.Context, in GameTypeInput) (GameType, error) {
	name, slug, err := normalizeGameType(in.Name, in.Slug)
	if err != nil {
		return GameType{}, err
	}
	if taken, err := s.slugTaken(ctx, slug, 0); err != nil {
		return GameType{}, err
	} else if taken {
		return GameType{}, ErrSlugTaken
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_types (name, slug, bgg_search, position)
		 VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM game_types))`,
		name, slug, in.BGGSearch)
	if err != nil {
		return GameType{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GameType{}, err
	}
	return s.GetGameType(ctx, id)
}

func (s *Store) UpdateGameType(ctx context.Context, id int64, upd GameTypeUpdate) (GameType, error) {
	current, err := s.GetGameType(ctx, id)
	if err != nil {
		return GameType{}, err
	}
	name, slug := current.Name, current.Slug
	if upd.Name != nil {
		name = *upd.Name
	}
	if upd.Slug != nil {
		slug = *upd.Slug
	}
	name, slug, err = normalizeGameType(name, slug)
	if err != nil {
		return GameType{}, err
	}
	if taken, err := s.slugTaken(ctx, slug, id); err != nil {
		return GameType{}, err
	} else if taken {
		return GameType{}, ErrSlugTaken
	}
	bgg := current.BGGSearch
	if upd.BGGSearch != nil {
		bgg = *upd.BGGSearch
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE game_types SET name = ?, slug = ?, bgg_search = ? WHERE id = ?`, name, slug, bgg, id); err != nil {
		return GameType{}, err
	}
	return s.GetGameType(ctx, id)
}

// MoveGameType scambia la posizione con la vicina sopra (up) o sotto.
// Agli estremi non fa niente: la freccia è solo un tocco a vuoto.
func (s *Store) MoveGameType(ctx context.Context, id int64, up bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT position FROM game_types WHERE id = ?`, id).Scan(&pos); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	q := `SELECT id, position FROM game_types WHERE position > ? ORDER BY position LIMIT 1`
	if up {
		q = `SELECT id, position FROM game_types WHERE position < ? ORDER BY position DESC LIMIT 1`
	}
	var otherID int64
	var otherPos int
	if err := tx.QueryRowContext(ctx, q, pos).Scan(&otherID, &otherPos); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_types SET position = ? WHERE id = ?`, otherPos, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_types SET position = ? WHERE id = ?`, pos, otherID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteGameType(ctx context.Context, id int64) error {
	gt, err := s.GetGameType(ctx, id)
	if err != nil {
		return err
	}
	if gt.GameCount > 0 {
		return &GameTypeInUseError{Count: gt.GameCount}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM game_types WHERE id = ?`, id)
	return err
}

// DefaultGameTypeID è la tipologia di un gioco creato senza indicarla:
// la prima in ordine, quella della prima tab.
func (s *Store) DefaultGameTypeID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM game_types ORDER BY position, id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (s *Store) GameTypeExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_types WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}
```

Nota su `DeleteGameType`: il controllo e il DELETE non sono nella stessa transazione. Con un solo admin tipico va bene; se il reviewer lo chiede, spostali in una tx.

- [ ] **Step 5: Esegui i test, verifica che passino**

Run: `GOTEST ./internal/games/ ./internal/db/`
Expected: PASS (inclusi i test esistenti su `kind`, non ancora toccati).

- [ ] **Step 6: Commit** (solo con consenso)

```bash
git add backend/internal/db/migrations/0027_game_types.sql backend/internal/games/types.go backend/internal/games/types_test.go
git commit -m "feat: game_types table and store"
```

---

### Task 2: I giochi usano `game_type_id`

**Files:**
- Modify: `backend/internal/games/store.go` (struct `Game`, `GameUpdate`, `CreateGame`, `GetGame`, `ListGames`, `UpdateGame`; rimuovi `KindBoard`, `KindRPG`, `Kinds`, `ValidKind`)
- Modify: `backend/internal/games/store_test.go:380-447` (test su kind)
- Modify: `backend/internal/httpapi/games_handlers.go:34-58,119,162`
- Modify: `backend/internal/httpapi/games_read_handlers.go:105,124-133`
- Modify: `backend/internal/httpapi/games_responses.go:22`
- Modify: `backend/internal/httpapi/events_responses.go:40`
- Modify: `backend/internal/httpapi/qr_handlers.go:116` (card del foglio)
- Modify: `backend/internal/httpapi/games_read_handlers_test.go:216-257`

**Interfaces:**
- Consumes: `DefaultGameTypeID`, `GameTypeExists` (Task 1).
- Produces: `games.Game.GameTypeID int64`, `games.GameUpdate.GameTypeID *int64`; JSON `gameTypeId` in: summary gioco, giochi di un evento, card di `/api/games/qr`; richieste `POST /api/games` e `PATCH /api/games/{id}` accettano `gameTypeId` (numero).

- [ ] **Step 1: Riscrivi i test di store su kind**

In `store_test.go` sostituisci i test da `TestCreateGame_DefaultsKind…` (riga ~383) a `TestValidKind` compreso con:

```go
func TestCreateGame_DefaultsToFirstGameType(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created, err := store.CreateGame(ctx, games.Game{Name: "Azul"})
	if err != nil {
		t.Fatal(err)
	}
	if created.GameTypeID != 1 {
		t.Fatalf("type = %d, want 1 (GDT)", created.GameTypeID)
	}
	// Il default segue l'ordine: tolta la prima tipologia (vuota), vale la successiva.
	if err := store.MoveGameType(ctx, 2, true); err != nil {
		t.Fatal(err)
	}
	next, _ := store.CreateGame(ctx, games.Game{Name: "Vampire"})
	if next.GameTypeID != 2 {
		t.Fatalf("type = %d, want 2 after reorder", next.GameTypeID)
	}
}

func TestDeleteFirstGameType_DefaultMovesOn(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// GDT (1) è vuota e prima in ordine: si può eliminare.
	if err := store.DeleteGameType(ctx, 1); err != nil {
		t.Fatal(err)
	}
	g, err := store.CreateGame(ctx, games.Game{Name: "Azul"})
	if err != nil {
		t.Fatal(err)
	}
	if g.GameTypeID != 2 {
		t.Fatalf("type = %d, want 2", g.GameTypeID)
	}
}

func TestUpdateGame_GameTypeAndHidden(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created, err := store.CreateGame(ctx, games.Game{Name: "D&D"})
	if err != nil {
		t.Fatal(err)
	}
	typeID, hidden := int64(2), true
	updated, err := store.UpdateGame(ctx, created.ID, games.GameUpdate{GameTypeID: &typeID, HiddenFromCatalog: &hidden})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GameTypeID != 2 || !updated.HiddenFromCatalog {
		t.Fatalf("updated = %+v", updated)
	}
	listed, _ := store.ListGames(ctx)
	if len(listed) != 1 || listed[0].GameTypeID != 2 {
		t.Fatalf("list does not carry the type: %+v", listed)
	}
}
```

`TestDeleteGameType` (Task 1) può restare con l'SQL diretto.

- [ ] **Step 2: Riscrivi i test HTTP su kind**

In `games_read_handlers_test.go` sostituisci `TestGameKind_DefaultsToBoardAndIsEditable` e `TestCreateGame_AcceptsKind` con:

```go
func TestGameType_DefaultsToFirstAndIsEditable(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	if got := listGameNames(t, router, nil)["Azul"]["gameTypeId"]; got != float64(1) {
		t.Fatalf("gameTypeId = %v, want 1", got)
	}
	if rec := patchGame(t, router, cookie, id, map[string]any{"gameTypeId": 2}); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if got := listGameNames(t, router, nil)["Azul"]["gameTypeId"]; got != float64(2) {
		t.Fatalf("gameTypeId = %v, want 2", got)
	}
	if rec := patchGame(t, router, cookie, id, map[string]any{"gameTypeId": 99}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown type: expected 400, got %d", rec.Code)
	}
}

func TestCreateGame_AcceptsGameType(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	for typeID, want := range map[int]int{2: http.StatusCreated, 99: http.StatusBadRequest} {
		payload, _ := json.Marshal(map[string]any{"languageCode": "it", "name": fmt.Sprintf("Gioco %d", typeID), "gameTypeId": typeID})
		req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("type %d: expected %d, got %d %s", typeID, want, rec.Code, rec.Body.String())
		}
	}
	if got := listGameNames(t, router, nil)["Gioco 2"]["gameTypeId"]; got != float64(2) {
		t.Fatalf("gameTypeId = %v, want 2", got)
	}
}
```

Cerca altri test che controllano `"kind"` nelle risposte degli eventi o del QR (`grep -rn '"kind"' backend/internal/httpapi/*_test.go`) e aggiornali a `"gameTypeId"`.

Run: `GOTEST ./internal/games/ ./internal/httpapi/ -run 'GameType|Kind'`
Expected: FAIL in compilazione (`GameTypeID` non esiste).

- [ ] **Step 3: Aggiorna `store.go`**

- In `Game` sostituisci il campo `Kind string` e il suo commento con:

```go
	// GameTypeID è la tipologia del gioco (tabella game_types). Zero in
	// CreateGame vale la prima tipologia in ordine: chi non la indica
	// ottiene quella della prima tab.
	GameTypeID int64
```

- Cancella il blocco `const ( KindBoard … )`, `var Kinds` e `func ValidKind`.
- In `GameUpdate` sostituisci `Kind *string` con `GameTypeID *int64`.
- In `CreateGame` sostituisci il default di `Kind` con:

```go
	if g.GameTypeID == 0 {
		id, err := s.DefaultGameTypeID(ctx)
		if err != nil {
			return Game{}, err
		}
		g.GameTypeID = id
	}
```

  e nell'INSERT sostituisci la colonna `kind` con `game_type_id` e l'argomento `g.Kind` con `g.GameTypeID`.
- In `GetGame` e `ListGames`: nella SELECT `kind` → `game_type_id`, nello Scan `&g.Kind` → `&g.GameTypeID`.
- In `UpdateGame`: `if upd.GameTypeID != nil { current.GameTypeID = *upd.GameTypeID }`, e nell'UPDATE `kind = ?` → `game_type_id = ?` con `current.GameTypeID`.

- [ ] **Step 4: Aggiorna gli handler**

`games_handlers.go` — nel `createGameRequest` sostituisci `Kind`:

```go
	// GameTypeID è la tipologia (tabella game_types): assente vale la
	// prima in ordine.
	GameTypeID *int64 `json:"gameTypeId"`
```

Sostituisci il controllo `if req.Kind != "" && !games.ValidKind(req.Kind)` con:

```go
	if req.GameTypeID != nil && !s.validGameType(w, r, *req.GameTypeID) {
		return
	}
```

e alle righe ~119 e ~162 `Kind: req.Kind` → `GameTypeID: derefID(req.GameTypeID)`. Aggiungi in fondo al file:

```go
// validGameType scrive già la risposta d'errore quando l'id non va: il
// chiamante deve solo uscire.
func (s *Server) validGameType(w http.ResponseWriter, r *http.Request, id int64) bool {
	ok, err := s.Games.GameTypeExists(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check game type")
		return false
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "tipologia di gioco sconosciuta")
		return false
	}
	return true
}

func derefID(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
```

`games_read_handlers.go` — in `updateGameRequest` `Kind *string `json:"kind"`` → `GameTypeID *int64 `json:"gameTypeId"``; il controllo `ValidKind` diventa `if req.GameTypeID != nil && !s.validGameType(w, r, *req.GameTypeID) { return }`; `Kind: req.Kind` → `GameTypeID: req.GameTypeID`.

`games_responses.go:22` — `"kind": g.Kind` → `"gameTypeId": g.GameTypeID`.
`events_responses.go:40` — `"kind": g.Kind` → `"gameTypeId": g.GameTypeID`.
`qr_handlers.go:116` — `card := map[string]any{"id": g.ID, "title": g.Name, "gameTypeId": g.GameTypeID}`.

- [ ] **Step 5: Esegui l'intera suite**

Run: `GOTEST ./...`
Expected: PASS. Se qualche test fuori da quelli toccati usa ancora `Kind`/`KindRPG`, aggiornalo a `GameTypeID: 2`.

Verifica anche che non restino riferimenti: `grep -rn 'Kind\b\|KindRPG\|KindBoard\|ValidKind' backend/internal --include='*.go'` non deve trovare nulla legato alla tipologia (restano i `mediaKind` ecc. se esistono).

- [ ] **Step 6: Commit** (solo con consenso)

```bash
git add backend/internal
git commit -m "feat: games reference game_types instead of a hardcoded kind"
```

---

### Task 3: API `/api/game-types`

**Files:**
- Create: `backend/internal/httpapi/game_types_handlers.go`
- Modify: `backend/internal/httpapi/router.go` (rotte)
- Test: `backend/internal/httpapi/game_types_handlers_test.go`

**Interfaces:**
- Consumes: store del Task 1, `s.hasAdminSession(r)` (esistente), `writeJSON`/`writeError`, `parseIDParam`.
- Produces: JSON tipologia `{"id","name","slug","bggSearch","position"}` più `"gameCount"` solo con sessione admin (in lista) e sempre nelle risposte delle rotte admin. Rotte:
  - `GET /api/game-types` (pubblica)
  - `POST /api/game-types` → 201
  - `PATCH /api/game-types/{id}` → 200
  - `POST /api/game-types/{id}/move` body `{"direction":"up"|"down"}` → 204
  - `DELETE /api/game-types/{id}` → 204; 409 `{"error":…, "games":N}`

- [ ] **Step 1: Scrivi i test che falliscono**

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func gameTypesRequest(t *testing.T, router http.Handler, cookie *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func listGameTypes(t *testing.T, router http.Handler, cookie *http.Cookie) []map[string]any {
	t.Helper()
	rec := gameTypesRequest(t, router, cookie, http.MethodGet, "/api/game-types", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGameTypes_PublicListHidesCounts(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	public := listGameTypes(t, router, nil)
	if len(public) != 2 || public[0]["slug"] != "GDT" || public[0]["bggSearch"] != true {
		t.Fatalf("public = %v", public)
	}
	if _, ok := public[0]["gameCount"]; ok {
		t.Fatal("gameCount is admin-only")
	}
	admin := listGameTypes(t, router, cookie)
	if admin[0]["gameCount"] != float64(0) {
		t.Fatalf("admin = %v", admin)
	}
}

func TestGameTypes_WritesNeedAdmin(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/game-types"},
		{http.MethodPatch, "/api/game-types/1"},
		{http.MethodPost, "/api/game-types/1/move"},
		{http.MethodDelete, "/api/game-types/1"},
	} {
		if rec := gameTypesRequest(t, router, nil, c.method, c.path, map[string]any{}); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", c.method, c.path, rec.Code)
		}
	}
}

func TestGameTypes_CreateUpdateMove(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := gameTypesRequest(t, router, cookie, http.MethodPost, "/api/game-types",
		map[string]any{"name": "Magic: The Gathering", "slug": "mtg", "bggSearch": false})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	json.NewDecoder(rec.Body).Decode(&created)
	if created["slug"] != "MTG" || created["position"] != float64(3) {
		t.Fatalf("created = %v", created)
	}
	id := int64(created["id"].(float64))

	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, "/api/game-types",
		map[string]any{"name": "Doppione", "slug": "gdt"}); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate slug: %d", rec.Code)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, "/api/game-types",
		map[string]any{"name": "", "slug": "XY"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name: %d", rec.Code)
	}

	if rec := gameTypesRequest(t, router, cookie, http.MethodPatch, fmt.Sprintf("/api/game-types/%d", id),
		map[string]any{"name": "Magic"}); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodPatch, "/api/game-types/99",
		map[string]any{"name": "X"}); rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing: %d", rec.Code)
	}

	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, fmt.Sprintf("/api/game-types/%d/move", id),
		map[string]any{"direction": "up"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body.String())
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodPost, fmt.Sprintf("/api/game-types/%d/move", id),
		map[string]any{"direction": "sideways"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad direction: %d", rec.Code)
	}
	list := listGameTypes(t, router, cookie)
	if list[1]["name"] != "Magic" {
		t.Fatalf("order after move = %v", list)
	}
}

func TestGameTypes_DeleteBlockedWhenInUse(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createTestGame(t, router, cookie, "Azul") // prende GDT (id 1)

	rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("in use: %d", rec.Code)
	}
	var body map[string]any
	json.NewDecoder(rec.Body).Decode(&body)
	if body["games"] != float64(1) {
		t.Fatalf("body = %v", body)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/2", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("empty: %d", rec.Code)
	}
	if rec := gameTypesRequest(t, router, cookie, http.MethodDelete, "/api/game-types/2", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("gone: %d", rec.Code)
	}
}
```

Run: `GOTEST ./internal/httpapi/ -run GameTypes`
Expected: FAIL (404 sulle rotte).

- [ ] **Step 2: Implementa gli handler**

`backend/internal/httpapi/game_types_handlers.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"boardgames-manager/internal/games"
)

func toGameTypeResponse(gt games.GameType, withCount bool) map[string]any {
	out := map[string]any{
		"id": gt.ID, "name": gt.Name, "slug": gt.Slug,
		"bggSearch": gt.BGGSearch, "position": gt.Position,
	}
	// Quanti giochi ha una tipologia è un dato di gestione: la pagina
	// pubblica conta i giochi che ha già in mano.
	if withCount {
		out["gameCount"] = gt.GameCount
	}
	return out
}

func (s *Server) listGameTypesHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Games.ListGameTypes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list game types")
		return
	}
	admin := s.hasAdminSession(r)
	out := make([]map[string]any, 0, len(list))
	for _, gt := range list {
		out = append(out, toGameTypeResponse(gt, admin))
	}
	writeJSON(w, http.StatusOK, out)
}

// writeGameTypeError traduce gli errori dello store; restituisce false se
// l'errore non è uno di quelli noti.
func writeGameTypeError(w http.ResponseWriter, err error) bool {
	var inUse *games.GameTypeInUseError
	switch {
	case errors.Is(err, games.ErrNotFound):
		writeError(w, http.StatusNotFound, "tipologia non trovata")
	case errors.Is(err, games.ErrGameTypeName), errors.Is(err, games.ErrGameTypeSlug):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, games.ErrSlugTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &inUse):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "games": inUse.Count})
	default:
		return false
	}
	return true
}

type gameTypeRequest struct {
	Name      *string `json:"name"`
	Slug      *string `json:"slug"`
	BGGSearch *bool   `json:"bggSearch"`
}

func (s *Server) createGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	var req gameTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in := games.GameTypeInput{BGGSearch: true}
	if req.Name != nil {
		in.Name = *req.Name
	}
	if req.Slug != nil {
		in.Slug = *req.Slug
	}
	if req.BGGSearch != nil {
		in.BGGSearch = *req.BGGSearch
	}
	gt, err := s.Games.CreateGameType(r.Context(), in)
	if err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not create game type")
		}
		return
	}
	writeJSON(w, http.StatusCreated, toGameTypeResponse(gt, true))
}

func (s *Server) updateGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game type id")
		return
	}
	var req gameTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	gt, err := s.Games.UpdateGameType(r.Context(), id, games.GameTypeUpdate{Name: req.Name, Slug: req.Slug, BGGSearch: req.BGGSearch})
	if err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not update game type")
		}
		return
	}
	writeJSON(w, http.StatusOK, toGameTypeResponse(gt, true))
}

func (s *Server) moveGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game type id")
		return
	}
	var req struct {
		Direction string `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Direction != "up" && req.Direction != "down") {
		writeError(w, http.StatusBadRequest, `direction deve essere "up" o "down"`)
		return
	}
	if err := s.Games.MoveGameType(r.Context(), id, req.Direction == "up"); err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not move game type")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game type id")
		return
	}
	if err := s.Games.DeleteGameType(r.Context(), id); err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not delete game type")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

In `router.go`: tra le rotte pubbliche, accanto a `r.Get("/api/games", …)`:

```go
	r.Get("/api/game-types", s.listGameTypesHandler)
```

Nel gruppo `protected`, dopo `protected.Delete("/api/games/{id}", …)`:

```go
		protected.Post("/api/game-types", s.createGameTypeHandler)
		protected.Patch("/api/game-types/{id}", s.updateGameTypeHandler)
		protected.Post("/api/game-types/{id}/move", s.moveGameTypeHandler)
		protected.Delete("/api/game-types/{id}", s.deleteGameTypeHandler)
```

- [ ] **Step 3: Esegui l'intera suite**

Run: `GOTEST ./...`
Expected: PASS.

- [ ] **Step 4: Commit** (solo con consenso)

```bash
git add backend/internal/httpapi
git commit -m "feat: admin API for game types"
```

---

### Task 4: Store frontend, pastiglie e form gioco

**Files:**
- Create: `frontend/src/stores/gameTypes.ts`
- Delete: `frontend/src/utils/gameKinds.ts`
- Modify: `frontend/src/utils/game.ts:60-61` (`kind` → `gameTypeId`)
- Modify: `frontend/src/views/GameNewView.vue` (select, `onBgg`, payload, testo sotto il titolo)
- Modify: `frontend/src/views/GameAdminDetailView.vue:15,246,585-591`
- Modify: `frontend/src/app.css:2813-2816` (`.kind-chip.kind-rpg` → palette)

**Interfaces:**
- Produces:

```ts
export interface GameType { id: number; name: string; slug: string; bggSearch: boolean; position: number; gameCount?: number }
useGameTypesStore(): {
  list: GameType[]; loaded: boolean
  load(force?: boolean): Promise<void>
  byId(id: number | null | undefined): GameType | undefined
  defaultType: GameType | undefined
  colorClass(id: number | null | undefined): string   // 'type-color-0'..'type-color-5'
}
```

- [ ] **Step 1: Crea lo store**

`frontend/src/stores/gameTypes.ts`:

```ts
import { defineStore } from 'pinia'
import { api } from '../api/client'

/** Una tipologia di gioco (GDT, GDR, MTG...), gestita dall'admin. */
export interface GameType {
  id: number
  name: string
  slug: string
  /** Se la creazione di un gioco di questa tipologia passa da BoardGameGeek. */
  bggSearch: boolean
  position: number
  /** Solo con sessione admin. */
  gameCount?: number
}

/**
 * Quanti colori ha la palette delle pastiglie (`.type-color-N` in app.css).
 * Il colore segue l'ordine: la prima tipologia prende il neutro, la seconda
 * l'accento, e così via ricominciando.
 */
const PALETTE_SIZE = 6

export const useGameTypesStore = defineStore('gameTypes', {
  state: () => ({
    list: [] as GameType[],
    loaded: false,
  }),
  getters: {
    defaultType: (state) => state.list[0],
  },
  actions: {
    /** Una richiesta per sessione; `force` dopo una modifica admin. */
    async load(force = false) {
      if (this.loaded && !force) {
        return
      }
      try {
        this.list = await api.get<GameType[]>('/game-types')
      } catch (e) {
        // Senza tipologie le liste restano senza tab e le pastiglie neutre:
        // la pagina si usa lo stesso.
        console.error('could not load game types', e)
      } finally {
        this.loaded = true
      }
    },
    byId(id: number | null | undefined) {
      return this.list.find((t) => t.id === id)
    },
    colorClass(id: number | null | undefined) {
      const i = this.list.findIndex((t) => t.id === id)
      return `type-color-${i < 0 ? 0 : i % PALETTE_SIZE}`
    },
  },
})
```

- [ ] **Step 2: Palette pastiglie in `app.css`**

Sostituisci il blocco `.kind-chip.kind-rpg { … }` (e il suo commento) con:

```css
/* Il colore della pastiglia segue l'ordine delle tipologie (store
   gameTypes, PALETTE_SIZE = 6): la prima resta neutra, la seconda prende
   il colore della casa, poi i toni del sistema. Non lo sceglie l'admin:
   così qualunque tipologia nuova resta dentro la palette. */
.type-color-0 { background: var(--card); color: var(--ink); }
.type-color-1 { background: var(--accent); color: #fff; }
.type-color-2 { background: var(--felt); color: var(--felt-text); }
.type-color-3 { background: var(--gold-text); color: #fff; }
.type-color-4 { background: var(--success); color: #fff; }
.type-color-5 { background: var(--ink); color: var(--card); }
```

Il `.kind-chip` base resta com'è (la sua `background: var(--card)` viene sovrascritta dalla classe colore, che sta dopo nel file). Rinomina `.kind-chip` → `.type-chip` in tutto `app.css` (incluso il commento a riga ~2699) e nei template.

- [ ] **Step 3: Aggiorna i tipi e la pastiglia**

`utils/game.ts:60-61`:

```ts
  /** La tipologia (store gameTypes). */
  gameTypeId: number
```

In `EventDetailView.vue` (riga ~27) `kind: string` → `gameTypeId: number`, e la pastiglia (righe ~448-451):

```vue
        <span
          v-if="types.byId(g.gameTypeId)"
          class="type-chip"
          :class="types.colorClass(g.gameTypeId)"
          :title="types.byId(g.gameTypeId)!.name"
        >
          <span aria-hidden="true">{{ types.byId(g.gameTypeId)!.slug }}</span>
          <span class="visually-hidden">{{ types.byId(g.gameTypeId)!.name }}</span>
        </span>
```

con `const types = useGameTypesStore()` e `onMounted(() => types.load())` (o dentro l'`onMounted` esistente). Il filtro "Tipo" di questa vista lo sostituisce il Task 5: per ora rimuovi `SelectFilter` "Tipo", `kind`, `hasKinds` e il ramo `kind.value` di `visibleGames`/`resetFilters`, in modo che il build passi. Stessa rimozione in `CatalogView.vue` (`kind`, `showKindFilter`, ramo del filtro, `SelectFilter` "Tipo", campo `kind` dell'interfaccia → `gameTypeId: number`).

- [ ] **Step 4: `GameNewView.vue`**

Sostituisci l'import di `gameKinds` con `import { useGameTypesStore } from '../stores/gameTypes'` e:

```ts
const types = useGameTypesStore()
const gameTypeId = ref<number | null>(null)
const selectedType = computed(() => types.byId(gameTypeId.value) ?? types.defaultType)
const kindOnBgg = computed(() => selectedType.value?.bggSearch ?? true)
const isManual = computed(() => manual.value || !kindOnBgg.value)

onMounted(async () => {
  await types.load()
  gameTypeId.value = types.defaultType?.id ?? null
})
```

(aggiungi `onMounted` all'import da `vue` se manca; se esiste già un `onMounted`, metti lì le due righe). Nei due payload `kind: kind.value` → `gameTypeId: gameTypeId.value ?? undefined`. La select:

```vue
          <select v-model="gameTypeId">
            <option v-for="t in types.list" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
```

Il testo sotto il titolo:

```vue
          {{ kindOnBgg ? "Cercalo su BoardGameGeek, o inseriscilo a mano se non c'è." : `${selectedType?.name ?? 'Questa tipologia'}: si inserisce a mano.` }}
```

- [ ] **Step 5: `GameAdminDetailView.vue`**

Import store al posto di `gameKinds`; `const types = useGameTypesStore()` e `types.load()` nell'`onMounted` esistente. `saveClassification(patch: { gameTypeId?: number; hiddenFromCatalog?: boolean })`. La select:

```vue
              <select
                :value="game.gameTypeId"
                @change="saveClassification({ gameTypeId: Number(($event.target as HTMLSelectElement).value) })"
              >
                <option v-for="t in types.list" :key="t.id" :value="t.id">{{ t.name }}</option>
              </select>
```

Se dentro `saveClassification` il risultato aggiorna `game.value.kind`, cambialo in `gameTypeId`.

- [ ] **Step 6: Cancella `utils/gameKinds.ts` e verifica il build**

```bash
rm frontend/src/utils/gameKinds.ts
grep -rn "gameKinds\|kind-chip\|\.kind\b" frontend/src   # nessun risultato atteso (salvo mediaKind)
cd frontend && npm run build
```

Expected: build OK, nessun errore `vue-tsc`.

- [ ] **Step 7: Commit** (solo con consenso)

```bash
git add frontend/src
git commit -m "feat: game types store replaces hardcoded kinds in the frontend"
```

---

### Task 5: Componente `GameTypeTabs` nelle cinque liste

**Files:**
- Create: `frontend/src/components/GameTypeTabs.vue`
- Modify: `frontend/src/app.css` (stili `.type-tabs`, vicino a `.catalog-filters` ~riga 6362)
- Modify: `frontend/src/views/CatalogView.vue`
- Modify: `frontend/src/views/EventDetailView.vue`
- Modify: `frontend/src/views/GamesView.vue`
- Modify: `frontend/src/components/EventGamesPicker.vue`
- Modify: `frontend/src/views/QrSheetView.vue`
- Modify: `frontend/src/views/EventAdminDetailView.vue`, `frontend/src/views/EventNewView.vue` (passano `gameTypeId` nei `PickerGame`, arriva già da `/games`)

**Interfaces:**
- Consumes: `useGameTypesStore` (Task 4); `gameTypeId` su giochi, giochi-evento e card QR (Task 2).
- Produces: `<GameTypeTabs v-model="typeId" :games="list" />` con `typeId: Ref<number | null>` (`null` = "Tutti"), `games: readonly { gameTypeId: number }[]`. Non renderizza nulla con meno di due tipologie presenti. Helper esportato `matchesType(g, typeId)`.

- [ ] **Step 1: Crea il componente**

`frontend/src/components/GameTypeTabs.vue`:

```vue
<script lang="ts">
/** Vero se il gioco sta nella tab scelta (`null` = "Tutti"). */
export function matchesType(g: { gameTypeId: number }, typeId: number | null) {
  return typeId === null || g.gameTypeId === typeId
}
</script>

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useGameTypesStore } from '../stores/gameTypes'

/**
 * Le tab per tipologia sopra una lista di giochi. Compaiono solo se la
 * lista mescola almeno due tipologie, e mostrano solo quelle presenti:
 * una tab che porta a una lista vuota è un tocco sprecato. L'ordine è
 * quello deciso dall'admin.
 */
const props = defineProps<{
  games: readonly { gameTypeId: number }[]
  modelValue: number | null
}>()
const emit = defineEmits<{ 'update:modelValue': [value: number | null] }>()

const types = useGameTypesStore()
onMounted(() => types.load())

const tabs = computed(() =>
  types.list
    .map((t) => ({ type: t, count: props.games.filter((g) => g.gameTypeId === t.id).length }))
    .filter((t) => t.count > 0),
)

// Se la tab scelta sparisce (la lista cambia), si torna a "Tutti": una
// tab invisibile che filtra tutto via lascerebbe la pagina vuota.
watch(tabs, (list) => {
  if (props.modelValue !== null && !list.some((t) => t.type.id === props.modelValue)) {
    emit('update:modelValue', null)
  }
})
</script>

<template>
  <div v-if="tabs.length >= 2" class="type-tabs" role="tablist" aria-label="Tipologia">
    <button
      type="button"
      role="tab"
      :aria-selected="modelValue === null"
      :class="{ 'is-active': modelValue === null }"
      @click="emit('update:modelValue', null)"
    >
      Tutti <span class="type-tab-count">{{ games.length }}</span>
    </button>
    <button
      v-for="t in tabs"
      :key="t.type.id"
      type="button"
      role="tab"
      :aria-selected="modelValue === t.type.id"
      :class="{ 'is-active': modelValue === t.type.id }"
      :title="t.type.name"
      @click="emit('update:modelValue', t.type.id)"
    >
      {{ t.type.name }} <span class="type-tab-count">{{ t.count }}</span>
    </button>
  </div>
</template>
```

- [ ] **Step 2: Stili in `app.css`**

```css
/* Tab per tipologia sopra le liste di giochi. Sul telefono scorrono in
   orizzontale invece di andare a capo: restano una riga sola, e la lista
   sotto non si sposta cambiando tab. */
.type-tabs {
  display: flex;
  gap: 0.4rem;
  overflow-x: auto;
  scrollbar-width: none;
  margin: 0 0 1rem;
  padding-bottom: 0.1rem;
}
.type-tabs::-webkit-scrollbar { display: none; }
.type-tabs button {
  flex: none;
  min-height: 44px;
  padding: 0.45rem 0.9rem;
  border: 1px solid var(--card-line);
  border-radius: 999px;
  background: var(--card);
  color: var(--ink);
  font: inherit;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
}
.type-tabs button.is-active {
  background: var(--felt);
  border-color: var(--felt);
  color: var(--felt-text);
}
.type-tab-count {
  margin-left: 0.25rem;
  font-weight: 400;
  opacity: 0.75;
}
@media print {
  .type-tabs { display: none; }
}
```

Nota: `li button` ha una regola che colora d'allarme ogni bottone in una lista (vedi commento in `app.css` vicino a `.event-game-actions`). Le tab non stanno in un `li`, quindi non serve override; se nel picker finiscono dentro una lista, aggiungi `.type-tabs button` con specificità sufficiente.

- [ ] **Step 3: Integra nelle viste**

In ognuna: `import GameTypeTabs, { matchesType } from '../components/GameTypeTabs.vue'`, `const typeId = ref<number | null>(null)`.

- **`CatalogView.vue`**: `<GameTypeTabs v-model="typeId" :games="games" />` subito prima di `<div class="catalog-filters">`; in `visible` aggiungi `&& matchesType(g, typeId.value)`; `filtering` include `typeId.value !== null`; `resetFilters` azzera `typeId`.
- **`EventDetailView.vue`**: `<GameTypeTabs v-model="typeId" :games="event.games" />` prima di `<div v-if="showFilters" class="event-filters">`; `visibleGames` aggiunge `&& matchesType(g, typeId.value)`; `resetFilters` azzera `typeId`.
- **`GamesView.vue`**: aggiungi `gameTypeId: number` a `GameSummary`, `const visible = computed(() => games.value.filter((g) => matchesType(g, typeId.value)))`, `<GameTypeTabs v-model="typeId" :games="games" />` sopra `<div class="game-grid">`, `v-for="g in visible"`.
- **`EventGamesPicker.vue`**: aggiungi `gameTypeId: number` a `PickerGame`; `<GameTypeTabs v-model="typeId" :games="available" />` sopra il campo di ricerca; `filtered` parte da `available.value.filter((g) => matchesType(g, typeId.value))`. Le righe già scelte non si filtrano.
- **`QrSheetView.vue`**: aggiungi `gameTypeId: number` a `SheetCard`; `const visibleCards = computed(() => cards.value.filter((c) => matchesType(c, typeId.value)))`; `<GameTypeTabs v-model="typeId" :games="cards" />` sopra `.qr-sheet` (fuori dall'area stampata); `v-for="c in visibleCards"`. Si stampa solo la tab scelta.

Controlla che `EventAdminDetailView.vue` e `EventNewView.vue` passino al picker gli oggetti di `/games` così come arrivano (con `gameTypeId`); se li rimappano, includi il campo.

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: OK.

- [ ] **Step 5: Commit** (solo con consenso)

```bash
git add frontend/src
git commit -m "feat: game type tabs on every game list"
```

---

### Task 6: Pagina admin "Tipologie"

**Files:**
- Create: `frontend/src/views/GameTypesView.vue`
- Modify: `frontend/src/router/index.ts` (rotta `/admin/game-types`, name `admin-game-types`)
- Modify: `frontend/src/components/AppShell.vue:66` (voce menu)
- Modify: `frontend/src/app.css` (stili `.type-admin-list`)

**Interfaces:**
- Consumes: API del Task 3, `useGameTypesStore().load(true)` per rinfrescare lo store dopo ogni modifica.

- [ ] **Step 1: Rotta e menu**

In `router/index.ts` aggiungi accanto a `admin-users`:

```ts
    { path: '/admin/game-types', name: 'admin-game-types', component: GameTypesView },
```

con `import GameTypesView from '../views/GameTypesView.vue'` accanto agli altri import di viste (segui lo stile esistente: import statico o lazy come le altre).

In `AppShell.vue`, in `adminItems` dopo "Giochi":

```ts
  { label: 'Tipologie', to: '/admin/game-types', icon: 'box', matches: (p) => p.startsWith('/admin/game-types') },
```

Se `AppShell` ha un set di icone con nomi fissi, scegline una esistente (es. `sliders` o `box`); non aggiungere SVG nuovi se non serve. Verifica che il `matches` di "Giochi" (`startsWith('/admin/games')`) **non** catturi `/admin/game-types` — non lo fa (`games` ≠ `game-`).

- [ ] **Step 2: La vista**

`frontend/src/views/GameTypesView.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import { useGameTypesStore, type GameType } from '../stores/gameTypes'

/**
 * Le tipologie di gioco: ognuna diventa una tab sopra le liste di giochi,
 * nell'ordine di questa pagina. Una tipologia con giochi non si elimina:
 * prima si spostano i giochi dalla loro scheda.
 */
const store = useGameTypesStore()
const error = ref('')
const editingId = ref<number | null>(null)
const draft = ref({ name: '', slug: '', bggSearch: true })
const newType = ref({ name: '', slug: '', bggSearch: true })
const busy = ref(false)

async function refresh() {
  await store.load(true)
}

async function run(action: () => Promise<unknown>) {
  error.value = ''
  busy.value = true
  try {
    await action()
    await refresh()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function startEdit(t: GameType) {
  editingId.value = t.id
  draft.value = { name: t.name, slug: t.slug, bggSearch: t.bggSearch }
}

const saveEdit = (id: number) =>
  run(async () => {
    await api.patch(`/game-types/${id}`, draft.value)
    editingId.value = null
  })

const move = (id: number, direction: 'up' | 'down') =>
  run(() => api.post(`/game-types/${id}/move`, { direction }))

const remove = (id: number) => run(() => api.delete(`/game-types/${id}`))

const create = () =>
  run(async () => {
    await api.post('/game-types', newType.value)
    newType.value = { name: '', slug: '', bggSearch: true }
  })

onMounted(refresh)
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Tipologie di gioco</h1>
        <p class="page-meta">Ogni tipologia è una tab sopra le liste di giochi, in quest'ordine.</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <div class="panel-card">
      <ul role="list" class="type-admin-list">
        <li v-for="(t, i) in store.list" :key="t.id">
          <template v-if="editingId === t.id">
            <form class="type-admin-edit" @submit.prevent="saveEdit(t.id)">
              <label>Nome <input v-model="draft.name" required /></label>
              <label>Sigla <input v-model="draft.slug" required maxlength="6" /></label>
              <label class="checkbox-label">
                <input v-model="draft.bggSearch" type="checkbox" /> Cerca su BoardGameGeek
              </label>
              <div class="form-actions">
                <button type="submit" :disabled="busy">Salva</button>
                <button type="button" class="secondary" @click="editingId = null">Annulla</button>
              </div>
            </form>
          </template>
          <template v-else>
            <span class="type-chip is-inline" :class="store.colorClass(t.id)">{{ t.slug }}</span>
            <div class="type-admin-text">
              <strong>{{ t.name }}</strong>
              <span class="page-meta">
                {{ t.bggSearch ? 'Da BoardGameGeek' : 'Inserimento a mano' }} ·
                {{ t.gameCount === 1 ? '1 gioco' : `${t.gameCount ?? 0} giochi` }}
              </span>
            </div>
            <div class="type-admin-actions">
              <button type="button" class="icon-button" :disabled="busy || i === 0" aria-label="Sposta su" @click="move(t.id, 'up')">↑</button>
              <button type="button" class="icon-button" :disabled="busy || i === store.list.length - 1" aria-label="Sposta giù" @click="move(t.id, 'down')">↓</button>
              <button type="button" class="secondary" :disabled="busy" @click="startEdit(t)">Modifica</button>
              <button
                type="button"
                class="danger"
                :disabled="busy || (t.gameCount ?? 0) > 0"
                :title="(t.gameCount ?? 0) > 0 ? `Ha ${t.gameCount} giochi: spostali prima` : 'Elimina'"
                @click="remove(t.id)"
              >
                Elimina
              </button>
            </div>
          </template>
        </li>
      </ul>
    </div>

    <form class="panel-card type-admin-edit" @submit.prevent="create">
      <div class="section-head"><h2>Nuova tipologia</h2></div>
      <label>Nome <input v-model="newType.name" required placeholder="Magic: The Gathering" /></label>
      <label>Sigla <input v-model="newType.slug" required maxlength="6" placeholder="MTG" /></label>
      <label class="checkbox-label">
        <input v-model="newType.bggSearch" type="checkbox" /> Cerca su BoardGameGeek
      </label>
      <div class="form-actions">
        <button type="submit" :disabled="busy">Aggiungi</button>
      </div>
    </form>
  </div>
</template>
```

Prima di scrivere il template, controlla in `app.css` e nelle viste admin esistenti (es. `UsersView.vue`) i nomi reali delle classi per bottoni secondari/pericolosi/icona (`secondary`, `danger`, `icon-button` sono ipotesi) (`api.patch`/`api.delete` esistono in `client.ts`, e una 204 torna `undefined`). Adegua il template a ciò che esiste; non inventare classi nuove se ce n'è già una equivalente. Il `.type-chip` in riga ha `position: absolute`: aggiungi una variante `.type-chip.is-inline { position: static; }` se non esiste.

Il messaggio "Ha N giochi" deve essere visibile anche su mobile (dove il `title` non si vede): mostralo come testo nella riga `page-meta` quando `gameCount > 0`, ad esempio aggiungendo " · non eliminabile" dopo il conteggio.

- [ ] **Step 3: Stili `.type-admin-list`**

```css
/* La lista delle tipologie: pastiglia, testo, azioni. Su telefono le
   azioni vanno a capo sotto il testo invece di schiacciarlo. */
.type-admin-list { display: grid; gap: 0.75rem; margin: 0; padding: 0; }
.type-admin-list li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.6rem 0.9rem;
}
.type-admin-text { display: grid; flex: 1 1 12rem; }
.type-admin-actions { display: flex; flex-wrap: wrap; gap: 0.4rem; }
.type-admin-edit { display: grid; gap: 0.6rem; }
```

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: OK.

- [ ] **Step 5: Commit** (solo con consenso)

```bash
git add frontend/src
git commit -m "feat: admin page to manage game types"
```

---

### Task 7: Documentazione, verifica end-to-end, impeccable

**Files:**
- Modify: `DESIGN.md` (tab per tipologia, palette pastiglie `.type-color-N`, `.type-chip`)
- Modify: `README.md` (le tipologie si gestiscono da Admin → Tipologie; MTG come esempio)
- Modify: `CLAUDE.md` (sezione *Catalogo giochi & arricchimento*: una riga "Ogni gioco ha una tipologia (`game_types`, gestita da admin) con flag ricerca BGG; le liste si dividono in tab per tipologia.")

- [ ] **Step 1: Aggiorna i tre documenti** con le righe sopra; in `DESIGN.md` sostituisci eventuali riferimenti a `.kind-chip`/`kind-rpg`.

- [ ] **Step 2: Suite completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./...
cd frontend && npm run build
```

Expected: tutto PASS / build OK. Riporta l'output.

- [ ] **Step 3: Verifica nel browser** (Claude in Chrome, `docker compose up -d --build`, http://localhost:8080)

1. DB esistente: in Admin → Tipologie ci sono GDT e GDR con i conteggi giusti; i giochi di ruolo di prima hanno la pastiglia GDR.
2. Crea "Magic: The Gathering" / `mtg` senza BGG → appare come `MTG` in fondo.
3. Aggiungi gioco → scegli MTG → si apre direttamente l'inserimento manuale; salva.
4. Tab: catalogo pubblico, pagina evento pubblica (aggiungi il gioco MTG a un evento), catalogo admin, picker giochi di creazione/dettaglio evento, foglio QR (anteprima di stampa: solo la tab scelta, tab non stampate).
5. Tab che sparisce: nel catalogo pubblico scegli la tab MTG, poi nascondi dal catalogo l'unico gioco MTG e ricarica → nessuna tab MTG, lista su "Tutti".
6. Prova a eliminare GDT → bottone disabilitato con "non eliminabile"; sposta MTG su/giù, anche agli estremi.
7. Viewport mobile (375px) su catalogo e pagina evento: tab su una riga, scorrevoli. Console senza errori.

- [ ] **Step 4: `/impeccable`** (ultimo task, per le regole del progetto) su: `GameTypeTabs`, `GameTypesView`, pastiglie. Usa `polish` o `audit`; applica i fix, poi ripeti `npm run build`.

- [ ] **Step 5: Commit** (solo con consenso)

```bash
git add DESIGN.md README.md CLAUDE.md frontend/src
git commit -m "docs: game types and type tabs"
```
