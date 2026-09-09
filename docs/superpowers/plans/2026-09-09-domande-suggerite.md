# Domande suggerite generate dal manuale — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Le tre domande suggerite nella chat pubblica le scrive il modello leggendo i titoli di sezione del manuale, e l'admin può riscriverle a mano senza che una reindicizzazione gliele cancelli.

**Architecture:** Una tabella `game_suggested_question` con tre righe per gioco (`position` 0-2) e un flag `edited` per riga. `ai.SuggestQuestions` genera le tre domande dai titoli distinti che `manuals.Summary` già calcola, con validazione meccanica della risposta perché il testo finisce sulla scheda pubblica. L'indicizzazione le rigenera best-effort saltando le posizioni modificate a mano; tre rotte admin permettono di leggerle, riscriverle e forzarne la rigenerazione.

**Tech Stack:** Go 1.25, chi, SQLite (modernc.org/sqlite), Vue 3 `<script setup>` + TypeScript, Vite.

**Spec:** `docs/superpowers/specs/2026-09-09-domande-suggerite-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto (binario x86_64 su Mac arm64). Ogni `go test`/`go build`/`go vet` va lanciato così, riusando i due volumi nominati:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Non sostituire `bgm-gomodcache`/`bgm-gocache` con volumi anonimi: ogni esecuzione ripartirebbe da zero. I target `backend-*` del `Makefile` **non funzionano** su questa macchina.
- **`npm` in locale**, senza Docker: `cd frontend && npm run build` (fa anche il type-check con `vue-tsc`).
- **Migrazioni forward-only.** Nuovo file `NNNN_nome.sql` in `backend/internal/db/migrations/`, embeddato con `//go:embed`, applicato in ordine alfabetico. Nessun down, nessuna modifica a un file già rilasciato. Il numero più alto oggi è `0015_game_sources.sql`.
- **Nessuna dipendenza nuova**, né Go né npm. Il progetto preferisce stdlib e codice esplicito.
- **Nessun i18n.** UI in italiano, stringhe direttamente nei componenti.
- **Commit in inglese**, conventional commits (`feat:`, `fix:`, `docs:`). Ogni commit chiude un task.
- **`git commit` sì, `git push` no**: non spingere nulla se non richiesto esplicitamente.
- **Tre domande, sempre.** Non due, non cinque: `position` ∈ {0,1,2}.

---

## File Structure

**Backend**

| File | Responsabilità |
|---|---|
| `backend/internal/db/migrations/0016_suggested_questions.sql` | *(creare)* la tabella e il suo indice unico |
| `backend/internal/manuals/questions.go` | *(creare)* `SuggestedQuestion` e i quattro metodi di `Store` che leggono/scrivono le domande |
| `backend/internal/manuals/questions_test.go` | *(creare)* test dello store, incluso il caso che conta (rigenerazione che preserva le modificate) |
| `backend/internal/ai/suggest.go` | *(creare)* `QuestionSuggester`, `SuggestQuestions`, `ErrSuggestionsRejected`, validazione |
| `backend/internal/ai/suggest_test.go` | *(creare)* test HTTP-level della generazione |
| `backend/internal/ai/suggest_internal_test.go` | *(creare)* test della validazione pura |
| `backend/internal/httpapi/questions_handlers.go` | *(creare)* le tre rotte admin e il helper `suggester(ctx)` |
| `backend/internal/httpapi/questions_handlers_test.go` | *(creare)* test delle rotte |
| `backend/internal/httpapi/router.go` | *(modificare)* campo `Suggester` nello `Server`, tre rotte in `protected` |
| `backend/internal/httpapi/manuals_handlers.go` | *(modificare)* innesto best-effort in coda a `indexMediaHandler` |
| `backend/internal/httpapi/games_responses.go` | *(modificare)* `sourceHeadings` → `suggestedQuestions` |
| `backend/internal/manuals/store.go` | *(modificare)* via il cap `maxSuggestionHeadings` |

Le domande vivono in `manuals.Store` e non in `games.Store` perché derivano dai dati che quello store già possiede (i `heading` dei chunk) e appartengono al sottosistema della chat: `games_responses.go` chiama già `s.Manuals.Summary`.

Un file nuovo `questions.go` invece di allungare `store.go` (535 righe, già il più grande del package): le domande sono una responsabilità distinta dai chunk e dalla ricerca FTS.

**Frontend**

| File | Responsabilità |
|---|---|
| `frontend/src/components/ManualChatPanel.vue` | *(modificare)* rende le domande ricevute; via `headingToQuestion` e la computed `suggestions` |
| `frontend/src/components/ManualChat.vue` | *(modificare)* la prop `headings` diventa `suggestedQuestions` |
| `frontend/src/views/GameDetailView.vue` | *(modificare)* legge `suggestedQuestions` dalla risposta |
| `frontend/src/components/SuggestedQuestionsPanel.vue` | *(creare)* l'editor admin: tre campi, rigenera, segno su quali sono a mano |
| `frontend/src/views/GameAdminDetailView.vue` | *(modificare)* monta il pannello nella sezione "Chatbot"; corregge una frase ora falsa |

---

## Task 1: Tabella e store delle domande

**Files:**
- Create: `backend/internal/db/migrations/0016_suggested_questions.sql`
- Create: `backend/internal/manuals/questions.go`
- Create: `backend/internal/manuals/questions_test.go`

**Interfaces:**
- Consumes: `manuals.NewStore(conn *sql.DB) *Store` (esistente, `store.go:44`)
- Produces:
  ```go
  type SuggestedQuestion struct {
      Position int
      Text     string
      Edited   bool
  }
  func (s *Store) SuggestedQuestions(ctx context.Context, gameID int64) ([]SuggestedQuestion, error)
  func (s *Store) SaveGeneratedQuestions(ctx context.Context, gameID int64, texts []string) error
  func (s *Store) SaveAllQuestions(ctx context.Context, gameID int64, texts []string) error
  func (s *Store) SaveEditedQuestions(ctx context.Context, gameID int64, texts []string) error
  ```

- [ ] **Step 1: Scrivi la migrazione**

Crea `backend/internal/db/migrations/0016_suggested_questions.sql`:

```sql
-- Le tre domande suggerite mostrate nello stato di riposo della chat.
-- Per GIOCO e non per manuale né per lingua: la chat è per gioco e cerca in
-- tutte le fonti insieme, quindi una domanda suggerita non appartiene a un
-- singolo documento.
CREATE TABLE game_suggested_question (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,

    -- 0, 1, 2: l'ordine in cui compaiono a schermo.
    position INTEGER NOT NULL,

    text TEXT NOT NULL,

    -- 1 = riscritta a mano dall'admin. Una reindicizzazione rigenera solo
    -- le righe con edited = 0: sostituire uno scan brutto con uno buono
    -- aggiorna le domande da sé, senza mai perdere il lavoro manuale.
    edited INTEGER NOT NULL DEFAULT 0,

    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Rende impossibile la posizione 1 due volte per lo stesso gioco, ed è
-- l'indice su cui si appoggia l'upsert di SaveEditedQuestions.
CREATE UNIQUE INDEX idx_suggested_question_game_pos
    ON game_suggested_question(game_id, position);
```

- [ ] **Step 2: Scrivi i test dello store (falliscono)**

Crea `backend/internal/manuals/questions_test.go`. Il package di test è `manuals_test`, e l'helper del DB in memoria è `newTestDB(t *testing.T) *sql.DB`, già definito in `store_test.go:43` (apre `:memory:` e applica le migrazioni): usa quello, non uno nuovo.

```go
package manuals_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/manuals"
)

// seedGame crea un gioco minimo: le domande hanno una foreign key su
// games, quindi senza una riga vera l'insert fallisce.
func seedGameForQuestions(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO games (name) VALUES ('Gioco di Prova')`)
	if err != nil {
		t.Fatalf("seed game: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

func TestSuggestedQuestions_EmptyWhenNothingSaved(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID := seedGameForQuestions(t, conn)

	got, err := store.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("un gioco senza domande deve restituire una lista vuota, ottenuto %v", got)
	}
}

func TestSaveGeneratedQuestions_WritesThreeInOrder(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID := seedGameForQuestions(t, conn)

	texts := []string{"Come si prepara?", "Cosa faccio nel turno?", "Come finisce?"}
	if err := store.SaveGeneratedQuestions(context.Background(), gameID, texts); err != nil {
		t.Fatalf("save generated: %v", err)
	}

	got, err := store.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande, ottenute %d: %v", len(got), got)
	}
	for i, q := range got {
		if q.Position != i {
			t.Fatalf("posizione %d attesa in indice %d, ottenuta %d", i, i, q.Position)
		}
		if q.Text != texts[i] {
			t.Fatalf("posizione %d: atteso %q, ottenuto %q", i, texts[i], q.Text)
		}
		if q.Edited {
			t.Fatalf("posizione %d: una domanda generata non è edited", i)
		}
	}
}

// TestSaveGeneratedQuestions_PreservesEdited è IL test di questo task: è il
// comportamento scelto in fase di design. Una reindicizzazione riscrive le
// domande generate dal modello e non tocca mai quelle scritte a mano.
func TestSaveGeneratedQuestions_PreservesEdited(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveGeneratedQuestions(ctx, gameID,
		[]string{"Generata A?", "Generata B?", "Generata C?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}
	// L'admin riscrive la seconda: solo quella diventa edited.
	if err := store.SaveEditedQuestions(ctx, gameID,
		[]string{"Generata A?", "Scritta a mano?", "Generata C?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}
	// Una reindicizzazione rigenera tutte e tre.
	if err := store.SaveGeneratedQuestions(ctx, gameID,
		[]string{"Rigenerata A?", "Rigenerata B?", "Rigenerata C?"}); err != nil {
		t.Fatalf("save generated dopo edit: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	want := []string{"Rigenerata A?", "Scritta a mano?", "Rigenerata C?"}
	for i, q := range got {
		if q.Text != want[i] {
			t.Fatalf("posizione %d: atteso %q, ottenuto %q (tutte: %v)", i, want[i], q.Text, got)
		}
	}
	if !got[1].Edited {
		t.Fatal("la domanda scritta a mano deve restare marcata edited")
	}
	if got[0].Edited || got[2].Edited {
		t.Fatal("le domande rigenerate non devono essere marcate edited")
	}
}

// TestSaveEditedQuestions_MarksOnlyChangedTexts: il flag lo decide il
// server confrontando col testo salvato, non un campo che arriva dal
// client. Senza questo, un salvataggio che non cambia nulla congelerebbe
// tutte tre le domande per sempre.
func TestSaveEditedQuestions_MarksOnlyChangedTexts(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveGeneratedQuestions(ctx, gameID,
		[]string{"Generata A?", "Generata B?", "Generata C?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}
	// Rimanda le tre domande cambiandone solo una.
	if err := store.SaveEditedQuestions(ctx, gameID,
		[]string{"Generata A?", "Cambiata?", "Generata C?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if got[0].Edited || got[2].Edited {
		t.Fatalf("un testo rimandato invariato non diventa edited: %v", got)
	}
	if !got[1].Edited {
		t.Fatal("il solo testo cambiato deve diventare edited")
	}
}

// TestSaveEditedQuestions_UpsertsOnAGameWithNoRows: un gioco mai
// indicizzato non ha righe, e l'admin deve poter scrivere le sue tre
// domande a mano prima (o invece) di qualunque generazione.
func TestSaveEditedQuestions_UpsertsOnAGameWithNoRows(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveEditedQuestions(ctx, gameID,
		[]string{"Prima?", "Seconda?", "Terza?"}); err != nil {
		t.Fatalf("save edited su gioco senza righe: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande, ottenute %d", len(got))
	}
	for i, q := range got {
		if !q.Edited {
			t.Fatalf("posizione %d: una domanda creata da zero a mano nasce edited", i)
		}
	}
}

// TestSaveAllQuestions_OverwritesEditedToo: è il pulsante "rigenera", che
// l'admin premette deliberatamente — quindi sovrascrive tutto e azzera i
// flag.
func TestSaveAllQuestions_OverwritesEditedToo(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveEditedQuestions(ctx, gameID,
		[]string{"A mano 1?", "A mano 2?", "A mano 3?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}
	if err := store.SaveAllQuestions(ctx, gameID,
		[]string{"Nuova 1?", "Nuova 2?", "Nuova 3?"}); err != nil {
		t.Fatalf("save all: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	for i, q := range got {
		if q.Text != []string{"Nuova 1?", "Nuova 2?", "Nuova 3?"}[i] {
			t.Fatalf("posizione %d non sovrascritta: %q", i, q.Text)
		}
		if q.Edited {
			t.Fatalf("posizione %d: rigenera azzera edited, ottenuto true", i)
		}
	}
}

func TestSuggestedQuestions_CascadeOnGameDelete(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveGeneratedQuestions(ctx, gameID,
		[]string{"A?", "B?", "C?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}
	if _, err := conn.Exec(`DELETE FROM games WHERE id = ?`, gameID); err != nil {
		t.Fatalf("delete game: %v", err)
	}

	var count int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM game_suggested_question WHERE game_id = ?`, gameID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("la cascata deve portare via le domande col gioco, restano %d righe", count)
	}
}
```

Aggiungi `"database/sql"` agli import.

- [ ] **Step 3: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -run TestSuggestedQuestions -run TestSave -count=1
```

Atteso: FAIL in build, `store.SuggestedQuestions undefined`.

- [ ] **Step 4: Implementa `questions.go`**

Crea `backend/internal/manuals/questions.go`:

```go
package manuals

import (
	"context"
	"database/sql"
	"fmt"
)

// SuggestedQuestionCount è quante domande suggerite ha un gioco: tre, come
// i tre bottoni nello stato di riposo della chat. Non è configurabile —
// vedi i non-obiettivi della spec.
const SuggestedQuestionCount = 3

// SuggestedQuestion è una delle tre domande mostrate nello stato di riposo
// della chat. Edited dice che l'ha riscritta l'admin: è il flag che
// protegge il suo lavoro da una reindicizzazione.
type SuggestedQuestion struct {
	Position int
	Text     string
	Edited   bool
}

// SuggestedQuestions restituisce le domande di un gioco in ordine di
// posizione. Una lista vuota è un esito normale: un gioco mai indicizzato
// non ne ha, e il frontend ripiega sulle domande fisse.
func (s *Store) SuggestedQuestions(ctx context.Context, gameID int64) ([]SuggestedQuestion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT position, text, edited FROM game_suggested_question
		 WHERE game_id = ? ORDER BY position`, gameID)
	if err != nil {
		return nil, fmt.Errorf("suggested questions: %w", err)
	}
	defer rows.Close()

	var out []SuggestedQuestion
	for rows.Next() {
		var q SuggestedQuestion
		if err := rows.Scan(&q.Position, &q.Text, &q.Edited); err != nil {
			return nil, fmt.Errorf("scan suggested question: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// SaveGeneratedQuestions scrive le domande prodotte dal modello SALTANDO le
// posizioni che l'admin ha riscritto a mano. È il percorso della
// reindicizzazione: uno scan migliore aggiorna le domande da sé, il lavoro
// manuale non si perde mai.
//
// La clausola WHERE excluded.edited = 0 non basterebbe: excluded è la riga
// in arrivo, che ha sempre edited = 0. Il confronto va fatto sulla riga
// ESISTENTE, ed è per questo che l'upsert ha la sua condizione.
func (s *Store) SaveGeneratedQuestions(ctx context.Context, gameID int64, texts []string) error {
	return s.saveQuestions(ctx, gameID, texts,
		`INSERT INTO game_suggested_question (game_id, position, text, edited)
		 VALUES (?, ?, ?, 0)
		 ON CONFLICT(game_id, position) DO UPDATE SET text = excluded.text
		 WHERE game_suggested_question.edited = 0`)
}

// SaveAllQuestions sovrascrive tutte e tre le domande e azzera edited: è il
// pulsante "rigenera", premuto deliberatamente dall'admin. Se lo premi, lo
// stai chiedendo — anche per le domande che avevi scritto a mano.
func (s *Store) SaveAllQuestions(ctx context.Context, gameID int64, texts []string) error {
	return s.saveQuestions(ctx, gameID, texts,
		`INSERT INTO game_suggested_question (game_id, position, text, edited)
		 VALUES (?, ?, ?, 0)
		 ON CONFLICT(game_id, position) DO UPDATE SET text = excluded.text, edited = 0`)
}

// SaveEditedQuestions salva i testi che arrivano dal pannello admin,
// marcando edited SOLO quelli diversi da ciò che era salvato. Il confronto
// lo fa il server e non un flag del client: solo il server sa cosa aveva
// scritto il modello, e un client che rimanda invariata una domanda
// generata non deve poterla promuovere a "scritta a mano" — altrimenti un
// salvataggio senza modifiche congelerebbe le tre domande per sempre.
//
// È un upsert perché un gioco mai indicizzato non ha righe: l'admin deve
// poter scrivere le sue tre domande prima (o invece) di ogni generazione, e
// quelle nascono edited.
func (s *Store) SaveEditedQuestions(ctx context.Context, gameID int64, texts []string) error {
	if len(texts) != SuggestedQuestionCount {
		return fmt.Errorf("suggested questions: attesi %d testi, ricevuti %d",
			SuggestedQuestionCount, len(texts))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	for i, text := range texts {
		// Il testo attuale, se c'è: serve a decidere se questo salvataggio
		// è una modifica o un no-op.
		var current string
		err := tx.QueryRowContext(ctx,
			`SELECT text FROM game_suggested_question WHERE game_id = ? AND position = ?`,
			gameID, i).Scan(&current)
		switch {
		case err == sql.ErrNoRows:
			current = "" // nessuna riga: qualunque testo è nuovo, quindi a mano
		case err != nil:
			return fmt.Errorf("read current question %d: %w", i, err)
		}

		edited := 1
		if text == current {
			// Rimandata invariata: conserva il flag che aveva, non
			// promuoverla.
			if _, err := tx.ExecContext(ctx,
				`UPDATE game_suggested_question SET text = ?
				 WHERE game_id = ? AND position = ?`, text, gameID, i); err != nil {
				return fmt.Errorf("update question %d: %w", i, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO game_suggested_question (game_id, position, text, edited)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT(game_id, position) DO UPDATE SET text = excluded.text, edited = 1`,
			gameID, i, text, edited); err != nil {
			return fmt.Errorf("upsert question %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// saveQuestions è il corpo comune di SaveGeneratedQuestions e
// SaveAllQuestions: stessa transazione, stesso ciclo, solo la query
// dell'upsert cambia.
func (s *Store) saveQuestions(ctx context.Context, gameID int64, texts []string, stmt string) error {
	if len(texts) != SuggestedQuestionCount {
		return fmt.Errorf("suggested questions: attesi %d testi, ricevuti %d",
			SuggestedQuestionCount, len(texts))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	for i, text := range texts {
		if _, err := tx.ExecContext(ctx, stmt, gameID, i, text); err != nil {
			return fmt.Errorf("save question %d: %w", i, err)
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 5: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -count=1
```

Atteso: `ok`. Se `TestSaveGeneratedQuestions_PreservesEdited` fallisce, il problema è quasi certamente la `WHERE` dell'upsert: la condizione va sulla riga esistente (`game_suggested_question.edited = 0`), non su `excluded.edited`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0016_suggested_questions.sql \
        backend/internal/manuals/questions.go \
        backend/internal/manuals/questions_test.go
git commit -m "feat: store the three suggested questions per game

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Generazione delle domande col modello

**Files:**
- Create: `backend/internal/ai/suggest.go`
- Create: `backend/internal/ai/suggest_internal_test.go`
- Create: `backend/internal/ai/suggest_test.go`

**Interfaces:**
- Consumes: `(*HTTPClient).postChat(ctx, payload []byte, timeout time.Duration) (string, error)` e `(*HTTPClient).configured() bool`, entrambi già in `ai/ask.go` e `ai/client.go`. `chatRequest`/`chatMessage` sono i tipi già usati da `Segment`.
- Produces:
  ```go
  type QuestionSuggester interface {
      SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error)
  }
  var ErrSuggestionsRejected error
  func (c *HTTPClient) SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error)
  ```

- [ ] **Step 1: Scrivi il test della validazione pura (falisce)**

Crea `backend/internal/ai/suggest_internal_test.go`:

```go
package ai

import (
	"strings"
	"testing"
)

// TestParseSuggestions fissa cosa si accetta dal modello. La validazione è
// meccanica e non un'esortazione nel prompt perché questo testo va sulla
// SCHEDA PUBBLICA: un modello che risponde "Ecco tre domande:" non deve
// poter mettere quella riga a schermo.
func TestParseSuggestions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		ok   bool
	}{
		{
			name: "tre domande pulite",
			in:   "Come si prepara il gioco?\nCosa posso fare nel mio turno?\nCome finisce la partita?",
			want: []string{"Come si prepara il gioco?", "Cosa posso fare nel mio turno?", "Come finisce la partita?"},
			ok:   true,
		},
		{
			name: "righe vuote in mezzo si ignorano",
			in:   "Come si prepara?\n\nCosa faccio nel turno?\n\n\nCome finisce?\n",
			want: []string{"Come si prepara?", "Cosa faccio nel turno?", "Come finisce?"},
			ok:   true,
		},
		{
			name: "spazi ai bordi si tolgono",
			in:   "  Come si prepara?  \n\tCosa faccio?\t\nCome finisce?",
			want: []string{"Come si prepara?", "Cosa faccio?", "Come finisce?"},
			ok:   true,
		},
		{name: "due domande non bastano", in: "Come si prepara?\nCome finisce?", ok: false},
		{
			name: "quattro domande sono troppe",
			in:   "Uno?\nDue?\nTre?\nQuattro?",
			ok:   false,
		},
		{
			// Il caso che la validazione esiste per fermare.
			name: "un preambolo conta come riga e sfora",
			in:   "Ecco tre domande:\nCome si prepara?\nCosa faccio?\nCome finisce?",
			ok:   false,
		},
		{
			name: "una riga che non è una domanda",
			in:   "Come si prepara?\nQuesto gioco è bello.\nCome finisce?",
			ok:   false,
		},
		{
			// La riga lunga si costruisce con strings.Repeat invece di
			// scriverla a mano: una domanda "abbastanza lunga" contata a
			// occhio può finire sotto il tetto e far passare il test per
			// il motivo sbagliato.
			name: "una domanda troppo lunga per un bottone",
			in:   "Come si prepara?\n" + strings.Repeat("x", MaxSuggestionChars+1) + "?\nCome finisce?",
			ok:   false,
		},
		{name: "risposta vuota", in: "", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSuggestions(tc.in)
			if tc.ok && err != nil {
				t.Fatalf("atteso valido, rifiutato con %v", err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("atteso rifiuto, accettato %v", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("attese %d domande, ottenute %d: %v", len(tc.want), len(got), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("domanda %d: atteso %q, ottenuto %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Lancia il test e verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -run TestParseSuggestions -count=1
```

Atteso: FAIL in build, `undefined: parseSuggestions`.

- [ ] **Step 3: Implementa `suggest.go`**

Crea `backend/internal/ai/suggest.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// suggestTimeout è corto rispetto a segmentTimeout: la richiesta è ~200
// token di titoli e la risposta sono tre righe. Se non torna in mezzo
// minuto non tornerà.
const suggestTimeout = 30 * time.Second

// MaxSuggestionChars è il tetto per domanda. Le domande vivono in tre
// bottoni su uno schermo di telefono: una domanda di duecento caratteri
// non è una domanda suggerita, è un paragrafo.
//
// Esportata perché il tetto è UNO: una domanda scritta a mano dall'admin
// vive negli stessi tre bottoni di una generata, e il validatore della
// rotta PUT (httpapi) deve usare questo valore, non una sua copia.
const MaxSuggestionChars = 120

// ErrSuggestionsRejected dice che la risposta del modello non è tre
// domande. È un errore distinto da un guasto di rete perché il chiamante
// deve poterlo raccontare diversamente: al pannello admin serve "il
// modello non ha risposto come doveva, riprova", non un errore generico.
var ErrSuggestionsRejected = errors.New("ai suggestions rejected: response is not three questions")

// QuestionSuggester è l'astrazione che serve all'indicizzazione e al
// pannello admin. HTTPClient la implementa; i test iniettano un finto.
// Stesso schema di Segmenter e Transcriber.
type QuestionSuggester interface {
	SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error)
}

// suggestSystemPrompt chiede tre domande e vieta tutto il resto. Non è una
// garanzia — la mitigazione vera è parseSuggestions — ma è ciò che rende
// il rifiuto raro invece che normale.
const suggestSystemPrompt = "Ricevi il nome di un gioco da tavolo e l'elenco dei titoli di sezione del suo regolamento. " +
	"Scrivi TRE domande che un giocatore farebbe al tavolo, in italiano, a cui il regolamento risponde.\n" +
	"Regole assolute:\n" +
	"1. Esattamente tre domande, una per riga. Nessuna numerazione, nessun elenco puntato, nessun preambolo, nessun commento.\n" +
	"2. Ogni riga deve finire con un punto di domanda.\n" +
	"3. Ogni domanda sta sotto i 120 caratteri: sono tre bottoni su uno schermo di telefono.\n" +
	"4. IGNORA i titoli che non sono regole: il nome dell'autore, il contenuto della scatola, l'indice, i ringraziamenti, i crediti.\n" +
	"5. Scrivi domande come le porrebbe un giocatore (\"Quando finisce la partita?\"), non come una ricerca nel manuale (\"Cosa dice il manuale sulla fine della partita?\").\n" +
	"6. Preferisci le domande che si fanno davvero durante una partita: preparazione, cosa si può fare nel proprio turno, come si contano i punti, quando finisce."

// SuggestQuestions chiede al modello tre domande per la scheda del gioco,
// partendo dai titoli di sezione del manuale già indicizzato.
//
// Usa il modello di TESTO (c.Model) e non quello vision: qui non c'è
// nessuna immagine. E non riprova sugli errori transitori — vale lo stesso
// confine di Segment: il retry vive in Transcribe, l'unica chiamata che si
// fa N volte per un solo documento.
func (c *HTTPClient) SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error) {
	if !c.configured() {
		return nil, ErrNotConfigured
	}
	if len(headings) == 0 {
		// Nessun titolo da cui partire: il chiamante deve dirlo all'admin
		// ("indicizza prima un manuale"), non ricevere tre domande
		// inventate dal nulla.
		return nil, ErrSuggestionsRejected
	}

	user := fmt.Sprintf("Gioco: %s\n\nTitoli delle sezioni del regolamento:\n- %s",
		gameName, strings.Join(headings, "\n- "))

	payload, err := json.Marshal(chatRequest{
		Model: c.Model,
		// temperature 0: le domande di un manuale non devono cambiare a
		// ogni indicizzazione. Se l'admin ne vuole altre, c'è "rigenera".
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: suggestSystemPrompt},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return nil, err
	}

	out, err := c.postChat(ctx, payload, suggestTimeout)
	if err != nil {
		return nil, err
	}
	return parseSuggestions(out)
}

// parseSuggestions valida la risposta del modello: esattamente tre righe
// non vuote, ciascuna una domanda e ciascuna abbastanza corta per un
// bottone. Qualunque altra cosa è ErrSuggestionsRejected.
//
// Le righe vuote si scartano prima di contare (un modello che separa le
// domande con una riga bianca non ha sbagliato niente di sostanziale), ma
// una riga di testo in più — un preambolo, un commento — fa sforare il
// conto ed è esattamente ciò che si vuole fermare.
func parseSuggestions(raw string) ([]string, error) {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}

	if len(out) != 3 {
		return nil, fmt.Errorf("%w: attese 3 righe, ricevute %d", ErrSuggestionsRejected, len(out))
	}
	for i, q := range out {
		if !strings.HasSuffix(q, "?") {
			return nil, fmt.Errorf("%w: la riga %d non è una domanda: %q", ErrSuggestionsRejected, i+1, q)
		}
		if len([]rune(q)) > MaxSuggestionChars {
			return nil, fmt.Errorf("%w: la riga %d supera %d caratteri", ErrSuggestionsRejected, i+1, MaxSuggestionChars)
		}
	}
	return out, nil
}
```

Nota su `len([]rune(q))`: il tetto è in **caratteri**, non byte. Su testo italiano accentato `len(q)` conterebbe di più e rifiuterebbe domande legittime.

- [ ] **Step 4: Lancia il test e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -run TestParseSuggestions -count=1
```

Atteso: PASS.

- [ ] **Step 5: Scrivi il test HTTP-level (falisce)**

Crea `backend/internal/ai/suggest_test.go`:

```go
package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
)

func TestSuggestQuestions_UsesTheTextModelAndTheHeadings(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		io.WriteString(w, `{"choices":[{"message":{"content":"Come si piazza una tessera?\nQuando finisce la partita?\nCome si contano i punti?"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp")
	got, err := client.SuggestQuestions(context.Background(), "Carcassonne",
		[]string{"Preparazione", "Piazzare le tessere", "Conteggio dei punti"})
	if err != nil {
		t.Fatalf("suggest questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande, ottenute %d: %v", len(got), got)
	}
	if got[0] != "Come si piazza una tessera?" {
		t.Fatalf("prima domanda inattesa: %q", got[0])
	}

	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("il body non è JSON valido: %v", err)
	}
	// Qui non c'è nessuna immagine: deve usare il modello di testo.
	if sent.Model != "deepseek-v4-flash" {
		t.Fatalf("atteso il modello di testo, inviato %q", sent.Model)
	}
	// I titoli devono arrivare al modello, altrimenti inventa.
	if !strings.Contains(gotBody, "Piazzare le tessere") {
		t.Fatalf("i titoli di sezione non sono nel prompt:\n%s", gotBody)
	}
	if !strings.Contains(gotBody, "Carcassonne") {
		t.Fatalf("il nome del gioco non è nel prompt:\n%s", gotBody)
	}
}

func TestSuggestQuestions_RejectsAPreamble(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"Ecco tre domande:\nUno?\nDue?\nTre?"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	_, err := client.SuggestQuestions(context.Background(), "Gioco", []string{"Preparazione"})
	if !errors.Is(err, ai.ErrSuggestionsRejected) {
		t.Fatalf("atteso ErrSuggestionsRejected, ottenuto %v", err)
	}
}

func TestSuggestQuestions_WithoutHeadingsIsRejectedWithoutCallingTheProvider(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{"choices":[{"message":{"content":"Uno?\nDue?\nTre?"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	_, err := client.SuggestQuestions(context.Background(), "Gioco", nil)
	if !errors.Is(err, ai.ErrSuggestionsRejected) {
		t.Fatalf("senza titoli è un rifiuto, ottenuto %v", err)
	}
	if calls != 0 {
		t.Fatalf("senza titoli non c'è niente da chiedere: fatte %d chiamate", calls)
	}
}

func TestSuggestQuestions_WithoutProviderIsNotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	_, err := client.SuggestQuestions(context.Background(), "Gioco", []string{"Preparazione"})
	if !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}
```

- [ ] **Step 6: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -count=1
```

Atteso: `ok`. Se `TestSuggestQuestions_UsesTheTextModelAndTheHeadings` fallisce sul modello, `configured()` o `chatRequest` sono stati usati male — confronta con `Segment` in `ai/segment.go`, che ha esattamente la stessa forma.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/ai/suggest.go \
        backend/internal/ai/suggest_internal_test.go \
        backend/internal/ai/suggest_test.go
git commit -m "feat: generate three suggested questions from the manual headings

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Rigenerazione in coda all'indicizzazione

**Files:**
- Modify: `backend/internal/httpapi/router.go` (campo `Suggester` nello `Server`)
- Create: `backend/internal/httpapi/questions_handlers.go` (per ora solo `suggester(ctx)` e `regenerateQuestions`)
- Modify: `backend/internal/httpapi/manuals_handlers.go` (in coda a `indexMediaHandler`)
- Modify: `backend/internal/httpapi/manuals_handlers_test.go` (finto e test)

**Interfaces:**
- Consumes: `ai.QuestionSuggester`, `ai.ErrSuggestionsRejected` (Task 2); `(*manuals.Store).SuggestedQuestions`, `.SaveGeneratedQuestions` (Task 1); `manuals.SuggestedQuestionCount` (Task 1)
- Produces:
  ```go
  // in httpapi
  func (s *Server) suggester(ctx context.Context) ai.QuestionSuggester
  func (s *Server) regenerateQuestions(ctx context.Context, gameID int64, all bool) error
  // Server gains: Suggester ai.QuestionSuggester
  ```

- [ ] **Step 1: Scrivi i test (falliscono)**

Aggiungi a `backend/internal/httpapi/manuals_handlers_test.go`. Il finto va accanto agli altri (`fakeSegmenter`, `pageTranscriber`):

```go
// fakeSuggester sta al posto del provider per SuggestQuestions: conta le
// chiamate (serve al test che verifica che NON venga chiamato) e cattura i
// titoli ricevuti.
type fakeSuggester struct {
	calls       atomic.Int64
	lastGame    string
	lastHeading []string
	out         []string
	err         error
}

func (f *fakeSuggester) SuggestQuestions(ctx context.Context, gameName string, headings []string) ([]string, error) {
	f.calls.Add(1)
	f.lastGame = gameName
	f.lastHeading = headings
	if f.err != nil {
		return nil, f.err
	}
	if f.out != nil {
		return f.out, nil
	}
	return []string{"Generata 1?", "Generata 2?", "Generata 3?"}, nil
}

func TestIndexMedia_GeneratesSuggestedQuestions(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	sug := &fakeSuggester{}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if sug.calls.Load() != 1 {
		t.Fatalf("attesa 1 chiamata a SuggestQuestions, fatte %d", sug.calls.Load())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande salvate, ottenute %d: %v", len(got), got)
	}
	if got[0].Text != "Generata 1?" {
		t.Fatalf("prima domanda inattesa: %q", got[0].Text)
	}
}

// TestIndexMedia_SuggestionFailureStillIndexes: la generazione è
// best-effort. Trasformare un'indicizzazione riuscita in un errore per una
// domanda suggerita sarebbe fuori scala rispetto al valore della feature.
func TestIndexMedia_SuggestionFailureStillIndexes(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Suggester = &fakeSuggester{err: ai.ErrSuggestionsRejected}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("una generazione fallita non deve far fallire l'indicizzazione: %d %s",
			rec.Code, rec.Body.String())
	}
	// I chunk devono esserci comunque.
	hits, _, err := server.Manuals.Search(context.Background(), gameID, "it", []string{"mazzo"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("i chunk devono essere stati indicizzati anche senza domande suggerite")
	}
}

// TestIndexMedia_SkipsSuggestionWhenAllThreeAreEdited: se non c'è niente da
// riscrivere non c'è motivo di pagare la chiamata.
func TestIndexMedia_SkipsSuggestionWhenAllThreeAreEdited(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	sug := &fakeSuggester{}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	// Tutte tre scritte a mano prima dell'indicizzazione.
	if err := server.Manuals.SaveEditedQuestions(context.Background(), gameID,
		[]string{"Mia 1?", "Mia 2?", "Mia 3?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}

	rec := postIndex(cookie, router, gameID, mediaID)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if sug.calls.Load() != 0 {
		t.Fatalf("con tutte tre edited non c'è niente da generare: fatte %d chiamate", sug.calls.Load())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if got[0].Text != "Mia 1?" {
		t.Fatalf("le domande scritte a mano devono essere intatte: %v", got)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run "TestIndexMedia_Generates|TestIndexMedia_Suggestion|TestIndexMedia_Skips" -count=1
```

Atteso: FAIL in build, `server.Suggester undefined`.

- [ ] **Step 3: Aggiungi il campo `Suggester` allo `Server`**

In `backend/internal/httpapi/router.go`, subito dopo il campo `Segmenter`:

```go
	// Suggester, quando è valorizzato, è il generatore delle tre domande
	// suggerite. Nil = costruito per richiesta dalle impostazioni, stesso
	// schema di AI/Vision/Asker/Segmenter.
	Suggester ai.QuestionSuggester
```

- [ ] **Step 4: Crea `questions_handlers.go` con il helper e la rigenerazione**

Crea `backend/internal/httpapi/questions_handlers.go`:

```go
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/manuals"
)

// suggester restituisce il generatore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di segmenter() e transcriber(), e per la stessa ragione:
// cambiare modello non deve richiedere un riavvio.
func (s *Server) suggester(ctx context.Context) ai.QuestionSuggester {
	if s.Suggester != nil {
		return s.Suggester
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("questions: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// errNoHeadings dice che il gioco non ha nessuna fonte indicizzata, quindi
// non ci sono titoli da cui generare. Distinto dagli errori del provider
// perché il pannello admin deve dire "indicizza prima un manuale" e non
// "riprova".
var errNoHeadings = errors.New("questions: il gioco non ha nessuna fonte indicizzata")

// regenerateQuestions genera le tre domande e le salva. Con all = true
// sovrascrive anche quelle scritte a mano e azzera i flag (è il pulsante
// "rigenera"); con all = false rispetta le posizioni modificate (è la
// reindicizzazione).
//
// Non scrive niente sulla ResponseWriter: i due chiamanti raccontano
// l'esito in modo diverso — l'indicizzazione lo ignora, il pulsante lo
// riporta all'admin.
func (s *Server) regenerateQuestions(ctx context.Context, gameID int64, all bool) error {
	// Il nome del gioco lo carica questa funzione, non il chiamante:
	// indexMediaHandler ha in scope solo gameID, e farglielo caricare
	// significherebbe scriverlo due volte per i due chiamanti.
	game, err := s.Games.GetGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("questions: get game %d: %w", gameID, err)
	}

	summary, err := s.Manuals.Summary(ctx, gameID)
	if err != nil {
		return fmt.Errorf("questions: summary: %w", err)
	}
	if len(summary.Headings) == 0 {
		return errNoHeadings
	}

	texts, err := s.suggester(ctx).SuggestQuestions(ctx, game.Name, summary.Headings)
	if err != nil {
		return err
	}
	if len(texts) != manuals.SuggestedQuestionCount {
		// SuggestQuestions valida già il conteggio; questo è il controllo
		// che protegge lo store da un finto scritto male nei test.
		return fmt.Errorf("%w: ricevute %d domande", ai.ErrSuggestionsRejected, len(texts))
	}

	if all {
		return s.Manuals.SaveAllQuestions(ctx, gameID, texts)
	}
	return s.Manuals.SaveGeneratedQuestions(ctx, gameID, texts)
}

// allQuestionsEdited dice se le tre domande sono tutte scritte a mano: in
// quel caso una reindicizzazione non ha niente da riscrivere e la chiamata
// al modello si salta del tutto.
func allQuestionsEdited(qs []manuals.SuggestedQuestion) bool {
	if len(qs) < manuals.SuggestedQuestionCount {
		return false
	}
	for _, q := range qs {
		if !q.Edited {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Innesta la generazione in `indexMediaHandler`**

In `backend/internal/httpapi/manuals_handlers.go`, **subito prima** della costruzione di `resp := map[string]any{...}` in coda a `indexMediaHandler`, inserisci:

```go
	// Le tre domande suggerite si rigenerano qui, best-effort: un errore si
	// logga e si ignora. Aggiungere qualche secondo a un'operazione che su
	// un manuale scansionato ne dura più di cento non si nota, ma
	// trasformare un'indicizzazione riuscita in un errore per una domanda
	// suggerita sarebbe fuori scala rispetto al valore della feature.
	//
	// Le posizioni che l'admin ha riscritto a mano non si toccano (all =
	// false), e se sono tutte e tre a mano la chiamata al modello non parte
	// nemmeno.
	if existing, qErr := s.Manuals.SuggestedQuestions(r.Context(), gameID); qErr != nil {
		log.Printf("index: read suggested questions for game %d: %v", gameID, qErr)
	} else if !allQuestionsEdited(existing) {
		if qErr := s.regenerateQuestions(r.Context(), gameID, false); qErr != nil {
			log.Printf("index: suggested questions for game %d: %v", gameID, qErr)
		}
	}
```

Nota: `indexMediaHandler` ha in scope `gameID` ma **non** il gioco — verificato. Per questo `regenerateQuestions` carica il gioco da sé e prende solo l'id: nessun chiamante deve procurarsi il nome.

- [ ] **Step 6: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -count=1
```

Atteso: `ok`. Se `TestIndexMedia_SkipsSuggestionWhenAllThreeAreEdited` fallisce con una chiamata fatta, `allQuestionsEdited` sta ricevendo meno di tre righe: controlla che `SaveEditedQuestions` le abbia scritte tutte tre.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/router.go \
        backend/internal/httpapi/questions_handlers.go \
        backend/internal/httpapi/manuals_handlers.go \
        backend/internal/httpapi/manuals_handlers_test.go
git commit -m "feat: regenerate suggested questions after indexing

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Le tre rotte admin

**Files:**
- Modify: `backend/internal/httpapi/questions_handlers.go`
- Create: `backend/internal/httpapi/questions_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `(*Server).regenerateQuestions`, `errNoHeadings` (Task 3); i metodi dello store (Task 1); `writeJSON`/`writeError` (`httpapi/json.go`)
- Produces: le rotte `GET`/`PUT`/`POST` su `/api/games/{id}/suggested-questions`

- [ ] **Step 1: Scrivi i test (falliscono)**

Crea `backend/internal/httpapi/questions_handlers_test.go`:

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

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

func questionsPath(gameID int64) string {
	return fmt.Sprintf("/api/games/%d/suggested-questions", gameID)
}

// seedBareGame crea un gioco senza lingue né media: basta alle rotte delle
// domande, che non guardano le fonti (tranne regenerate).
func seedBareGame(t *testing.T, server *httpapi.Server) int64 {
	t.Helper()
	game, err := server.Games.CreateGame(context.Background(), games.Game{Name: "Carcassonne"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return game.ID
}

func TestGetSuggestedQuestions_RequiresAuth(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodGet, questionsPath(gameID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("atteso 401, ottenuto %d", rec.Code)
	}
}

func TestGetSuggestedQuestions_EmptyGameReturnsThreeBlanks(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodGet, questionsPath(gameID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Questions []struct {
			Text   string `json:"text"`
			Edited bool   `json:"edited"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non JSON: %v (%s)", err, rec.Body.String())
	}
	// Sempre tre voci: il pannello admin ha tre campi da riempire, e uno
	// slot vuoto è una voce con testo vuoto, non una voce assente.
	if len(resp.Questions) != 3 {
		t.Fatalf("attese 3 voci anche su un gioco vuoto, ottenute %d", len(resp.Questions))
	}
	for i, q := range resp.Questions {
		if q.Text != "" {
			t.Fatalf("voce %d: atteso testo vuoto, ottenuto %q", i, q.Text)
		}
	}
}

func TestPutSuggestedQuestions_SavesAndMarksEdited(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	body := `{"questions":["Come si piazza una tessera?","Quando finisce?","Quanti punti vale un castello?"]}`
	req := httptest.NewRequest(http.MethodPut, questionsPath(gameID), bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 || got[0].Text != "Come si piazza una tessera?" {
		t.Fatalf("domande non salvate: %v", got)
	}
	if !got[0].Edited {
		t.Fatal("un testo scritto dall'admin nasce edited")
	}
}

func TestPutSuggestedQuestions_RejectsTheWrongCount(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	body := `{"questions":["Solo una?"]}`
	req := httptest.NewRequest(http.MethodPut, questionsPath(gameID), bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("atteso 400 per un conteggio sbagliato, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutSuggestedQuestions_RejectsAnEmptyText(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	body := `{"questions":["Buona?","   ","Anche buona?"]}`
	req := httptest.NewRequest(http.MethodPut, questionsPath(gameID), bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("una domanda vuota finirebbe in un bottone vuoto: atteso 400, ottenuto %d", rec.Code)
	}
}

func TestRegenerateSuggestedQuestions_WithoutAnIndexSaysToIndexFirst(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Suggester = &fakeSuggester{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodPost, questionsPath(gameID)+"/regenerate", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("indicizza")) {
		t.Fatalf("il messaggio deve dire di indicizzare prima un manuale: %s", rec.Body.String())
	}
}

// TestRegenerateSuggestedQuestions_OverwritesEditedToo: è un pulsante
// premuto a mano, quindi sovrascrive tutto — la decisione di prodotto
// presa in fase di design.
func TestRegenerateSuggestedQuestions_OverwritesEditedToo(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	sug := &fakeSuggester{out: []string{"Nuova 1?", "Nuova 2?", "Nuova 3?"}}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	// Indicizza (così ci sono titoli) e poi riscrivi tutte tre a mano.
	if rec := postIndex(cookie, router, gameID, mediaID); rec.Code != http.StatusOK {
		t.Fatalf("index: atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if err := server.Manuals.SaveEditedQuestions(context.Background(), gameID,
		[]string{"Mia 1?", "Mia 2?", "Mia 3?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, questionsPath(gameID)+"/regenerate", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if got[0].Text != "Nuova 1?" {
		t.Fatalf("rigenera deve sovrascrivere anche le domande a mano: %v", got)
	}
	if got[0].Edited {
		t.Fatal("rigenera azzera edited")
	}
}

func TestRegenerateSuggestedQuestions_ProviderRejectionIsAClearError(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Suggester = &fakeSuggester{err: ai.ErrSuggestionsRejected}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")
	if rec := postIndex(cookie, router, gameID, mediaID); rec.Code != http.StatusOK {
		t.Fatalf("index: atteso 200, ottenuto %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, questionsPath(gameID)+"/regenerate", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("riprova")) {
		t.Fatalf("il messaggio deve invitare a riprovare: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run "SuggestedQuestions" -count=1
```

Atteso: FAIL — 404 sulle rotte (il router non le conosce).

- [ ] **Step 3: Aggiungi i tre handler**

In `backend/internal/httpapi/questions_handlers.go`, in coda:

```go
// suggestedQuestionsResponse manda SEMPRE tre voci, anche per un gioco che
// non ne ha nessuna: il pannello admin ha tre campi da riempire, e uno slot
// vuoto è una voce col testo vuoto, non una voce assente. Senza questo il
// frontend dovrebbe pareggiare la lista da solo.
func suggestedQuestionsResponse(qs []manuals.SuggestedQuestion) map[string]any {
	out := make([]map[string]any, manuals.SuggestedQuestionCount)
	for i := range out {
		out[i] = map[string]any{"text": "", "edited": false}
	}
	for _, q := range qs {
		if q.Position < 0 || q.Position >= manuals.SuggestedQuestionCount {
			continue // una riga fuori range non esiste, ma non deve andare in panic
		}
		out[q.Position] = map[string]any{"text": q.Text, "edited": q.Edited}
	}
	return map[string]any{"questions": out}
}

func (s *Server) getSuggestedQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	qs, err := s.Manuals.SuggestedQuestions(r.Context(), gameID)
	if err != nil {
		log.Printf("questions: read for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the suggested questions")
		return
	}
	writeJSON(w, http.StatusOK, suggestedQuestionsResponse(qs))
}

func (s *Server) putSuggestedQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}

	var body struct {
		Questions []string `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corpo della richiesta non valido")
		return
	}
	if len(body.Questions) != manuals.SuggestedQuestionCount {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"servono esattamente %d domande", manuals.SuggestedQuestionCount))
		return
	}
	texts := make([]string, len(body.Questions))
	for i, q := range body.Questions {
		texts[i] = strings.TrimSpace(q)
		if texts[i] == "" {
			// Una domanda vuota finirebbe in un bottone vuoto nella chat.
			writeError(w, http.StatusBadRequest, "nessuna delle tre domande può essere vuota")
			return
		}
		if len([]rune(texts[i])) > ai.MaxSuggestionChars {
			writeError(w, http.StatusBadRequest, fmt.Sprintf(
				"ogni domanda deve stare sotto i %d caratteri", ai.MaxSuggestionChars))
			return
		}
	}

	if err := s.Manuals.SaveEditedQuestions(r.Context(), gameID, texts); err != nil {
		log.Printf("questions: save for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not save the suggested questions")
		return
	}

	qs, err := s.Manuals.SuggestedQuestions(r.Context(), gameID)
	if err != nil {
		log.Printf("questions: read back for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the suggested questions")
		return
	}
	writeJSON(w, http.StatusOK, suggestedQuestionsResponse(qs))
}

func (s *Server) regenerateSuggestedQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	// Il 404 su un gioco inesistente prima di qualunque lavoro:
	// regenerateQuestions ricaricherà il gioco per il suo nome, ma un id
	// inventato deve rispondere 404 e non un errore del provider.
	if _, err := s.Games.GetGame(r.Context(), gameID); err != nil {
		writeError(w, http.StatusNotFound, "gioco non trovato")
		return
	}

	// A differenza dell'indicizzazione, qui l'esito si racconta: è un
	// pulsante premuto a mano, e chi lo preme deve sapere se ha funzionato.
	switch err := s.regenerateQuestions(r.Context(), gameID, true); {
	case err == nil:
	case errors.Is(err, errNoHeadings):
		writeError(w, http.StatusUnprocessableEntity,
			"Per generare le domande serve un manuale già indicizzato: indicizza prima un documento nella sezione Chatbot.")
		return
	case errors.Is(err, ai.ErrNotConfigured):
		writeError(w, http.StatusUnprocessableEntity,
			"Nessun provider AI configurato: controlla le impostazioni.")
		return
	case errors.Is(err, ai.ErrSuggestionsRejected):
		writeError(w, http.StatusUnprocessableEntity,
			"Il modello non ha risposto con tre domande valide: riprova.")
		return
	default:
		log.Printf("questions: regenerate for game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway, "Il provider AI non ha risposto: riprova.")
		return
	}

	qs, err := s.Manuals.SuggestedQuestions(r.Context(), gameID)
	if err != nil {
		log.Printf("questions: read back for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the suggested questions")
		return
	}
	writeJSON(w, http.StatusOK, suggestedQuestionsResponse(qs))
}
```

Aggiungi `"encoding/json"`, `"net/http"` e `"strings"` agli import.

`parseIDParam(r, name) (int64, error)` è l'helper già usato dagli altri handler (`games_read_handlers.go:14`) e `GetGame(ctx, id) (Game, error)` è il metodo dello store dei giochi (`games/store.go:88`): sono quelli, non inventarne altri.

- [ ] **Step 4: Registra le rotte**

In `backend/internal/httpapi/router.go`, nel blocco `protected`, accanto alla rotta `index`:

```go
		protected.Get("/api/games/{id}/suggested-questions", s.getSuggestedQuestionsHandler)
		protected.Put("/api/games/{id}/suggested-questions", s.putSuggestedQuestionsHandler)
		protected.Post("/api/games/{id}/suggested-questions/regenerate", s.regenerateSuggestedQuestionsHandler)
```

- [ ] **Step 5: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -count=1
```

Atteso: `ok`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/questions_handlers.go \
        backend/internal/httpapi/questions_handlers_test.go \
        backend/internal/httpapi/router.go
git commit -m "feat: admin routes to read, edit and regenerate suggested questions

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: L'API pubblica manda le domande invece dei titoli

**Files:**
- Modify: `backend/internal/httpapi/games_responses.go:72-78`
- Modify: `backend/internal/manuals/store.go` (via `maxSuggestionHeadings`)
- Modify: `backend/internal/httpapi/games_read_handlers_test.go` (o dove è testata la risposta di `/api/games/{id}`)

**Interfaces:**
- Consumes: `(*manuals.Store).SuggestedQuestions` (Task 1)
- Produces: il campo `suggestedQuestions: []string` nella risposta di `GET /api/games/{id}`; il campo `sourceHeadings` non esiste più

- [ ] **Step 1: Scrivi il test (falisce)**

Trova il file che testa la risposta di `/api/games/{id}` con `grep -rln "sourceHeadings" backend/internal/httpapi/` e aggiungi lì:

```go
// TestGameDetail_ExposesSuggestedQuestionsNotHeadings: la scheda pubblica
// manda le domande già formulate, non i titoli di sezione da cui il
// frontend le costruiva con una tabella fissa. Solo le domande NON vuote
// escono: il frontend ripiega sulle domande fisse quando la lista è vuota,
// e tre stringhe vuote non sono una lista vuota.
func TestGameDetail_ExposesSuggestedQuestionsNotHeadings(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedBareGame(t, server)

	if err := server.Manuals.SaveGeneratedQuestions(context.Background(), gameID,
		[]string{"Come si piazza una tessera?", "Quando finisce?", "Quanti punti?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", gameID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if _, ok := resp["sourceHeadings"]; ok {
		t.Fatal("sourceHeadings non deve più esistere nella risposta")
	}
	qs, ok := resp["suggestedQuestions"].([]any)
	if !ok {
		t.Fatalf("suggestedQuestions manca o non è una lista: %s", rec.Body.String())
	}
	if len(qs) != 3 || qs[0] != "Come si piazza una tessera?" {
		t.Fatalf("domande inattese: %v", qs)
	}
}

func TestGameDetail_SuggestedQuestionsIsAlwaysAnArray(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", gameID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	qs, ok := resp["suggestedQuestions"].([]any)
	if !ok {
		t.Fatalf("un gioco senza domande deve mandare una lista vuota, non null: %s", rec.Body.String())
	}
	if len(qs) != 0 {
		t.Fatalf("attesa lista vuota, ottenuta %v", qs)
	}
}
```

Se `seedBareGame` è definita in `questions_handlers_test.go` (Task 4) è già visibile: stesso package `httpapi_test`.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestGameDetail_ -count=1
```

Atteso: FAIL — `suggestedQuestions manca`, e `sourceHeadings` c'è ancora.

- [ ] **Step 3: Cambia la risposta**

In `backend/internal/httpapi/games_responses.go`, sostituisci il blocco `headings`:

```go
	// Le tre domande suggerite, già formulate. Prima qui uscivano i titoli
	// di sezione (`sourceHeadings`) e il frontend li trasformava in domande
	// con una tabella fissa: quella catena produceva "Cosa dice il manuale
	// su di Klaus-Jürgen Wrede?" su un manuale reale, perché prendeva i
	// primi titoli in ordine di pagina — copertina e contenuto della
	// scatola — e ripiegava su un template per tutto ciò che la tabella non
	// conosceva.
	//
	// Solo le domande NON vuote: il pannello pubblico ripiega sulle sue tre
	// domande fisse quando la lista è vuota, e tre stringhe vuote non sono
	// una lista vuota. Sempre un array, mai null.
	questions := []string{}
	if qs, err := s.Manuals.SuggestedQuestions(ctx, g.ID); err != nil {
		// Un errore qui non deve costare la scheda del gioco: senza
		// domande suggerite il frontend usa le sue tre fisse.
		log.Printf("game detail: suggested questions for game %d: %v", g.ID, err)
	} else {
		for _, q := range qs {
			if strings.TrimSpace(q.Text) != "" {
				questions = append(questions, q.Text)
			}
		}
	}
	detail["suggestedQuestions"] = questions
```

Verifica che `log` e `strings` siano fra gli import del file; se mancano, aggiungili.

- [ ] **Step 4: Togli il cap sui titoli**

`maxSuggestionHeadings` limitava a 8 i titoli distinti mandati al **telefono**. Ora la destinazione è il prompt (`SuggestQuestions`), dove quaranta titoli sono qualche centinaio di token e più contesto significa domande migliori.

In `backend/internal/manuals/store.go`: cancella la costante `maxSuggestionHeadings` e il punto in `Summary` che la applica (cercalo con `grep -n maxSuggestionHeadings backend/internal/manuals/store.go`). `Summary.Headings` **resta**: è l'input della generazione, letto lato server e mai spedito al browser.

Aggiorna il commento di `Summary.Headings` nella dichiarazione della struct: non dice più "per il JSON del frontend" ma "input di ai.SuggestQuestions".

- [ ] **Step 5: Lancia tutta la suite**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 sh -c "go vet ./... && go test ./... -count=1"
```

Atteso: tutto `ok`. Altri test possono asserire su `sourceHeadings` o su `maxSuggestionHeadings`: aggiornali, non commentarli. Se un test verificava il cap a 8 titoli, cancellalo — quel comportamento non esiste più di proposito.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/games_responses.go \
        backend/internal/manuals/store.go \
        backend/internal/httpapi/
git commit -m "feat: serve suggested questions instead of section headings

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: La chat pubblica rende le domande ricevute

**Files:**
- Modify: `frontend/src/components/ManualChatPanel.vue:96-121` (via `fallbackQuestions` no, via `headingToQuestion` e `suggestions`)
- Modify: `frontend/src/components/ManualChat.vue:29,93,139`
- Modify: `frontend/src/views/GameDetailView.vue:24,44,157`

**Interfaces:**
- Consumes: il campo `suggestedQuestions: string[]` della risposta di `GET /api/games/{id}` (Task 5)
- Produces: nessuna interfaccia per i task successivi

- [ ] **Step 1: Cambia `ManualChatPanel.vue`**

Nel blocco `defineProps`, sostituisci la prop `headings`:

```ts
  /** Le tre domande suggerite, già formulate dal modello. Vuota = si usano le fisse. */
  suggestedQuestions: string[]
```

Cancella la costante `headingToQuestion` **per intero** (il `Record<string, string>` con le sette voci) e sostituisci la computed `suggestions`:

```ts
// Le domande suggerite arrivano già formulate dal server, generate dal
// modello sui titoli del manuale vero. Prima si costruivano qui da quei
// titoli con una tabella fissa, e su un manuale reale il risultato era
// «Cosa dice il manuale su "di Klaus-Jürgen Wrede"?»: il template non
// poteva fare di meglio, perché una domanda non è un titolo di sezione con
// un giro di frase intorno.
const suggestions = computed<string[]>(() =>
  props.suggestedQuestions.length >= 3 ? props.suggestedQuestions.slice(0, 3) : fallbackQuestions,
)
```

`fallbackQuestions` resta **esattamente come è**: è l'unica parte della macchina precedente che era già formulata bene.

- [ ] **Step 2: Cambia `ManualChat.vue`**

Nella `defineProps` sostituisci `headings: string[]` con:

```ts
  suggestedQuestions: string[]
```

e nei due punti in cui passa la prop al pannello (righe ~93 e ~139) sostituisci `:headings="headings"` con `:suggested-questions="suggestedQuestions"`.

- [ ] **Step 3: Cambia `GameDetailView.vue`**

- riga ~24: `const sourceHeadings = ref<string[]>([])` → `const suggestedQuestions = ref<string[]>([])`
- riga ~44: `sourceHeadings.value = game.value.sourceHeadings ?? []` → `suggestedQuestions.value = game.value.suggestedQuestions ?? []`
- riga ~157: `:headings="sourceHeadings"` → `:suggested-questions="suggestedQuestions"`

Se esiste un tipo TypeScript per la risposta del gioco (cercalo con `grep -rn "sourceHeadings" frontend/src`), aggiorna anche quello.

- [ ] **Step 4: Build e type-check**

```bash
cd frontend && npm run build
```

Atteso: build riuscita senza errori `vue-tsc`. Un errore che nomina `headings` significa che un punto della catena è rimasto indietro.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ManualChatPanel.vue \
        frontend/src/components/ManualChat.vue \
        frontend/src/views/GameDetailView.vue
git commit -m "feat: render the suggested questions the server generated

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Il pannello admin per modificarle

**Files:**
- Create: `frontend/src/components/SuggestedQuestionsPanel.vue`
- Modify: `frontend/src/views/GameAdminDetailView.vue` (sezione "Chatbot", ~riga 540-580)

**Interfaces:**
- Consumes: le tre rotte del Task 4; `api.get`/`api.put`/`api.post` da `frontend/src/api/client.ts`
- Produces: nessuna interfaccia per i task successivi

- [ ] **Step 1: Crea il componente**

Crea `frontend/src/components/SuggestedQuestionsPanel.vue`:

```vue
<script setup lang="ts">
/**
 * Le tre domande suggerite che la chat mostra nello stato di riposo.
 *
 * Sono generate dal modello a ogni indicizzazione, ma una domanda riscritta
 * a mano non viene più toccata: è la ragione per cui il pannello mostra
 * quali sono a mano — è l'informazione che spiega perché una domanda non è
 * cambiata dopo un reindex.
 */
import { computed, onMounted, ref } from 'vue'

import { api } from '../api/client'

const props = defineProps<{
  gameId: number
  /** Senza provider AI la rigenerazione non è possibile: il pulsante si spegne. */
  aiConfigured: boolean
}>()

type Question = { text: string; edited: boolean }

const questions = ref<Question[]>([
  { text: '', edited: false },
  { text: '', edited: false },
  { text: '', edited: false },
])
const loading = ref(false)
const saving = ref(false)
const regenerating = ref(false)
const error = ref('')
const saved = ref(false)

const path = computed(() => `/games/${props.gameId}/suggested-questions`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ questions: Question[] }>(path.value)
    questions.value = res.questions
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile leggere le domande suggerite.'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    const res = await api.put<{ questions: Question[] }>(path.value, {
      questions: questions.value.map((q) => q.text),
    })
    questions.value = res.questions
    saved.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile salvare le domande.'
  } finally {
    saving.value = false
  }
}

async function regenerate() {
  regenerating.value = true
  error.value = ''
  saved.value = false
  try {
    const res = await api.post<{ questions: Question[] }>(`${path.value}/regenerate`)
    questions.value = res.questions
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile rigenerare le domande.'
  } finally {
    regenerating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="suggested-questions">
    <h3>Domande suggerite</h3>
    <p class="field-hint">
      Le tre domande che la chat propone prima che qualcuno scriva. Si rigenerano da sé a ogni
      indicizzazione, tranne quelle che riscrivi qui: quelle restano come le hai messe.
    </p>

    <p v-if="loading" class="empty-note">Caricamento…</p>

    <template v-else>
      <ol class="suggested-questions-list">
        <li v-for="(q, i) in questions" :key="i">
          <label :for="`suggested-question-${i}`" class="visually-hidden">Domanda {{ i + 1 }}</label>
          <input
            :id="`suggested-question-${i}`"
            v-model="q.text"
            type="text"
            :maxlength="120"
            placeholder="Nessuna domanda: indicizza un manuale o scrivila a mano"
          />
          <span v-if="q.edited" class="suggested-questions-badge">scritta a mano</span>
        </li>
      </ol>

      <div class="suggested-questions-actions">
        <button type="button" :disabled="saving" @click="save">
          {{ saving ? 'Salvataggio…' : 'Salva' }}
        </button>
        <button
          type="button"
          class="secondary"
          :disabled="regenerating || !props.aiConfigured"
          @click="regenerate"
        >
          {{ regenerating ? 'Rigenerazione…' : 'Rigenera' }}
        </button>
      </div>

      <p v-if="saved" class="field-hint">Domande salvate.</p>
      <p v-if="error" class="error">{{ error }}</p>
    </template>
  </div>
</template>
```

**Tre dettagli già verificati, da rispettare:**
- **I percorsi non portano il prefisso `/api`**: lo aggiunge `api/client.ts`. Quindi `/games/${gameId}/suggested-questions`, come fa `ManualPrepPanel.vue:52`. Il codice sopra è già scritto così — non aggiungere `/api`.
- **La classe per un'etichetta solo-screen-reader è `visually-hidden`** (`app.css:99`), non `sr-only`: il markup sopra la usa già. `field-hint`, `empty-note` ed `error` esistono già anche loro.
- **Nessun componente di questo progetto ha un blocco `<style>`**: tutti gli stili stanno in `frontend/src/app.css`. Gli stili nuovi (`.suggested-questions`, `.suggested-questions-list`, `.suggested-questions-badge`, `.suggested-questions-actions`) vanno lì, con i token esistenti.

- [ ] **Step 2: Montalo nella sezione Chatbot**

In `frontend/src/views/GameAdminDetailView.vue`, importa il componente e montalo **dentro** la `<section>` della sezione "Chatbot", subito dopo la `</template>` che chiude la lista di `ManualPrepPanel` e prima del `<p v-else class="empty-note">`:

```vue
        <SuggestedQuestionsPanel :game-id="game.id" :ai-configured="aiConfigured" />
```

Va montato **fuori** dal `v-if="indexableMedia.length > 0"`, perché l'admin deve poter scrivere le tre domande a mano anche su un gioco senza documenti indicizzabili (il `PUT` è un upsert proprio per questo).

- [ ] **Step 3: Correggi una frase diventata falsa**

Nella stessa sezione c'è questo hint:

> «per una scansione lunga può richiedere alcuni minuti, **una pagina alla volta** — resta su questa pagina finché non finisce.»

Non è più vero: la trascrizione manda `transcribeConcurrency` (2) pagine alla volta. Sostituisci «una pagina alla volta» con «poche pagine alla volta». Il resto della frase resta.

- [ ] **Step 4: Build e type-check**

```bash
cd frontend && npm run build
```

Atteso: build riuscita senza errori `vue-tsc`.

- [ ] **Step 5: Prova a mano nell'app vera**

```bash
docker compose up -d --build
```

Su http://localhost:8080, come admin, nella pagina di un gioco con un manuale indicizzato: le tre domande devono comparire, «Rigenera» deve cambiarle, modificarne una e salvare deve mostrarle come «scritta a mano», e una reindicizzazione deve lasciare quella intatta cambiando le altre due.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/SuggestedQuestionsPanel.vue \
        frontend/src/views/GameAdminDetailView.vue \
        frontend/src/app.css
git commit -m "feat: admin panel to edit and regenerate the suggested questions

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Pass `impeccable` e documentazione

**Files:**
- Modify: `DESIGN.md` (se il pannello introduce un pattern nuovo)
- Modify: `frontend/src/app.css`, `frontend/src/components/SuggestedQuestionsPanel.vue` (quel che esce dal pass)

- [ ] **Step 1: Lancia `/impeccable` sul pannello admin**

Superficie: il pannello «Domande suggerite» nella sezione Chatbot della pagina admin del gioco. Modalità `polish`.

Guarda in particolare: tre campi di testo in fila sono una lista o un form?; il badge «scritta a mano» si legge come stato o come pulsante?; «Salva» e «Rigenera» hanno la gerarchia giusta (uno è distruttivo per il lavoro manuale, l'altro no); cosa si vede mentre `regenerating` è vero, dato che la chiamata dura secondi.

- [ ] **Step 2: Lancia `/impeccable` sullo stato di riposo della chat**

Superficie: le tre domande suggerite nello stato di riposo di `ManualChatPanel`, in barra desktop e in dialog mobile. Modalità `audit`.

Le domande sono cambiate di natura — prima erano tutte della stessa lunghezza («Cosa dice il manuale su X?»), ora sono frasi vere di lunghezza variabile. Verifica che tre domande lunghe non rompano il layout dei bottoni su uno schermo di telefono.

- [ ] **Step 3: Applica quel che esce e ricostruisci**

```bash
cd frontend && npm run build
```

- [ ] **Step 4: Aggiorna `DESIGN.md` se serve**

Se il pannello ha introdotto un pattern nuovo (una lista di campi editabili con un badge di stato), documentalo accanto agli altri. Se ha riusato pattern esistenti, non aggiungere niente.

`README.md` **non** va toccato: non cambiano né l'avvio, né le variabili d'ambiente, né una funzionalità visibile a chi installa l'app.

- [ ] **Step 5: Suite completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 sh -c "go vet ./... && go test -race ./... -count=1"
```

Atteso: tutto `ok`. Il package `httpapi` con `-race` ci mette ~150 secondi: non è un blocco.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "polish: refine the suggested questions panel

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Self-review di questo piano

**Copertura della spec** — ogni sezione ha un task: §3 modello dati → Task 1; §4 generazione e validazione → Task 2; §5 innesto sull'indicizzazione → Task 3; §6 API admin → Task 4; §8 lato pubblico e §9 cancellazioni → Task 5 e 6; §7 pannello admin → Task 7; §10 test → distribuiti nei task, più il pass `impeccable` nel Task 8. I non-obiettivi della §11 non generano task per definizione.

**Nomi verificati contro il codice**, non assunti: `newTestDB` (`manuals/store_test.go:43`), `parseIDParam(r, name) (int64, error)` (`httpapi/games_read_handlers.go:14`), `GetGame` (`games/store.go:88`), `visually-hidden` (`app.css:99`), e i percorsi dell'API client senza prefisso `/api` (`ManualPrepPanel.vue:52`). Cinque su cinque erano diversi da come li avevo scritti in prima stesura: da qui la verifica prima della consegna invece di lasciarla all'esecutore.

**La cascata funziona**: `db.Open` apre il DSN con `_pragma=foreign_keys(1)` (`db/db.go:11`), quindi `ON DELETE CASCADE` è attivo e il test sulla cascata è legittimo.

**Una cosa che il piano corregge di passaggio**: l'hint del pannello admin dice «una pagina alla volta», falso da quando la trascrizione ne manda due insieme (Task 7, Step 3).
