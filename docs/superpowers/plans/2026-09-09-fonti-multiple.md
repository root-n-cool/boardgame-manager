# Fonti multiple — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generalizzare la fonte delle risposte da «un manuale PDF paginato» a «un pezzo di testo con un riferimento citabile», accettando `.pdf`, `.txt`, `.md` e `.docx`, e lasciando il modello dati già capace di ospitare le FAQ di BoardGameGeek.

**Architecture:** Il markdown diventa la rappresentazione interna comune: ogni formato converge su markdown (l'OCR lo produce già, `.md` lo è, il `.docx` mappa i suoi stili, `.txt` e i PDF testuali lo ricevono dal modello), si parsano le sezioni, si spezzano in chunk. Una tabella sola, `game_source_chunk`, con `reference` / `reference_detail` / `heading`. Il tool restituisce un array strutturato invece di testo formattato, così le citazioni non si indovinano più con una regex.

**Tech Stack:** Go 1.25 (chi, `modernc.org/sqlite` con FTS5, `ledongthuc/pdf`, `archive/zip` + `encoding/xml` della stdlib per il docx), Vue 3 + TypeScript, `deep-chat`.

**Spec:** `docs/superpowers/specs/2026-09-09-fonti-multiple-design.md` — e la spec che questa supera in parte, `docs/superpowers/specs/2026-09-08-domande-sul-manuale-design.md`, per la ricerca FTS5 con più parole chiave, il loop dell'agente e la chat pubblica, che restano in vigore.

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto (binario x86_64 su Mac arm64):
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" -v "$(pwd)/data:/data" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Non sostituire `bgm-gomodcache` / `bgm-gocache` con volumi anonimi. I target `backend-*` del `Makefile` non funzionano su questa macchina.
- **`npm` in locale**, in `frontend/`, senza Docker.
- **Migrazioni forward-only.** Nuovo file `NNNN_nome.sql`, embeddato, applicato in ordine alfabetico. Mai modificare un file già rilasciato.
- **Nessuna dipendenza nuova**, Go o npm. Il docx si legge con `archive/zip` + `encoding/xml` della stdlib.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n.
- **JSON dell'API in camelCase.**
- **`CGO_ENABLED=0`**: niente estensioni SQLite native, niente librerie che richiedono CGO.
- **Gate unico sul provider:** senza provider AI configurato non esiste niente di questa feature — né la chat pubblica, né il bottone «Prepara», per **nessun** formato.
- **Nessun inserimento manuale del testo**, in nessuna forma. È il vincolo di prodotto che origina tutto il piano.
- **Nessuna conservazione del testo estratto.** Solo chunk.
- **Nessun percorso inline.** Il tool si dichiara sempre.
- **Commit e push solo se richiesti.** Gli step «Commit» creano commit locali; **non fare push**. Messaggi in inglese, conventional commits.
- **Attenzione a `backend/go.sum`.** Nella sessione precedente un comando `go` in un worktree ha aggiunto quattro volte 36 righe superflue (gli hash del grafo di build di `modernc.org/sqlite`). Dopo ogni run, `git status`; se è cresciuto, `git checkout -- backend/go.sum` prima di committare.

## Una lezione della fase precedente, che vincola come si scrivono i test qui

Il piano del 2026-09-08 ha prodotto **undici test incapaci di fallire** per la proprietà che nominavano — ognuno da codice del piano stesso, due da ricette dettate dal controller. Esempi reali: un'asserzione `Contains(body, "4")` soddisfatta da `"deepseek-v4-flash"`; un test sull'overlap del chunker che passava anche cancellando l'overlap; un test «non sconfina su un altro gioco» verde anche quando la ricerca non restituiva nulla.

Questo piano contiene quindi **meno codice di test e più istruzioni su come verificarlo**. Per ogni test che scrivi:

1. Chiediti: **passerebbe ancora se cancellassi il comportamento che dice di proteggere?**
2. Se la risposta è sì o non lo sai, **provalo**: rompi il comportamento, guarda il rosso, ripristina, guarda il verde.
3. Riporta entrambi gli esiti nel report. Un'asserzione che *sembra* verifica è peggio di nessuna verifica, perché ferma chi guarderebbe.

Se trovi un test di questo piano che non può fallire, **correggilo e dillo nel report** — è un contributo, non una deviazione.

---

## File Structure

**Backend — creati**

| File | Responsabilità |
|---|---|
| `backend/internal/db/migrations/0015_game_sources.sql` | Elimina `manual_page`/`manual_chunk`, crea `game_source_chunk` + FTS5 + 3 trigger |
| `backend/internal/manuals/markdown.go` | `ParseSections`, `ChunkSections` — dal markdown alle sezioni ai chunk |
| `backend/internal/manuals/markdown_test.go` | |
| `backend/internal/manuals/docx.go` | `DocxToMarkdown` con `archive/zip` + `encoding/xml` |
| `backend/internal/manuals/docx_test.go` | |
| `backend/internal/manuals/testdocx.go` | `NewDocx(...)` — costruisce un .docx valido per i test di due pacchetti |
| `backend/internal/ai/segment.go` | `Segment` — testo piatto in, markdown con titoli fuori, a finestre se lungo |
| `backend/internal/ai/segment_test.go` | |

**Backend — riscritti in modo sostanziale**

| File | Cosa cambia |
|---|---|
| `backend/internal/manuals/store.go` | Tutta la persistenza e la ricerca sulla tabella nuova; via `StoredPage`, `Corpus`, `ManualText`, `FormatCorpus`, `FormatIndex`, `FormatSearchResult` |
| `backend/internal/manuals/chunk.go` | `Chunk([]Page)` cade; resta `splitToSize` e la sua famiglia, usata da `ChunkSections`. `DetectHeading` cade: il markdown lo sostituisce |
| `backend/internal/httpapi/manuals_handlers.go` | Quattro rotte → due; il routing per formato; il gate sul provider; i messaggi d'errore |
| `backend/internal/ai/ask.go` | Via il ramo inline; tool rinominato; `SearchFunc` restituisce JSON; prompt aggiornato |
| `backend/internal/httpapi/ask_handler.go` | `linkifyCitations` sostituita dalla mappa `reference → path` |

**Backend — modificati**

| File | Modifica |
|---|---|
| `backend/internal/storage/store.go` | `ManualCategory` accetta quattro tipi MIME |
| `backend/internal/httpapi/games_responses.go` | `manualHeadings` → `sourceHeadings`, più il conteggio per fonte |
| `backend/internal/httpapi/events_responses.go` | `GamesWithPages` → `GamesWithChunks` |
| `backend/internal/httpapi/router.go` | Due rotte invece di quattro |

**Frontend**

| File | Modifica |
|---|---|
| `frontend/src/components/ManualPrepPanel.vue` | Da 253 righe a un bottone e una riga di stato |
| `frontend/src/views/GameAdminDetailView.vue` | `accept` allargato a quattro tipi |
| `frontend/src/utils/game.ts` | `manualHeadings` → `sourceHeadings` |
| `frontend/src/views/GameDetailView.vue` | La rinomina |
| `frontend/src/app.css` | Via gli stili della bozza |
| `DESIGN.md` / `README.md` | Le sezioni che descrivono un flusso che non esiste più |

---

### Task 1: La tabella nuova e la persistenza

**Files:**
- Create: `backend/internal/db/migrations/0015_game_sources.sql`
- Rewrite: `backend/internal/manuals/store.go` (solo la metà di persistenza; la ricerca è Task 6)
- Test: `backend/internal/manuals/store_test.go`

**Interfaces:**
- Consumes: `manuals.TextChunk` (esistente, da `chunk.go`) — solo temporaneamente, sostituito in Task 3.
- Produces:
  ```go
  type SourceChunk struct {
      ReferenceType   string // "document" | "faq"
      Reference       string
      ReferenceDetail string // "" quando la fonte non ne ha
      Heading         string // "" quando la sezione non ha titolo
      LanguageCode    string // "" per una FAQ
      Seq             int
      Text            string
  }
  type SourceSummary struct {
      HasChunks bool
      Headings  []string          // distinti, in ordine di seq, per l'indice del prompt
      PerMedia  map[int64]int     // game_media_id → numero di chunk, per il pannello admin
  }
  func NewStore(conn *sql.DB) *Store
  func (s *Store) ReplaceSource(ctx context.Context, gameID int64, mediaID *int64, chunks []SourceChunk) error
  func (s *Store) DeleteSource(ctx context.Context, mediaID int64) error
  func (s *Store) Summary(ctx context.Context, gameID int64) (SourceSummary, error)
  func (s *Store) GamesWithChunks(ctx context.Context, gameIDs []int64) (map[int64]bool, error)
  ```

`mediaID` è un `*int64` perché una FAQ non ne ha: è il tipo che rende impossibile passare uno zero e credere che significhi «nessun media».

- [ ] **Step 1: Scrivere la migrazione**

Creare `backend/internal/db/migrations/0015_game_sources.sql` col contenuto SQL della sezione 3 della spec, **verbatim** — compresi i `DROP` e i tre trigger. Copiarlo dalla spec, non riscriverlo a memoria: i nomi delle colonne li consumano cinque task.

- [ ] **Step 2: Scrivere il test della persistenza**

In `store_test.go`, sostituire i test di `ReplacePages`/`ListPages`/`DeletePages`. Il seed rimane quello che c'è (SQL diretto, senza importare `internal/games`).

Il test che conta di più — e l'unico di questo step che va scritto con attenzione — è la **cascata**, perché è l'unica garanzia strutturale della tabella nuova:

```go
func TestReplaceSource_CascadesWhenTheMediaGoes(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base",
			ReferenceDetail: "pagina 1", Heading: "Preparazione",
			LanguageCode: "it", Seq: 0, Text: "Si distribuiscono cinque carte."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var indexed int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts`).Scan(&indexed)
	if indexed != 1 {
		t.Fatalf("l'indice FTS5 non è in pari coi chunk: %d", indexed)
	}

	if _, err := conn.ExecContext(ctx, `DELETE FROM game_media WHERE id = ?`, mediaID); err != nil {
		t.Fatalf("delete media: %v", err)
	}

	var chunks, stillIndexed int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk`).Scan(&chunks)
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts`).Scan(&stillIndexed)
	if chunks != 0 || stillIndexed != 0 {
		t.Fatalf("la cascata ha lasciato %d chunk e %d righe indicizzate", chunks, stillIndexed)
	}
}
```

Nota su cosa prova: la cascata FK **e** che i trigger scattino sulla cancellazione a cascata, che non è ovvio e nella fase precedente è stato verificato a mano una volta. Se `stillIndexed` non fosse zero, la ricerca restituirebbe chunk di un manuale cancellato — un guasto silenzioso.

Gli altri test di questo step (idempotenza di `ReplaceSource`, `DeleteSource`, `Summary` che restituisce i titoli distinti in ordine, `GamesWithChunks` con più giochi) scrivili tu seguendo lo stesso stile. **Per ognuno applica i tre passi della sezione sui test**: chiediti se passerebbe cancellando il comportamento, e se non sei sicuro provalo.

Un'avvertenza specifica su `Summary`: un test che asserisce «i titoli ci sono» passa anche se l'ordine è casuale. Se l'ordine conta — e conta, perché l'indice nel prompt deve rispecchiare il documento — l'asserzione deve essere sulla **sequenza**, non sull'insieme.

- [ ] **Step 3: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" -v "$(pwd)/data:/data" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: FAIL in compilazione, `SourceChunk` non esiste.

- [ ] **Step 4: Implementare la persistenza**

Riscrivere la metà di persistenza di `store.go`. `ReplaceSource` sostituisce tutti i chunk di quella fonte in **una transazione** — `DELETE WHERE game_media_id = ?` poi gli `INSERT` — così non esiste uno stato con chunk nuovi e indice vecchio.

`Summary` fa **una** query e serve tre cose: `HasChunks`, i `Headings` distinti in ordine di `seq`, e `PerMedia`. È il pattern che la fase precedente ha già adottato per evitare una query per riga sul percorso pubblico.

Cancellare: `StoredPage`, `ReplacePages`, `ListPages`, `DeletePages`, `HasPages`, `ManualSummary`, `GamesWithPages`, `Corpus`, `ManualText`, `FormatCorpus`, `FormatIndex`. I loro chiamanti si rompono: è atteso, li sistemano i task 6-8. **Se il pacchetto `httpapi` non compila alla fine di questo task è normale** — verifica solo `go test ./internal/manuals/`, e dillo nel report.

- [ ] **Step 5: Eseguire i test del pacchetto**

```bash
docker run --rm -v "$(pwd)/backend:/app" -v "$(pwd)/data:/data" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -v
```

Atteso: PASS. La migrazione viene applicata da ogni test che apre un DB, quindi un errore SQL si manifesta come fallimento diffuso.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0015_game_sources.sql \
  backend/internal/manuals/store.go backend/internal/manuals/store_test.go
git commit -m "feat: replace the manual tables with a generic source_chunk table"
```

---

### Task 2: Dal markdown alle sezioni ai chunk

**Files:**
- Create: `backend/internal/manuals/markdown.go`, `backend/internal/manuals/markdown_test.go`
- Modify: `backend/internal/manuals/chunk.go`

**Interfaces:**
- Consumes: `splitToSize`, `MaxChunkChars`, `ChunkOverlapChars` (esistenti in `chunk.go`).
- Produces:
  ```go
  type Section struct {
      Heading string // "" per il testo che precede il primo titolo
      Body    string
      Offset  int    // offset in byte nel markdown d'origine dove comincia il corpo
  }
  type SectionChunk struct {
      Heading string
      Seq     int // progressivo su tutto il documento, non per sezione
      Offset  int // offset in byte nel markdown d'origine dove comincia il chunk
      Text    string
  }
  func ParseSections(md string) []Section
  func ChunkSections(sections []Section) []SectionChunk
  ```

`Offset` esiste per un solo motivo: nel Task 5 serve a decidere **in quale pagina del PDF** un chunk comincia. Senza, `reference_detail` non si può calcolare.

- [ ] **Step 1: Scrivere i test**

Tre proprietà meritano un test, e le scrivo io perché sono quelle che si sbagliano:

```go
func TestParseSections_KeepsThePreambleAndEveryHeading(t *testing.T) {
	md := "Un gioco per 2-5 giocatori.\n\n" +
		"## Preparazione\nSi distribuiscono cinque carte.\n\n" +
		"## Fase di Upkeep\nOgni giocatore paga una moneta.\n\n" +
		"### Eccezione\nChi non può pagare demolisce.\n"

	got := manuals.ParseSections(md)
	if len(got) != 4 {
		t.Fatalf("attese 4 sezioni (preambolo + 3 titoli), ottenute %d: %+v", len(got), got)
	}
	// Il preambolo è una sezione senza titolo, non un errore: molti
	// regolamenti aprono con un paragrafo introduttivo.
	if got[0].Heading != "" || !strings.Contains(got[0].Body, "2-5 giocatori") {
		t.Fatalf("il preambolo è andato perso: %+v", got[0])
	}
	if got[1].Heading != "Preparazione" || got[3].Heading != "Eccezione" {
		t.Fatalf("titoli sbagliati: %q, %q", got[1].Heading, got[3].Heading)
	}
	// L'offset deve puntare nel markdown originale: è ciò che permette di
	// risalire alla pagina del PDF in cui la sezione comincia.
	if md[got[1].Offset:got[1].Offset+3] != "Si " {
		t.Fatalf("offset della sezione 1 sbagliato: punta a %q", md[got[1].Offset:got[1].Offset+10])
	}
}
```

Perché questo test discrimina: se `ParseSections` scartasse il preambolo restituirebbe 3 sezioni; se contasse solo i `##` ignorando i `###` ne restituirebbe 3; se l'offset fosse relativo al corpo invece che al documento l'ultima asserzione fallirebbe.

```go
func TestChunkSections_CarriesTheHeadingOfItsOwnSection(t *testing.T) {
	// Due sezioni, la prima abbastanza lunga da spezzarsi: ogni chunk deve
	// portare il titolo della PROPRIA sezione, non il primo del documento.
	long := strings.Repeat("Ogni giocatore paga una moneta per edificio. ", 40)
	sections := []manuals.Section{
		{Heading: "Fase di Upkeep", Body: long, Offset: 0},
		{Heading: "Fine partita", Body: "La partita termina subito.", Offset: len(long) + 100},
	}

	chunks := manuals.ChunkSections(sections)
	if len(chunks) < 3 {
		t.Fatalf("attesi più chunk dalla prima sezione più uno dalla seconda, ottenuti %d", len(chunks))
	}
	last := chunks[len(chunks)-1]
	if last.Heading != "Fine partita" {
		t.Fatalf("l'ultimo chunk deve portare il titolo della sua sezione, porta %q", last.Heading)
	}
	if chunks[0].Heading != "Fase di Upkeep" {
		t.Fatalf("il primo chunk porta %q", chunks[0].Heading)
	}
	// Seq è progressivo su tutto il documento: due sezioni non ripartono da 0,
	// altrimenti "il chunk vicino" (seq ± 1) attraverserebbe le sezioni a caso.
	for i, c := range chunks {
		if c.Seq != i {
			t.Fatalf("seq non progressivo sul documento: chunk %d ha seq %d", i, c.Seq)
		}
	}
}
```

Perché discrimina: un'implementazione che assegnasse a tutti i chunk il titolo della prima sezione passerebbe l'asserzione su `chunks[0]` e fallirebbe quella su `last` — che è il punto.

Il terzo test è sull'`Offset` dei chunk dentro una sezione lunga. Scrivilo tu, e **provalo rompendolo**: se `Offset` fosse sempre quello della sezione invece di avanzare col chunk, il tuo test deve diventare rosso. Se non diventa rosso, il test è sbagliato, non l'implementazione.

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" -v "$(pwd)/data:/data" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/manuals/ -run 'ParseSections|ChunkSections' -v
```

- [ ] **Step 3: Implementare**

`ParseSections` riconosce i titoli ATX (`#` … `######`) a inizio riga, gestendo la forma chiusa (`## Titolo ##`). Non serve un parser markdown completo: ci interessano i titoli e il testo fra di essi.

`ChunkSections` riusa `splitToSize` sul corpo di ogni sezione — il chunker con sovrapposizione che c'è già e che è stato verificato nella fase precedente. `Seq` è un contatore su tutto il documento.

Cancellare da `chunk.go`: `Chunk([]Page)`, `DetectHeading`, `stripHeadingMarkers`, `headingOrdinal`, `headingWords`. **`DetectHeading` esce di scena** — era l'euristica che guardava la prima riga di ogni pagina e produceva al massimo un titolo per pagina; il markdown la sostituisce del tutto. Restano `splitToSize`, `splitUnits`, `tailFrom`, `sentenceEnd`, `MaxChunkChars`, `ChunkOverlapChars` e i loro test.

- [ ] **Step 4: Eseguire i test**

Atteso: PASS su tutto `./internal/manuals/`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/manuals/markdown.go backend/internal/manuals/markdown_test.go \
  backend/internal/manuals/chunk.go backend/internal/manuals/chunk_test.go
git commit -m "feat: find sections in markdown and chunk them"
```

---

### Task 3: Il docx, con la stdlib

**Files:**
- Create: `backend/internal/manuals/docx.go`, `backend/internal/manuals/docx_test.go`, `backend/internal/manuals/testdocx.go`

**Interfaces:**
- Produces:
  ```go
  func DocxToMarkdown(raw []byte) (string, error)
  // in testdocx.go, esportata perché la usano i test di due pacchetti:
  func NewDocx(paragraphs []DocxParagraph) []byte
  type DocxParagraph struct { Style string; Text string } // Style: "Heading1".."Heading6", "" per un paragrafo normale
  ```

`NewDocx` sta in un file **non-test** e si esporta, per la stessa ragione per cui `NewScannedPDF` fu esportata nella fase precedente: `httpapi` non può importare il pacchetto di test di `manuals`, e duplicare la costruzione di uno zip in due posti è come le due copie divergono.

- [ ] **Step 1: Capire il formato prima di scrivere il test**

Un `.docx` è uno zip. Il documento sta in `word/document.xml`, e la struttura che ci interessa è:

```xml
<w:body>
  <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Preparazione</w:t></w:r></w:p>
  <w:p><w:r><w:t>Si distribuiscono </w:t></w:r><w:r><w:t>cinque carte.</w:t></w:r></w:p>
</w:body>
```

Due cose da sapere, che decidono l'implementazione:

- **Il testo di un paragrafo è spezzato su più `<w:r>`** (i "run", che portano la formattazione). Il testo del paragrafo è la concatenazione dei `<w:t>` dei suoi run. Un'implementazione che leggesse solo il primo `<w:t>` perderebbe metà delle frasi — e su un documento reale se ne accorgerebbe solo chi lo rilegge.
- **Il nome dello stile varia**: Word scrive `Heading1`, altri produttori `heading 1`, e i documenti italiani a volte `Titolo1`. Normalizza (minuscole, via gli spazi) e riconosci sia `heading` sia `titolo`.

- [ ] **Step 2: Scrivere il test**

Il test che conta è quello sui run spezzati, perché è il difetto probabile:

```go
func TestDocxToMarkdown_JoinsTheRunsOfAParagraph(t *testing.T) {
	// Un paragrafo il cui testo Word ha spezzato su tre run: se
	// l'implementazione ne legge uno solo, la frase arriva mutilata.
	raw := manuals.NewDocxWithRuns([]manuals.DocxParagraph{
		{Style: "Heading1", Text: "Fase di Upkeep"},
	}, map[int][]string{
		1: {"Ogni giocatore ", "paga una moneta ", "per ciascun edificio."},
	})

	md, err := manuals.DocxToMarkdown(raw)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if !strings.Contains(md, "# Fase di Upkeep") {
		t.Fatalf("lo stile Heading1 non è diventato un titolo markdown:\n%s", md)
	}
	if !strings.Contains(md, "Ogni giocatore paga una moneta per ciascun edificio.") {
		t.Fatalf("i run non sono stati uniti:\n%s", md)
	}
}
```

Questo richiede che `NewDocx` sappia costruire un paragrafo con più run — quindi progetta la firma dell'helper di conseguenza, o esponi una seconda funzione come nell'esempio. **Il test deve costruire il caso difficile, non quello facile.**

Gli altri test — la mappatura `Heading1/2/3` → `#`/`##`/`###`, le varianti del nome dello stile, uno zip che non è un docx, un docx senza `word/document.xml` — scrivili tu. Ricorda i tre passi: per ognuno chiediti se passerebbe cancellando il comportamento.

- [ ] **Step 3: Eseguire, verificare il rosso, implementare, verificare il verde**

`DocxToMarkdown` apre lo zip con `archive/zip`, legge `word/document.xml`, e lo scorre con un `xml.Decoder` in streaming — non un `Unmarshal` su una struct dell'intero documento, perché lo schema è vasto e ci interessano tre elementi.

**Nessuna dipendenza nuova.** Se ti trovi a volerne una, fermati e riporta: la spec dice esplicitamente che questo formato si legge con la stdlib.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/manuals/docx.go backend/internal/manuals/docx_test.go \
  backend/internal/manuals/testdocx.go
git commit -m "feat: read a docx into markdown with the standard library"
```

---

### Task 4: La segmentazione generata dal modello

**Files:**
- Create: `backend/internal/ai/segment.go`, `backend/internal/ai/segment_test.go`

**Interfaces:**
- Consumes: `HTTPClient.postChat`, `ErrNotConfigured`, `chatMessage` (esistenti).
- Produces:
  ```go
  const SegmentWindowMaxChars = 60000
  type Segmenter interface {
      Segment(ctx context.Context, text string) (string, error)
  }
  func (c *HTTPClient) Segment(ctx context.Context, text string) (string, error)
  ```

Serve a `.txt` e ai PDF con layer testo: testo piatto dentro, lo **stesso testo** con i titoli markdown inseriti fuori.

- [ ] **Step 1: Il prompt, e il rischio che porta**

Il prompt chiede di inserire titoli markdown dove una sezione comincia, e di **non riscrivere il contenuto**. È un vincolo espresso a parole a un modello economico, non una garanzia meccanica: un titolo inventato è innocuo, un paragrafo riscritto introduce una regola che il manuale non contiene.

Quindi implementa anche la mitigazione che la spec nomina: **confrontare la lunghezza del testo senza i titoli prima e dopo**, e rifiutare una risposta che se ne discosti oltre una soglia (il 15% è ragionevole: i titoli aggiunti crescono, il contenuto no). Un errore qui è meglio di una regola inventata, perché l'admin lo vede.

- [ ] **Step 2: Scrivere i test**

Due test, e sono entrambi sul comportamento che protegge l'utente:

```go
func TestSegment_RejectsAResponseThatRewritesTheContent(t *testing.T) {
	// Il modello restituisce metà del testo: è il caso in cui ha riassunto
	// invece di segmentare, ed è quello che introdurrebbe regole inventate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"## Preparazione\nSi distribuiscono."}}]}`)
	}))
	defer srv.Close()

	long := "Si distribuiscono cinque carte a ciascun giocatore. " +
		strings.Repeat("Poi si mescola il mazzo e si posa al centro del tavolo. ", 20)

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Segment(context.Background(), long); err == nil {
		t.Fatal("una risposta che ha perso il contenuto deve essere un errore, non un manuale mutilato")
	}
}

func TestSegment_WithoutAProviderIsNotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	if _, err := client.Segment(context.Background(), "testo"); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}
```

Il terzo comportamento — le **finestre** per un testo sopra `SegmentWindowMaxChars` — scrivilo tu, e qui l'avvertenza è precisa: un test che passa un testo lungo e conta le richieste al finto provider **verifica il numero di chiamate, non la continuità**. La proprietà che conta è che l'ultimo titolo di una finestra arrivi come contesto alla successiva. Asserisci **sul corpo della seconda richiesta**, e prova a rompere il passaggio del contesto per vedere il rosso.

- [ ] **Step 3: Rosso, implementare, verde, commit**

```bash
git add backend/internal/ai/segment.go backend/internal/ai/segment_test.go
git commit -m "feat: have the model add markdown headings to unstructured text"
```

---

### Task 5: L'ingestione — un'azione sola, quattro formati

**Files:**
- Rewrite: `backend/internal/httpapi/manuals_handlers.go`
- Modify: `backend/internal/storage/store.go`, `backend/internal/httpapi/router.go`
- Test: `backend/internal/httpapi/manuals_handlers_test.go`

**Interfaces:**
- Consumes: tutto quanto prodotto dai task 1-4, più `HasTextLayer`, `ExtractText`, `ExtractPageImages`, `Transcribe` (esistenti).
- Produces: `POST /api/games/{id}/languages/{lang}/media/{mediaId}/index`, `DELETE` sullo stesso path; `Server.Segmenter ai.Segmenter`.

- [ ] **Step 1: I quattro tipi MIME**

In `backend/internal/storage/store.go`, `ManualCategory.AllowedTypes` accetta:

```go
"application/pdf":  ".pdf",
"text/plain":       ".txt",
"text/markdown":    ".md",
"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
```

Il limite di 20 MB resta. Nota: `http.DetectContentType` non riconosce `text/markdown` — restituisce `text/plain` per un `.md`. Verifica come `Save` determina il tipo e, se serve, accetta `text/plain` per entrambi distinguendo poi dall'estensione. **Questo va verificato leggendo `Save`, non assunto.**

- [ ] **Step 2: Il routing per formato**

Il cuore del task. Da file a markdown, cinque strade:

```
.md    → il contenuto è già markdown
.docx  → DocxToMarkdown
.txt   → Segment(testo)
.pdf   → HasTextLayer?
           sì → ExtractText per pagina → se la media per pagina è sotto
                minAvgUsableCharsPerPage, cade sul ramo vision
                altrimenti → Segment(testo unito) e si tengono le pagine
           no → ExtractPageImages → Transcribe per pagina (già markdown)
```

La logica di preferenza del PDF esiste già in questo file e va conservata: la media di caratteri per pagina, il fallback su vision, il 422 solo quando entrambi i percorsi non danno nulla. **Non riscriverla da zero**: leggila e adattala.

- [ ] **Step 3: Pagine e sezioni, cioè il pezzo delicato**

Per un PDF si ottiene il markdown **per pagina**. Poi:

1. concatena il markdown delle pagine in ordine, tenendo per ciascuna l'offset in cui comincia;
2. `ParseSections` sul testo unito;
3. `ChunkSections`;
4. per ogni chunk, `ReferenceDetail = "pagina N"` dove `N` è la pagina il cui intervallo di offset contiene `chunk.Offset`.

Per gli altri formati non ci sono pagine: `ReferenceDetail = 'sezione «' + chunk.Heading + '»'`, e stringa vuota quando la sezione non ha titolo.

**I tre tipi in gioco, perché è facile confonderli.** `SectionChunk` (Task 2) è
ciò che produce il chunker: `Heading`, `Seq`, `Offset`, `Text`. `SourceChunk`
(Task 1) è ciò che si persiste: aggiunge `ReferenceType`, `Reference`,
`ReferenceDetail`, `LanguageCode`. **Questo task è il posto dove il primo
diventa il secondo** — l'unico che conosce sia il file che le sezioni.
`SourceHit` (Task 6) è ciò che esce al modello: i quattro campi del contratto,
senza `Heading` né `Offset`.

`Reference` viene da `game_media.title` — **non** da `url_or_path`, che è uno sha256. E va **disambiguata**: `title` è nullable e testo libero, quindi due media possono collidere. Se la `reference` è già usata da un'altra fonte dello stesso gioco, aggiungi la lingua; se ancora, un ordinale. Senza questo la mappa `reference → path` del Task 7 punta al file sbagliato — che è precisamente il bug che quel task esiste per chiudere.

- [ ] **Step 4: Il gate sul provider e i messaggi**

`POST .../index` risponde **404** se il provider non è configurato: la rotta si comporta come inesistente, come già fa `POST .../ask`. Il pannello non mostra il bottone, quindi non è un caso che un utente incontri navigando.

I messaggi d'errore, che ora sono l'unica cosa fra l'admin e un vicolo cieco:

| Causa | Messaggio (italiano, rivolto a una persona) |
|---|---|
| Modello vision non configurato, PDF scansionato | nominare il campo nelle impostazioni |
| PDF né testo né immagini | suggerire di convertire il file |
| docx senza contenuto testuale | dirlo, senza fingere un guasto generico |
| Segmentazione rifiutata (contenuto riscritto) | dire che la lettura non è affidabile e invitare a riprovare |

- [ ] **Step 5: I test**

Il test che conta più di tutti è quello che **nessuno può scrivere per te senza pensarci**: che le pagine finiscano nel `reference_detail` giusto quando una sezione attraversa due pagine. Costruiscilo con un PDF scansionato di tre pagine (`manuals.NewScannedPDFPages(3)` esiste dalla fase precedente) e un finto trascrittore che restituisce markdown noto per pagina, con una sezione che comincia a pagina 1 e continua a pagina 2.

Asserisci che un chunk che comincia nella parte di pagina 2 porti `"pagina 2"` — e **provalo rompendolo**: se la mappatura offset→pagina fosse sbagliata (per esempio prendesse sempre la prima pagina), il test deve diventare rosso.

Gli altri: le due rotte protette (401 senza sessione), il 404 senza provider, un `.md` che si indicizza senza chiamare né vision né segmentazione, un `.txt` che chiama la segmentazione, il `DELETE` che rimuove.

Un'avvertenza dalla fase precedente: il test «richiede autenticazione» che cicla su più rotte **può passare per il motivo sbagliato** se una rotta restituisce 400 prima di arrivare all'auth. Verifica che il middleware corra prima della validazione.

- [ ] **Step 6: Montare le rotte**

Sostituire le quattro con due in `router.go`. Aggiungere `Segmenter` al `Server` e il costruttore per richiesta dalle impostazioni, con lo stesso schema di `translator`/`transcriber`/`asker`.

**Due passi che nessun test coglie:** `cmd/server/main.go` e `testhelpers_test.go` vanno aggiornati se la forma del `Server` cambia. Verifica con `go build ./...`.

- [ ] **Step 7: Suite intera, poi commit**

```bash
git add backend/internal/httpapi/ backend/internal/storage/store.go backend/cmd/server/main.go
git commit -m "feat: index a manual from pdf, txt, md or docx in one action"
```

---

### Task 6: La ricerca e il contratto strutturato del tool

**Files:**
- Rewrite: la metà di ricerca di `backend/internal/manuals/store.go`
- Modify: `backend/internal/ai/ask.go`
- Test: `backend/internal/manuals/store_test.go`, `backend/internal/ai/ask_test.go`

**Interfaces:**
- Produces:
  ```go
  // manuals
  type SourceHit struct {
      ReferenceType   string `json:"reference_type"`
      Reference       string `json:"reference"`
      ReferenceDetail string `json:"reference_detail"`
      Text            string `json:"text"`
      FoundWith []string `json:"-"` // diagnostica, non esce al modello
  }
  func (s *Store) Search(ctx context.Context, gameID int64, preferLang string, keywords []string) ([]SourceHit, []string, error) // hit, parole senza risultati, errore
  func MarshalHits(hits []SourceHit, missing []string) (string, error)
  // ai
  const SearchToolName = "cerca_nelle_fonti"
  ```

- [ ] **Step 1: Riscrivere `Search` sulla tabella nuova**

La logica resta quella verificata nella fase precedente e **non va reinventata**: una query FTS5 per parola chiave (non un unico `OR`, perché una parola comune sommergerebbe una rara), nessun `LIMIT` in SQL — il taglio avviene in Go **dopo** il riordino per lingua preferita, altrimenti si scarta una riga nella lingua giusta che BM25 classifica fuori finestra — il fallback su `escapeFTS` solo quando la sintassi è illegale, il tetto di `maxKeywords`, e l'ordine mantenuto in una **slice**, mai in una mappa (una mappa Go randomizza l'iterazione, ed è stato un bug reale).

Cambia: le colonne, e il fatto che `Search` restituisce `[]SourceHit` con i quattro campi del contratto invece di `Hit` con la pagina.

`attachNeighbours` diventa `seq ± 1` **nella fonte** invece che nella pagina, e la query filtra su `game_media_id` + `seq`.

- [ ] **Step 2: `MarshalHits` e la fine di `FormatSearchResult`**

Il risultato del tool è l'array JSON della spec. Le parole chiave senza risultati **restano parte del contratto** — è ciò che dice al modello quali ipotesi lessicali sono cadute — quindi vanno nel payload, e il posto naturale è un oggetto che contiene l'array e la lista:

```json
{"risultati": [ ... ], "nessun_risultato_per": ["pareggio", "ripescare"]}
```

Decidilo tu se annidare così o mandare due campi separati, ma **non perdere la lista**: nella fase precedente era la cosa che riparava la cecità di FTS5 ai sinonimi.

`FormatSearchResult` si cancella insieme ai suoi test.

- [ ] **Step 3: `ask.go` — via l'inline, rinomina, prompt**

Cancellare: `InlineCorpusMaxChars`, `AskRequest.CorpusText`, il parametro `inline` di `askSystemPrompt` e il ramo che dichiarava zero tool. **Il tool si dichiara sempre** quando `req.Search != nil`.

Rinominare `searchToolName` in `SearchToolName = "cerca_nelle_fonti"` (esportata, la usa il test dell'handler) e aggiornare la descrizione del tool: cerca «nelle fonti del gioco», non «nel manuale».

Il prompt di sistema cambia sulle citazioni: il modello riceve `reference` e `reference_detail` e deve citarli **così come li ha ricevuti**, perché è su quella stringa esatta che il server costruisce il link (Task 7). Dirglielo esplicitamente: inventare o abbreviare un riferimento rompe il link.

- [ ] **Step 4: I test, e l'unico che scrivo io**

```go
func TestAsk_AlwaysDeclaresTheTool(t *testing.T) {
	// Il ramo inline non esiste più: anche un corpus minuscolo deve vedere
	// il tool dichiarato, altrimenti un manuale corto non è interrogabile.
	srv := &askServer{t: t, responses: []string{answerOnly}}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	client := ai.NewHTTPClient(ts.URL, "sk-test", "m")
	if _, err := client.Ask(context.Background(), ai.AskRequest{
		GameName: "Wingspan",
		Turns:    []ai.Turn{{Role: "user", Text: "come finisce?"}},
		Search:   func(ctx context.Context, kw []string) (string, error) { return "[]", nil },
	}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !strings.Contains(srv.requests[0], `"tools":`) {
		t.Fatalf("il tool deve essere dichiarato sempre:\n%s", srv.requests[0])
	}
}
```

Nota che cerca la **chiave** `"tools":` e non il nome del tool: il nome comparirebbe comunque nella cronologia dei tool call rimandata indietro, ed è esattamente l'errore che un test della fase precedente conteneva.

Gli altri test di `ask.go` esistono e vanno adattati, non riscritti. Quello sull'echo verbatim del messaggio assistant (con `"refusal":null` nella fixture) **conservalo intatto**: verifica una proprietà che nessun'altra cosa protegge.

- [ ] **Step 5: Suite, commit**

```bash
git add backend/internal/manuals/ backend/internal/ai/
git commit -m "feat: return structured source hits and always offer the search tool"
```

---

### Task 7: Le citazioni che non indovinano più

**Files:**
- Modify: `backend/internal/httpapi/ask_handler.go`, `backend/internal/httpapi/games_responses.go`, `backend/internal/httpapi/events_responses.go`
- Test: `backend/internal/httpapi/ask_handler_test.go`

- [ ] **Step 1: Sostituire `linkifyCitations`**

Oggi cerca `pag. N` con una regex e prova a dedurre quale PDF fosse; con due manuali sbagliava, e la review finale della fase precedente ha imposto di **non linkare affatto** in quel caso.

Ora l'handler **sa** cosa ha mandato: mentre serve la closure di ricerca, accumula la mappa `reference → url_or_path` delle fonti effettivamente restituite. A risposta pronta, sostituisce le occorrenze di ciascuna `reference` conosciuta con un link markdown:

- `document` → `/api/uploads/<url_or_path>` più `#page=N` quando `reference_detail` è una pagina;
- `faq` → la `reference` è già l'URL.

La regex `citationRe` si cancella. **La guardia contro il doppio link resta**: se la risposta contiene già `](/api/uploads/`, non si riscrive.

- [ ] **Step 2: Il test che dimostra che il bug è chiuso**

È il test più importante del task, perché il comportamento vecchio era «non linkare» e quello nuovo è «linkare quello giusto»:

Chiamalo `TestAskHandler_LinksEachCitationToItsOwnManual`, e costruiscilo così:

- semina **due** media sullo stesso gioco, con `title` distinti e `url_or_path`
  distinti, ed entrambi con chunk indicizzati;
- il `fakeAsker` restituisce una risposta che cita **entrambe** le reference,
  ognuna con il proprio `reference_detail`;
- asserisci che il link della prima citazione contenga il `url_or_path` del
  **primo** media e quello della seconda il `url_or_path` del **secondo**.

L'asserzione da evitare è «la risposta contiene `/api/uploads/`»: sarebbe vera
anche se entrambi i link puntassero al primo file, che è esattamente il bug.

Poi **provalo rompendolo**: fai in modo che la mappa restituisca sempre il primo
path, e verifica che il test diventi rosso. Se non diventa rosso il test non
discrimina e va rifatto — è la forma di difetto che questo piano esiste per non
ripetere.

- [ ] **Step 3: Ricostruire l'indice del prompt**

`FormatIndex` è stata cancellata nel Task 1 e `Summary.Headings` fornisce i
titoli, ma **nessuno costruisce più la stringa** che finisce in
`AskRequest.CorpusIndex`. Va fatto qui, dove l'handler costruisce già la
richiesta:

```
Fonti: Carcassonne_Base_&_Fiume_ITA.pdf — Preparazione · Turno del giocatore · Fase di Upkeep
```

Raggruppata per `reference`, i titoli nell'ordine di `seq` che `Summary` già
restituisce. Senza questo step l'indice arriva vuoto e cade l'idea di design
che evita al modello la ricerca esplorativa — che in questo disegno conta più
di prima, perché il tool si usa sempre.

Il test: un gioco con fonti indicizzate, e l'asserzione che `asker.got.CorpusIndex`
contenga un titolo reale. Attenzione a non scrivere l'asserzione su una stringa
che comparirebbe comunque nel prompt di sistema.

- [ ] **Step 4: Le rinomine**

`games_responses.go`: `manualHeadings` → `sourceHeadings`, e `Summary` fornisce anche `PerMedia` per il pannello admin. `events_responses.go`: `GamesWithPages` → `GamesWithChunks`. `canAsk` resta com'è.

- [ ] **Step 5: Suite, `go build ./...`, commit**

```bash
git add backend/internal/httpapi/
git commit -m "fix: build citation links from the references actually sent"
```

---

### Task 8: Il pannello admin si riduce

**Files:**
- Rewrite: `frontend/src/components/ManualPrepPanel.vue`
- Modify: `frontend/src/views/GameAdminDetailView.vue`, `frontend/src/utils/game.ts`, `frontend/src/views/GameDetailView.vue`, `frontend/src/app.css`

- [ ] **Step 1: Cosa resta e cosa va**

Il componente passa da 253 righe a poche decine. **Resta:** lo stato («12 sezioni indicizzate» / «non preparato»), «Prepara per le domande», «Rimuovi indice», gli errori, e il `window.confirm` su «Rimuovi indice» — che è ancora un'azione distruttiva, anche se non c'è più una bozza da perdere.

**Va:** `draft`, le textarea per pagina, `handleBeforeUnload`, il confirm su «Prepara di nuovo», `discardDraft`, `save`, `loadSaved`, `source`, e in `app.css` gli stili `.manual-prep-pages`, `.manual-prep-page`, `.manual-prep-page-empty` e i loro vicini.

**Nuovo:** senza provider il bottone non compare, e il pannello dice il motivo **pratico** — la chat non esiste senza provider, quindi indicizzare adesso non serve a nulla. Non una spiegazione tecnica.

- [ ] **Step 2: L'`accept` dell'upload**

`GameAdminDetailView.vue:600` ha `accept="application/pdf"`. Diventa i quattro tipi. Nota che `accept` è un suggerimento del browser, non una validazione: il gate vero è `ManualCategory` lato server, già fatto nel Task 5.

- [ ] **Step 3: Le rinomine nel frontend**

`utils/game.ts`: `manualHeadings` → `sourceHeadings` in `GameDetail`. `GameDetailView.vue`: la ref e la prop. `ManualChatPanel.vue` riceve `headings` e non cambia.

- [ ] **Step 4: Verificare, e guardarlo davvero**

```bash
cd frontend && npm run build
```

Verde — è l'unico gate automatico del frontend, e fa anche il type-check.

Poi guardalo nel browser. `docker compose up -d --build` funziona su questa macchina (verificato). `./data/app.db` ha il gioco 3 (Carcassonne) col manuale scansionato.

**Verifica e riporta con precisione:** che senza provider il bottone non ci sia e il motivo si legga; che con provider configurato ci sia; che «Rimuovi indice» chieda conferma. **Non descrivere comportamento che non hai osservato** — nella fase precedente due verifiche erano esatte sullo stato e mancavano il risultato: una controllò `open: false` e `existsInDom: true` su un dialog che era ancora visibile a schermo.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/
git commit -m "refactor: reduce the prep panel to one action"
```

---

### Task 9: Documentazione e verifica finale

**Files:**
- Modify: `DESIGN.md`, `README.md`

- [ ] **Step 1: `DESIGN.md`**

La sezione «Preparazione di un manuale» descrive un flusso che non esiste più: le caselle per pagina, l'etichetta `sticky`, il tag oro sulla pagina vuota, `min(70vh, 48rem)`. Riscrivila su quel che resta, e **di' perché è cambiata** — che l'inserimento manuale è stato rimosso per scelta di prodotto, non semplificato via.

La nota sui token duplicati a mano in `ManualChatPanel.vue` **resta valida e va conservata**: elenca dieci token e nessun altro posto lo segnala.

- [ ] **Step 2: `README.md`**

Aggiornare: i formati accettati (quattro invece di uno); che **senza provider AI la preparazione non è disponibile**, per nessun formato; che la preparazione di uno scan lungo è una richiesta HTTP che dura minuti e un reverse proxy con `proxy_read_timeout` breve la taglia — la nota c'è già e va conservata; e togliere ogni riferimento alla correzione manuale del testo.

- [ ] **Step 3: Verifica finale**

```bash
docker run --rm -v "$(pwd)/backend:/app" -v "$(pwd)/data:/data" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
cd frontend && npm run build && cd ..
docker compose up -d --build
```

Tutti verdi, e l'app che risponde. Nessuna affermazione di «funziona» senza questo output.

- [ ] **Step 4: Commit**

```bash
git add DESIGN.md README.md
git commit -m "docs: record the source model and the removal of manual entry"
```

---

### Task 10: Passata `/impeccable`

Ultimo task, obbligatorio per il `CLAUDE.md` su ogni intervento che tocca il frontend.

- [ ] **Step 1: Lanciare `/impeccable` in modalità `polish`** sulla superficie modificata: `ManualPrepPanel.vue` ridotto, l'upload a quattro formati, e i messaggi d'errore — che in questo disegno **sono il prodotto**, perché sono l'unica cosa fra l'admin e un vicolo cieco.

La passata del 2026-09-08 su questo pannello riguardava in gran parte la metà che è sparita, quindi va rifatta, non riletta.

---

## Note di esecuzione

**Ordine.** I task 1→7 sono backend e in sequenza per i tipi. Il 5 è il più grosso e dipende da 1-4. L'8 dipende dal 7 solo per la rinomina di `sourceHeadings`. Il 9 e il 10 chiudono.

**Il pacchetto `httpapi` non compila fra il Task 1 e il Task 5.** È voluto: il Task 1 cancella l'API che `httpapi` usa, e i chiamanti si sistemano nel 5. Verifica `./internal/manuals/` da solo nei task intermedi e dillo nel report, invece di inseguire errori di compilazione altrove.

**La verifica che vale più di tutte le altre non è nel piano**, perché richiede il credito API dell'utente: che `deepseek-v4-flash-vision-exp` legga in modo leggibile un regolamento scansionato, e che la segmentazione non parafrasi. Questo disegno ci scommette sopra più del precedente — via la correzione manuale, via il percorso inline. Se l'utente dà il via libera, farla **prima** del Task 5 cambierebbe le priorità di tutto il resto.
