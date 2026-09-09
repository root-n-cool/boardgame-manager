# Domande sul manuale — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un partecipante apre la scheda pubblica di un gioco dal telefono, chiede una regola a parole sue e riceve una risposta fondata sul manuale, con il numero di pagina cliccabile.

**Architecture:** Il manuale entra una volta sola (estrazione dal layer testo, o trascrizione delle pagine scansionate con un modello multimodale), viene confermato dall'admin e salvato in SQLite come pagine + chunk indicizzati con FTS5. A domanda, un LLM riceve un unico tool `cerca_nel_manuale` che accetta un **elenco** di parole chiave; se il manuale è corto entra intero nel contesto e il tool non viene nemmeno dichiarato.

**Tech Stack:** Go 1.25 (chi, `modernc.org/sqlite` con FTS5), `github.com/ledongthuc/pdf` (unica dipendenza Go nuova), Vue 3 + TypeScript, `deep-chat` 2.5.1 (unica dipendenza npm nuova).

**Spec:** `docs/superpowers/specs/2026-09-08-domande-sul-manuale-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto (binario x86_64 su Mac arm64). Ogni `go` va lanciato così, riusando i due volumi nominati:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Non sostituire `bgm-gomodcache` / `bgm-gocache` con volumi anonimi. I target `backend-*` del `Makefile` non funzionano su questa macchina.
- **`npm` in locale**, in `frontend/`, senza Docker.
- **Migrazioni forward-only.** Nuovo file `NNNN_nome.sql` in `backend/internal/db/migrations/`, embeddato, applicato in ordine alfabetico. Mai modificare un file già rilasciato. Nessun down/rollback.
- **Dipendenze nuove: esattamente due**, entrambe già approvate in fase di design — `github.com/ledongthuc/pdf` (Go) e `deep-chat@2.5.1` (npm). Nient'altro: il progetto preferisce stdlib e codice esplicito.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n.
- **JSON dell'API in camelCase** (`canAsk`, non `can_ask`): è la convenzione di `games_responses.go`.
- **Degradazione senza AI**: nessun campo obbligatorio, nessun errore in UI perché il provider manca. Stessa regola di SMTP e della traduzione BGG.
- **`CGO_ENABLED=0`**: niente estensioni SQLite native, niente librerie che richiedono CGO.
- **Commit e push solo se richiesti.** Gli step "Commit" di questo piano creano commit locali su `main`; **non fare push**. Messaggi in inglese, conventional commits.
- **Mobile-first** sulle pagine pubbliche: si usano in piedi al tavolo.
- **Ultimo task obbligatorio**: `/impeccable` sulla superficie modificata.

---

## File Structure

**Backend — creati**

| File | Responsabilità |
|---|---|
| `backend/internal/db/migrations/0014_manual_qa.sql` | Tabelle `manual_page`, `manual_chunk`, indice FTS5 + trigger, colonna `ai_vision_model` |
| `backend/internal/manuals/pdf.go` | `HasTextLayer`, `ExtractPageImages` (stdlib), `ExtractText` (ledongthuc) |
| `backend/internal/manuals/pdf_test.go` | Test dei tre, con fixture PDF sintetiche costruite nel test |
| `backend/internal/manuals/chunk.go` | `Chunk`, `DetectHeading` |
| `backend/internal/manuals/chunk_test.go` | |
| `backend/internal/manuals/store.go` | `Store`: `ReplacePages`, `ListPages`, `DeletePages`, `Search`, `Corpus` |
| `backend/internal/manuals/store_test.go` | |
| `backend/internal/ai/ask.go` | `Ask` (loop con tool), `Transcribe` (vision) |
| `backend/internal/ai/ask_test.go` | |
| `backend/internal/httpapi/manuals_handlers.go` | I quattro handler admin |
| `backend/internal/httpapi/manuals_handlers_test.go` | |
| `backend/internal/httpapi/ask_handler.go` | `POST /api/games/{id}/ask` |
| `backend/internal/httpapi/ask_handler_test.go` | |

**Backend — modificati**

| File | Modifica |
|---|---|
| `backend/internal/settings/store.go` | Campo `AIVisionModel` in `Settings`, `Get`, `Update` |
| `backend/internal/httpapi/settings_handlers.go` | Il campo in ingresso e in uscita |
| `backend/internal/httpapi/games_responses.go` | `canAsk` in `toGameDetail` |
| `backend/internal/httpapi/router.go` | `Manuals` nel `Server`, cinque rotte, `askLimiter` |
| `backend/internal/httpapi/testhelpers_test.go` | `Manuals` negli helper |
| `backend/go.mod` / `go.sum` | `github.com/ledongthuc/pdf` |

**Frontend — creati**

| File | Responsabilità |
|---|---|
| `frontend/src/components/ManualChat.vue` | Le due forme della chat: sidebar (≥1100px) e bottone tondo + `<dialog>` |
| `frontend/src/components/ManualChatPanel.vue` | Il contenuto comune: stato di riposo, domande suggerite, montaggio di deep-chat |
| `frontend/src/components/ManualPrepPanel.vue` | Pannello admin: prepara, anteprima pagina per pagina, salva |

Due componenti e non uno: `ManualChat.vue` decide **dove** vive la chat (media query, stato collassato, dialog), `ManualChatPanel.vue` **cos'è** la chat. Senza la separazione il secondo finirebbe duplicato nei due rami del template.

**Frontend — modificati**

| File | Modifica |
|---|---|
| `frontend/src/utils/game.ts` | `canAsk: boolean` in `GameDetail` |
| `frontend/src/views/GameDetailView.vue` | Griglia a due colonne + `ManualChat` |
| `frontend/src/views/GameAdminDetailView.vue` | `ManualPrepPanel` nel pannello media |
| `frontend/src/views/ManageBookingView.vue` | Rimando a `/games/:id?chat=1` |
| `frontend/src/views/EventDetailView.vue` | Rimando a `/games/:id?chat=1` |
| `frontend/src/app.css` | Classi della sidebar, del bottone tondo, del dialog |
| `frontend/package.json` | `deep-chat` |
| `DESIGN.md` | Sezione sulla chat e sul web component in shadow DOM |
| `README.md` | `ai_vision_model`, e i tre limiti della dettatura |

**Scostamento dalla spec, da applicare:** la spec indica il breakpoint della sidebar a 900px. Misurato sul codice, `.app-page` è `max-width: 56rem` (896px): una sidebar da 22rem lì dentro lascerebbe al testo 34rem, sotto la misura leggibile. Il breakpoint corretto è **1100px** — 56rem di colonna leggibile + 22rem di sidebar + i margini di `.app-page` — e sotto quella soglia vale la forma mobile (bottone tondo + dialog), che copre bene anche i tablet.

---

### Task 1: Migrazione e impostazione del modello vision

**Files:**
- Create: `backend/internal/db/migrations/0014_manual_qa.sql`
- Modify: `backend/internal/settings/store.go`
- Modify: `backend/internal/httpapi/settings_handlers.go`
- Test: `backend/internal/settings/store_test.go`

**Interfaces:**
- Consumes: niente (primo task).
- Produces: le tabelle `manual_page`, `manual_chunk`, `manual_chunk_fts`; `settings.Settings.AIVisionModel string`.

- [ ] **Step 1: Scrivere il test che fallisce**

In `backend/internal/settings/store_test.go`, aggiungere:

```go
func TestUpdate_RoundTripsTheVisionModel(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	current, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	current.AIVisionModel = "deepseek-v4-flash-vision-exp"
	if err := store.Update(ctx, current); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.AIVisionModel != "deepseek-v4-flash-vision-exp" {
		t.Fatalf("expected the vision model to round-trip, got %q", got.AIVisionModel)
	}

	// Svuotarlo deve tornare stringa vuota, non NULL letto come "NULL":
	// è il campo che dice "nessuna trascrizione automatica".
	got.AIVisionModel = ""
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("update to empty: %v", err)
	}
	cleared, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if cleared.AIVisionModel != "" {
		t.Fatalf("expected an empty vision model, got %q", cleared.AIVisionModel)
	}
}
```

Se `newTestStore` non esiste con questa firma in `store_test.go`, leggere il file e riusare l'helper già presente.

- [ ] **Step 2: Eseguire il test e verificare che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/ -run TestUpdate_RoundTripsTheVisionModel -v
```

Atteso: FAIL, `current.AIVisionModel undefined`.

- [ ] **Step 3: Scrivere la migrazione**

Creare `backend/internal/db/migrations/0014_manual_qa.sql`:

```sql
-- Trascrizione del manuale, una riga per pagina. È modificabile dall'admin
-- e resta la fonte di verità: non è una cache dell'estrazione, che infatti
-- non viene mai rieseguita da sola.
CREATE TABLE manual_page (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_media_id INTEGER NOT NULL REFERENCES game_media(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL,
    text TEXT NOT NULL,
    -- Il titolo di sezione rilevato, se c'è: alimenta l'indice iniettato
    -- nel contesto del modello.
    heading TEXT,
    source TEXT NOT NULL CHECK (source IN ('pdf_text', 'vision', 'manual')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(game_media_id, page_number)
);

-- I chunk cercabili, derivati dalle pagine: rigenerabili in qualunque
-- momento senza perdere niente di inserito a mano.
CREATE TABLE manual_chunk (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    game_media_id INTEGER NOT NULL REFERENCES game_media(id) ON DELETE CASCADE,
    language_code TEXT NOT NULL,
    page_number INTEGER NOT NULL,
    -- Ordinale del chunk dentro la pagina, per allegare il vicino.
    seq INTEGER NOT NULL,
    text TEXT NOT NULL
);

-- game_id è denormalizzato di proposito: la ricerca filtra sempre per gioco,
-- così è un indice B-tree invece di due join.
CREATE INDEX idx_manual_chunk_game ON manual_chunk(game_id);
CREATE INDEX idx_manual_chunk_page ON manual_chunk(game_media_id, page_number, seq);

-- Indice FTS5 in external-content: indicizza manual_chunk senza duplicarne
-- il testo. I tre trigger lo tengono in pari.
CREATE VIRTUAL TABLE manual_chunk_fts USING fts5(
    text, content='manual_chunk', content_rowid='id'
);
CREATE TRIGGER manual_chunk_ai AFTER INSERT ON manual_chunk BEGIN
    INSERT INTO manual_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER manual_chunk_ad AFTER DELETE ON manual_chunk BEGIN
    INSERT INTO manual_chunk_fts(manual_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
END;
CREATE TRIGGER manual_chunk_au AFTER UPDATE ON manual_chunk BEGIN
    INSERT INTO manual_chunk_fts(manual_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
    INSERT INTO manual_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;

-- Modello multimodale per la trascrizione dei manuali scansionati. Vuoto =
-- nessuna trascrizione automatica, l'admin scrive il testo a mano. Il
-- modello di chat (ai_model) può essere solo-testo, quindi serve un campo
-- separato: deepseek-v4-flash non accetta immagini, la sua variante
-- deepseek-v4-flash-vision-exp sì.
ALTER TABLE app_settings ADD COLUMN ai_vision_model TEXT;
```

- [ ] **Step 4: Aggiungere il campo a `settings.Settings`**

In `backend/internal/settings/store.go`, dentro lo `struct Settings`, subito dopo `AIModel string`:

```go
	// AIVisionModel è il modello multimodale per leggere i manuali
	// scansionati. Separato da AIModel perché un modello di chat può
	// essere solo-testo. Vuoto = nessuna trascrizione automatica, e non è
	// un errore.
	AIVisionModel string
```

In `Get`, aggiungere `aiVisionModel` alla lista delle `sql.NullString`, alla `SELECT` e allo `Scan`, poi `out.AIVisionModel = aiVisionModel.String`:

```go
	var baseURL, bggToken, aiBaseURL, aiAPIKey, aiModel, aiVisionModel sql.NullString
	// ...
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token, ai_base_url, ai_api_key, ai_model, ai_vision_model,
		        smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_address, smtp_from_name, smtp_tls_mode
		 FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken, &aiBaseURL, &aiAPIKey, &aiModel, &aiVisionModel,
		&smtpHost, &smtpPort, &smtpUser, &smtpPass, &smtpFrom, &smtpFromName, &smtpTLS)
	// ...
	out.AIVisionModel = aiVisionModel.String
```

In `Update`, aggiungere `ai_vision_model = ?` alla `SET` e `nullIfEmpty(in.AIVisionModel)` agli argomenti, **nella stessa posizione** (subito dopo `ai_model`).

- [ ] **Step 5: Eseguire il test e verificare che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/ -v
```

Atteso: PASS.

- [ ] **Step 6: Esporre il campo nell'API impostazioni**

Leggere `backend/internal/httpapi/settings_handlers.go` e aggiungere `AIVisionModel` esattamente come è trattato `AIModel` — **non** come `AIAPIKey`: il nome di un modello non è un segreto, quindi esce in chiaro, non mascherato. Nel JSON la chiave è `aiVisionModel`.

- [ ] **Step 7: Eseguire la suite intera**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: tutti i pacchetti PASS. La migrazione viene applicata da ogni test che apre un DB, quindi un errore SQL qui si manifesta come fallimento diffuso.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/db/migrations/0014_manual_qa.sql \
  backend/internal/settings/store.go backend/internal/settings/store_test.go \
  backend/internal/httpapi/settings_handlers.go
git commit -m "feat: add manual Q&A schema and the vision model setting"
```

---

### Task 2: Riconoscere e smontare un PDF, senza dipendenze

**Files:**
- Create: `backend/internal/manuals/pdf.go`
- Test: `backend/internal/manuals/pdf_test.go`

**Interfaces:**
- Consumes: niente.
- Produces:
  ```go
  type Page struct {
      Number int
      Text   string
  }
  type PageImage struct {
      Number int
      JPEG   []byte
      Width  int
      Height int
  }
  func HasTextLayer(pdf []byte) bool
  func ExtractPageImages(pdf []byte) ([]PageImage, error)
  ```

- [ ] **Step 1: Scrivere i test che falliscono**

Creare `backend/internal/manuals/pdf_test.go`. Le fixture si costruiscono nel test invece di committare un PDF vero: il manuale reale è materiale del club, pesa 2 MB e sarebbe un binario in git — una fixture sintetica è piccola, deterministica e senza problemi di licenza.

```go
package manuals_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"boardgames-manager/internal/manuals"
)

// buildPDF assembla un PDF valido, xref compresa, dagli oggetti dati.
// objs[i] è il corpo dell'oggetto numero i+1, già serializzato.
func buildPDF(t *testing.T, objs []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf,
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objs)+1, xref)
	return buf.Bytes()
}

// tinyJPEG restituisce un JPEG valido di w x h, decodificabile da image/jpeg.
func tinyJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// scannedPDF: due pagine, ognuna un solo XObject JPEG a piena pagina.
// È la forma del manuale reale del club, verificata in fase di design.
func scannedPDF(t *testing.T) []byte {
	t.Helper()
	jpg1 := tinyJPEG(t, 24, 32)
	jpg2 := tinyJPEG(t, 20, 28)
	content := "q 200 0 0 260 0 0 cm /Im0 Do Q"
	imgObj := func(jpg []byte, w, h int) string {
		return fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
				"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
			w, h, len(jpg), jpg)
	}
	page := func(imgRef, contentRef string) string {
		return fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] "+
				"/Resources << /XObject << /Im0 %s >> >> /Contents %s >>", imgRef, contentRef)
	}
	streamObj := func(s string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	return buildPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",                      // 1
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",        // 2
		page("5 0 R", "7 0 R"),                                   // 3
		page("6 0 R", "8 0 R"),                                   // 4
		imgObj(jpg1, 24, 32),                                     // 5
		imgObj(jpg2, 20, 28),                                     // 6
		streamObj(content),                                       // 7
		streamObj(content),                                       // 8
	})
}

// textPDF: una pagina con un vero layer testo, font standard non embeddato.
func textPDF(t *testing.T) []byte {
	t.Helper()
	content := "BT /F1 12 Tf 20 200 Td (Fase di Upkeep) Tj 0 -20 Td (Ogni giocatore paga una moneta.) Tj ET"
	return buildPDF(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 260] " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})
}

func TestHasTextLayer(t *testing.T) {
	if manuals.HasTextLayer(scannedPDF(t)) {
		t.Fatal("una scansione non ha layer testo, ma HasTextLayer ha detto sì")
	}
	if !manuals.HasTextLayer(textPDF(t)) {
		t.Fatal("un PDF con operatori Tj ha layer testo, ma HasTextLayer ha detto no")
	}
}

func TestExtractPageImages_ReturnsOneJPEGPerScannedPage(t *testing.T) {
	imgs, err := manuals.ExtractPageImages(scannedPDF(t))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("attese 2 immagini, ottenute %d", len(imgs))
	}
	if imgs[0].Number != 1 || imgs[1].Number != 2 {
		t.Fatalf("numerazione pagine sbagliata: %d, %d", imgs[0].Number, imgs[1].Number)
	}
	if imgs[0].Width != 24 || imgs[0].Height != 32 {
		t.Fatalf("dimensioni pagina 1 sbagliate: %dx%d", imgs[0].Width, imgs[0].Height)
	}
	// Il byte stream deve essere un JPEG davvero decodificabile: è quello
	// che finisce, in base64, nella richiesta al modello multimodale.
	if _, err := jpeg.Decode(bytes.NewReader(imgs[0].JPEG)); err != nil {
		t.Fatalf("la pagina 1 non è un JPEG valido: %v", err)
	}
}

func TestExtractPageImages_OnAPDFWithoutImages(t *testing.T) {
	imgs, err := manuals.ExtractPageImages(textPDF(t))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(imgs) != 0 {
		t.Fatalf("attese 0 immagini su un PDF di solo testo, ottenute %d", len(imgs))
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: FAIL in compilazione, il pacchetto `manuals` non esiste.

- [ ] **Step 3: Implementare `HasTextLayer` e `ExtractPageImages`**

Creare `backend/internal/manuals/pdf.go`:

```go
// Package manuals gestisce il ciclo di vita del testo di un manuale di
// gioco: come si estrae da un PDF, come si spezza in chunk cercabili e come
// si cerca dentro con FTS5.
package manuals

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // registra il decoder JPEG per image.DecodeConfig
	"regexp"
)

// Page è una pagina di manuale come testo. Number è 1-based, come la
// numerazione che l'utente legge sul PDF: è quella che finisce nella
// citazione della risposta.
type Page struct {
	Number int
	Text   string
}

// PageImage è una pagina scansionata: il JPEG così com'era dentro il PDF,
// pronto da mandare in base64 a un modello multimodale.
type PageImage struct {
	Number int
	JPEG   []byte
	Width  int
	Height int
}

// textOperators trova gli operatori PDF che disegnano testo: `(...) Tj`,
// `[...] TJ`, `' ` e `"`. La loro presenza è ciò che distingue un PDF con
// layer testo da una scansione, dove le pagine sono solo immagini.
var textOperators = regexp.MustCompile(`\)\s*Tj|\]\s*TJ|\)\s*'|\)\s*"`)

// HasTextLayer dice se vale la pena provare l'estrazione testo. Guarda i
// content stream non compressi; un PDF che comprime tutto in FlateDecode
// risponde false e finisce sul percorso vision, che è la degradazione
// giusta: peggio sarebbe estrarre stringa vuota e non accorgersene.
func HasTextLayer(pdf []byte) bool {
	return textOperators.Match(pdf)
}

// dctImage individua un XObject immagine con filtro DCTDecode e cattura
// larghezza e altezza dal dizionario. I JPEG dentro un PDF non sono
// ricodificati: il flusso è il file JPEG, quindi estrarlo è una copia.
var dctImage = regexp.MustCompile(
	`(?s)/Subtype\s*/Image(.{0,400}?)/Filter\s*/DCTDecode(.{0,400}?)stream\r?\n`)

var widthRe = regexp.MustCompile(`/Width\s+(\d+)`)
var heightRe = regexp.MustCompile(`/Height\s+(\d+)`)

// ExtractPageImages restituisce le immagini a piena pagina di un PDF
// scansionato, nell'ordine in cui compaiono nel file — che per uno scan
// prodotto da uno scanner è l'ordine delle pagine.
//
// Non usa una libreria PDF di proposito: uno scan è "una immagine per
// pagina", e per quel caso bastano il dizionario dell'XObject e una copia
// del flusso. Un PDF con più immagini per pagina qui non è supportato, ed è
// coerente: quello è un PDF impaginato, che ha un layer testo e va
// sull'altro percorso.
func ExtractPageImages(pdf []byte) ([]PageImage, error) {
	matches := dctImage.FindAllSubmatchIndex(pdf, -1)
	out := make([]PageImage, 0, len(matches))
	for _, m := range matches {
		// m[1] è la fine dell'intero match, cioè subito dopo "stream\n".
		start := m[1]
		end := bytes.Index(pdf[start:], []byte("endstream"))
		if end < 0 {
			return nil, fmt.Errorf("immagine a pagina %d: stream senza endstream", len(out)+1)
		}
		jpg := bytes.TrimRight(pdf[start:start+end], "\r\n")

		cfg, _, err := image.DecodeConfig(bytes.NewReader(jpg))
		if err != nil {
			// Un flusso che si dichiara DCTDecode ma non è un JPEG leggibile
			// non è utilizzabile dal modello: si salta invece di far
			// fallire tutto il manuale.
			continue
		}
		// Le dimensioni dichiarate nel dizionario e quelle del JPEG devono
		// coincidere; in caso di disaccordo vince il JPEG, che è ciò che il
		// modello vedrà davvero.
		dict := pdf[m[0]:m[1]]
		_ = widthRe.FindSubmatch(dict)
		_ = heightRe.FindSubmatch(dict)

		out = append(out, PageImage{
			Number: len(out) + 1,
			JPEG:   jpg,
			Width:  cfg.Width,
			Height: cfg.Height,
		})
	}
	return out, nil
}
```

Nota per chi implementa: le due `FindSubmatch` scartate esistono perché la spec parla delle dimensioni dichiarate; se non servono, **eliminarle** insieme a `widthRe`/`heightRe` invece di lasciare codice morto.

- [ ] **Step 4: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: PASS su tutti e tre i test.

- [ ] **Step 5: Verificare sul manuale reale**

La fixture sintetica dimostra la logica, non che funzioni su un file prodotto da uno scanner vero. Aggiungere un test opzionale che si salta quando il file non c'è:

```go
// TestExtractPageImages_OnTheRealManual gira solo se in ./data c'è un
// manuale scansionato: è la verifica che la logica regge su un file
// prodotto da uno scanner, non solo sulla fixture. Salta in CI.
func TestExtractPageImages_OnTheRealManual(t *testing.T) {
	paths, _ := filepath.Glob("../../../data/uploads/*.pdf")
	if len(paths) == 0 {
		t.Skip("nessun PDF in ./data/uploads: verifica saltata")
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if manuals.HasTextLayer(raw) {
			continue // questo va sull'altro percorso
		}
		imgs, err := manuals.ExtractPageImages(raw)
		if err != nil {
			t.Fatalf("extract %s: %v", p, err)
		}
		if len(imgs) == 0 {
			t.Fatalf("%s è una scansione ma non ne è uscita nessuna pagina", p)
		}
		t.Logf("%s: %d pagine, la prima %dx%d", filepath.Base(p), len(imgs), imgs[0].Width, imgs[0].Height)
	}
}
```

Aggiungere `os` e `path/filepath` agli import. Lanciarlo con `-v` e leggere il log: sul manuale del club deve dire 4 pagine intorno a 2110x3101.

```bash
docker run --rm -v "$(pwd)/backend:/app" -v "$(pwd)/data:/data-ro:ro" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -run RealManual -v
```

Se il path relativo non risolve dentro il container, il test si limita a fare `Skip`: è accettabile, e la verifica si può fare a mano una volta.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/manuals/
git commit -m "feat: detect PDF text layers and extract scanned page images"
```

---

### Task 3: Estrazione testo con ledongthuc/pdf

**Files:**
- Create: nulla (si aggiunge a `pdf.go`)
- Modify: `backend/internal/manuals/pdf.go`, `backend/go.mod`, `backend/go.sum`
- Test: `backend/internal/manuals/pdf_test.go`

**Interfaces:**
- Consumes: `manuals.Page` (Task 2).
- Produces: `func ExtractText(pdf []byte) ([]Page, error)`.

- [ ] **Step 1: Scrivere il test che fallisce**

In `pdf_test.go`:

```go
func TestExtractText_ReadsOnePageOfRealText(t *testing.T) {
	pages, err := manuals.ExtractText(textPDF(t))
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("attesa 1 pagina, ottenute %d", len(pages))
	}
	if pages[0].Number != 1 {
		t.Fatalf("numero pagina atteso 1, ottenuto %d", pages[0].Number)
	}
	if !strings.Contains(pages[0].Text, "Upkeep") {
		t.Fatalf("il testo della pagina non contiene 'Upkeep': %q", pages[0].Text)
	}
	if !strings.Contains(pages[0].Text, "moneta") {
		t.Fatalf("il testo della pagina non contiene la seconda riga: %q", pages[0].Text)
	}
}

func TestExtractText_OnAScanReturnsNoText(t *testing.T) {
	pages, err := manuals.ExtractText(scannedPDF(t))
	// Una scansione può far restituire pagine vuote o un errore di parsing:
	// entrambi sono esiti accettabili. Ciò che NON deve accadere è tornare
	// testo inventato, o andare in panic.
	if err != nil {
		return
	}
	for _, p := range pages {
		if strings.TrimSpace(p.Text) != "" {
			t.Fatalf("pagina %d di una scansione ha prodotto testo: %q", p.Number, p.Text)
		}
	}
}
```

Aggiungere `strings` agli import.

- [ ] **Step 2: Eseguire il test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -run ExtractText -v
```

Atteso: FAIL, `undefined: manuals.ExtractText`.

- [ ] **Step 3: Aggiungere la dipendenza**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go get github.com/ledongthuc/pdf
```

`ledongthuc/pdf` ha zero dipendenze transitive (il suo `go.mod` è due righe): dopo il comando, `go.mod` deve avere **una** riga in più. Se ne compaiono altre, fermarsi e segnalarlo.

- [ ] **Step 4: Implementare `ExtractText`**

In `backend/internal/manuals/pdf.go`, aggiungere l'import `"github.com/ledongthuc/pdf"` e:

```go
// ExtractText legge il layer testo di un PDF, una Page per pagina.
//
// Non ricostruisce l'impaginazione: su un manuale a più colonne le colonne
// possono uscire interlacciate. È un limite accettato, non un difetto da
// aggirare qui — un manuale che esce male si manda per il percorso vision,
// che su impaginazioni dense dà comunque risultati migliori di qualunque
// estrattore di testo.
func ExtractText(raw []byte) ([]Page, error) {
	reader, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("apertura pdf: %w", err)
	}

	total := reader.NumPage()
	pages := make([]Page, 0, total)
	for n := 1; n <= total; n++ {
		page := reader.Page(n)
		if page.V.IsNull() {
			continue
		}
		// GetPlainText vuole una mappa di font condivisa fra le pagine:
		// passarne una nuova per pagina rifà lo stesso lavoro N volte.
		text, err := page.GetPlainText(nil)
		if err != nil {
			// Una pagina illeggibile non deve far perdere le altre: il
			// manuale resta utilizzabile e l'admin vede il buco
			// nell'anteprima, dove può riempirlo a mano.
			pages = append(pages, Page{Number: n, Text: ""})
			continue
		}
		pages = append(pages, Page{Number: n, Text: normalizeWhitespace(text)})
	}
	return pages, nil
}

// normalizeWhitespace compatta gli spazi ripetuti e uniforma gli a capo,
// senza fondere i paragrafi: il chunking (chunk.go) taglia sui paragrafi,
// quindi la riga vuota fra due paragrafi è informazione da conservare.
func normalizeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = regexp.MustCompile(`[ \t]+`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
```

Aggiungere `"strings"` agli import di `pdf.go`.

Compilare le regexp a livello di package invece che dentro la funzione, come già fatto per `textOperators`: `regexp.MustCompile` dentro una funzione chiamata per ogni pagina ricompila ogni volta.

- [ ] **Step 5: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: PASS.

**Se `TestExtractText_ReadsOnePageOfRealText` fallisce** perché la libreria non digerisce la fixture sintetica (font standard non embeddato, xref minimale): **non** cambiare l'implementazione per far passare il test. Sostituire la fixture con un PDF piccolo e vero, salvato in `backend/internal/manuals/testdata/text-sample.pdf` — generabile stampando in PDF una pagina di testo da qualunque editor — e adattare il test a leggerlo. La logica sotto prova è la libreria, non la nostra costruzione del PDF.

- [ ] **Step 6: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/manuals/
git commit -m "feat: extract text from PDFs that have a text layer"
```

---

### Task 4: Chunking e rilevamento dei titoli

**Files:**
- Create: `backend/internal/manuals/chunk.go`
- Test: `backend/internal/manuals/chunk_test.go`

**Interfaces:**
- Consumes: `manuals.Page` (Task 2).
- Produces:
  ```go
  const MaxChunkChars = 1000
  const ChunkOverlapChars = 100
  type Chunk struct {
      PageNumber int
      Seq        int
      Text       string
  }
  func Chunk(pages []Page) []Chunk
  func DetectHeading(text string) string
  ```

- [ ] **Step 1: Scrivere i test che falliscono**

Creare `backend/internal/manuals/chunk_test.go`:

```go
package manuals_test

import (
	"strings"
	"testing"

	"boardgames-manager/internal/manuals"
)

func TestChunk_KeepsShortPagesWhole(t *testing.T) {
	pages := []manuals.Page{{Number: 3, Text: "Turno del giocatore. Si pescano due carte."}}
	chunks := manuals.Chunk(pages)
	if len(chunks) != 1 {
		t.Fatalf("una pagina corta è un chunk solo, ottenuti %d", len(chunks))
	}
	if chunks[0].PageNumber != 3 || chunks[0].Seq != 0 {
		t.Fatalf("pagina/seq attesi 3/0, ottenuti %d/%d", chunks[0].PageNumber, chunks[0].Seq)
	}
}

func TestChunk_SplitsLongPagesWithoutBreakingSentences(t *testing.T) {
	// Una pagina lunga il triplo del massimo: deve uscire in più chunk,
	// ognuno sotto il limite, e nessuno deve cominciare a metà frase.
	sentence := "Ogni giocatore paga una moneta per ciascun edificio posseduto e ne verifica la produzione. "
	long := strings.Repeat(sentence, 40)
	chunks := manuals.Chunk([]manuals.Page{{Number: 4, Text: long}})

	if len(chunks) < 2 {
		t.Fatalf("attesi più chunk da una pagina lunga, ottenuti %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > manuals.MaxChunkChars {
			t.Fatalf("chunk %d supera il limite: %d caratteri", i, len(c.Text))
		}
		if c.PageNumber != 4 {
			t.Fatalf("chunk %d ha perso il numero di pagina: %d", i, c.PageNumber)
		}
		if c.Seq != i {
			t.Fatalf("seq non progressivo: chunk %d ha seq %d", i, c.Seq)
		}
		first := strings.TrimSpace(c.Text)
		if first == "" {
			t.Fatalf("chunk %d è vuoto", i)
		}
		// Un chunk che comincia con una minuscola è una frase tagliata a
		// metà, cioè il difetto che la sovrapposizione deve evitare.
		if i > 0 && strings.ToLower(first[:1]) == first[:1] && strings.ToUpper(first[:1]) != first[:1] {
			t.Fatalf("chunk %d comincia a metà frase: %q", i, first[:40])
		}
	}
}

func TestChunk_OverlapsSoARuleOnTheBoundaryIsFindable(t *testing.T) {
	// La regola sta a cavallo del taglio: deve comparire intera in almeno
	// un chunk, altrimenti non la trova né il chunk prima né quello dopo.
	filler := strings.Repeat("Testo di riempimento del regolamento. ", 25)
	rule := "Se due giocatori sono in pareggio vince chi ha meno edifici demoliti."
	chunks := manuals.Chunk([]manuals.Page{{Number: 8, Text: filler + rule + " " + filler}})

	found := false
	for _, c := range chunks {
		if strings.Contains(c.Text, rule) {
			found = true
		}
	}
	if !found {
		t.Fatal("la regola a cavallo del taglio non compare intera in nessun chunk")
	}
}

func TestChunk_IsDeterministic(t *testing.T) {
	pages := []manuals.Page{{Number: 1, Text: strings.Repeat("Una frase del manuale. ", 120)}}
	a := manuals.Chunk(pages)
	b := manuals.Chunk(pages)
	if len(a) != len(b) {
		t.Fatalf("due esecuzioni danno %d e %d chunk", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("chunk %d differisce fra due esecuzioni", i)
		}
	}
}

func TestChunk_SkipsEmptyPages(t *testing.T) {
	chunks := manuals.Chunk([]manuals.Page{
		{Number: 1, Text: "   \n  "},
		{Number: 2, Text: "Contenuto vero."},
	})
	if len(chunks) != 1 || chunks[0].PageNumber != 2 {
		t.Fatalf("una pagina vuota non produce chunk; ottenuti %d", len(chunks))
	}
}

func TestDetectHeading(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Fase di Upkeep\nOgni giocatore paga una moneta per ogni edificio.", "Fase di Upkeep"},
		{"CONTEGGIO DEI PUNTI\nOgni edificio vale i punti stampati.", "CONTEGGIO DEI PUNTI"},
		// Una prima riga che è già una frase compiuta non è un titolo.
		{"La partita termina quando la pila di pesca si esaurisce e non è possibile pescare.", ""},
		// Troppo lunga per essere un titolo.
		{strings.Repeat("parola ", 20) + "\naltro testo", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := manuals.DetectHeading(c.in); got != c.want {
			t.Fatalf("DetectHeading(%.30q) = %q, atteso %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -run 'Chunk|DetectHeading' -v
```

Atteso: FAIL, `undefined: manuals.Chunk`.

- [ ] **Step 3: Implementare**

Creare `backend/internal/manuals/chunk.go`:

```go
package manuals

import (
	"regexp"
	"strings"
	"unicode"
)

// MaxChunkChars è la dimensione massima di un chunk. Mille caratteri sono
// ~250 token: cinque chunk stanno in 1.500 token di payload, che è il
// budget deciso per una chiamata al tool.
const MaxChunkChars = 1000

// ChunkOverlapChars è quanto un chunk ripete della coda del precedente.
// Serve a un caso preciso: una regola a cavallo del taglio, che senza
// sovrapposizione non si troverebbe né prima né dopo.
const ChunkOverlapChars = 100

// Chunk è un pezzo cercabile di manuale. PageNumber è ciò che rende
// possibile la citazione; Seq è l'ordinale nella pagina, e serve ad
// allegare il chunk vicino quando il match cade su un bordo.
type Chunk struct {
	PageNumber int
	Seq        int
	Text       string
}

// sentenceEnd trova la fine di una frase: punto, esclamativo, interrogativo
// o due punti, seguiti da spazio.
var sentenceEnd = regexp.MustCompile(`[.!?:]\s+`)

// Chunk spezza le pagine in chunk, tagliando prima sui paragrafi e poi
// sulle frasi, senza mai spezzare una frase. Deterministico: lo stesso
// input dà sempre gli stessi chunk, così manual_chunk si può svuotare e
// ricostruire da manual_page in qualunque momento.
func Chunk(pages []Page) []Chunk {
	var out []Chunk
	for _, p := range pages {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		for i, body := range splitToSize(text) {
			out = append(out, Chunk{PageNumber: p.Number, Seq: i, Text: body})
		}
	}
	return out
}

// splitToSize riduce un testo a pezzi sotto MaxChunkChars, con la coda del
// pezzo precedente ripetuta in testa al successivo.
func splitToSize(text string) []string {
	if len(text) <= MaxChunkChars {
		return []string{text}
	}

	units := splitUnits(text)
	var out []string
	var cur strings.Builder

	flush := func() {
		body := strings.TrimSpace(cur.String())
		if body == "" {
			return
		}
		out = append(out, body)
		cur.Reset()
		// La coda del chunk appena chiuso apre il prossimo, tagliata
		// all'inizio di frase più vicino per non cominciare a metà parola.
		cur.WriteString(tailFrom(body))
	}

	for _, u := range units {
		if cur.Len()+len(u) > MaxChunkChars && strings.TrimSpace(cur.String()) != "" {
			flush()
		}
		// Un'unità più lunga del massimo da sola: entra comunque, perché
		// spezzarla a caso produrrebbe un chunk che comincia a metà parola.
		cur.WriteString(u)
	}
	if body := strings.TrimSpace(cur.String()); body != "" {
		out = append(out, body)
	}
	return out
}

// splitUnits spezza in paragrafi e, per i paragrafi troppo lunghi, in
// frasi. Le unità conservano lo spazio finale, così ricomporle non incolla
// le parole.
func splitUnits(text string) []string {
	var units []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len(para) <= MaxChunkChars {
			units = append(units, para+"\n\n")
			continue
		}
		last := 0
		for _, m := range sentenceEnd.FindAllStringIndex(para, -1) {
			units = append(units, para[last:m[1]])
			last = m[1]
		}
		if last < len(para) {
			units = append(units, para[last:])
		}
	}
	return units
}

// tailFrom restituisce gli ultimi ~ChunkOverlapChars caratteri di body,
// allineati all'inizio di una frase quando ce n'è una nella finestra.
func tailFrom(body string) string {
	if len(body) <= ChunkOverlapChars {
		return body + " "
	}
	window := body[len(body)-ChunkOverlapChars:]
	if m := sentenceEnd.FindStringIndex(window); m != nil {
		window = window[m[1]:]
	}
	window = strings.TrimSpace(window)
	if window == "" {
		return ""
	}
	return window + " "
}

// headingWords è il massimo di parole che può avere un titolo di sezione.
const headingWords = 8

// DetectHeading restituisce il titolo di sezione di una pagina, o stringa
// vuota. Un titolo è la prima riga quando è corta, non finisce con un punto
// e comincia in maiuscolo: sono le tre proprietà che distinguono
// "Fase di Upkeep" da "La partita termina quando...".
//
// Alimenta l'indice del manuale iniettato nel prompt, che è ciò che evita
// al modello la chiamata esplorativa al tool.
func DetectHeading(text string) string {
	first := strings.TrimSpace(text)
	if first == "" {
		return ""
	}
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = strings.TrimSpace(first[:idx])
	}
	if first == "" {
		return ""
	}
	if len(strings.Fields(first)) > headingWords {
		return ""
	}
	if strings.HasSuffix(first, ".") {
		return ""
	}
	r := []rune(first)[0]
	if !unicode.IsUpper(r) {
		return ""
	}
	return first
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: PASS. `TestChunk_SplitsLongPagesWithoutBreakingSentences` e `TestDetectHeading` sono i due che possono richiedere di aggiustare le soglie: aggiustare **l'implementazione**, non le asserzioni, salvo che un'asserzione sia palesemente sbagliata — in quel caso spiegarlo nel messaggio di commit.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/manuals/chunk.go backend/internal/manuals/chunk_test.go
git commit -m "feat: chunk manual pages and detect section headings"
```

---

### Task 5: Lo store — pagine, chunk e ricerca FTS5

**Files:**
- Create: `backend/internal/manuals/store.go`
- Test: `backend/internal/manuals/store_test.go`

**Interfaces:**
- Consumes: `manuals.Page`, `manuals.Chunk`, `manuals.Chunk()`, `manuals.DetectHeading()` (Task 2/4); le tabelle di Task 1.
- Produces:
  ```go
  type StoredPage struct {
      PageNumber int
      Text       string
      Heading    string
      Source     string // "pdf_text" | "vision" | "manual"
  }
  type Hit struct {
      PageNumber   int
      LanguageCode string
      ManualTitle  string
      Text         string
      FoundWith    []string
  }
  type SearchResult struct {
      Hits    []Hit
      Missing []string
  }
  type ManualText struct {
      Title        string
      LanguageCode string
      Path         string // url_or_path del media: serve al link "pag. 7"
      Pages        []StoredPage
  }
  type Corpus struct {
      Chars   int
      Manuals []ManualText
  }
  func NewStore(conn *sql.DB) *Store
  func (s *Store) ReplacePages(ctx context.Context, gameID, mediaID int64, languageCode string, pages []StoredPage) error
  func (s *Store) ListPages(ctx context.Context, mediaID int64) ([]StoredPage, error)
  func (s *Store) DeletePages(ctx context.Context, mediaID int64) error
  func (s *Store) HasPages(ctx context.Context, gameID int64) (bool, error)
  func (s *Store) Corpus(ctx context.Context, gameID int64) (Corpus, error)
  func (s *Store) Search(ctx context.Context, gameID int64, preferLang string, keywords []string) (SearchResult, error)
  func FormatSearchResult(r SearchResult) string
  ```

- [ ] **Step 1: Scrivere i test che falliscono**

Creare `backend/internal/manuals/store_test.go`:

```go
package manuals_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/manuals"
)

// seed prepara un gioco con una lingua e un manuale, e restituisce
// (gameID, mediaID). Scrive in SQL diretto: questo pacchetto non deve
// dipendere da internal/games solo per allestire i test.
func seed(t *testing.T, conn *sql.DB, gameName, lang, mediaTitle string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	g, err := conn.ExecContext(ctx, `INSERT INTO games (name, seats) VALUES (?, 1)`, gameName)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := g.LastInsertId()
	l, err := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, ?, 1, ?)`, gameID, lang, gameName)
	if err != nil {
		t.Fatalf("insert language: %v", err)
	}
	langID, _ := l.LastInsertId()
	m, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'manuale.pdf', ?)`, langID, mediaTitle)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	mediaID, _ := m.LastInsertId()
	return gameID, mediaID
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Ogni connessione a ":memory:" è un database a sé: il pool va fissato
	// a una, altrimenti una seconda connessione vede uno schema vuoto.
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

var regolePagine = []manuals.StoredPage{
	{PageNumber: 3, Heading: "Turno del giocatore", Source: "pdf_text",
		Text: "Turno del giocatore\nAll'inizio del proprio turno il giocatore pesca due carte dal mazzo comune e ne scarta una."},
	{PageNumber: 4, Heading: "Fase di Upkeep", Source: "pdf_text",
		Text: "Fase di Upkeep\nAl termine di ogni round ogni giocatore paga una moneta per ciascun edificio posseduto."},
	{PageNumber: 5, Heading: "Rimescolare", Source: "pdf_text",
		Text: "Rimescolare\nQuando la pila di pesca si esaurisce in una partita a due giocatori si rimescola la pila degli scarti."},
	{PageNumber: 8, Heading: "Conteggio e pareggi", Source: "pdf_text",
		Text: "Conteggio e pareggi\nSe due giocatori totalizzano lo stesso punteggio vince chi ha piu' monete in riserva."},
}

func TestReplacePages_StoresPagesAndBuildsChunks(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine); err != nil {
		t.Fatalf("replace: %v", err)
	}

	pages, err := store.ListPages(ctx, mediaID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pages) != 4 {
		t.Fatalf("attese 4 pagine, ottenute %d", len(pages))
	}
	if pages[0].PageNumber != 3 || pages[3].PageNumber != 8 {
		t.Fatalf("pagine non ordinate per numero: %d..%d", pages[0].PageNumber, pages[3].PageNumber)
	}
	if pages[1].Heading != "Fase di Upkeep" {
		t.Fatalf("heading perso: %q", pages[1].Heading)
	}

	var chunks int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_id = ?`, gameID).Scan(&chunks); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if chunks == 0 {
		t.Fatal("ReplacePages non ha costruito nessun chunk")
	}
	var indexed int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk_fts`).Scan(&indexed); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if indexed != chunks {
		t.Fatalf("l'indice FTS5 ha %d righe e i chunk %d: i trigger non allineano", indexed, chunks)
	}
}

func TestReplacePages_IsIdempotentAndRebuilds(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	// Un secondo salvataggio con una pagina sola deve lasciare una pagina
	// sola: è il comportamento del bottone "salva" dell'admin, che
	// sostituisce quel che c'era.
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine[:1]); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	pages, _ := store.ListPages(ctx, mediaID)
	if len(pages) != 1 {
		t.Fatalf("attesa 1 pagina dopo il secondo salvataggio, ottenute %d", len(pages))
	}
	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_id = ?`, gameID).Scan(&chunks)
	if chunks != 1 {
		t.Fatalf("i chunk non sono stati ricostruiti: %d", chunks)
	}
	var indexed int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk_fts`).Scan(&indexed)
	if indexed != 1 {
		t.Fatalf("l'indice FTS5 ha %d righe dopo la ricostruzione", indexed)
	}
}

func TestSearch_FindsByKeywordAndReportsTheMisses(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine); err != nil {
		t.Fatalf("replace: %v", err)
	}

	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep", "rimescola", "ripescare"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato per 'Upkeep', che è nel manuale")
	}
	var pages []int
	for _, h := range res.Hits {
		pages = append(pages, h.PageNumber)
		if h.ManualTitle != "Regolamento base" {
			t.Fatalf("titolo del manuale perso: %q", h.ManualTitle)
		}
		if h.LanguageCode != "it" {
			t.Fatalf("lingua persa: %q", h.LanguageCode)
		}
		if len(h.FoundWith) == 0 {
			t.Fatalf("hit a pagina %d senza FoundWith", h.PageNumber)
		}
	}
	if !containsInt(pages, 4) {
		t.Fatalf("'Upkeep' è a pagina 4, pagine trovate %v", pages)
	}
	// 'ripescare' non c'è nel manuale: deve comparire fra i mancanti, che
	// è l'informazione con cui il modello decide se riprovare.
	if !containsString(res.Missing, "ripescare") {
		t.Fatalf("'ripescare' doveva essere fra i mancanti, Missing = %v", res.Missing)
	}
	if containsString(res.Missing, "Upkeep") {
		t.Fatalf("'Upkeep' è stato trovato ma è fra i mancanti: %v", res.Missing)
	}
}

func TestSearch_DeduplicatesAndAccumulatesFoundWith(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	// Due parole chiave che colpiscono lo stesso chunk: deve uscire una
	// volta sola, con entrambe in FoundWith.
	res, err := store.Search(ctx, gameID, "it", []string{"pila", "rimescola"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	seen := map[int]int{}
	for _, h := range res.Hits {
		seen[h.PageNumber]++
	}
	for page, n := range seen {
		if n > 1 {
			t.Fatalf("pagina %d compare %d volte: manca la deduplicazione", page, n)
		}
	}
	for _, h := range res.Hits {
		if h.PageNumber == 5 && len(h.FoundWith) < 2 {
			t.Fatalf("la pagina 5 è stata trovata da due parole ma FoundWith = %v", h.FoundWith)
		}
	}
}

func TestSearch_SurvivesAnApostrophe(t *testing.T) {
	// Regressione trovata in fase di design: una parola chiave con
	// l'apostrofo passata grezza a MATCH dà "fts5: syntax error".
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	for _, kw := range []string{"all'inizio", `virgoletta"dentro`, "chi vince in caso di parita'"} {
		res, err := store.Search(ctx, gameID, "it", []string{kw})
		if err != nil {
			t.Fatalf("Search(%q) ha restituito errore: %v", kw, err)
		}
		_ = res // il punto è che non erra: trovare o no è secondario
	}
}

func TestSearch_DoesNotLeakIntoAnotherGame(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameA, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento A")
	gameB, mediaB := seed(t, conn, "Brass", "it", "Regolamento B")
	store.ReplacePages(ctx, gameA, mediaA, "it", regolePagine)
	store.ReplacePages(ctx, gameB, mediaB, "it", []manuals.StoredPage{
		{PageNumber: 1, Source: "manual", Text: "Fase di Upkeep di un gioco completamente diverso."},
	})

	res, err := store.Search(ctx, gameA, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, h := range res.Hits {
		if h.ManualTitle != "Regolamento A" {
			t.Fatalf("la ricerca su gameA ha restituito %q: sconfina su un altro gioco", h.ManualTitle)
		}
	}
}

func TestSearch_PrefersTheRequestedLanguage(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaIT := seed(t, conn, "Wingspan", "it", "Regolamento italiano")

	// Una seconda lingua sullo stesso gioco, con un manuale che contiene la
	// stessa parola chiave.
	l, _ := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'en', 0, 'Wingspan')`, gameID)
	langEN, _ := l.LastInsertId()
	m, _ := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'rules-en.pdf', 'English rulebook')`, langEN)
	mediaEN, _ := m.LastInsertId()

	store.ReplacePages(ctx, gameID, mediaIT, "it", []manuals.StoredPage{
		{PageNumber: 4, Source: "manual", Text: "Fase di Upkeep: si paga una moneta per edificio."},
	})
	store.ReplacePages(ctx, gameID, mediaEN, "en", []manuals.StoredPage{
		{PageNumber: 9, Source: "manual", Text: "Upkeep phase: pay one coin per building."},
	})

	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato")
	}
	if res.Hits[0].LanguageCode != "it" {
		t.Fatalf("con preferLang=it il primo risultato deve essere italiano, è %q", res.Hits[0].LanguageCode)
	}
	// Entrambe le lingue restano cercabili: il modello deve poter citare il
	// regolamento inglese quando l'italiano non dice nulla.
	if len(res.Hits) < 2 {
		t.Fatalf("attesi risultati da entrambe le lingue, ottenuti %d", len(res.Hits))
	}
}

func TestCorpusAndHasPages(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")

	has, err := store.HasPages(ctx, gameID)
	if err != nil {
		t.Fatalf("has pages: %v", err)
	}
	if has {
		t.Fatal("un gioco senza manuale preparato non ha pagine")
	}

	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	has, _ = store.HasPages(ctx, gameID)
	if !has {
		t.Fatal("dopo ReplacePages il gioco ha pagine")
	}

	corpus, err := store.Corpus(ctx, gameID)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if len(corpus.Manuals) != 1 {
		t.Fatalf("atteso 1 manuale nel corpus, ottenuti %d", len(corpus.Manuals))
	}
	if len(corpus.Manuals[0].Pages) != 4 {
		t.Fatalf("attese 4 pagine nel corpus, ottenute %d", len(corpus.Manuals[0].Pages))
	}
	if corpus.Chars == 0 {
		t.Fatal("Chars a zero: è la misura con cui si decide se il manuale entra intero nel contesto")
	}
}

func TestDeletePages_AndMediaCascade(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	if err := store.DeletePages(ctx, mediaID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	pages, _ := store.ListPages(ctx, mediaID)
	if len(pages) != 0 {
		t.Fatalf("attese 0 pagine dopo DeletePages, ottenute %d", len(pages))
	}
	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_id = ?`, gameID).Scan(&chunks)
	if chunks != 0 {
		t.Fatalf("DeletePages ha lasciato %d chunk", chunks)
	}

	// La cascata: cancellare il media porta via pagine e chunk senza che
	// nessuno lo chieda. Serve che le foreign key siano attive nella
	// connessione (db.Open lo fa con PRAGMA foreign_keys).
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)
	if _, err := conn.ExecContext(ctx, `DELETE FROM game_media WHERE id = ?`, mediaID); err != nil {
		t.Fatalf("delete media: %v", err)
	}
	var leftPages, leftChunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_page WHERE game_media_id = ?`, mediaID).Scan(&leftPages)
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_media_id = ?`, mediaID).Scan(&leftChunks)
	if leftPages != 0 || leftChunks != 0 {
		t.Fatalf("la cascata ha lasciato %d pagine e %d chunk", leftPages, leftChunks)
	}
}

func TestFormatSearchResult(t *testing.T) {
	out := manuals.FormatSearchResult(manuals.SearchResult{
		Hits: []manuals.Hit{
			{PageNumber: 7, LanguageCode: "it", ManualTitle: "Regolamento base",
				Text: "La partita termina quando la pila si esaurisce.", FoundWith: []string{"pila", "fine partita"}},
		},
		Missing: []string{"pareggio"},
	})
	for _, want := range []string{"[1]", "pag. 7", "Regolamento base", "pila, fine partita", "La partita termina", "pareggio"} {
		if !strings.Contains(out, want) {
			t.Fatalf("il payload non contiene %q:\n%s", want, out)
		}
	}

	empty := manuals.FormatSearchResult(manuals.SearchResult{Missing: []string{"a", "b"}})
	if !strings.Contains(strings.ToLower(empty), "nessun risultato") {
		t.Fatalf("senza risultati il payload deve dirlo al modello:\n%s", empty)
	}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func containsString(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: FAIL in compilazione, `undefined: manuals.NewStore`.

- [ ] **Step 3: Implementare lo store**

Creare `backend/internal/manuals/store.go`:

```go
package manuals

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// hitsPerKeyword è quanti chunk risalgono per ogni parola chiave. Due:
// abbastanza perché una parola chiave conti qualcosa nel payload, poco
// abbastanza perché otto parole chiave non producano un muro di testo.
const hitsPerKeyword = 2

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

// StoredPage è una pagina di manuale come sta nel database: il testo, il
// titolo di sezione rilevato e da dove viene.
type StoredPage struct {
	PageNumber int
	Text       string
	Heading    string
	Source     string
}

// Hit è un chunk trovato dalla ricerca. FoundWith dice con quali parole
// chiave è emerso: è informazione che il modello usa, non decorazione.
type Hit struct {
	PageNumber   int
	LanguageCode string
	ManualTitle  string
	Text         string
	FoundWith    []string
}

// SearchResult tiene insieme quel che si è trovato e quel che non si è
// trovato. Missing non è un errore: è ciò che dice al modello quali
// ipotesi lessicali sono cadute, così può riprovare con altre parole.
type SearchResult struct {
	Hits    []Hit
	Missing []string
}

type ManualText struct {
	Title        string
	LanguageCode string
	// Path è il nome del file su disco (game_media.url_or_path): serve a
	// trasformare "pag. 7" in un link al PDF a quella pagina.
	Path  string
	Pages []StoredPage
}

// Corpus è tutto il testo dei manuali di un gioco. Chars è la misura con
// cui si decide se il manuale entra intero nel contesto del modello,
// rendendo superfluo il tool.
type Corpus struct {
	Chars   int
	Manuals []ManualText
}

// ReplacePages sostituisce le pagine di un manuale e ricostruisce i chunk,
// tutto in una transazione: non esiste uno stato intermedio in cui le
// pagine sono nuove e l'indice è vecchio.
func (s *Store) ReplacePages(ctx context.Context, gameID, mediaID int64, languageCode string, pages []StoredPage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_page WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("clear pages: %w", err)
	}
	// I chunk si cancellano con una DELETE e non con la cascata, perché le
	// pagine e i chunk sono legati al media ma i chunk vanno ricostruiti
	// anche quando le pagine non cambiano (parametri di chunking diversi).
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_chunk WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("clear chunks: %w", err)
	}

	plain := make([]Page, 0, len(pages))
	for _, p := range pages {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			// Una pagina vuota si salva comunque: l'admin deve vedere il
			// buco nell'anteprima e poterlo riempire. Semplicemente non
			// produce chunk.
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO manual_page (game_media_id, page_number, text, heading, source)
				 VALUES (?, ?, '', NULL, ?)`, mediaID, p.PageNumber, p.Source); err != nil {
				return fmt.Errorf("insert empty page %d: %w", p.PageNumber, err)
			}
			continue
		}
		heading := p.Heading
		if heading == "" {
			heading = DetectHeading(text)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO manual_page (game_media_id, page_number, text, heading, source)
			 VALUES (?, ?, ?, ?, ?)`,
			mediaID, p.PageNumber, text, nullIfEmpty(heading), p.Source); err != nil {
			return fmt.Errorf("insert page %d: %w", p.PageNumber, err)
		}
		plain = append(plain, Page{Number: p.PageNumber, Text: text})
	}

	for _, c := range Chunk(plain) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO manual_chunk (game_id, game_media_id, language_code, page_number, seq, text)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			gameID, mediaID, languageCode, c.PageNumber, c.Seq, c.Text); err != nil {
			return fmt.Errorf("insert chunk p%d s%d: %w", c.PageNumber, c.Seq, err)
		}
	}

	return tx.Commit()
}

func (s *Store) ListPages(ctx context.Context, mediaID int64) ([]StoredPage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT page_number, text, COALESCE(heading, ''), source
		 FROM manual_page WHERE game_media_id = ? ORDER BY page_number`, mediaID)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer rows.Close()
	var out []StoredPage
	for rows.Next() {
		var p StoredPage
		if err := rows.Scan(&p.PageNumber, &p.Text, &p.Heading, &p.Source); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeletePages(ctx context.Context, mediaID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_chunk WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_page WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("delete pages: %w", err)
	}
	return tx.Commit()
}

// HasPages è la condizione, insieme al provider configurato, che governa la
// comparsa della chat in UI. Una COUNT invece di Corpus: qui interessa solo
// il sì o no, non il testo.
func (s *Store) HasPages(ctx context.Context, gameID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM manual_page p
		 JOIN game_media m ON m.id = p.game_media_id
		 JOIN game_languages l ON l.id = m.game_language_id
		 WHERE l.game_id = ? AND TRIM(p.text) <> ''`, gameID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("has pages: %w", err)
	}
	return n > 0, nil
}

func (s *Store) Corpus(ctx context.Context, gameID int64) (Corpus, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(m.title, 'Manuale'), m.url_or_path, l.language_code,
		        p.page_number, p.text, COALESCE(p.heading, ''), p.source
		 FROM manual_page p
		 JOIN game_media m ON m.id = p.game_media_id
		 JOIN game_languages l ON l.id = m.game_language_id
		 WHERE l.game_id = ? AND TRIM(p.text) <> ''
		 ORDER BY m.id, p.page_number`, gameID)
	if err != nil {
		return Corpus{}, fmt.Errorf("corpus: %w", err)
	}
	defer rows.Close()

	var out Corpus
	type key struct {
		title string
		lang  string
	}
	index := map[key]int{}
	for rows.Next() {
		var title, path, lang string
		var p StoredPage
		if err := rows.Scan(&title, &path, &lang, &p.PageNumber, &p.Text, &p.Heading, &p.Source); err != nil {
			return Corpus{}, err
		}
		out.Chars += len(p.Text)
		k := key{title, lang}
		i, ok := index[k]
		if !ok {
			out.Manuals = append(out.Manuals, ManualText{Title: title, Path: path, LanguageCode: lang})
			i = len(out.Manuals) - 1
			index[k] = i
		}
		out.Manuals[i].Pages = append(out.Manuals[i].Pages, p)
	}
	return out, rows.Err()
}

// Search esegue una query FTS5 per ogni parola chiave e unisce i risultati.
// Una query per parola e non un unico OR: con l'OR una parola comune
// sommerge una rara, mentre così ogni variante ha i suoi due posti
// garantiti — ed è quello che rende utile passare i sinonimi tutti insieme.
func (s *Store) Search(ctx context.Context, gameID int64, preferLang string, keywords []string) (SearchResult, error) {
	var res SearchResult
	order := []int64{}
	byID := map[int64]*Hit{}

	for _, raw := range keywords {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		hits, err := s.searchOne(ctx, gameID, preferLang, kw)
		if err != nil {
			return SearchResult{}, err
		}
		if len(hits) == 0 {
			res.Missing = append(res.Missing, kw)
			continue
		}
		for id, h := range hits {
			if existing, ok := byID[id]; ok {
				existing.FoundWith = append(existing.FoundWith, kw)
				continue
			}
			h.FoundWith = []string{kw}
			copied := h
			byID[id] = &copied
			order = append(order, id)
		}
	}

	for _, id := range order {
		res.Hits = append(res.Hits, *byID[id])
	}
	return res, nil
}

// searchOne cerca una sola parola chiave. Restituisce una mappa id -> Hit
// perché il chiamante deduplica sull'id del chunk.
func (s *Store) searchOne(ctx context.Context, gameID int64, preferLang, keyword string) (map[int64]Hit, error) {
	query := func(match string) (*sql.Rows, error) {
		return s.db.QueryContext(ctx,
			`SELECT c.id, c.page_number, c.language_code, COALESCE(m.title, 'Manuale'), c.text
			 FROM manual_chunk_fts f
			 JOIN manual_chunk c ON c.id = f.rowid
			 JOIN game_media m ON m.id = c.game_media_id
			 WHERE manual_chunk_fts MATCH ? AND c.game_id = ?
			 ORDER BY (c.language_code = ?) DESC, rank
			 LIMIT ?`, match, gameID, preferLang, hitsPerKeyword)
	}

	rows, err := query(keyword)
	if err != nil {
		// La sintassi FTS5 si rompe su un apostrofo o una virgoletta. Gli
		// operatori però sono utili (pesc*, "frase esatta"), quindi non si
		// filtrano a monte: si riprova con la parola neutralizzata solo
		// quando la prima forma è illegale.
		rows, err = query(escapeFTS(keyword))
		if err != nil {
			return nil, fmt.Errorf("fts search %q: %w", keyword, err)
		}
	}
	defer rows.Close()

	out := map[int64]Hit{}
	for rows.Next() {
		var id int64
		var h Hit
		if err := rows.Scan(&id, &h.PageNumber, &h.LanguageCode, &h.ManualTitle, &h.Text); err != nil {
			return nil, err
		}
		out[id] = h
	}
	return out, rows.Err()
}

// escapeFTS trasforma una parola chiave in una stringa FTS5 sempre legale:
// racchiusa in doppi apici, con i doppi apici interni raddoppiati. Perde gli
// operatori, che è esattamente il compromesso voluto — meglio una ricerca
// letterale che un errore.
func escapeFTS(kw string) string {
	return `"` + strings.ReplaceAll(kw, `"`, `""`) + `"`
}

// FormatSearchResult rende il payload che il modello legge come risultato
// del tool. Testo semplice e non JSON: un LLM lo legge meglio, e costa meno
// token.
func FormatSearchResult(r SearchResult) string {
	var b strings.Builder
	for i, h := range r.Hits {
		fmt.Fprintf(&b, "[%d] %s (%s), pag. %d  ·  trovato con: %s\n%s\n\n",
			i+1, h.ManualTitle, h.LanguageCode, h.PageNumber,
			strings.Join(h.FoundWith, ", "), h.Text)
	}
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "Nessun risultato per: %s\n", strings.Join(r.Missing, ", "))
	}
	out := strings.TrimSpace(b.String())
	if len(r.Hits) == 0 {
		return "Nessun risultato per nessuna parola chiave. " +
			"Riprova con altri termini, oppure di' che il manuale non lo dice.\n" + out
	}
	return out
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
```

- [ ] **Step 3b: Allegare il chunk adiacente ai bordi di pagina (spec 5.2)**

Aggiungere il test:

```go
func TestSearch_AttachesTheNeighbourChunkAtAPageBoundary(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	// Una pagina lunga abbastanza da produrre più chunk, con la parola
	// chiave nel PRIMO e il seguito nel secondo.
	coda := strings.Repeat("Testo che continua il regolamento oltre il taglio. ", 30)
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 4, Source: "manual",
			Text: "Fase di Upkeep. Ogni giocatore paga una moneta. " + coda},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE page_number = 4`).Scan(&chunks)
	if chunks < 2 {
		t.Skipf("la pagina ha prodotto %d chunk: niente bordo da verificare", chunks)
	}

	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato")
	}
	// Il chunk trovato è il primo della pagina: il payload deve portarsi
	// dietro anche il seguito, altrimenti una regola che continua nel
	// chunk successivo arriva al modello troncata.
	if !strings.Contains(res.Hits[0].Text, "continua il regolamento") {
		t.Fatalf("il chunk adiacente non è stato allegato:\n%s", res.Hits[0].Text)
	}
}
```

Poi, in `store.go`: aggiungere due campi non esportati a `Hit`, leggerli in `searchOne` e allegare il vicino in `Search`.

```go
type Hit struct {
	PageNumber   int
	LanguageCode string
	ManualTitle  string
	Text         string
	FoundWith    []string

	// Non esportati: servono solo a trovare il chunk adiacente, e non
	// hanno senso per chi legge il risultato.
	seq     int
	mediaID int64
}
```

In `searchOne`, estendere la SELECT a `c.seq, c.game_media_id` e lo `Scan` a `&h.seq, &h.mediaID`.

In `Search`, prima del `return`, sostituire il ciclo finale con:

```go
	for _, id := range order {
		res.Hits = append(res.Hits, *byID[id])
	}
	if err := s.attachNeighbours(ctx, res.Hits); err != nil {
		return SearchResult{}, err
	}
	return res, nil
```

E aggiungere:

```go
// attachNeighbours allega al testo di un hit il chunk adiacente della
// stessa pagina, quando il match cade sul primo o sull'ultimo chunk.
//
// La sovrapposizione di 100 caratteri del chunking copre la frase spezzata;
// questo copre il caso diverso in cui la regola *continua* per un altro
// paragrafo. Costa ~100 token e toglie in radice la seconda chiamata al
// tool del tipo "fammi leggere il resto".
func (s *Store) attachNeighbours(ctx context.Context, hits []Hit) error {
	for i := range hits {
		h := &hits[i]
		var maxSeq int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(seq), 0) FROM manual_chunk
			 WHERE game_media_id = ? AND page_number = ?`,
			h.mediaID, h.PageNumber).Scan(&maxSeq); err != nil {
			return fmt.Errorf("max seq: %w", err)
		}
		if maxSeq == 0 {
			continue // pagina di un chunk solo: non c'è nessun vicino
		}

		neighbour := -1
		after := false
		switch {
		case h.seq == 0:
			neighbour, after = 1, true
		case h.seq == maxSeq:
			neighbour, after = h.seq-1, false
		default:
			continue // non è un bordo: il chunk ha contesto da entrambi i lati
		}

		var text string
		err := s.db.QueryRowContext(ctx,
			`SELECT text FROM manual_chunk
			 WHERE game_media_id = ? AND page_number = ? AND seq = ?`,
			h.mediaID, h.PageNumber, neighbour).Scan(&text)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("neighbour chunk: %w", err)
		}
		if after {
			h.Text = h.Text + " " + text
		} else {
			h.Text = text + " " + h.Text
		}
	}
	return nil
}
```

Aggiungere `"errors"` agli import di `store.go`.


- [ ] **Step 4: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: PASS su tutti. `TestSearch_PrefersTheRequestedLanguage` è quello che può richiedere un aggiustamento dell'`ORDER BY`: se SQLite rifiuta di mescolare `rank` con un'espressione, ordinare in Go dopo aver letto i risultati (stabile, per `language_code == preferLang` prima) invece di forzare l'SQL.

- [ ] **Step 5: Verificare che `db.Open` attivi le foreign key**

`TestDeletePages_AndMediaCascade` passa solo se la connessione ha `PRAGMA foreign_keys = ON`. Controllare `backend/internal/db/`:

```bash
grep -rn "foreign_keys" backend/internal/db/
```

Se non c'è, **non** aggiungerlo in questo task: il test della cascata fallirebbe e va segnalato come scoperta, perché attivare le foreign key su un database esistente può far emergere violazioni in tutto lo schema. In quel caso, marcare quella parte del test con `t.Skip("PRAGMA foreign_keys non attivo: vedi nota")` e riportarlo nel commit.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/manuals/store.go backend/internal/manuals/store_test.go
git commit -m "feat: store manual pages and search them with FTS5"
```

---

### Task 6: Trascrizione di una pagina scansionata

**Files:**
- Create: `backend/internal/ai/ask.go` (solo `Transcribe` in questo task)
- Test: `backend/internal/ai/ask_test.go`

**Interfaces:**
- Consumes: `ai.HTTPClient`, `ai.ErrNotConfigured` (esistenti).
- Produces:
  ```go
  // sul tipo HTTPClient
  func (c *HTTPClient) Transcribe(ctx context.Context, jpeg []byte, pageNumber int) (string, error)
  // e il campo nuovo
  type HTTPClient struct { /* ... */ VisionModel string }
  func NewHTTPClientWithVision(baseURL, apiKey, model, visionModel string) *HTTPClient
  ```

- [ ] **Step 1: Scrivere i test che falliscono**

Creare `backend/internal/ai/ask_test.go`:

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

func TestTranscribe_SendsTheImageAsAnImageURLPart(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"## Fase di Upkeep\n\nOgni giocatore paga una moneta."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp")
	out, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8, 0xFF, 0xD9}, 4)
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if !strings.Contains(out, "Upkeep") {
		t.Fatalf("trascrizione inattesa: %q", out)
	}

	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("il body non è JSON valido: %v", err)
	}
	// Il modello di trascrizione è quello vision, NON quello di chat: il
	// modello di chat configurato può essere solo-testo.
	if sent.Model != "deepseek-v4-flash-vision-exp" {
		t.Fatalf("atteso il modello vision, inviato %q", sent.Model)
	}
	if len(sent.Messages) != 2 {
		t.Fatalf("attesi system + user, inviati %d messaggi", len(sent.Messages))
	}
	// Il contenuto utente deve essere un array di parti, con una parte
	// image_url che porta un data URI base64.
	if !strings.Contains(gotBody, `"type":"image_url"`) {
		t.Fatalf("manca la parte image_url:\n%s", gotBody)
	}
	if !strings.Contains(gotBody, "data:image/jpeg;base64,") {
		t.Fatalf("l'immagine non è un data URI base64:\n%s", gotBody)
	}
	// Il numero di pagina serve al modello per non inventare intestazioni.
	if !strings.Contains(gotBody, "4") {
		t.Fatalf("il numero di pagina non è nel prompt:\n%s", gotBody)
	}
}

func TestTranscribe_WithoutAVisionModelIsNotConfigured(t *testing.T) {
	// Provider configurato ma senza modello vision: è il caso dell'admin
	// che ha l'AI per le traduzioni e non ha impostato il campo nuovo.
	client := ai.NewHTTPClientWithVision("https://example.invalid", "sk-test", "deepseek-v4-flash", "")
	_, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8}, 1)
	if !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}

func TestTranscribe_EmptyAnswerIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"   "}}]}`)
	}))
	defer srv.Close()
	client := ai.NewHTTPClientWithVision(srv.URL, "sk-test", "m", "mv")
	if _, err := client.Transcribe(context.Background(), []byte{0xFF, 0xD8}, 1)  ; err == nil {
		t.Fatal("una trascrizione vuota deve essere un errore: l'admin deve poter riprovare quella pagina")
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -run Transcribe -v
```

Atteso: FAIL, `undefined: ai.NewHTTPClientWithVision`.

- [ ] **Step 3: Estendere il client e implementare `Transcribe`**

In `backend/internal/ai/client.go`, aggiungere il campo allo struct e il costruttore:

```go
type HTTPClient struct {
	// BaseURL è la radice OpenAI-compatible, senza /chat/completions.
	BaseURL string
	APIKey  string
	Model   string
	// VisionModel è il modello per leggere le pagine scansionate. Separato
	// da Model perché un modello di chat può essere solo-testo:
	// deepseek-v4-flash non accetta immagini, deepseek-v4-flash-vision-exp
	// sì. Vuoto = nessuna trascrizione automatica, e non è un guasto.
	VisionModel string
	HTTPClient  *http.Client
}

// NewHTTPClientWithVision è NewHTTPClient più il modello di trascrizione.
// NewHTTPClient resta per i chiamanti che traducono e non leggono manuali.
func NewHTTPClientWithVision(baseURL, apiKey, model, visionModel string) *HTTPClient {
	c := NewHTTPClient(baseURL, apiKey, model)
	c.VisionModel = strings.TrimSpace(visionModel)
	return c
}
```

Creare `backend/internal/ai/ask.go`:

```go
package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// transcribeTimeout è più generoso di requestTimeout: una pagina di manuale
// a 2000x3000 pixel è molta immagine da leggere, e un modello economico non
// è veloce.
const transcribeTimeout = 120 * time.Second

// contentPart è una parte del contenuto di un messaggio nel formato
// OpenAI: o testo, o un'immagine come data URI.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// multipartMessage è un messaggio il cui contenuto è un array di parti.
// chatMessage non basta: il suo Content è una stringa.
type multipartMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type visionRequest struct {
	Model       string             `json:"model"`
	Temperature float64            `json:"temperature"`
	Messages    []json.RawMessage  `json:"messages"`
}

const transcribeSystemPrompt = "Trascrivi in markdown il testo della pagina di regolamento che ricevi come immagine. " +
	"Riporta TUTTO il testo leggibile, nell'ordine di lettura, senza riassumere e senza commentare. " +
	"Conserva titoli, elenchi e tabelle. Ignora le illustrazioni che non contengono testo. " +
	"Se la pagina è illeggibile o non contiene testo, rispondi con la sola parola VUOTA."

// Transcribe legge una pagina scansionata di manuale e ne restituisce il
// testo in markdown. Si paga una volta per pagina, all'ingestione: è il
// motivo per cui a domanda non si mandano più immagini al modello.
func (c *HTTPClient) Transcribe(ctx context.Context, jpegBytes []byte, pageNumber int) (string, error) {
	if c.BaseURL == "" || c.APIKey == "" || c.VisionModel == "" {
		return "", ErrNotConfigured
	}

	system, err := json.Marshal(chatMessage{Role: "system", Content: transcribeSystemPrompt})
	if err != nil {
		return "", err
	}
	user, err := json.Marshal(multipartMessage{
		Role: "user",
		Content: []contentPart{
			{Type: "text", Text: fmt.Sprintf("Pagina %d del regolamento.", pageNumber)},
			{Type: "image_url", ImageURL: &imageURL{
				URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegBytes),
			}},
		},
	})
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(visionRequest{
		Model:       c.VisionModel,
		Temperature: 0,
		Messages:    []json.RawMessage{system, user},
	})
	if err != nil {
		return "", err
	}

	out, err := c.postChat(ctx, payload, transcribeTimeout)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(strings.TrimSpace(out), "VUOTA") {
		// Una pagina di sole illustrazioni: non è un errore, ma non è
		// nemmeno testo. Il chiamante la salva vuota.
		return "", nil
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("il modello non ha restituito testo per la pagina %d", pageNumber)
	}
	return strings.TrimSpace(out), nil
}

// postChat manda una richiesta già serializzata a /chat/completions e
// restituisce il contenuto della prima scelta. Estratto perché Translate,
// Transcribe e Ask fanno la stessa danza di HTTP ed errori.
func (c *HTTPClient) postChat(ctx context.Context, payload []byte, timeout time.Duration) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("ai provider returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse ai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("ai provider returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
```

Aggiungere `"time"` agli import di `ask.go`.

- [ ] **Step 4: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -v
```

Atteso: PASS, compresi i test preesistenti di `Translate`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/
git commit -m "feat: transcribe scanned manual pages with a vision model"
```

---

### Task 7: L'agente — loop con il tool, e il caso in cui il tool non serve

**Files:**
- Modify: `backend/internal/ai/ask.go`
- Modify: `backend/internal/manuals/store.go` (i due formattatori per il prompt)
- Test: `backend/internal/ai/ask_test.go`, `backend/internal/manuals/store_test.go`

**Interfaces:**
- Consumes: `HTTPClient.postChat` (Task 6), `manuals.Corpus` (Task 5).
- Produces:
  ```go
  // in manuals
  func FormatCorpus(c Corpus) string  // tutto il testo, con marcatori di pagina
  func FormatIndex(c Corpus) string   // "Preparazione p.2 · Fase di Upkeep p.4"

  // in ai
  const InlineCorpusMaxChars = 18000
  type Turn struct {
      Role string // "user" | "assistant"
      Text string
  }
  type SearchFunc func(ctx context.Context, keywords []string) (string, error)
  type AskRequest struct {
      GameName    string
      Turns       []Turn
      CorpusChars int
      CorpusText  string
      CorpusIndex string
      Search      SearchFunc
  }
  func (c *HTTPClient) Ask(ctx context.Context, req AskRequest) (string, error)
  ```

`SearchFunc` restituisce una **stringa** già formattata, non `[]Hit`: così `internal/ai` non conosce SQLite né i tipi di `manuals`, e il loop si testa con una closure di due righe.

- [ ] **Step 1: Scrivere il test dei due formattatori**

In `backend/internal/manuals/store_test.go`:

```go
func TestFormatIndexAndCorpus(t *testing.T) {
	corpus := manuals.Corpus{
		Chars: 120,
		Manuals: []manuals.ManualText{{
			Title: "Regolamento base", LanguageCode: "it",
			Pages: []manuals.StoredPage{
				{PageNumber: 4, Heading: "Fase di Upkeep", Text: "Ogni giocatore paga una moneta."},
				{PageNumber: 7, Heading: "", Text: "La partita termina quando la pila si esaurisce."},
			},
		}},
	}

	idx := manuals.FormatIndex(corpus)
	if !strings.Contains(idx, "Fase di Upkeep p.4") {
		t.Fatalf("l'indice non riporta il titolo con la pagina: %q", idx)
	}
	// Una pagina senza heading non inquina l'indice con una riga vuota.
	if strings.Contains(idx, "p.7") {
		t.Fatalf("una pagina senza titolo non va nell'indice: %q", idx)
	}

	full := manuals.FormatCorpus(corpus)
	for _, want := range []string{"Regolamento base", "pag. 4", "pag. 7", "paga una moneta", "pila si esaurisce"} {
		if !strings.Contains(full, want) {
			t.Fatalf("il corpus non contiene %q:\n%s", want, full)
		}
	}
}
```

- [ ] **Step 2: Implementare i due formattatori**

In `backend/internal/manuals/store.go`:

```go
// FormatIndex rende l'indice del manuale per il prompt: solo i titoli di
// sezione con la loro pagina, ~200 token. È quel che evita al modello la
// chiamata esplorativa al tool — uno che vede l'indice scrive le parole
// chiave giuste al primo colpo, uno cieco tira a indovinare e richiama.
func FormatIndex(c Corpus) string {
	var parts []string
	for _, m := range c.Manuals {
		for _, p := range m.Pages {
			if strings.TrimSpace(p.Heading) == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s p.%d", p.Heading, p.PageNumber))
		}
	}
	return strings.Join(parts, " · ")
}

// FormatCorpus rende tutto il testo dei manuali, con i marcatori di pagina
// che permettono al modello di citare. Serve al caso sotto soglia, dove il
// manuale entra intero nel contesto e non c'è nessun tool da chiamare.
func FormatCorpus(c Corpus) string {
	var b strings.Builder
	for _, m := range c.Manuals {
		fmt.Fprintf(&b, "=== %s (%s) ===\n\n", m.Title, m.LanguageCode)
		for _, p := range m.Pages {
			fmt.Fprintf(&b, "--- pag. %d ---\n%s\n\n", p.PageNumber, p.Text)
		}
	}
	return strings.TrimSpace(b.String())
}
```

- [ ] **Step 3: Eseguire il test dei formattatori**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -run FormatIndexAndCorpus -v
```

Atteso: PASS.

- [ ] **Step 4: Scrivere i test del loop, che falliscono**

In `backend/internal/ai/ask_test.go`:

```go
// askServer è un provider finto scriptato: ogni richiesta consuma la
// prossima risposta della lista, e le richieste ricevute restano
// ispezionabili.
type askServer struct {
	t         *testing.T
	responses []string
	requests  []string
}

func (a *askServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		a.requests = append(a.requests, string(body))
		if len(a.responses) == 0 {
			a.t.Fatalf("il provider finto ha ricevuto %d richieste ma non ha più risposte", len(a.requests))
		}
		next := a.responses[0]
		a.responses = a.responses[1:]
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, next)
	}
}

const answerOnly = `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"La partita finisce subito. Regolamento base, pag. 7."}}]}`

func toolCallResponse(args string) string {
	return `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,` +
		`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"cerca_nel_manuale","arguments":` +
		strconv.Quote(args) + `}}]}}]}`
}

func TestAsk_CallsTheToolThenAnswers(t *testing.T) {
	srv := &askServer{t: t, responses: []string{
		toolCallResponse(`{"parole_chiave":["pila pesca","fine partita","pareggio"]}`),
		answerOnly,
	}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	var gotKeywords []string
	client := ai.NewHTTPClient(ts.URL, "sk-test", "deepseek-v4-flash")
	out, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "finite le carte che si fa?"}},
		CorpusChars: 90000, // sopra soglia: il tool serve
		CorpusIndex: "Fase di Upkeep p.4 · Fine partita p.7",
		Search: func(ctx context.Context, kw []string) (string, error) {
			gotKeywords = kw
			return "[1] Regolamento base (it), pag. 7  ·  trovato con: pila pesca\nLa partita termina...", nil
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !strings.Contains(out, "pag. 7") {
		t.Fatalf("risposta inattesa: %q", out)
	}
	if len(gotKeywords) != 3 || gotKeywords[0] != "pila pesca" {
		t.Fatalf("le parole chiave non sono arrivate alla ricerca: %v", gotKeywords)
	}
	if len(srv.requests) != 2 {
		t.Fatalf("attese 2 richieste al provider, fatte %d", len(srv.requests))
	}

	// Prima richiesta: il tool va dichiarato e l'indice va nel prompt.
	if !strings.Contains(srv.requests[0], "cerca_nel_manuale") {
		t.Fatalf("il tool non è dichiarato nella prima richiesta:\n%s", srv.requests[0])
	}
	if !strings.Contains(srv.requests[0], "Fine partita p.7") {
		t.Fatalf("l'indice del manuale non è nel prompt:\n%s", srv.requests[0])
	}
	// Seconda richiesta: deve contenere il messaggio assistant con i
	// tool_calls E il risultato con il suo tool_call_id, altrimenti il
	// provider rifiuta la conversazione.
	if !strings.Contains(srv.requests[1], `"tool_call_id":"call_1"`) {
		t.Fatalf("il risultato del tool non è legato alla chiamata:\n%s", srv.requests[1])
	}
	if !strings.Contains(srv.requests[1], "La partita termina") {
		t.Fatalf("il risultato della ricerca non è stato rimandato al modello:\n%s", srv.requests[1])
	}
}

func TestAsk_ShortManualGoesInlineWithNoTool(t *testing.T) {
	srv := &askServer{t: t, responses: []string{answerOnly}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	searched := false
	client := ai.NewHTTPClient(ts.URL, "sk-test", "deepseek-v4-flash")
	_, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "come finisce?"}},
		CorpusChars: 4000, // sotto InlineCorpusMaxChars
		CorpusText:  "=== Regolamento base (it) ===\n\n--- pag. 7 ---\nLa partita termina subito.",
		CorpusIndex: "Fine partita p.7",
		Search: func(ctx context.Context, kw []string) (string, error) {
			searched = true
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if searched {
		t.Fatal("con un manuale corto la ricerca non va nemmeno sfiorata")
	}
	if len(srv.requests) != 1 {
		t.Fatalf("attesa 1 sola richiesta, fatte %d", len(srv.requests))
	}
	if strings.Contains(srv.requests[0], "cerca_nel_manuale") {
		t.Fatalf("il tool NON va dichiarato quando il manuale è inline:\n%s", srv.requests[0])
	}
	if !strings.Contains(srv.requests[0], "La partita termina subito") {
		t.Fatalf("il testo del manuale non è nel prompt:\n%s", srv.requests[0])
	}
}

func TestAsk_SendsTheConversationHistory(t *testing.T) {
	srv := &askServer{t: t, responses: []string{answerOnly}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	_, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		CorpusChars: 100,
		CorpusText:  "manuale",
		Turns: []ai.Turn{
			{Role: "user", Text: "come si contano i punti?"},
			{Role: "assistant", Text: "Ogni edificio vale i punti stampati."},
			{Role: "user", Text: "e se siamo pari?"},
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	for _, want := range []string{"come si contano i punti?", "Ogni edificio vale", "e se siamo pari?"} {
		if !strings.Contains(srv.requests[0], want) {
			t.Fatalf("lo storico non è arrivato al provider, manca %q:\n%s", want, srv.requests[0])
		}
	}
}

func TestAsk_StopsAStuckModelAndStillAnswers(t *testing.T) {
	// Un modello che chiama il tool all'infinito: la guardia deve fermarlo
	// e forzare una risposta togliendo il tool, non restituire un errore.
	// Non è un limite sull'utente, è protezione da un bug del modello.
	responses := []string{}
	for i := 0; i < 5; i++ {
		responses = append(responses, toolCallResponse(`{"parole_chiave":["x"]}`))
	}
	responses = append(responses, answerOnly)

	srv := &askServer{t: t, responses: responses}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	calls := 0
	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	out, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "?"}},
		CorpusChars: 90000,
		Search: func(ctx context.Context, kw []string) (string, error) {
			calls++
			return "niente", nil
		},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if out == "" {
		t.Fatal("la guardia deve produrre una risposta, non il vuoto")
	}
	if calls != ai.MaxToolIterations {
		t.Fatalf("il tool doveva essere chiamato %d volte, chiamato %d", ai.MaxToolIterations, calls)
	}
	// L'ultima richiesta è quella senza tool: è così che si forza la
	// risposta invece di lasciare il modello a girare.
	last := srv.requests[len(srv.requests)-1]
	if strings.Contains(last, "cerca_nel_manuale") {
		t.Fatalf("l'ultima richiesta doveva essere senza tool:\n%s", last)
	}
}

func TestAsk_AcceptsKeywordsSentAsAString(t *testing.T) {
	// I modelli economici a volte mandano una stringa dove lo schema dice
	// array. Rifiutarla significa perdere la domanda per un dettaglio di
	// serializzazione, quindi si accetta e si divide.
	srv := &askServer{t: t, responses: []string{
		toolCallResponse(`{"parole_chiave":"pareggio, stesso punteggio"}`),
		answerOnly,
	}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	var got []string
	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	if _, err := client.Ask(context.Background(), ai.AskRequest{
		GameName:    "Wingspan",
		Turns:       []ai.Turn{{Role: "user", Text: "?"}},
		CorpusChars: 90000,
		Search: func(ctx context.Context, kw []string) (string, error) {
			got = kw
			return "ok", nil
		},
	}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(got) != 2 || got[0] != "pareggio" || got[1] != "stesso punteggio" {
		t.Fatalf("una stringa di parole chiave va divisa in due, ottenuto %v", got)
	}
}

func TestAsk_WithoutAProviderIsNotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	_, err := client.Ask(context.Background(), ai.AskRequest{GameName: "Wingspan"})
	if !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}
```

Aggiungere `"strconv"` agli import del test.

- [ ] **Step 5: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -run Ask -v
```

Atteso: FAIL, `undefined: ai.AskRequest`.

- [ ] **Step 6: Implementare `Ask`**

In `backend/internal/ai/ask.go`, aggiungere:

```go
// InlineCorpusMaxChars è la soglia sotto la quale il manuale entra intero
// nel contesto e il tool non viene nemmeno dichiarato. ~6.000 token,
// stimati a 3 caratteri per token (conservativo per l'italiano).
//
// È la leva più efficace per ridurre le chiamate al tool: non offrirlo.
// Un manuale di 4 pagine sta largamente sotto.
const InlineCorpusMaxChars = 18000

// MaxToolIterations ferma un modello che si incarta a richiamare lo stesso
// tool in ciclo. Non è un limite sull'utente: è protezione da un bug del
// modello, e quando scatta si forza una risposta togliendo il tool invece
// di restituire un errore.
const MaxToolIterations = 5

// askTimeout è il tetto complessivo di una domanda, giri del tool
// compresi. Un handler HTTP senza timeout tiene una goroutine occupata per
// sempre.
const askTimeout = 60 * time.Second

// Turn è un messaggio della conversazione così come arriva dal browser.
// Lo storico non è persistito da nessuna parte: vive in deep-chat e torna
// indietro a ogni domanda.
type Turn struct {
	Role string
	Text string
}

// SearchFunc cerca nel manuale e restituisce il payload già formattato per
// il modello. È una funzione e non un'interfaccia con i tipi di manuals:
// così questo pacchetto non sa niente di SQLite, e il loop si testa con una
// closure.
type SearchFunc func(ctx context.Context, keywords []string) (string, error)

type AskRequest struct {
	GameName string
	Turns    []Turn
	// CorpusChars decide inline vs tool; CorpusText è il manuale intero
	// (serve solo sotto soglia); CorpusIndex è l'indice dei titoli (serve
	// sempre, ed è quel che evita la chiamata esplorativa).
	CorpusChars int
	CorpusText  string
	CorpusIndex string
	Search      SearchFunc
}

const searchToolName = "cerca_nel_manuale"

// searchToolSchema è dove sta il lavoro di "far fare al modello una sola
// chiamata": la descrizione del parametro chiede le varianti tutte insieme,
// con un esempio. Non c'è nessuna regola nel prompt che vieti la seconda
// chiamata — si rende inutile, non si proibisce.
const searchToolSchema = `{
  "type": "object",
  "properties": {
    "parole_chiave": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Da 3 a 8 varianti della stessa cosa: sinonimi, il termine tecnico e quello colloquiale, singolare e plurale. Esempio: [\"pareggio\", \"stesso punteggio\", \"parità\", \"spareggio\"]. Supporta \"frase esatta\", OR, AND e i prefissi con *."
    }
  },
  "required": ["parole_chiave"]
}`

type toolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolDef struct {
	Type     string          `json:"type"`
	Function toolFunctionDef `json:"function"`
}

type askRequestBody struct {
	Model       string            `json:"model"`
	Temperature float64           `json:"temperature"`
	Messages    []json.RawMessage `json:"messages"`
	Tools       []toolDef         `json:"tools,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type askChoice struct {
	FinishReason string          `json:"finish_reason"`
	Message      json.RawMessage `json:"message"`
}

type askResponseBody struct {
	Choices []askChoice `json:"choices"`
}

type assistantMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

type toolResultMessage struct {
	Role       string `json:"role"`
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// Ask risponde a una domanda sulle regole di un gioco. Se il manuale sta
// sotto soglia entra intero nel prompt e il giro è uno solo; altrimenti il
// modello riceve il tool di ricerca e l'indice del manuale.
func (c *HTTPClient) Ask(ctx context.Context, req AskRequest) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	inline := req.CorpusChars > 0 && req.CorpusChars <= InlineCorpusMaxChars

	system, err := json.Marshal(chatMessage{Role: "system", Content: askSystemPrompt(req, inline)})
	if err != nil {
		return "", err
	}
	messages := []json.RawMessage{system}
	for _, t := range req.Turns {
		role := "user"
		if t.Role == "assistant" || t.Role == "ai" {
			role = "assistant"
		}
		raw, err := json.Marshal(chatMessage{Role: role, Content: t.Text})
		if err != nil {
			return "", err
		}
		messages = append(messages, raw)
	}

	tools := []toolDef{}
	if !inline && req.Search != nil {
		tools = append(tools, toolDef{
			Type: "function",
			Function: toolFunctionDef{
				Name: searchToolName,
				Description: "Cerca nel regolamento del gioco. Passa in un'unica chiamata " +
					"tutte le varianti lessicali plausibili: la ricerca è lessicale, " +
					"quindi più varianti trovano più cose.",
				Parameters: json.RawMessage(searchToolSchema),
			},
		})
	}

	for iteration := 0; ; iteration++ {
		// Alla scadenza della guardia si rifà la richiesta senza tool: il
		// modello è costretto a rispondere con quello che ha.
		active := tools
		if iteration >= MaxToolIterations {
			active = nil
		}

		payload, err := json.Marshal(askRequestBody{
			Model:       c.Model,
			Temperature: 0.2,
			Messages:    messages,
			Tools:       active,
		})
		if err != nil {
			return "", err
		}

		raw, err := c.postRaw(ctx, payload)
		if err != nil {
			return "", err
		}
		var parsed askResponseBody
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", fmt.Errorf("parse ai response: %w", err)
		}
		if len(parsed.Choices) == 0 {
			return "", errors.New("ai provider returned no choices")
		}
		choice := parsed.Choices[0]

		var msg assistantMessage
		// Content può arrivare null quando ci sono tool_calls: un errore di
		// unmarshal qui non deve far perdere i tool_calls, quindi si ignora
		// e si guarda cosa si è riusciti a leggere.
		_ = json.Unmarshal(choice.Message, &msg)

		if len(msg.ToolCalls) == 0 {
			answer := strings.TrimSpace(msg.Content)
			if answer == "" {
				return "", errors.New("ai provider returned an empty answer")
			}
			return answer, nil
		}
		if active == nil {
			// Il modello insiste col tool in una richiesta che non ne
			// dichiara nessuno: non c'è altro da fare che dirlo.
			return "", errors.New("ai provider kept calling a tool that was not offered")
		}

		// Il messaggio assistant va rimandato verbatim, altrimenti il
		// provider non riconosce a cosa risponde il risultato del tool.
		messages = append(messages, choice.Message)

		for _, call := range msg.ToolCalls {
			result := "Tool sconosciuto."
			if call.Function.Name == searchToolName && req.Search != nil {
				keywords := parseKeywords(call.Function.Arguments)
				out, err := req.Search(ctx, keywords)
				if err != nil {
					log.Printf("ask: manual search failed: %v", err)
					result = "La ricerca nel manuale non è disponibile in questo momento."
				} else {
					result = out
				}
			}
			resultRaw, err := json.Marshal(toolResultMessage{
				Role: "tool", ToolCallID: call.ID, Content: result,
			})
			if err != nil {
				return "", err
			}
			messages = append(messages, resultRaw)
		}

		if iteration+1 >= MaxToolIterations {
			log.Printf("ask: il modello ha chiamato %s %d volte: forzo la risposta", searchToolName, iteration+1)
		}
	}
}

// parseKeywords legge l'argomento del tool. Accetta sia l'array previsto
// dallo schema sia una stringa separata da virgole, perché i modelli
// economici a volte mandano la seconda: rifiutarla significherebbe perdere
// la domanda per un dettaglio di serializzazione.
func parseKeywords(arguments string) []string {
	var asArray struct {
		Keywords []string `json:"parole_chiave"`
	}
	if err := json.Unmarshal([]byte(arguments), &asArray); err == nil && len(asArray.Keywords) > 0 {
		return trimAll(asArray.Keywords)
	}
	var asString struct {
		Keywords string `json:"parole_chiave"`
	}
	if err := json.Unmarshal([]byte(arguments), &asString); err == nil && strings.TrimSpace(asString.Keywords) != "" {
		return trimAll(strings.Split(asString.Keywords, ","))
	}
	return nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// postRaw è postChat senza l'estrazione del contenuto: Ask deve guardare
// finish_reason e tool_calls, non solo il testo.
func (c *HTTPClient) postRaw(ctx context.Context, payload []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: askTimeout}
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("ai provider returned status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// askSystemPrompt costruisce le istruzioni. La regola che conta è la terza:
// al tavolo una regola inventata fa più danno di un "non lo dice".
func askSystemPrompt(req AskRequest, inline bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sei l'assistente regole di %q per un'associazione di giochi da tavolo. ", req.GameName)
	b.WriteString("Chi ti scrive è in piedi a un tavolo, con le carte in mano: rispondi in italiano, breve, come si parla. ")
	b.WriteString("Rispondi SOLO con quello che c'è nel regolamento. ")
	b.WriteString("Se il regolamento non lo dice, dillo chiaramente invece di dedurre: al tavolo una regola inventata fa danno. ")
	b.WriteString("Cita sempre la pagina da cui viene la risposta, nella forma \"Regolamento base, pag. 7\". ")
	b.WriteString("Non inventare nomi di carte, valori o numeri che non hai letto.\n\n")

	if req.CorpusIndex != "" {
		fmt.Fprintf(&b, "Indice del regolamento: %s\n\n", req.CorpusIndex)
	}
	if inline {
		b.WriteString("Il regolamento completo:\n\n")
		b.WriteString(req.CorpusText)
	} else {
		b.WriteString("Per leggere il regolamento usa lo strumento di ricerca. ")
		b.WriteString("Se una ricerca non trova nulla, riprova con altre parole prima di dire che il manuale non lo dice.")
	}
	return b.String()
}
```

Aggiungere `"log"` agli import di `ask.go`.

- [ ] **Step 7: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/ -v
```

Atteso: PASS su tutti, `Translate` compreso.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/ai/ backend/internal/manuals/store.go backend/internal/manuals/store_test.go
git commit -m "feat: add the manual Q&A agent loop with a single search tool"
```

---

### Task 8: Handler admin — prepara, rivedi, salva

**Files:**
- Create: `backend/internal/httpapi/manuals_handlers.go`
- Modify: `backend/internal/httpapi/router.go`, `backend/internal/httpapi/testhelpers_test.go`
- Test: `backend/internal/httpapi/manuals_handlers_test.go`

**Interfaces:**
- Consumes: `manuals.Store` (Task 5), `HTTPClient.Transcribe` (Task 6), `storage.Store` (esistente).
- Produces: quattro rotte protette; `Server.Manuals *manuals.Store`; `Server.Vision ai.Transcriber`.

- [ ] **Step 1: Aggiungere `Manuals` al Server e agli helper di test**

In `backend/internal/httpapi/router.go`, dentro `type Server struct`, dopo `Storage`:

```go
	Manuals *manuals.Store
	// Vision, quando è valorizzato, è il trascrittore da usare. Lasciato a
	// nil il server ne costruisce uno per richiesta dalle impostazioni,
	// come per AI: cambiare modello non richiede un riavvio, e i test
	// possono iniettare un finto.
	Vision ai.Transcriber
```

In `internal/ai/ask.go`, dichiarare l'interfaccia accanto a `Translator`:

```go
// Transcriber è l'astrazione che serve agli handler: leggere una pagina
// scansionata. HTTPClient la implementa.
type Transcriber interface {
	Transcribe(ctx context.Context, jpeg []byte, pageNumber int) (string, error)
}
```

In `backend/internal/httpapi/testhelpers_test.go`, aggiungere `Manuals: manuals.NewStore(conn),` alla costruzione del `Server` e l'import di `boardgames-manager/internal/manuals`.

- [ ] **Step 2: Scrivere i test che falliscono**

Creare `backend/internal/httpapi/manuals_handlers_test.go`. Leggere prima `game_media_handlers_test.go` per riusare gli helper esistenti di login e di creazione gioco/lingua/media invece di reinventarli.

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
)

// fakeTranscriber finge un modello multimodale: restituisce un testo che
// contiene il numero di pagina, così i test verificano l'accoppiamento.
type fakeTranscriber struct {
	calls int
	err   error
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, jpeg []byte, page int) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return "Trascrizione della pagina " + itoa(page), nil
}

func itoa(n int) string  { return strconv.Itoa(n) }
func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func aiNotConfigured() error { return ai.ErrNotConfigured }
```

Aggiungere `"strconv"` e `"boardgames-manager/internal/ai"` agli import del test.

```go
func TestExtractManual_ReturnsPagesWithoutSaving(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	tr := &fakeTranscriber{}
	server.Vision = tr
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router, server)

	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	req := httptest.NewRequest(http.MethodPost,
		"/api/games/"+itoa64(gameID)+"/languages/it/media/"+itoa64(mediaID)+"/extract", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Source string `json:"source"`
		Pages  []struct {
			PageNumber int    `json:"pageNumber"`
			Text       string `json:"text"`
			Heading    string `json:"heading"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if body.Source != "vision" {
		t.Fatalf("un PDF scansionato va sul percorso vision, source = %q", body.Source)
	}
	if len(body.Pages) == 0 {
		t.Fatal("nessuna pagina proposta")
	}
	if tr.calls != len(body.Pages) {
		t.Fatalf("una richiesta per pagina: %d pagine, %d chiamate", len(body.Pages), tr.calls)
	}

	// extract NON deve salvare: la conferma dell'admin è un altro giro.
	pages, err := manuals.NewStore(conn).ListPages(context.Background(), mediaID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("extract ha salvato %d pagine: doveva solo proporle", len(pages))
	}
}

func TestExtractManual_WithoutAVisionModelReturnsEmptyPages(t *testing.T) {
	// Nessun modello vision configurato: l'anteprima si apre comunque, con
	// le pagine vuote, e l'admin scrive il testo a mano. La funzione non si
	// blocca perché manca l'AI.
	server, conn := newTestServerWithDB(t)
	server.Vision = &fakeTranscriber{err: aiNotConfigured()}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router, server)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	req := httptest.NewRequest(http.MethodPost,
		"/api/games/"+itoa64(gameID)+"/languages/it/media/"+itoa64(mediaID)+"/extract", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("senza modello vision l'anteprima si apre comunque: atteso 200, ottenuto %d (%s)",
			rec.Code, rec.Body.String())
	}
	var body struct {
		Pages []struct {
			Text string `json:"text"`
		} `json:"pages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Pages) == 0 {
		t.Fatal("attese pagine vuote da riempire a mano, ottenute zero")
	}
	for i, p := range body.Pages {
		if p.Text != "" {
			t.Fatalf("pagina %d non doveva avere testo: %q", i, p.Text)
		}
	}
}

func TestPutManualPages_SavesAndBuildsTheIndex(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router, server)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	payload := `{"pages":[
	  {"pageNumber":4,"text":"Fase di Upkeep\nOgni giocatore paga una moneta.","source":"vision"},
	  {"pageNumber":7,"text":"Fine partita\nLa partita termina subito.","source":"manual"}
	]}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/games/"+itoa64(gameID)+"/languages/it/media/"+itoa64(mediaID)+"/pages",
		strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	store := manuals.NewStore(conn)
	pages, _ := store.ListPages(context.Background(), mediaID)
	if len(pages) != 2 {
		t.Fatalf("attese 2 pagine salvate, ottenute %d", len(pages))
	}
	if pages[0].Heading != "Fase di Upkeep" {
		t.Fatalf("l'heading doveva essere rilevato al salvataggio, è %q", pages[0].Heading)
	}
	// I chunk devono essere pronti: la ricerca funziona subito dopo il salvataggio.
	res, err := store.Search(context.Background(), gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("dopo il salvataggio la ricerca non trova nulla: i chunk non sono stati costruiti")
	}
}

func TestManualPages_RequireAuth(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/extract"},
		{http.MethodGet, "/pages"},
		{http.MethodPut, "/pages"},
		{http.MethodDelete, "/pages"},
	} {
		path := "/api/games/" + itoa64(gameID) + "/languages/it/media/" + itoa64(mediaID) + tc.path
		req := httptest.NewRequest(tc.method, path, strings.NewReader(`{"pages":[]}`))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s senza sessione: atteso 401, ottenuto %d", tc.method, path, rec.Code)
		}
	}
}

func TestDeleteManualPages(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router, server)
	gameID, mediaID := seedGameWithScannedManual(t, server, conn)

	store := manuals.NewStore(conn)
	store.ReplacePages(context.Background(), gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 1, Text: "Testo qualsiasi.", Source: "manual"},
	})

	req := httptest.NewRequest(http.MethodDelete,
		"/api/games/"+itoa64(gameID)+"/languages/it/media/"+itoa64(mediaID)+"/pages", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("atteso 204, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	pages, _ := store.ListPages(context.Background(), mediaID)
	if len(pages) != 0 {
		t.Fatalf("restano %d pagine", len(pages))
	}
}
```

**Helper da scrivere in questo test file** (o riusare se già esistono con altro nome in `game_media_handlers_test.go`):

- `loginAsAdmin(t, router, server) *http.Cookie` — bootstrap del primo admin e login, restituisce il cookie di sessione.
- `seedGameWithScannedManual(t, server, conn) (gameID, mediaID int64)` — crea un gioco con lingua `it` e un media `file` il cui contenuto su disco è un PDF scansionato. Il PDF si costruisce con lo stesso `scannedPDF` di `internal/manuals`: **non** importarlo dal pacchetto di test di `manuals` (non è esportabile fra pacchetti di test), ricopiare l'helper qui oppure — meglio — spostare `scannedPDF`/`buildPDF`/`tinyJPEG` in un file `backend/internal/manuals/testpdf.go` **esportato** (`manuals.NewScannedPDF()`, `manuals.NewTextPDF()`) e usarlo da entrambi. Documentare nel commento che serve ai test.

- [ ] **Step 3: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run Manual -v
```

Atteso: FAIL, le rotte non esistono (404 invece di 200).

- [ ] **Step 4: Implementare gli handler**

Creare `backend/internal/httpapi/manuals_handlers.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// transcriber restituisce il trascrittore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di translator() in translate.go, e per la stessa ragione:
// cambiare modello non deve richiedere un riavvio.
func (s *Server) transcriber(ctx context.Context) ai.Transcriber {
	if s.Vision != nil {
		return s.Vision
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("manuals: could not load settings: %v", err)
		return ai.NewHTTPClientWithVision("", "", "", "")
	}
	return ai.NewHTTPClientWithVision(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel, cfg.AIVisionModel)
}

// manualTarget risolve i tre parametri di rotta in un manuale concreto, e
// verifica che il media appartenga davvero a quel gioco e a quella lingua:
// senza il controllo, l'id di un media di un altro gioco passerebbe.
func (s *Server) manualTarget(r *http.Request) (gameID int64, mediaID int64, lang string, media games.GameMedia, err error) {
	gameID, err = parseIDParam(r, "id")
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("invalid game id")
	}
	mediaID, err = parseIDParam(r, "mediaId")
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("invalid media id")
	}
	lang = strings.ToLower(strings.TrimSpace(chi.URLParam(r, "lang")))
	if lang == "" {
		return 0, 0, "", games.GameMedia{}, errors.New("language code is required")
	}

	gl, err := s.Games.GetLanguage(r.Context(), gameID, lang)
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("language not found")
	}
	list, err := s.Games.ListMedia(r.Context(), gl.ID)
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("could not load media")
	}
	for _, m := range list {
		if m.ID == mediaID {
			return gameID, mediaID, lang, m, nil
		}
	}
	return 0, 0, "", games.GameMedia{}, errors.New("media not found for this game and language")
}

type manualPageResponse struct {
	PageNumber int    `json:"pageNumber"`
	Text       string `json:"text"`
	Heading    string `json:"heading"`
	Source     string `json:"source"`
}

// extractManualHandler propone il testo di un manuale senza salvarlo.
// Non salva di proposito: l'admin conferma sempre, come già per
// l'arricchimento BGG.
func (s *Server) extractManualHandler(w http.ResponseWriter, r *http.Request) {
	_, _, _, media, err := s.manualTarget(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if media.Type != "file" || !strings.HasSuffix(strings.ToLower(media.URLOrPath), ".pdf") {
		writeError(w, http.StatusConflict, "questo media non è un manuale PDF")
		return
	}

	raw, err := s.Storage.Read(media.URLOrPath)
	if err != nil {
		log.Printf("manuals: read %s: %v", media.URLOrPath, err)
		writeError(w, http.StatusNotFound, "il file del manuale non è più sul disco")
		return
	}

	if manuals.HasTextLayer(raw) {
		pages, err := manuals.ExtractText(raw)
		if err != nil {
			log.Printf("manuals: extract text: %v", err)
			writeError(w, http.StatusUnprocessableEntity,
				"non è stato possibile leggere il testo di questo PDF: puoi scrivere le pagine a mano")
			return
		}
		out := make([]manualPageResponse, 0, len(pages))
		for _, p := range pages {
			out = append(out, manualPageResponse{
				PageNumber: p.Number, Text: p.Text,
				Heading: manuals.DetectHeading(p.Text), Source: "pdf_text",
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"source": "pdf_text", "pages": out})
		return
	}

	images, err := manuals.ExtractPageImages(raw)
	if err != nil {
		log.Printf("manuals: extract images: %v", err)
		writeError(w, http.StatusUnprocessableEntity,
			"non è stato possibile leggere le pagine di questo PDF: puoi scrivere il testo a mano")
		return
	}
	if len(images) == 0 {
		writeError(w, http.StatusUnprocessableEntity,
			"questo PDF non ha né testo né pagine leggibili: puoi scrivere il testo a mano")
		return
	}

	// Una richiesta per pagina, non tutte insieme: se la pagina 3 fallisce
	// non si perdono le altre, e l'admin riprova solo quella.
	vision := s.transcriber(r.Context())
	out := make([]manualPageResponse, 0, len(images))
	for _, img := range images {
		text, err := vision.Transcribe(r.Context(), img.JPEG, img.Number)
		if err != nil {
			// Senza modello vision, o con un guasto del provider, la pagina
			// esce vuota: l'anteprima si apre comunque e si riempie a mano.
			if !errors.Is(err, ai.ErrNotConfigured) {
				log.Printf("manuals: transcribe page %d: %v", img.Number, err)
			}
			out = append(out, manualPageResponse{PageNumber: img.Number, Source: "manual"})
			continue
		}
		out = append(out, manualPageResponse{
			PageNumber: img.Number, Text: text,
			Heading: manuals.DetectHeading(text), Source: "vision",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "vision", "pages": out})
}

func (s *Server) listManualPagesHandler(w http.ResponseWriter, r *http.Request) {
	_, mediaID, _, _, err := s.manualTarget(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pages, err := s.Manuals.ListPages(r.Context(), mediaID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load manual pages")
		return
	}
	out := make([]manualPageResponse, 0, len(pages))
	for _, p := range pages {
		out = append(out, manualPageResponse{
			PageNumber: p.PageNumber, Text: p.Text, Heading: p.Heading, Source: p.Source,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": out})
}

type putManualPagesRequest struct {
	Pages []struct {
		PageNumber int    `json:"pageNumber"`
		Text       string `json:"text"`
		Source     string `json:"source"`
	} `json:"pages"`
}

func (s *Server) putManualPagesHandler(w http.ResponseWriter, r *http.Request) {
	gameID, mediaID, lang, _, err := s.manualTarget(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body putManualPagesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	pages := make([]manuals.StoredPage, 0, len(body.Pages))
	for _, p := range body.Pages {
		if p.PageNumber < 1 {
			writeError(w, http.StatusBadRequest, "il numero di pagina parte da 1")
			return
		}
		source := p.Source
		switch source {
		case "pdf_text", "vision", "manual":
		default:
			// Una pagina scritta o corretta a mano è "manual": è il default
			// più onesto quando il client non lo dice.
			source = "manual"
		}
		pages = append(pages, manuals.StoredPage{
			PageNumber: p.PageNumber, Text: p.Text, Source: source,
		})
	}

	if err := s.Manuals.ReplacePages(r.Context(), gameID, mediaID, lang, pages); err != nil {
		log.Printf("manuals: replace pages of media %d: %v", mediaID, err)
		writeError(w, http.StatusInternalServerError, "could not save manual pages")
		return
	}
	s.listManualPagesHandler(w, r)
}

func (s *Server) deleteManualPagesHandler(w http.ResponseWriter, r *http.Request) {
	_, mediaID, _, _, err := s.manualTarget(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Manuals.DeletePages(r.Context(), mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete manual pages")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

**Verificare** che `storage.Store` abbia un metodo per leggere un file salvato. `grep -n "func (s \*Store)" backend/internal/storage/store.go`: se non c'è un `Read(filename string) ([]byte, error)`, aggiungerlo in questo task — è tre righe (`os.ReadFile(filepath.Join(s.baseDir, filename))`) più la validazione che `filename` non contenga separatori di percorso, per non farne un modo di leggere file arbitrari.

- [ ] **Step 5: Montare le rotte**

In `backend/internal/httpapi/router.go`, nel blocco `protected`, accanto alle rotte media esistenti:

```go
		protected.Post("/api/games/{id}/languages/{lang}/media/{mediaId}/extract", s.extractManualHandler)
		protected.Get("/api/games/{id}/languages/{lang}/media/{mediaId}/pages", s.listManualPagesHandler)
		protected.Put("/api/games/{id}/languages/{lang}/media/{mediaId}/pages", s.putManualPagesHandler)
		protected.Delete("/api/games/{id}/languages/{lang}/media/{mediaId}/pages", s.deleteManualPagesHandler)
```

Aggiungere l'import di `boardgames-manager/internal/manuals`.

- [ ] **Step 6: Eseguire i test e verificare che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -v
```

Atteso: PASS, compresi tutti i test preesistenti.

- [ ] **Step 7: Verificare che `cmd/server` monti lo store**

`grep -n "httpapi.Server{" backend/cmd/server/main.go` e aggiungere `Manuals: manuals.NewStore(conn),`. Senza questo il binario va in panic al primo uso della chat, e nessun test lo coglierebbe.

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add backend/internal/httpapi/ backend/internal/manuals/ backend/internal/ai/ backend/cmd/server/main.go
git commit -m "feat: add admin endpoints to prepare a manual for questions"
```

---

### Task 9: L'endpoint pubblico e il flag `canAsk`

**Files:**
- Create: `backend/internal/httpapi/ask_handler.go`
- Modify: `backend/internal/httpapi/games_responses.go`, `backend/internal/httpapi/router.go`
- Test: `backend/internal/httpapi/ask_handler_test.go`

**Interfaces:**
- Consumes: `manuals.Store.Corpus/Search/HasPages`, `manuals.FormatCorpus/FormatIndex/FormatSearchResult` (Task 5, 7), `HTTPClient.Ask` (Task 7).
- Produces: `POST /api/games/{id}/ask`; `canAsk` in `toGameDetail`; `Server.Asker ai.Asker`.

- [ ] **Step 1: Dichiarare l'interfaccia `Asker`**

In `backend/internal/ai/ask.go`:

```go
// Asker è l'astrazione che serve all'handler pubblico. HTTPClient la
// implementa; i test iniettano un finto.
type Asker interface {
	Ask(ctx context.Context, req AskRequest) (string, error)
}
```

In `router.go`, dentro `Server`:

```go
	// Asker, quando è valorizzato, è l'agente da usare per le domande sui
	// manuali. Nil = costruito per richiesta dalle impostazioni.
	Asker ai.Asker
```

- [ ] **Step 2: Scrivere i test che falliscono**

Creare `backend/internal/httpapi/ask_handler_test.go`:

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
)

// fakeAsker cattura la AskRequest che l'handler costruisce: è lì che si
// verifica che il corpus, l'indice e la ricerca siano stati agganciati.
type fakeAsker struct {
	got    ai.AskRequest
	answer string
	err    error
}

func (f *fakeAsker) Ask(ctx context.Context, req ai.AskRequest) (string, error) {
	f.got = req
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

// seedGameWithPreparedManual crea un gioco con un manuale già indicizzato.
func seedGameWithPreparedManual(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	ctx := context.Background()
	g, err := conn.ExecContext(ctx, `INSERT INTO games (name, seats) VALUES ('Wingspan', 1)`)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := g.LastInsertId()
	l, _ := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'it', 1, 'Wingspan')`, gameID)
	langID, _ := l.LastInsertId()
	m, _ := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'manuale.pdf', 'Regolamento base')`, langID)
	mediaID, _ := m.LastInsertId()

	store := manuals.NewStore(conn)
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 4, Heading: "Fase di Upkeep", Source: "vision",
			Text: "Fase di Upkeep\nOgni giocatore paga una moneta per ciascun edificio."},
		{PageNumber: 7, Heading: "Fine partita", Source: "vision",
			Text: "Fine partita\nLa partita termina quando la pila di pesca si esaurisce."},
	}); err != nil {
		t.Fatalf("replace pages: %v", err)
	}
	return gameID
}

func postAsk(t *testing.T, router http.Handler, gameID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/games/"+strconv.FormatInt(gameID, 10)+"/ask", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAskHandler_AnswersInTheShapeDeepChatExpects(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "La partita finisce subito.\n\n_Regolamento base — pag. 7_"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID,
		`{"messages":[{"role":"user","text":"finite le carte che si fa?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	// deep-chat legge {"text": ...}: qualunque altra forma gli fa mostrare
	// un errore generico invece della risposta.
	var body struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if !strings.Contains(body.Text, "pag. 7") {
		t.Fatalf("il campo text non contiene la risposta: %q", body.Text)
	}

	// La AskRequest deve portare il nome del gioco, lo storico, e il
	// corpus con il suo indice.
	if asker.got.GameName != "Wingspan" {
		t.Fatalf("nome gioco: %q", asker.got.GameName)
	}
	if len(asker.got.Turns) != 1 || asker.got.Turns[0].Text != "finite le carte che si fa?" {
		t.Fatalf("storico non passato: %+v", asker.got.Turns)
	}
	if asker.got.CorpusChars == 0 {
		t.Fatal("CorpusChars a zero: l'handler non ha caricato il corpus")
	}
	if !strings.Contains(asker.got.CorpusIndex, "Fase di Upkeep p.4") {
		t.Fatalf("indice non passato: %q", asker.got.CorpusIndex)
	}
	if asker.got.Search == nil {
		t.Fatal("Search non agganciata: con un manuale lungo il modello non avrebbe come cercare")
	}
}

func TestAskHandler_TurnsPageCitationsIntoLinksToThePDF(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "La partita finisce subito. Regolamento base, pag. 7."}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	// È il pezzo che rende verificabile la risposta: si apre il manuale a
	// quella pagina invece di fidarsi.
	if !strings.Contains(body.Text, "[pag. 7](/api/uploads/manuale.pdf#page=7)") {
		t.Fatalf("la citazione non è diventata un link al PDF: %q", body.Text)
	}
}

func TestAskHandler_DoesNotDoubleLinkACitation(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "Vedi [pag. 7](/api/uploads/manuale.pdf#page=7)."}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if strings.Count(body.Text, "/api/uploads/") != 1 {
		t.Fatalf("un link già presente non va riscritto: %q", body.Text)
	}
}

func TestAskHandler_MapsDeepChatRolesToTheModel(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// deep-chat usa "ai" per le sue risposte, non "assistant".
	postAsk(t, router, gameID, `{"messages":[
	  {"role":"user","text":"come si contano i punti?"},
	  {"role":"ai","text":"Ogni edificio vale i punti stampati."},
	  {"role":"user","text":"e se siamo pari?"}
	]}`)

	if len(asker.got.Turns) != 3 {
		t.Fatalf("attesi 3 turni, ottenuti %d", len(asker.got.Turns))
	}
	if asker.got.Turns[1].Role != "assistant" {
		t.Fatalf("il ruolo 'ai' di deep-chat va tradotto in 'assistant', è %q", asker.got.Turns[1].Role)
	}
}

func TestAskHandler_SearchClosureIsScopedToTheGame(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// Un secondo gioco con la stessa parola chiave: la closure passata
	// all'agente non deve poterlo raggiungere. Il modello non ha il
	// game_id fra i parametri del tool proprio per questo.
	other := seedGameWithPreparedManual(t, conn)
	_ = other

	postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if asker.got.Search == nil {
		t.Fatal("Search non agganciata")
	}
	out, err := asker.got.Search(context.Background(), []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(out, "pag. 4") {
		t.Fatalf("la ricerca non trova la pagina del gioco chiesto:\n%s", out)
	}
	// Due giochi identici: se sconfinasse, lo stesso chunk uscirebbe due
	// volte e il payload avrebbe due voci con la stessa pagina.
	if strings.Count(out, "pag. 4") > 1 {
		t.Fatalf("la ricerca sconfina su un altro gioco:\n%s", out)
	}
}

func TestAskHandler_WithoutAProviderIs404(t *testing.T) {
	// Nessun provider configurato: la rotta si comporta come inesistente.
	// È la stessa degradazione di SMTP — nessun errore da spiegare, la
	// funzione semplicemente non c'è.
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{err: ai.ErrNotConfigured}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("atteso 404, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAskHandler_WithoutAPreparedManualIs404(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)

	// Gioco senza manuale indicizzato.
	g, _ := conn.Exec(`INSERT INTO games (name, seats) VALUES ('Senza manuale', 1)`)
	gameID, _ := g.LastInsertId()

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("atteso 404, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAskHandler_RejectsAnEmptyQuestion(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	for _, body := range []string{`{"messages":[]}`, `{"messages":[{"role":"user","text":"   "}]}`} {
		rec := postAsk(t, router, gameID, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: atteso 400, ottenuto %d", body, rec.Code)
		}
	}
}

func TestAskHandler_IsRateLimited(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	limited := false
	// Il limite è 20/minuto: oltre la trentesima deve scattare.
	for i := 0; i < 30; i++ {
		rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("l'endpoint pubblico deve avere un rate limit: una domanda costa una chiamata a pagamento")
	}
}

func TestGameDetail_ExposesCanAsk(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)

	// Senza provider e senza manuale: falso.
	g, _ := conn.Exec(`INSERT INTO games (name, seats) VALUES ('Nudo', 1)`)
	bareID, _ := g.LastInsertId()
	if canAsk(t, router, bareID) {
		t.Fatal("un gioco senza manuale e senza AI non può ricevere domande")
	}

	// Con manuale ma senza provider: ancora falso.
	preparedID := seedGameWithPreparedManual(t, conn)
	if canAsk(t, router, preparedID) {
		t.Fatal("senza provider AI configurato canAsk deve essere falso")
	}

	// Con provider e con manuale: vero.
	server.Asker = &fakeAsker{answer: "ok"}
	if !canAsk(t, router, preparedID) {
		t.Fatal("con provider e manuale preparato canAsk deve essere vero")
	}

	// Con provider ma senza manuale: falso.
	if canAsk(t, router, bareID) {
		t.Fatal("senza manuale preparato canAsk deve essere falso anche con l'AI attiva")
	}
}

func canAsk(t *testing.T, router http.Handler, gameID int64) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/games/"+strconv.FormatInt(gameID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET game %d: %d %s", gameID, rec.Code, rec.Body.String())
	}
	var body struct {
		CanAsk bool `json:"canAsk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	return body.CanAsk
}
```

Aggiungere `"database/sql"` agli import del test.

- [ ] **Step 3: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'Ask|CanAsk' -v
```

Atteso: FAIL.

- [ ] **Step 4: Implementare l'handler**

Creare `backend/internal/httpapi/ask_handler.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// asker restituisce l'agente per questa richiesta: quello iniettato se c'è,
// altrimenti uno costruito dalle impostazioni. Stesso schema di
// translator() e transcriber().
func (s *Server) asker(ctx context.Context) ai.Asker {
	if s.Asker != nil {
		return s.Asker
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("ask: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// aiConfigured dice se un provider è impostato, senza fare richieste. Serve
// a canAsk: la scheda gioco deve sapere se mostrare la chat prima che
// qualcuno faccia una domanda.
func (s *Server) aiConfigured(ctx context.Context) bool {
	if s.Asker != nil {
		return true
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return false
	}
	return cfg.AIBaseURL != "" && cfg.AIAPIKey != "" && cfg.AIModel != ""
}

// askRequestBody è la forma che manda deep-chat: la conversazione intera,
// tagliata dal componente a requestBodyLimits.maxMessages.
type askHTTPRequest struct {
	Messages []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	} `json:"messages"`
}

func (s *Server) askHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}

	var body askHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	turns := make([]ai.Turn, 0, len(body.Messages))
	for _, m := range body.Messages {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		// deep-chat chiama "ai" quel che il formato OpenAI chiama
		// "assistant": la traduzione va fatta qui, una volta.
		role := "user"
		if m.Role == "ai" || m.Role == "assistant" {
			role = "assistant"
		}
		turns = append(turns, ai.Turn{Role: role, Text: text})
	}
	if len(turns) == 0 {
		writeError(w, http.StatusBadRequest, "serve una domanda")
		return
	}

	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}

	corpus, err := s.Manuals.Corpus(r.Context(), gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load the manual")
		return
	}
	// Nessun manuale preparato: la rotta si comporta come inesistente,
	// esattamente come senza provider AI. Non c'è nulla da spiegare a un
	// partecipante — la chat, in quel caso, non è nemmeno comparsa.
	if len(corpus.Manuals) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// La lingua preferita è quella base del gioco: è quella in cui la
	// scheda pubblica mostra tutto il resto.
	preferLang := "it"
	if langs, err := s.Games.ListLanguages(r.Context(), gameID); err == nil {
		for _, l := range langs {
			if l.IsBaseLanguage {
				preferLang = l.LanguageCode
				break
			}
		}
	}

	// La closure di ricerca è legata al gioco: il game_id NON è un
	// parametro del tool, così il modello non può leggere il manuale di un
	// altro gioco.
	search := func(ctx context.Context, keywords []string) (string, error) {
		res, err := s.Manuals.Search(ctx, gameID, preferLang, keywords)
		if err != nil {
			return "", err
		}
		return manuals.FormatSearchResult(res), nil
	}

	answer, err := s.asker(r.Context()).Ask(r.Context(), ai.AskRequest{
		GameName:    game.Name,
		Turns:       turns,
		CorpusChars: corpus.Chars,
		CorpusText:  manuals.FormatCorpus(corpus),
		CorpusIndex: manuals.FormatIndex(corpus),
		Search:      search,
	})
	if errors.Is(err, ai.ErrNotConfigured) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		log.Printf("ask about game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway,
			"Non riesco a rispondere in questo momento. Riprova, o guarda il manuale nella scheda del gioco.")
		return
	}

	// deep-chat legge {"text": ...}.
	writeJSON(w, http.StatusOK, map[string]any{"text": linkifyCitations(answer, corpus, preferLang)})
}

// citationRe trova le citazioni di pagina nella risposta del modello.
var citationRe = regexp.MustCompile(`pag\.\s*(\d+)`)

// linkifyCitations trasforma "pag. 7" in un link markdown al PDF aperto a
// quella pagina. È il pezzo che chiude il cerchio: una risposta generata
// non va creduta sulla fiducia, si apre il manuale e si verifica — e in una
// discussione sulle regole è la differenza fra un aiuto e un oracolo.
//
// La riscrittura è nostra e non del modello: chiedere a un modello
// economico di costruire URL corretti è un modo affidabile di ottenere URL
// sbagliati. Il fragment #page=N è onorato dalla quasi totalità dei viewer.
func linkifyCitations(answer string, corpus manuals.Corpus, preferLang string) string {
	// Se il modello ha già prodotto un link, non si raddoppia.
	if strings.Contains(answer, "](/api/uploads/") {
		return answer
	}

	// Con più manuali si linka quello nella lingua preferita, che è la
	// stessa in cui la ricerca ha dato la precedenza ai risultati.
	path := ""
	for _, m := range corpus.Manuals {
		if !strings.HasSuffix(strings.ToLower(m.Path), ".pdf") {
			continue
		}
		if path == "" || m.LanguageCode == preferLang {
			path = m.Path
		}
		if m.LanguageCode == preferLang {
			break
		}
	}
	if path == "" {
		return answer
	}

	return citationRe.ReplaceAllStringFunc(answer, func(match string) string {
		page := citationRe.FindStringSubmatch(match)[1]
		return fmt.Sprintf("[%s](/api/uploads/%s#page=%s)", match, path, page)
	})
}
```

Aggiungere `"fmt"` e `"regexp"` agli import di `ask_handler.go`.

Nota su `CorpusText`: viene formattato anche quando il manuale è lungo e finirà sul percorso tool, dove non serve. Su un manuale da 40 pagine è ~100 KB di stringa costruita e buttata via a ogni domanda. **Se il profilo lo mostra**, spostare la formattazione dentro un `if corpus.Chars <= ai.InlineCorpusMaxChars`. Non farlo preventivamente: la chiarezza vale più di 100 KB di allocazione su una richiesta che fa comunque una chiamata di rete da secondi.

- [ ] **Step 5: Aggiungere `canAsk` alla risposta della scheda**

In `backend/internal/httpapi/games_responses.go`, in `toGameDetail`, prima del `return`:

```go
	// canAsk governa la comparsa della chat sulla scheda pubblica. Vero solo
	// se entrambe le condizioni valgono: provider AI configurato e manuale
	// preparato. In UI non esiste il pulsante disabilitato con la
	// spiegazione: se è falso, la chat non c'è.
	hasManual := false
	if s.Manuals != nil {
		if has, err := s.Manuals.HasPages(ctx, g.ID); err == nil {
			hasManual = has
		}
	}
	detail["canAsk"] = hasManual && s.aiConfigured(ctx)
```

`toGameSummary` **non** va toccato: l'elenco dei giochi non mostra la chat, e aggiungere il flag lì costerebbe una query per riga.

- [ ] **Step 6: Montare la rotta col rate limit**

In `router.go`, accanto agli altri limiter:

```go
	// Una domanda costa una o più chiamate a pagamento al provider. Il
	// limiter chiave su r.RemoteAddr, che dietro NAT è l'IP del wifi del
	// circolo: tutti a un tavolo condividono lo stesso bucket. Venti al
	// minuto stanno molto sopra l'uso reale — nessuno fa venti domande di
	// regole in un minuto — e limitano l'abuso.
	askLimiter := newRateLimiter(20, time.Minute)
```

e fra le rotte pubbliche, accanto a `r.Get("/api/games/{id}", ...)`:

```go
	r.With(askLimiter.middleware).Post("/api/games/{id}/ask", s.askHandler)
```

- [ ] **Step 7: Eseguire la suite intera**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: tutti i pacchetti PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/httpapi/ backend/internal/ai/ask.go
git commit -m "feat: answer manual questions on a public rate-limited endpoint"
```

---

### Task 10: La chat sulla scheda gioco — struttura, senza deep-chat

Questo task costruisce le due forme (sidebar e dialog) e lo **stato di riposo** con le domande suggerite, **senza ancora installare deep-chat**. Motivo: la struttura è verificabile da sola, e lo stato di riposo è ciò che permette alla sidebar di stare aperta per default senza costare i 105 KB.

**Files:**
- Create: `frontend/src/components/ManualChat.vue`, `frontend/src/components/ManualChatPanel.vue`
- Modify: `frontend/src/utils/game.ts`, `frontend/src/views/GameDetailView.vue`, `frontend/src/app.css`

**Interfaces:**
- Consumes: `GameDetail.canAsk` dall'API (Task 9).
- Produces:
  - `ManualChat.vue` — props `{ gameId: number; gameName: string }`.
  - `ManualChatPanel.vue` — props `{ gameId: number; gameName: string; headings: string[] }`, emit `close`.

- [ ] **Step 1: Aggiungere `canAsk` al tipo**

In `frontend/src/utils/game.ts`, dentro `GameDetail`, dopo `canTranslate`:

```ts
  /** Vero quando il gioco ha un manuale indicizzato e il provider AI è configurato. */
  canAsk: boolean
```

- [ ] **Step 2: Scrivere `ManualChatPanel.vue`**

```vue
<script setup lang="ts">
import { computed, defineAsyncComponent, ref } from 'vue'

/**
 * Il contenuto della chat, indipendente da dove vive: la sidebar su
 * desktop e il dialog a tutto schermo su mobile montano questo.
 *
 * Nello stato di riposo il markup è nostro: titolo, tre domande suggerite e
 * un finto campo di input. deep-chat si monta al primo gesto — clic sul
 * campo o su una domanda — perché pesa 105 KB gzip, più dell'intera app, e
 * chi apre una scheda gioco per leggerla non deve pagarli. È questo che
 * permette alla sidebar di stare aperta per default.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  /** I titoli di sezione del manuale, per le domande suggerite. */
  headings: string[]
}>()

const started = ref(false)
const pendingQuestion = ref('')

// Le domande suggerite: dai titoli del manuale quando ce ne sono almeno
// tre, altrimenti tre domande fisse. Una chat vuota su un telefono non
// suggerisce cosa farne.
const fallbackQuestions = [
  'Come finisce la partita?',
  'In quanti si gioca?',
  'Come si contano i punti?',
]

/** Rende un titolo di sezione come domanda. Tabella fissa, niente magia. */
const headingToQuestion: Record<string, string> = {
  'fine partita': 'Come finisce la partita?',
  'fine della partita': 'Come finisce la partita?',
  preparazione: 'Come si prepara il gioco?',
  punteggio: 'Come si contano i punti?',
  conteggio: 'Come si contano i punti?',
  turno: 'Cosa posso fare nel mio turno?',
  'turno del giocatore': 'Cosa posso fare nel mio turno?',
}

const suggestions = computed<string[]>(() => {
  const fromHeadings = props.headings
    .map((h) => headingToQuestion[h.trim().toLowerCase()] ?? `Cosa dice il manuale su "${h}"?`)
    .filter((q, i, all) => all.indexOf(q) === i)
  return fromHeadings.length >= 3 ? fromHeadings.slice(0, 3) : fallbackQuestions
})

function start(question = '') {
  pendingQuestion.value = question
  started.value = true
}
</script>

<template>
  <div class="manual-chat-panel">
    <p class="manual-chat-note">
      Risposte generate dal manuale: controlla sempre la pagina citata.
    </p>

    <div v-if="!started" class="manual-chat-rest">
      <p class="manual-chat-intro">
        Chiedi una regola di <strong>{{ gameName }}</strong> a parole tue.
      </p>
      <ul class="manual-chat-suggestions">
        <li v-for="q in suggestions" :key="q">
          <button type="button" @click="start(q)">{{ q }}</button>
        </li>
      </ul>
      <button type="button" class="manual-chat-fakeinput" @click="start()">
        Chiedi una regola…
      </button>
    </div>

    <!-- Il componente vero arriva nel task successivo. -->
    <p v-else class="manual-chat-placeholder">
      Chat in arrivo (domanda: {{ pendingQuestion || '—' }})
    </p>
  </div>
</template>
```

- [ ] **Step 3: Scrivere `ManualChat.vue`**

```vue
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ManualChatPanel from './ManualChatPanel.vue'

/**
 * Decide DOVE vive la chat, non cos'è.
 *
 * Da 1100px in su è una sidebar destra collassabile, sticky, col suo
 * scroll: la colonna di sinistra scorre e la chat resta ferma. La soglia è
 * 1100 e non 900 perché `.app-page` è larga 56rem: una sidebar da 22rem
 * dentro quello spazio lascerebbe al testo 34rem, sotto la misura
 * leggibile.
 *
 * Sotto 1100px è un bottone tondo in basso al centro che apre un <dialog>
 * nativo a tutto schermo. Il <dialog> regala focus trap, Esc e sfondo
 * inerte: lo stesso motivo per cui ModalDialog.vue lo usa.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  headings: string[]
}>()

const SIDEBAR_MIN_WIDTH = '(min-width: 1100px)'
const COLLAPSED_KEY = 'manual-chat-collapsed'

const route = useRoute()
const wide = ref(false)
const collapsed = ref(false)
const dialogOpen = ref(false)
const dialog = ref<HTMLDialogElement | null>(null)

let media: MediaQueryList | null = null
function syncWide(e: MediaQueryList | MediaQueryListEvent) {
  wide.value = e.matches
}

onMounted(() => {
  media = window.matchMedia(SIDEBAR_MIN_WIDTH)
  syncWide(media)
  media.addEventListener('change', syncWide)

  // Lo stato collassato è una preferenza dell'utente, non del gioco.
  try {
    collapsed.value = window.localStorage.getItem(COLLAPSED_KEY) === '1'
  } catch {
    // Finestra privata o storage bloccato: si parte aperta, che è il
    // default giusto per la scoperta.
  }

  // ?chat=1 arriva dai rimandi della pagina prenotazione e della scheda
  // evento: apre la chat senza far cercare all'utente dove sia.
  if (route.query.chat === '1') {
    collapsed.value = false
    if (!wide.value) {
      openDialog()
    }
  }
})

onBeforeUnmount(() => media?.removeEventListener('change', syncWide))

watch(collapsed, (value) => {
  try {
    window.localStorage.setItem(COLLAPSED_KEY, value ? '1' : '0')
  } catch {
    // Niente da fare: la preferenza vale solo per questa visita.
  }
})

function openDialog() {
  dialogOpen.value = true
  // showModal va chiamato dopo che il <dialog> è nel DOM.
  requestAnimationFrame(() => dialog.value?.showModal())
}

function closeDialog() {
  dialogOpen.value = false
  if (dialog.value?.open) {
    dialog.value.close()
  }
}
</script>

<template>
  <!-- Desktop: sidebar sticky, collassabile in una barra verticale. -->
  <aside v-if="wide" class="manual-chat-aside" :class="{ 'is-collapsed': collapsed }">
    <button
      type="button"
      class="manual-chat-toggle"
      :aria-expanded="!collapsed"
      :aria-label="collapsed ? 'Apri le domande sul manuale' : 'Chiudi le domande sul manuale'"
      @click="collapsed = !collapsed"
    >
      <span class="manual-chat-toggle-label">Chiedi al manuale</span>
      <span class="manual-chat-toggle-icon" aria-hidden="true">{{ collapsed ? '‹' : '›' }}</span>
    </button>
    <!--
      v-show e non v-if: collassare non deve smontare il pannello,
      altrimenti la conversazione in corso si perde a ogni clic.
    -->
    <div v-show="!collapsed" class="manual-chat-aside-body">
      <ManualChatPanel :game-id="gameId" :game-name="gameName" :headings="headings" />
    </div>
  </aside>

  <!-- Mobile: bottone tondo sempre visibile, e dialog a tutto schermo. -->
  <template v-else>
    <button type="button" class="manual-chat-fab" @click="openDialog">
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          d="M20 12a8 8 0 1 1-3.2-6.4M20 4v4h-4"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
        />
        <circle cx="9" cy="12" r="1.1" fill="currentColor" />
        <circle cx="12.5" cy="12" r="1.1" fill="currentColor" />
        <circle cx="16" cy="12" r="1.1" fill="currentColor" />
      </svg>
      Chiedi al manuale
    </button>

    <dialog
      v-if="dialogOpen"
      ref="dialog"
      class="manual-chat-dialog"
      @close="dialogOpen = false"
      @cancel="dialogOpen = false"
    >
      <div class="manual-chat-dialog-head">
        <h2>Chiedi al manuale</h2>
        <button type="button" class="modal-close" aria-label="Chiudi" @click="closeDialog">
          <svg viewBox="0 0 24 24" fill="none">
            <path d="M6 6l12 12M18 6L6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </div>
      <ManualChatPanel
        :game-id="gameId"
        :game-name="gameName"
        :headings="headings"
        @close="closeDialog"
      />
    </dialog>
  </template>
</template>
```

Nota sull'icona del bottone: quel path è un segnaposto. In fase di `/impeccable` va sostituito con qualcosa coerente col tono "dado & pedina" di `DESIGN.md` — un dado con un punto di domanda, o un fumetto con i pips.

- [ ] **Step 4: Montarla in `GameDetailView.vue`**

Racchiudere il contenuto esistente in una griglia e aggiungere il componente. Nel `<script setup>`:

```ts
import ManualChat from '../components/ManualChat.vue'

// I titoli di sezione del manuale alimentano le domande suggerite. Non
// c'è un endpoint dedicato: si ricavano dai media, e se non ci sono il
// pannello usa le sue domande fisse.
const manualHeadings = ref<string[]>([])
```

Nel template, sostituire il `<div v-if="game">` esterno con:

```html
  <div v-if="game" class="game-detail-layout" :class="{ 'has-chat': game.canAsk }">
    <div class="game-detail-main">
      <!-- tutto il contenuto attuale, invariato -->
    </div>

    <ManualChat
      v-if="game.canAsk"
      :game-id="game.id"
      :game-name="game.name"
      :headings="manualHeadings"
    />
  </div>
```

- [ ] **Step 5: Aggiungere gli stili**

In `frontend/src/app.css`, in fondo. Non esistono token `--space-*` in questo progetto: le spaziature sono in `rem` dirette, come nel resto del foglio.

```css
/* ---------- Chiedi al manuale ---------- */

/* Una colonna sola per default: la sidebar entra solo dove c'è spazio
   davvero. 1100px = 56rem di colonna leggibile + 22rem di chat + i
   margini di .app-page. */
.game-detail-layout {
  display: block;
}

@media (min-width: 1100px) {
  .game-detail-layout.has-chat {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 22rem;
    gap: 1.5rem;
    align-items: start;
  }

  .game-detail-layout.has-chat .game-detail-main {
    min-width: 0;
  }

  /* Sticky sotto la topbar (3.25rem), con il suo scroll interno: la
     colonna di sinistra scorre e la chat resta ferma. */
  .manual-chat-aside {
    position: sticky;
    top: calc(3.25rem + 1rem);
    display: flex;
    flex-direction: column;
    max-height: calc(100dvh - 3.25rem - 3rem);
    overflow: hidden;
    background: var(--card);
    border: 1px solid var(--card-line);
    border-radius: var(--radius);
    box-shadow: var(--shadow-card);
  }

  .manual-chat-aside.is-collapsed {
    /* Collassata: una barra stretta col titolo ruotato. */
    align-self: start;
    max-height: none;
  }

  .game-detail-layout.has-chat:has(.manual-chat-aside.is-collapsed) {
    grid-template-columns: minmax(0, 1fr) 2.75rem;
  }

  .manual-chat-aside.is-collapsed .manual-chat-toggle-label {
    writing-mode: vertical-rl;
    padding: 0.75rem 0;
  }

  .manual-chat-aside-body {
    min-height: 0;
    overflow-y: auto;
  }
}

.manual-chat-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  width: 100%;
  padding: 0.7rem 0.85rem;
  font-family: 'Display', system-ui, sans-serif;
  font-size: 0.95rem;
  color: var(--felt-text);
  background: var(--felt);
  border: 0;
  cursor: pointer;
}

.manual-chat-panel {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  padding: 0.85rem;
}

.manual-chat-note {
  margin: 0 0 0.75rem;
  font-size: 0.8rem;
  color: var(--ink-muted);
}

.manual-chat-intro {
  margin: 0 0 0.75rem;
}

.manual-chat-suggestions {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin: 0 0 0.85rem;
  padding: 0;
  list-style: none;
}

.manual-chat-suggestions button {
  width: 100%;
  padding: 0.5rem 0.7rem;
  font: inherit;
  color: var(--ink);
  text-align: left;
  background: var(--card-alt);
  border: 1px solid var(--card-line);
  border-radius: var(--radius-sm);
  cursor: pointer;
}

/* Ha le misure del campo vero, così montare deep-chat non fa saltare il
   layout. */
.manual-chat-fakeinput {
  width: 100%;
  padding: 0.6rem 0.75rem;
  font: inherit;
  color: var(--ink-muted);
  text-align: left;
  background: var(--bg);
  border: 1px solid var(--card-line);
  border-radius: var(--radius-sm);
  cursor: text;
}

/* Bottone tondo, in basso al centro. env(safe-area-inset-bottom) lo tiene
   sopra la barra home dell'iPhone. */
.manual-chat-fab {
  position: fixed;
  bottom: calc(1rem + env(safe-area-inset-bottom));
  left: 50%;
  z-index: 25;
  display: flex;
  gap: 0.45rem;
  align-items: center;
  padding: 0.7rem 1.15rem;
  font-family: 'Display', system-ui, sans-serif;
  font-size: 0.95rem;
  color: var(--felt-text);
  background: var(--felt);
  border: 1px solid var(--felt-line);
  border-radius: 999px;
  box-shadow: var(--shadow-lift);
  transform: translateX(-50%);
  cursor: pointer;
}

.manual-chat-fab svg {
  width: 1.15rem;
  height: 1.15rem;
}

/* Il bottone non deve coprire l'ultimo contenuto della pagina. */
@media (max-width: 1099px) {
  .game-detail-layout.has-chat {
    padding-bottom: 4rem;
  }
}

/* Il dialog occupa il viewport: dvh e non vh, perché su iOS Safari vh
   include la barra degli indirizzi e taglia l'input fuori schermo. */
.manual-chat-dialog {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: none;
  height: 100dvh;
  max-height: 100dvh;
  padding: 0;
  padding-bottom: env(safe-area-inset-bottom);
  color: var(--ink);
  background: var(--card);
  border: 0;
}

.manual-chat-dialog-head {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 0.85rem;
  background: var(--felt);
  border-bottom: 1px solid var(--felt-line);
}

.manual-chat-dialog-head h2 {
  margin: 0;
  font-size: 1.05rem;
  color: var(--felt-text);
}

.manual-chat-dialog-head .modal-close {
  color: var(--felt-text);
}
```

- [ ] **Step 6: Verificare il build e il type-check**

```bash
cd frontend && npm run build
```

Atteso: build verde. `vue-tsc` deve accettare `canAsk` su `GameDetail`; se protesta su `route.query.chat`, la comparazione va fatta con `String(route.query.chat) === '1'`.

- [ ] **Step 7: Guardarla nel browser**

```bash
docker compose up -d --build
```

Poi su http://localhost:8080, con un gioco che ha un manuale preparato: verificare a finestra larga che la sidebar ci sia e che il collasso funzioni e sopravviva a un ricaricamento; a finestra stretta (< 1100px) che compaia il bottone tondo, che il dialog si apra a tutto schermo, e che **Esc lo chiuda** — è il comportamento nativo che non stiamo scrivendo a mano.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/ManualChat.vue frontend/src/components/ManualChatPanel.vue \
  frontend/src/utils/game.ts frontend/src/views/GameDetailView.vue frontend/src/app.css
git commit -m "feat: add the manual chat shell to the public game page"
```

---

### Task 11: Montare deep-chat al primo gesto

**Files:**
- Modify: `frontend/package.json` (dipendenza), `frontend/vite.config.ts`, `frontend/src/components/ManualChatPanel.vue`

**Interfaces:**
- Consumes: `POST /api/games/{id}/ask` (Task 9), lo scheletro di Task 10.
- Produces: la chat funzionante.

- [ ] **Step 1: Installare deep-chat**

```bash
cd frontend && npm install deep-chat@2.5.1
```

Deve aggiungere `deep-chat` e le sue tre dipendenze transitive (`@microsoft/fetch-event-source`, `remarkable`, `speech-to-element`). Se ne compaiono altre, fermarsi e segnalarlo.

- [ ] **Step 2: Dichiarare `<deep-chat>` come custom element**

`<deep-chat>` è un web component: senza questa configurazione Vue lo tratta come un componente sconosciuto e stampa un avviso a ogni render. In `frontend/vite.config.ts`:

```ts
export default defineConfig({
  plugins: [
    vue({
      // deep-chat è un web component: senza questo Vue prova a risolverlo
      // come componente Vue e avvisa che non lo conosce.
      template: {
        compilerOptions: {
          isCustomElement: (tag) => tag === 'deep-chat',
        },
      },
    }),
    keepEmbedPlaceholder(),
  ],
  // ...resto invariato
})
```

- [ ] **Step 3: Caricare e montare il componente**

Sostituire il `<script setup>` di `ManualChatPanel.vue` con questa versione, che aggiunge il caricamento dinamico. Il resto (suggerimenti, stato di riposo) resta identico.

```ts
import { computed, nextTick, ref } from 'vue'

const props = defineProps<{
  gameId: number
  gameName: string
  headings: string[]
}>()

const started = ref(false)
const failed = ref(false)
const chat = ref<HTMLElement | null>(null)

// ... suggestions, fallbackQuestions, headingToQuestion: invariati dal task 10

/**
 * deep-chat è un web component, non un componente Vue:
 * defineAsyncComponent non serve. L'import dinamico registra l'elemento
 * personalizzato, e da quel momento <deep-chat> nel template si comporta.
 *
 * Si carica al primo gesto e non al mount perché pesa 105 KB gzip, più
 * dell'intera app: chi apre la scheda gioco per leggerla non deve pagarli.
 */
let loading: Promise<unknown> | null = null
function loadDeepChat() {
  loading ??= import('deep-chat')
  return loading
}

async function start(question = '') {
  try {
    await loadDeepChat()
  } catch (e) {
    // Rete andata via a metà, o chunk non raggiungibile: senza questo il
    // pannello resterebbe fermo sul finto input senza dire niente.
    console.error('caricamento della chat', e)
    failed.value = true
    return
  }
  started.value = true
  // Il template deve aver reso <deep-chat> prima di poterlo pilotare.
  await nextTick()
  const el = chat.value as (HTMLElement & {
    submitUserMessage?: (m: { text: string }) => void
    focusInput?: () => void
  }) | null
  if (!el) {
    return
  }
  if (question) {
    el.submitUserMessage?.({ text: question })
  } else {
    el.focusInput?.()
  }
}

/**
 * Le proprietà si passano come stringhe JSON negli attributi, che è la via
 * documentata del componente: legare un oggetto a un custom element
 * dipende da quando l'elemento viene definito, e qui viene definito dopo
 * il primo render.
 */
const connect = computed(() =>
  JSON.stringify({ url: `/api/games/${props.gameId}/ask`, method: 'POST' }),
)

// maxMessages conta i MESSAGGI, non i turni: 20 turni sono 40 messaggi. E
// senza il campo il default manda solo l'ultimo input, quindi la
// conversazione non arriverebbe affatto al server.
const requestBodyLimits = JSON.stringify({ maxMessages: 40 })

// Nessuna chiave: webSpeech usa la Web Speech API del browser. it-IT va
// messo esplicito, il default è en-US. Richiede HTTPS (o localhost): in
// LAN su http il microfono non funziona, ed è documentato nel README.
const speechToText = JSON.stringify({ webSpeech: { language: 'it-IT' } })

const textInput = JSON.stringify({ placeholder: { text: 'Chiedi una regola…' } })

const errorMessages = JSON.stringify({
  overrides: {
    default: 'Qualcosa non ha funzionato. Riprova.',
    service: 'Non riesco a rispondere in questo momento. Guarda il manuale nella scheda del gioco.',
    speechToText: 'La dettatura non è disponibile su questo browser. Scrivi la domanda.',
  },
})

// deep-chat vive in shadow DOM: i token di app.css non ci cascano dentro,
// quindi i colori si passano dalle sue proprietà. Vedi la sezione in
// DESIGN.md.
const messageStyles = JSON.stringify({
  default: {
    shared: { bubble: { borderRadius: '10px', fontSize: '0.95rem', maxWidth: '92%' } },
    user: { bubble: { backgroundColor: '#1f4d3a', color: '#f4efe1' } },
    ai: { bubble: { backgroundColor: '#f2ead4', color: '#241f18' } },
  },
})

const auxiliaryStyle = `
  ::-webkit-scrollbar { width: 8px; }
  ::-webkit-scrollbar-thumb { background-color: #ddd0ab; border-radius: 4px; }
`
```

E nel template, sostituire il segnaposto:

```html
    <p v-else-if="failed" class="error">
      La chat non si è caricata. Ricarica la pagina, oppure apri il manuale
      dalla scheda del gioco.
    </p>

    <deep-chat
      v-else
      ref="chat"
      class="manual-chat-widget"
      :connect="connect"
      :requestBodyLimits="requestBodyLimits"
      :speechToText="speechToText"
      :textInput="textInput"
      :errorMessages="errorMessages"
      :messageStyles="messageStyles"
      :auxiliaryStyle="auxiliaryStyle"
    />
```

Aggiungere in `app.css`:

```css
/* deep-chat dimensiona sé stesso da attributi: qui gli si dice solo di
   riempire il contenitore, che è la sidebar o il dialog. */
.manual-chat-widget {
  flex: 1;
  width: 100%;
  min-height: 18rem;
  border: 0;
  background-color: transparent;
}
```

- [ ] **Step 4: Verificare il build**

```bash
cd frontend && npm run build
```

Atteso: verde. Nell'output di Vite deve comparire un **chunk separato** per deep-chat, non un `index.js` gonfiato: è il segno che il lazy-load funziona. Verificarlo:

```bash
ls -la ../backend/internal/webui/dist/assets/*.js
```

Il chunk di deep-chat deve essere un file a sé, e `index-*.js` deve restare intorno ai 221 KB di prima. **Se `index.js` è cresciuto di ~700 KB**, l'import dinamico è stato risolto staticamente: controllare che non ci sia nessun `import 'deep-chat'` in testa al file.

- [ ] **Step 5: Provarla davvero**

```bash
cd .. && docker compose up -d --build
```

Su http://localhost:8080, con un provider AI configurato nelle impostazioni e un gioco con manuale preparato:

1. Aprire la scheda gioco a finestra larga: la sidebar mostra lo stato di riposo, e nel pannello Network **non** c'è il chunk di deep-chat.
2. Cliccare una domanda suggerita: il chunk si scarica, la chat si monta, la domanda parte da sola e arriva una risposta con la pagina citata.
3. Fare una domanda di seguito («e se siamo in tre?») e verificare nel Network che il body della richiesta contenga **tutti** i messaggi precedenti, non solo l'ultimo.
4. Restringere la finestra sotto 1100px: compare il bottone tondo, il dialog si apre a tutto schermo, Esc lo chiude.
5. Con una domanda su qualcosa che il manuale non dice, verificare che risponda «il manuale non lo dice» invece di inventare. È la proprietà che conta di più: al tavolo una regola inventata fa danno.

Annotare l'esito del punto 5 nel messaggio di commit: è la prima misura reale della qualità delle risposte.

- [ ] **Step 6: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/vite.config.ts \
  frontend/src/components/ManualChatPanel.vue frontend/src/app.css
git commit -m "feat: wire deep-chat into the manual chat, loaded on first use"
```

---

### Task 12: Pannello admin — prepara, rivedi, salva

**Files:**
- Create: `frontend/src/components/ManualPrepPanel.vue`
- Modify: `frontend/src/views/GameAdminDetailView.vue`, `frontend/src/views/SettingsView.vue`, `frontend/src/app.css`

**Interfaces:**
- Consumes: le quattro rotte admin (Task 8), `ai_vision_model` (Task 1).
- Produces: `ManualPrepPanel.vue` — props `{ gameId: number; lang: string; mediaId: number; mediaTitle: string }`.

- [ ] **Step 1: Il campo nelle impostazioni**

Leggere `frontend/src/views/SettingsView.vue` e aggiungere `aiVisionModel` accanto ad `aiModel`, con questa etichetta e questo aiuto — il testo conta, perché è l'unico posto dove l'admin scopre che il modello di chat non basta:

```
Modello per i manuali scansionati (opzionale)
Serve un modello che sappia leggere le immagini. Il modello di chat qui
sopra spesso non ne è capace: per esempio deepseek-v4-flash è solo testo,
la sua variante deepseek-v4-flash-vision-exp legge anche le pagine.
Lasciandolo vuoto i manuali scansionati si trascrivono a mano.
```

- [ ] **Step 2: Scrivere `ManualPrepPanel.vue`**

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

/**
 * Prepara un manuale PDF per le domande: estrae o trascrive, mostra il
 * risultato pagina per pagina, e salva solo dopo la conferma.
 *
 * L'anteprima editabile non è una comodità: la trascrizione di uno scan
 * può uscire disordinata, e senza la possibilità di correggerla l'admin
 * resterebbe bloccato. Il testo salvato è la fonte di verità, non una
 * cache di quel che ha detto il modello.
 */
const props = defineProps<{
  gameId: number
  lang: string
  mediaId: number
  mediaTitle: string
}>()

interface ManualPage {
  pageNumber: number
  text: string
  heading: string
  source: string
}

const saved = ref<ManualPage[]>([])
const draft = ref<ManualPage[] | null>(null)
const source = ref('')
const busy = ref(false)
const error = ref('')
const notice = ref('')

const base = `/games/${props.gameId}/languages/${props.lang}/media/${props.mediaId}`

async function loadSaved() {
  try {
    const res = await api.get<{ pages: ManualPage[] }>(`${base}/pages`)
    saved.value = res.pages || []
  } catch (e) {
    console.error('caricamento pagine manuale', e)
  }
}

onMounted(loadSaved)

async function prepare() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await api.post<{ source: string; pages: ManualPage[] }>(`${base}/extract`)
    source.value = res.source
    draft.value = res.pages
    const empty = res.pages.filter((p) => !p.text.trim()).length
    if (empty > 0) {
      notice.value =
        `${empty} pagine su ${res.pages.length} sono uscite vuote: ` +
        'scrivile a mano, oppure controlla il modello per i manuali scansionati nelle impostazioni.'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Preparazione non riuscita.'
  } finally {
    busy.value = false
  }
}

async function save() {
  if (!draft.value) {
    return
  }
  busy.value = true
  error.value = ''
  try {
    const res = await api.put<{ pages: ManualPage[] }>(`${base}/pages`, {
      pages: draft.value.map((p) => ({
        pageNumber: p.pageNumber,
        text: p.text,
        // Una pagina toccata a mano resta marcata come veniva: il campo
        // dice da dove arriva il testo, non chi l'ha rivisto.
        source: p.source,
      })),
    })
    saved.value = res.pages || []
    draft.value = null
    notice.value = 'Manuale indicizzato: le domande sulla scheda pubblica ora funzionano.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Salvataggio non riuscito.'
  } finally {
    busy.value = false
  }
}

async function remove() {
  busy.value = true
  error.value = ''
  try {
    await api.delete(`${base}/pages`)
    saved.value = []
    draft.value = null
    notice.value = 'Indice rimosso: la chat non compare più per questo gioco.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Rimozione non riuscita.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="manual-prep">
    <p class="manual-prep-state">
      <template v-if="saved.length">
        {{ saved.length }} pagine indicizzate.
      </template>
      <template v-else>Non preparato per le domande.</template>
    </p>

    <div class="manual-prep-actions">
      <button type="button" :disabled="busy" @click="prepare">
        {{ saved.length ? 'Prepara di nuovo' : 'Prepara per le domande' }}
      </button>
      <button v-if="saved.length" type="button" :disabled="busy" @click="remove">
        Rimuovi indice
      </button>
    </div>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="empty-note">{{ notice }}</p>

    <div v-if="draft" class="manual-prep-draft">
      <p class="row-meta">
        {{ source === 'vision' ? 'Trascritto dalle immagini delle pagine' : 'Estratto dal testo del PDF' }}.
        Correggi quel che serve, poi salva.
      </p>
      <div v-for="(page, i) in draft" :key="page.pageNumber" class="manual-prep-page">
        <label :for="`manual-page-${mediaId}-${i}`">Pagina {{ page.pageNumber }}</label>
        <textarea
          :id="`manual-page-${mediaId}-${i}`"
          v-model="draft[i].text"
          rows="8"
          spellcheck="false"
        ></textarea>
      </div>
      <div class="manual-prep-actions">
        <button type="button" :disabled="busy" @click="save">Salva e indicizza</button>
        <button type="button" :disabled="busy" @click="draft = null">Annulla</button>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 3: Montarlo in `GameAdminDetailView.vue`**

Leggere il pannello media del file e, per ogni media di tipo `file` il cui `url` finisce in `.pdf`, montare:

```html
<ManualPrepPanel
  v-if="media.type === 'file' && media.url.toLowerCase().endsWith('.pdf')"
  :game-id="game.id"
  :lang="activeLangCode"
  :media-id="media.id"
  :media-title="media.title || 'Manuale'"
/>
```

Se il pannello media è dentro `GameMediaList.vue` e quel componente è condiviso con la scheda pubblica, **non** metterlo lì: la preparazione è un'azione da admin, e `GameMediaList` è pubblico. In quel caso il pannello va in `GameAdminDetailView.vue`, accanto alla lista.

- [ ] **Step 4: Gli stili**

In `app.css`:

```css
/* ---------- Preparazione manuale (admin) ---------- */

.manual-prep {
  margin-top: 0.85rem;
  padding-top: 0.85rem;
  border-top: 1px solid var(--card-line);
}

.manual-prep-state {
  margin: 0 0 0.5rem;
  font-family: 'Data', ui-monospace, monospace;
  font-size: 0.85rem;
  color: var(--ink-muted);
}

.manual-prep-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin: 0.5rem 0;
}

.manual-prep-page {
  margin-bottom: 0.75rem;
}

.manual-prep-page label {
  display: block;
  margin-bottom: 0.2rem;
  font-family: 'Data', ui-monospace, monospace;
  font-size: 0.8rem;
  color: var(--ink-muted);
}

.manual-prep-page textarea {
  width: 100%;
  font-family: 'Data', ui-monospace, monospace;
  font-size: 0.85rem;
  line-height: 1.5;
}
```

Verificare che `textarea` non abbia già uno stile globale in conflitto: `grep -n "^textarea" frontend/src/app.css`.

- [ ] **Step 5: Verificare il build**

```bash
cd frontend && npm run build
```

- [ ] **Step 6: Provarlo sul manuale vero**

```bash
cd .. && docker compose up -d --build
```

Con un modello vision configurato, su http://localhost:8080/admin/games/:id: premere «Prepara per le domande» sul manuale scansionato reale e **leggere la trascrizione**. È la verifica che conta di tutto il percorso vision, e va fatta ora, non alla fine: se il modello economico restituisce testo disordinato lo si scopre qui, quando cambiare modello costa un campo nelle impostazioni.

Annotare nel commit quante pagine su quante sono uscite leggibili.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/ManualPrepPanel.vue frontend/src/views/GameAdminDetailView.vue \
  frontend/src/views/SettingsView.vue frontend/src/app.css
git commit -m "feat: let an admin prepare, review and index a manual"
```

---

### Task 13: Rimandi, documentazione e passata `/impeccable`

**Files:**
- Modify: `frontend/src/views/ManageBookingView.vue`, `frontend/src/views/EventDetailView.vue`, `DESIGN.md`, `README.md`

- [ ] **Step 1: Il rimando dalla pagina prenotazione**

In `ManageBookingView.vue`, accanto al nome del gioco prenotato (l'`<h2>{{ gameLabel }}</h2>` intorno alla riga 181), aggiungere il rimando. È la pagina che il partecipante ha già nella mail di conferma, quindi dal tavolo è un tap:

```html
<p v-if="booking?.gameId" class="row-meta">
  <router-link :to="{ path: `/games/${booking.gameId}`, query: { chat: '1' } }">
    Dubbi sulle regole? Chiedi al manuale
  </router-link>
</p>
```

Verificare come si chiama davvero il campo con l'id del gioco nella risposta della prenotazione: `grep -n "gameId\|game_id" frontend/src/views/ManageBookingView.vue`. Il rimando **non** va condizionato a `canAsk`, che questa pagina non conosce: se la chat non c'è, la scheda gioco semplicemente si apre senza. È un link in più, non un pulsante che promette una funzione assente.

- [ ] **Step 2: Il rimando dalla scheda evento**

In `EventDetailView.vue`, per ogni gioco in programma, lo stesso link a `/games/:id?chat=1`. Serve a chi sta scegliendo cosa giocare.

- [ ] **Step 3: La sezione in `DESIGN.md`**

Aggiungere una sezione che copra:

- Le due forme della chat (sidebar ≥1100px, bottone tondo + dialog sotto) e **perché** 1100 e non 900: `.app-page` è 56rem, e una sidebar da 22rem dentro quello spazio scenderebbe sotto la misura leggibile.
- Il bottone tondo: posizione in basso al centro, `env(safe-area-inset-bottom)`, e il padding compensativo sulla pagina.
- **deep-chat vive in shadow DOM**: i token di `app.css` non ci arrivano. I colori si passano da `messageStyles` e `auxiliaryStyle`, quindi **i valori dei token sono duplicati a mano in `ManualChatPanel.vue`**. Scriverlo esplicitamente, con il rimando al file: chi in futuro cambia `--felt` o `--card-alt` deve sapere che c'è un secondo posto da aggiornare, altrimenti la chat resta l'unica isola col vecchio colore.
- Lo stato di riposo con le domande suggerite, e il perché: deep-chat pesa più dell'intera app e si monta al primo gesto.

- [ ] **Step 4: Le note nel `README.md`**

- **`ai_vision_model`** fra le impostazioni opzionali: a cosa serve, e che il modello di chat spesso non legge immagini (`deepseek-v4-flash` è solo testo, `deepseek-v4-flash-vision-exp` no). Senza il campo, i manuali scansionati si trascrivono a mano e tutto il resto funziona.
- **La dettatura richiede HTTPS.** La Web Speech API è ristretta ai secure context: su `http://192.168.1.50:8080` in LAN il microfono non funziona, e non è un difetto dell'app. Funziona su `localhost` e dietro HTTPS (reverse proxy, Caddy, Tailscale).
- **Firefox non supporta la dettatura** (Chrome, Edge e Safari sì): la tastiera resta la strada principale.
- **L'audio della dettatura esce dal dispositivo**: l'implementazione di Chrome lo manda ai server Google per il riconoscimento. Non al server dell'app, e solo se l'utente tocca il microfono — ma va detto, perché il progetto promette «nessun servizio esterno obbligatorio».
- Come si prepara un manuale, in tre righe: carica il PDF fra i media, premi «Prepara per le domande», rivedi e salva.

- [ ] **Step 5: Verifica finale completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
cd frontend && npm run build && cd ..
```

Entrambi devono essere verdi. Nessuna dichiarazione di «funziona» senza questo output.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/ManageBookingView.vue frontend/src/views/EventDetailView.vue \
  DESIGN.md README.md
git commit -m "docs: document the manual Q&A feature and its dictation limits"
```

- [ ] **Step 7: `/impeccable` — task obbligatorio**

Lanciare `/impeccable` in modalità `polish` sulla superficie modificata:

- `ManualChat.vue` nelle due forme, con attenzione al **passaggio fra le due**: restringendo la finestra la sidebar diventa bottone, e una conversazione in corso viene smontata. Decidere se è accettabile (probabilmente sì: nessuno ridimensiona la finestra durante una partita) o se va conservata.
- Il bottone tondo: contrasto, area di tocco ≥44px, che non copra contenuto, `prefers-reduced-motion` rispettato.
- Il dialog: focus all'apertura, ritorno del focus al bottone alla chiusura, `aria-label`, comportamento della tastiera su iOS.
- Lo stato di riposo: che le domande suggerite non sembrino testo statico, e che il finto input sia riconoscibile come tale.
- L'icona segnaposto del bottone tondo, da sostituire con qualcosa nel tono "dado & pedina".
- `ManualPrepPanel.vue`: una textarea per pagina su un manuale da 40 pagine è una pagina lunghissima — valutare un accordion o una navigazione fra pagine.

---

## Note di esecuzione

**Ordine.** I task 1→9 sono backend e in sequenza: ognuno dipende dai tipi del precedente. Il 10 può partire in parallelo al 9 (usa solo `canAsk`, di cui conosce già la forma). L'11 richiede il 9 e il 10. Il 12 richiede l'8. Il 13 richiede tutto.

**Due verifiche non rimandabili.** Sono i due punti dove la spec fa una promessa che solo la realtà può confermare, e vanno fatte appena il codice le permette:

1. **Task 12, step 6** — la qualità della trascrizione del manuale scansionato reale. Se il modello economico restituisce testo disordinato, si scopre quando cambiare modello costa un campo nelle impostazioni, non a lavoro finito.
2. **Task 11, step 5, punto 5** — che il modello dica «il manuale non lo dice» invece di inventare. È la proprietà su cui poggia tutta la funzione: al tavolo una regola inventata fa più danno di una risposta mancante. Se non tiene, il prompt di sistema (`askSystemPrompt`) è la leva, e va sistemato prima di chiudere.

**Cosa fare se `ledongthuc/pdf` delude.** Il piano non prevede di sostituirla con qualcosa di più grosso: il fallback è di disegno, non di dipendenza — quel manuale si manda per il percorso vision, che su impaginazioni dense dà comunque risultati migliori. Le alternative sono già state valutate e scartate (spec 3.1): `rsc.io/pdf` è archiviata, `pdfcpu` non estrae testo, `go-pdfium` imbarca 5,7 MB di wasm.
