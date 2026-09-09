# Domande sul manuale — Design

Data: 2026-09-08

## 1. Panoramica e obiettivi

Un partecipante in piedi al tavolo, con le carte in mano e un dubbio sulle
regole, apre una pagina dal telefono, scrive «finite le carte che si fa?» e
riceve una risposta **con il numero di pagina del manuale**, così può
verificarla e chiudere la discussione.

Il sistema è un agente: un modello linguistico a cui viene dato un tool per
cercare nel regolamento del gioco. Il manuale entra nel sistema una volta
sola, all'upload, e da lì è testo indicizzato in SQLite.

Obiettivi:

- Risposte fondate sul manuale, sempre con la citazione della pagina.
- **Nessun servizio esterno in più**: nessun vector store con un server
  proprio, nessun processo aggiuntivo. Resta un binario, resta SQLite.
- Nessun account per chi chiede: la superficie è pubblica come la
  prenotazione.
- Degradazione totale: senza provider AI configurato la funzione non compare
  e non genera errori, esattamente come SMTP e come la traduzione BGG.

Non-obiettivi (sezione 12 per il dettaglio): OCR locale, chat persistita
lato server, ricerca cross-gioco, embedding.

## 2. Decisioni di architettura e perché

Tre vincoli del progetto hanno chiuso quasi tutte le porte, e conviene
lasciarli scritti perché non vengano ridiscussi a vuoto.

**Niente estensioni SQLite native.** `modernc.org/sqlite` è SQLite
transpilato in Go, non un binding CGO: non ha un linker dinamico, e
`load_extension` risponde `not authorized`. Verificato:

```
sqlite_version: 3.53.3
FTS5: SI                       (ENABLE_FTS5 nelle compile_options)
load_extension: SQL logic error: not authorized (1)
```

Quindi **sqlite-vec è escluso**. Adottarlo richiederebbe `mattn/go-sqlite3`,
cioè perdere `CGO_ENABLED=0`, la cross-compilazione e l'immagine distroless
static. Cambio sproporzionato al beneficio.

**Niente indice vettoriale, e non serve.** L'unica alternativa embedded
praticabile (`chromem-go`, pure-Go) fa comunque, per sua documentazione, *«an
exhaustive nearest neighbor search»*: brute force, come lo faremmo noi.
Misurato su questa macchina:

```
   200 chunk x 768 dim -> top-5 in   140µs
  5000 chunk x 768 dim -> top-5 in  3,12ms      (~150 manuali interi)
 50000 chunk x 768 dim -> top-5 in 32,7ms
```

E la ricerca è sempre filtrata per gioco, quindi l'ordine di grandezza reale
è 30-200 chunk. La scelta non era «brute force o indice vero»: era solo
*dove* mettere i vettori. `app.db` vince su una directory di file `.gob`
perché resta **un solo file da backuppare**, i chunk stanno in join con
`game_media` con `ON DELETE CASCADE` gratuito, e non aggiunge dipendenze.

**Il provider configurato non ha embedding.** OpenCode Zen
(`https://opencode.ai/zen/v1`, in uso oggi) non espone `/embeddings` in tutto
il catalogo. Una pipeline a embedding costringerebbe l'admin a configurare un
*secondo* provider solo per vettorializzare.

Da cui: **retrieval lessicale con FTS5**, che è già compilato dentro il
SQLite del progetto. Il chunk è l'unità di recupero — con gli embedding lo
sarebbe stato allo stesso modo: all'LLM torna sempre il *testo* dei chunk,
mai i vettori. Cambia solo il criterio di ordinamento, e resta sostituibile
in seguito senza toccare né il prompt né la UI.

### 2.1 Come si compensa la debolezza di FTS5

BM25 non conosce i sinonimi: `ripescare` non trova un manuale che dice
`pesca`. La compensazione **non** è un secondo indice, è il modello: un LLM
sa già che «pareggio» e «stesso punteggio» sono la stessa cosa, e quella
conoscenza è gratis. Perciò il tool accetta un **elenco** di parole chiave e
cerca ognuna separatamente. Verificato su testo di regole di prova:

```
cerca_nel_manuale(6 parole chiave) -> 1 chiamata, 215 token

[1] pag. 7  (trovato con: pila pesca esaurisce, fine partita)
[2] pag. 5  (trovato con: pila pesca esaurisce, rimescolare scarti)
[3] pag. 8  (trovato con: "stesso punteggio")
Nessun risultato per: pareggio, ripescare
```

La pagina 5 — *«quando la pila si esaurisce in una partita a due si
rimescolano gli scarti»* — è la risposta che conta, e una query singola
l'avrebbe mancata. La pagina 8 è arrivata solo via `"stesso punteggio"`,
mentre `pareggio` da solo non trovava nulla: **le varianti si coprono a
vicenda**. E la riga finale dice al modello quali tentativi lessicali sono
falliti, informazione che usa nel formulare la risposta.

## 3. Pacchetti Go

- **`internal/manuals`** (nuovo). Tutto il ciclo di vita del testo di un
  manuale.
  - `ExtractText(pdf []byte) ([]Page, error)` — estrazione dal layer testo,
    una `Page` per pagina del PDF.
  - `ExtractPageImages(pdf []byte) ([]Image, error)` — gli XObject
    `/DCTDecode` di un PDF scansionato, decodificati con `image/jpeg` della
    stdlib. **Zero dipendenze**, verificato sul manuale reale in `./data`:

    ```
    XObject JPEG trovati: 4
      pagina 1: jpeg 2110x3101, 436 KB
      pagina 2: jpeg 2112x3111, 437 KB
      pagina 3: jpeg 2110x3101, 503 KB
      pagina 4: jpeg 2115x3109, 529 KB
    ```

  - `HasTextLayer(pdf []byte) bool` — presenza di operatori di testo, decide
    quale dei due percorsi seguire.
  - `Chunk(pages []Page) []Chunk` — segmentazione (sezione 5.1).
  - `DetectHeading(page Page) string` — il titolo di sezione della pagina,
    per l'indice iniettato nel contesto.
  - `Store` — repository su `manual_page` e `manual_chunk`, con
    `Search(gameID int64, keywords []string) []Hit`.
- **`internal/ai`** (esteso). Accanto a `Translate`:
  - `Ask(ctx, req AskRequest) (AskResponse, error)` — il loop con i tool.
  - `Transcribe(ctx, img []byte, page int) (string, error)` — trascrizione di
    una pagina scansionata via modello multimodale.
  - `Searcher` — interfaccia iniettata dal chiamante, implementata da
    `manuals.Store`. `internal/ai` non conosce SQLite: riceve una funzione di
    ricerca, così il loop è testabile con un fake.
- **`internal/settings`** (esteso). `AIVisionModel`, stesso trattamento
  mascherato delle altre chiavi.
- **`internal/httpapi`** (esteso). Handler di sezione 7.

### 3.1 La libreria per l'estrazione testo: `ledongthuc/pdf`

Unica dipendenza Go nuova. Le alternative, con i dati verificati:

| Libreria | Stato | Verdetto |
|---|---|---|
| `rsc.io/pdf` | ⭐533, **archiviata** (2024-03) | Fuori: non si adotta una dipendenza archiviata |
| `pdfcpu/pdfcpu` | ⭐8827, attivissima | Fuori: **non estrae testo** — la issue #122 «Text extraction» è ancora aperta. È un toolkit di manipolazione |
| `dslipak/pdf` | ⭐91, fermo (2024-04) | Fuori: fork stagnante di `ledongthuc` |
| `klippa-app/go-pdfium` (wasm) | ⭐369, attiva | Fuori: qualità pdfium senza CGO, ma `pdfium.wasm` pesa 5,7 MB (sezione 12) |
| **`ledongthuc/pdf`** | ⭐618, push 2026-09-07 | **Scelta** |

`ledongthuc/pdf` ha **zero dipendenze transitive** (il suo `go.mod` è due
righe), è mantenuta, ed espone esattamente le due funzioni che servono:
`GetPlainText()` per il testo e `GetTextByRow()` per le righe con le
coordinate — le seconde alimentano `DetectHeading`, perché un titolo di
sezione si riconosce dal corpo del carattere più grande, non dal testo.

Il ragionamento sul perché non spendere di più: su un manuale di gioco —
denso, a colonne, con box laterali — *qualunque* estrattore di testo dà un
risultato mediocre. La qualità sta nel percorso vision, non nell'estrattore.
Spendere 5,7 MB di dipendenza per migliorare marginalmente il percorso meno
buono è comprare qualità nel posto sbagliato.

## 4. Modello dati

Nuova migrazione `0014_manual_qa.sql`.

```sql
-- Trascrizione del manuale, una riga per pagina. È modificabile dall'admin
-- e resta la fonte di verità: non è una cache dell'estrazione, che infatti
-- non viene mai rieseguita da sola.
CREATE TABLE manual_page (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_media_id INTEGER NOT NULL REFERENCES game_media(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL,
    text TEXT NOT NULL,
    -- heading: il titolo di sezione rilevato, se c'è. Alimenta l'indice
    -- iniettato nel contesto del modello (sezione 6.2).
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
    -- ordinale del chunk dentro la pagina, per allegare il vicino (5.2)
    seq INTEGER NOT NULL,
    text TEXT NOT NULL
);

-- game_id è denormalizzato di proposito: la ricerca filtra sempre per gioco,
-- e così è un indice B-tree invece di due join.
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

-- Modello multimodale per la trascrizione degli scan. Vuoto = nessuna
-- trascrizione automatica, l'admin scrive il testo a mano.
ALTER TABLE app_settings ADD COLUMN ai_vision_model TEXT;
```

Nessuna colonna per la stima dei token del manuale: si ricava con
`SELECT SUM(LENGTH(text)) FROM manual_page WHERE game_media_id = ?`, e una
colonna denormalizzata sarebbe solo un'occasione per andare fuori sincrono.

`ON DELETE CASCADE` su `game_media_id` significa che cancellare il manuale
porta via trascrizione e chunk: nessuna pulizia manuale, nessun orfano.

## 5. Ingestione

Parte da un'azione esplicita dell'admin sulla scheda gioco, non
automaticamente all'upload: la trascrizione di uno scan costa chiamate al
modello, e va decisa da chi paga.

```
manuale PDF già caricato (game_media type='file')
        │
        ▼  admin: "Prepara per le domande"
   HasTextLayer(pdf)?
        │
        ├── sì ──▶ ExtractText          ──▶  source='pdf_text'
        │
        └── no ──▶ ExtractPageImages    ──▶  Transcribe, una pagina per
                                             richiesta ──▶ source='vision'
        │
        ▼
   anteprima all'admin, pagina per pagina, ogni testo editabile
        │
        ▼  conferma
   manual_page  ──▶  Chunk  ──▶  manual_chunk (+FTS5 via trigger)
```

**Una richiesta per pagina**, non tutte insieme: se la pagina 3 fallisce non
si perdono le altre e si riprova solo quella. Il prompt di trascrizione dice
di trascrivere in markdown **senza riassumere**, di conservare tabelle ed
elenchi, e di ignorare le illustrazioni prive di testo.

**L'admin conferma sempre**, come già avviene per l'arricchimento BGG. Il
testo confermato è editabile per sempre: se la trascrizione esce sporca si
corregge, invece di restare bloccati. Un manuale senza PDF utilizzabile si
gestisce scrivendo il testo a mano (`source='manual'`) — quindi la funzione
esiste anche senza AI configurata.

### 5.1 Chunking

Taglio sui paragrafi, poi sulle frasi, fino a ~1000 caratteri, senza spezzare
una frase a metà; **100 caratteri di sovrapposizione** col chunk precedente,
perché una regola tagliata in due altrimenti non si trova né da un lato né
dall'altro. Ogni chunk porta `page_number` — è ciò che rende possibile la
citazione — e `seq`, l'ordinale nella pagina.

Il chunking è deterministico e rieseguibile: `manual_chunk` si può svuotare e
ricostruire da `manual_page` in qualunque momento, il che rende sicuro
cambiare i parametri in futuro.

### 5.2 Contesto attorno al risultato

Se il match cade sul primo o sull'ultimo chunk di una pagina, il payload
allega anche il chunk adiacente (`seq ± 1`, stessa pagina). Costa ~100 token
e toglie in radice la seconda chiamata del tipo «fammi leggere il resto».

## 6. L'agente

### 6.1 Quando il tool non serve affatto

Il modo più efficace di ridurre le chiamate al tool è non offrirlo. Se i
manuali stanno in un budget di **~6.000 token** (stimati come
`caratteri / 3`, conservativo per l'italiano), il testo entra **intero** nel
contesto e il tool non viene nemmeno dichiarato nella richiesta.

La soglia si valuta sulla **somma di tutti i manuali del gioco**, non su uno
solo: poiché la ricerca copre tutte le lingue (6.4), un criterio per-manuale
porterebbe al caso incoerente di un manuale inline e un altro raggiungibile
solo via tool. Sotto soglia entrano tutti, sopra soglia nessuno.

Il manuale reale presente oggi in `./data` sono 4 pagine, ~3.500 token
stimati: per quel gioco il numero di chiamate al tool è **zero**. Su un
catalogo di regolamenti da associazione questo copre probabilmente la
maggioranza dei giochi; il tool serve ai tomi da 40 pagine.

### 6.2 L'indice è sempre in contesto

Per i manuali oltre soglia, il prompt di sistema include l'indice ricavato
dagli `heading` di `manual_page` (~200 token):

```
Indice del manuale: Preparazione p.2 · Turno del giocatore p.3 ·
Fase di Upkeep p.4 · Rimescolare p.5 · Fine partita p.7 ·
Conteggio e pareggi p.8
```

È questo che elimina la chiamata *esplorativa*. Un modello che vede l'indice
sceglie le parole chiave giuste al primo colpo; uno cieco tira a indovinare,
sbaglia, e richiama.

### 6.3 Il tool

Uno solo. Niente `leggi_pagina(n)` accanto: ogni tool in più è un round trip
in più.

```json
{
  "name": "cerca_nel_manuale",
  "description": "Cerca nel regolamento del gioco. Passa un elenco di parole chiave alternative in un'unica chiamata: la ricerca è lessicale, quindi più varianti trovano più cose.",
  "parameters": {
    "type": "object",
    "properties": {
      "parole_chiave": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Da 3 a 8 varianti: sinonimi, il termine tecnico e quello colloquiale, singolare e plurale. Esempio: [\"pareggio\", \"stesso punteggio\", \"parità\", \"spareggio\"]. Supporta \"frase esatta\", OR, AND e prefissi con *."
      }
    },
    "required": ["parole_chiave"]
  }
}
```

**`game_id` non è un parametro**: lo inietta il server dalla richiesta. Il
modello non deve poter cambiare gioco, altrimenti una domanda su un gioco
può leggere il manuale di un altro.

La descrizione del parametro è dove sta il lavoro: è lei a far arrivare le
varianti in un colpo solo, senza doverlo ordinare nel prompt di sistema. Il
modello resta libero di chiamare il tool quante volte vuole — noi rendiamo
inutile la seconda chiamata, non la vietiamo.

**Payload di ritorno**, testo semplice, top-2 per parola chiave, unione
deduplicata (~200-600 token):

```
[1] Regolamento base (it), pag. 7  ·  trovato con: pila pesca esaurisce, fine partita
La partita termina immediatamente quando la pila di pesca si esaurisce...

[2] Regolamento base (it), pag. 5  ·  trovato con: rimescolare scarti
Quando la pila di pesca si esaurisce durante una partita a due giocatori...

Nessun risultato per: pareggio, ripescare
```

`snippet()` di FTS5 **non** si usa nel payload: un frammento evidenziato è
troppo corto per contenere una regola intera. Può servire a evidenziare in
UI, se lo vorremo.

La riga «Nessun risultato per: …» è **parte del contratto**, non un errore:
dice al modello quali ipotesi lessicali sono cadute. Se nessuna parola trova
nulla, il payload è `Nessun risultato per nessuna parola chiave.` e il
modello è liberissimo di riprovare.

**Sanitizzazione.** La sintassi FTS5 si rompe su un apostrofo — verificato:
`fts5: syntax error near "'"`. Gli operatori però sono utili (`pesc*`,
`"stesso punteggio"`), quindi non si filtrano: si prova la parola chiave così
com'è e, **solo se SQLite dà errore di sintassi**, si ricade su una versione
neutralizzata (racchiusa in doppi apici, con i doppi apici interni
raddoppiati). Robusto senza perdere potenza.

### 6.4 Multi-lingua

Un gioco può avere il manuale in più `game_language`. La ricerca copre tutti
i manuali del gioco; la lingua della pagina pubblica ha precedenza a parità
di rank, e ogni risultato dichiara la propria lingua, così il modello può
dire «il regolamento inglese, pag. 12».

### 6.5 Il loop

```
Ask(ctx, domanda, storico, gameID)
  │
  ├─ manuale sotto soglia?  ──▶ testo intero nel prompt, nessun tool
  │
  ├─ POST /chat/completions  (tools + indice + storico)
  │
  ├─ finish_reason == "tool_calls"  ──▶ Searcher.Search(...) ──▶ rimanda
  │                                      il risultato come messaggio tool
  │
  └─ finish_reason == "stop"  ──▶ risposta + pagine citate
```

Due guardie, nessuna delle quali è un limite sull'utente:

- **Timeout 60s** sul context, lo stesso `requestTimeout` che `internal/ai`
  usa già. Un handler HTTP senza timeout tiene una goroutine occupata per
  sempre.
- **5 iterazioni massime**, contro un modello che si incarta a richiamare lo
  stesso tool in ciclo. È un bug del modello, non un utente di cui diffidare:
  va loggata, non mostrata all'utente.

Lo storico arriva dal browser (sezione 8.4) e viene passato al modello come
messaggi precedenti.

### 6.6 Prompt di sistema

Sostanza, non testo definitivo:

- Sei l'assistente regole del gioco *{nome}* per un'associazione di giochi da
  tavolo. Rispondi in italiano, breve, come si parla a un tavolo.
- Rispondi **solo** con quello che c'è nel manuale. Se il manuale non lo dice,
  dillo chiaramente invece di dedurre: al tavolo una regola inventata fa
  danno.
- **Cita sempre la pagina** da cui viene la risposta.
- Se la ricerca non trova nulla, prova con altre parole prima di arrendersi.
- Non inventare nomi di carte, valori o numeri che non hai letto.

## 7. API HTTP

Protette (admin), accanto alle rotte media esistenti:

| Metodo | Rotta | Effetto |
|---|---|---|
| `POST` | `/api/games/{id}/languages/{lang}/media/{mediaId}/extract` | Estrae o trascrive, **non salva**: restituisce le pagine proposte per l'anteprima |
| `GET` | `/api/games/{id}/languages/{lang}/media/{mediaId}/pages` | Le pagine salvate, per la rilettura e la modifica |
| `PUT` | `/api/games/{id}/languages/{lang}/media/{mediaId}/pages` | Salva le pagine confermate e **ricostruisce** i chunk in transazione |
| `DELETE` | `/api/games/{id}/languages/{lang}/media/{mediaId}/pages` | Rimuove trascrizione e chunk |

Pubbliche:

| Metodo | Rotta | Effetto |
|---|---|---|
| `POST` | `/api/games/{id}/ask` | La domanda. Rate limit `newRateLimiter(20, time.Minute)` per IP |
| `GET` | `/api/games/{id}` | Esteso con `canAsk: bool` |

`canAsk` è vero solo se **entrambe** le condizioni valgono: provider AI
configurato, e il gioco ha almeno una `manual_page`. È il flag che governa la
comparsa di ogni punto d'ingresso nel frontend — non un pulsante disabilitato
con una spiegazione, ma l'assenza del pulsante.

Il rate limit a 20/minuto tiene conto del NAT: a un tavolo diverse persone
condividono l'IP del wifi del circolo.

**Contratto con deep-chat.** Richiesta come la manda il componente,
risposta nel formato che si aspetta:

```
POST /api/games/12/ask
{"messages": [{"role": "user", "text": "finite le carte che si fa?"}]}

200 OK
{"text": "La partita finisce subito: si completa il giro in corso e si\ncontano i punti. In due giocatori invece si rimescolano gli scarti\ne si continua.\n\n_Regolamento base — [pag. 7](/api/uploads/abc.pdf#page=7),\n[pag. 5](/api/uploads/abc.pdf#page=5)_"}
```

Le citazioni sono link markdown al PDF con il fragment `#page=N`: deep-chat
rende il markdown (dipende da `remarkable`), e la quasi totalità dei viewer
PDF onora `#page=`. **È questo che chiude il cerchio**: la risposta generata
non va creduta sulla fiducia, si apre il manuale a quella pagina e si
verifica. In una discussione sulle regole è la differenza fra un aiuto e un
oracolo di cui diffidare.

Se il provider non è configurato l'handler risponde 404 come una rotta
inesistente, coerentemente con `ai.ErrNotConfigured`.

## 8. Frontend — dove va la chat

Lo scenario non è «un utente al computer esplora un'app»: è **una persona in
piedi, con le mani occupate, che ha una discussione aperta al tavolo**. E chi
ha quel dubbio **può non aver prenotato quel gioco** — sta guardando cosa
giocare, o gioca alla copia di qualcun altro. Quindi la chat vive sulla
**scheda gioco pubblica** (`/games/:id`), l'unica pagina che tutti
raggiungono, e non su una rotta o su una pagina legata alla prenotazione.

Un solo componente nuovo, `ManualChat.vue`, montato in `GameDetailView.vue` e
condizionato a `canAsk`. Due forme secondo la larghezza. La soglia è **1100px**, non 900: `.app-page`
è `max-width: 56rem`, e una sidebar da 22rem lì dentro lascerebbe al testo
34rem, sotto la misura leggibile. Sotto quella soglia vale la forma mobile,
che copre bene anche i tablet.

### 8.1 Desktop (≥1100px): sidebar destra collassabile

`GameDetailView` passa a due colonne: contenuto a sinistra, sidebar della
chat a destra, ~360px.

```
┌───────────────────────────────┬──────────────────┐
│ ← Indietro                    │ Chiedi al        │
│ Wingspan                      │ manuale       ⟩  │  ⟩ = collassa
│ Proprietario: … · Classifica  ├──────────────────┤
│                               │                  │
│ ┌───────────────────────────┐ │  conversazione   │  scroll proprio
│ │ Scheda                    │ │                  │
│ └───────────────────────────┘ │                  │
│ ┌───────────────────────────┐ ├──────────────────┤
│ │ Media                     │ │ [ chiedi… ]  🎤 ➤│  ancorato
│ └───────────────────────────┘ └──────────────────┘
│                               ↑ sticky, altezza piena
└───────────────────────────────┘
```

La sidebar è `position: sticky` con altezza propria e **il suo** contenitore
di scroll: la colonna di sinistra scorre, la chat resta ferma. È questo che
evita il difetto classico della chat dentro una pagina che scorre, dove il
messaggio nuovo finisce fuori dalla vista.

Collassata diventa una barra verticale stretta col titolo ruotato e l'icona,
cliccabile per riaprirla. Lo stato sta in `localStorage` per gioco-agnostico
(una preferenza dell'utente, non del gioco), con `aria-expanded` sul
controllo.

### 8.2 Mobile (<1100px): bottone tondo e full screen

Un bottone tondeggiante **in basso al centro**, sempre visibile mentre si
scorre la scheda:

```
┌──────────────────┐         ┌──────────────────┐
│ Wingspan         │         │ Chiedi al manuale│ ×
│                  │         ├──────────────────┤
│ Scheda           │  tap    │                  │
│ Media            │  ────▶  │  conversazione   │
│                  │         │                  │
│      ╭────────╮  │         ├──────────────────┤
│      │ 💬 Regole│ │        │ [ chiedi… ]  🎤 ➤│
│      ╰────────╯  │         └──────────────────┘
└──────────────────┘          `<dialog>` a tutto schermo
```

- Posizione: `position: fixed; bottom: calc(1rem + env(safe-area-inset-bottom))`, centrato — il progetto non ha token di spaziatura, le misure sono in `rem` dirette. Il `safe-area-inset-bottom` evita che finisca sotto la barra home dell'iPhone.
- La pagina riceve un padding inferiore pari all'altezza del bottone, così non copre mai l'ultimo contenuto.
- Al tap si apre un **`<dialog>` nativo** con `showModal()`, a tutto schermo (`100dvh` — `dvh` e non `vh`, perché su iOS Safari `vh` include la barra degli indirizzi e taglia l'input fuori schermo).

**Si chiude in tre modi**, e nessuno costa codice: la × in testata, il tasto
Esc e il focus trap sono comportamento nativo di `<dialog>`. È lo stesso
primitivo che `ModalDialog.vue` usa già — nel suo commento: *«focus trap, Esc
e backdrop li gestisce il browser, quindi zero dipendenze e zero gestione
manuale del focus»*. Niente scroll lock fatto a mano, niente `inert`
applicato da noi: `showModal()` rende inerte il resto della pagina.

Non riuso `ModalDialog.vue` così com'è — è dimensionata per una conferma, non
per un pannello a tutto schermo — ma `ManualChat.vue` adotta lo stesso
pattern `<dialog>`.

### 8.3 Il peso: deep-chat si monta al primo tocco

Nessuno che apre una scheda gioco deve pagare 105 KB di chat che forse non
userà. Quindi la sidebar, **anche aperta**, nel suo stato di riposo è markup
nostro: titolo, una riga di inquadramento, tre domande suggerite come
pulsanti, e un finto campo di input delle stesse dimensioni di quello vero.

deep-chat si monta al **primo gesto** — clic sul campo, clic su una domanda
suggerita, o tap sul bottone tondo — via
`defineAsyncComponent(() => import('deep-chat'))`, lo stesso pattern già in
uso per `EventMap.vue`. Se l'ingresso è una domanda suggerita, quella viene
inviata subito dopo il mount.

Il risultato: la sidebar può stare **aperta per default** su desktop, dove la
scoperta conta, senza costare un byte a chi legge solo la scheda.

### 8.4 Configurazione di deep-chat

```html
<deep-chat
  connect='{"url": "/api/games/12/ask", "method": "POST"}'
  requestBodyLimits='{"maxMessages": 40}'
  speechToText='{"webSpeech": {"language": "it-IT"}}'
  textInput='{"placeholder": {"text": "Chiedi una regola…"}}'
/>
```

**`maxMessages: 40`, non 20**: il parametro conta i *messaggi*, non i turni,
quindi 20 turni sono 40 messaggi. E il default è insidioso — non impostandolo,
*«the request will only include the input text/files»*: la conversazione non
arriverebbe affatto al server.

Lo storico vive **solo nel browser**: nessuna tabella, nessuna sessione per un
utente anonimo. Chiudendo il dialog o ricaricando si ricomincia, ed è
accettabile — un dubbio al tavolo si esaurisce in due minuti. Su desktop,
collassare la sidebar **non** smonta il componente, quindi la conversazione
sopravvive al collasso.

Perché deep-chat e non un componente scritto in casa: è l'unica libreria chat
mantenuta e framework-agnostica (`vue-advanced-chat` fermo a dicembre 2025 e
pensato per messaggistica multi-stanza, `vue3-beautiful-chat` a gennaio 2025,
`botui` al 2022, `@chatscope` solo React), e porta gratis streaming,
auto-scroll, rendering markdown e speech-to-text. Peso misurato:

```
deep-chat  dist/deepChat.bundle.js  105 KB gzip
deep-chat  dist/deepChat.js         159 KB gzip
app attuale index.js (tutta)         70 KB gzip
```

**Non da CDN**: sposterebbe soltanto i byte aggiungendo DNS, TCP e TLS verso
un'origine terza, e soprattutto renderebbe la chat dipendente da un servizio
esterno raggiungibile — contro la promessa selfhost del progetto, proprio nel
posto (la sala di un circolo, con il wifi che c'è) dove serve affidabile.

### 8.5 Stati

- **Riposo** (markup nostro, sezione 8.3): una riga di inquadramento e tre
  domande d'esempio toccabili. Se il manuale ha almeno tre `heading` si usano
  i primi tre resi come domanda («Fine partita» → «Come finisce la partita?»)
  con una tabella di trasformazione fissa nel componente; altrimenti tre
  domande statiche («Come finisce la partita?», «In quanti si gioca?», «Come
  si contano i punti?»). Una chat vuota su un telefono non suggerisce cosa
  farne.
- **In attesa**: l'indicatore di deep-chat. Con il tool di mezzo la risposta
  può richiedere qualche secondo; la percezione migliora in fase 2 con lo
  streaming.
- **Risposta**: testo breve e, in coda, le pagine citate come link al PDF.
- **Nessuna risposta nel manuale**: il modello lo dice, ed è il comportamento
  giusto — non va mascherato.
- **Errore**: messaggi in italiano via `errorMessages.overrides`.
- **Avvertenza**: una riga fissa e discreta in testata — *risposte generate
  dal manuale: controlla la pagina citata*. Va detta una volta, non a ogni
  messaggio.

### 8.6 Vestizione e dettatura

deep-chat è un web component in **shadow DOM**: i token di `app.css` non ci
cascano dentro. Si veste con le sue proprietà — `style` per il contenitore,
`auxiliaryStyle` per l'interno, `messageStyles` per le bolle — passando i
valori dei token esistenti. Serve una sezione nuova in `DESIGN.md`, altrimenti
resta un'isola grafica e il prossimo che tocca i colori non sa dove guardare.
Il bottone tondo è invece markup nostro, quindi segue i token direttamente, e
rispetta il blocco `prefers-reduced-motion` già presente in `app.css`.

La **dettatura** usa la Web Speech API del browser: `webSpeech: true`, nessuna
chiave (la variante Azure, che una chiave la vuole, non si usa). Al tavolo,
con le mani occupate dalle carte, dettare invece di digitare è utile davvero.
Tre limiti da documentare nel `README`:

- **Serve HTTPS.** La Web Speech API è ristretta ai secure context: su
  `http://192.168.1.50:8080` — cioè come un circolo farebbe girare l'app in
  LAN — il microfono non funziona. Va detto a chi installa.
- **Firefox non la supporta** (Chrome, Edge e Safari sì). La tastiera resta la
  strada principale, la dettatura un extra che degrada da sé.
- **L'audio esce dal dispositivo**: l'implementazione di Chrome lo manda ai
  server Google per il riconoscimento. Non al nostro server, e solo se
  l'utente tocca il microfono — ma un progetto che promette «nessun servizio
  esterno obbligatorio» lo dichiara invece di lasciarlo scoprire.

### 8.7 Arrivare alla chat da altrove

La scheda gioco accetta `?chat=1`: con quel parametro la sidebar è aperta su
desktop e il dialog si apre subito su mobile. Serve a due rimandi, entrambi
secondari rispetto all'ingresso principale:

- **`ManageBookingView`** (`/prenotazione/:code`) — la pagina che il
  partecipante ha già nella mail di conferma: un'azione accanto al nome del
  gioco prenotato, «Dubbi sulle regole?», che porta a
  `/games/:id?chat=1`. Dal tavolo è un tap.
- **`EventDetailView`** (`/events/:id`) — per ogni gioco in programma, utile a
  chi sta scegliendo cosa giocare.

Nessuno dei due ospita la chat: la portano dove vive.

### 8.8 Amministrazione

In `GameAdminDetailView.vue`, dentro il pannello media, accanto a ogni manuale
PDF: stato («non preparato» / «12 pagine indicizzate»), azione «Prepara per le
domande», e l'anteprima pagina per pagina con i testi editabili. Il
salvataggio è unico e ricostruisce i chunk.

Se `ai_vision_model` non è configurato e il PDF è uno scan, l'anteprima si
apre con le pagine vuote e un avviso: si può scrivere il testo a mano. La
funzione non si blocca perché manca l'AI.

## 9. Impostazioni

Un campo nuovo in `SettingsView.vue`, nel blocco AI esistente: **modello per
la lettura dei manuali scansionati**, con l'indicazione che serve un modello
multimodale e che il campo è opzionale.

Nota per la documentazione: il modello configurato oggi,
`deepseek-v4-flash`, è **solo testo**. La variante multimodale sullo stesso
endpoint OpenCode Zen è `deepseek-v4-flash-vision-exp`. Un utente che lascia
lo stesso modello nei due campi vedrà la trascrizione fallire, quindi
l'etichetta lo dice esplicitamente.

## 10. Test

Backend, in Docker (`go test ./...`):

- **`internal/manuals`**
  - `HasTextLayer` su un PDF con testo e su uno scansionato.
  - `ExtractPageImages` sul PDF reale in `./data` come fixture: 4 JPEG,
    dimensioni attese, decodificabili.
  - `Chunk`: rispetto del limite, nessuna frase spezzata, sovrapposizione
    presente, `seq` progressivo, determinismo su due esecuzioni.
  - `DetectHeading` su pagine con e senza titolo.
  - `Search`: parola chiave che trova, una che non trova (deve comparire in
    «Nessun risultato per»), deduplicazione con annotazione `trovato con`
    multipla, **parola chiave con apostrofo** (la regressione trovata in
    fase di design), filtro per `game_id` che non sconfina su un altro gioco,
    chunk adiacente allegato ai bordi di pagina.
- **`internal/ai`**
  - `Ask` con `Searcher` fake: una chiamata al tool, risultato reiniettato,
    risposta finale.
  - Manuale sotto soglia: la richiesta **non** dichiara i tool.
  - Manuale sopra soglia: l'indice è nel prompt.
  - Guardia delle 5 iterazioni con un fake che chiama il tool all'infinito.
  - `ErrNotConfigured` senza provider.
  - `Transcribe`: richiesta ben formata con la parte immagine.
- **`internal/httpapi`**
  - `POST /api/games/{id}/ask` con provider fake: 200 e formato `{"text":…}`.
  - 404 senza provider configurato.
  - Rate limit oltre soglia.
  - `canAsk` in `GET /api/games/{id}` nelle quattro combinazioni.
  - `extract` non scrive nulla; `PUT pages` scrive e ricostruisce i chunk.
  - Cancellare il media porta via pagine e chunk (cascata).

Frontend: `npm run build` (che fa anche il type-check con `vue-tsc`).

**Ultimo task obbligatorio**: `/impeccable` sulla superficie modificata —
`ManualChat.vue` nelle sue due forme (sidebar desktop e dialog full screen),
il bottone tondo, i due rimandi con `?chat=1` e il pannello di
amministrazione.

## 11. Fasi

**Fase 1** — migrazione, `internal/manuals` (estrazione, chunking, ricerca
FTS5), `Ask` con il tool, endpoint pubblico, `ManualChat.vue` con deep-chat
(sidebar desktop collassabile + bottone tondo e dialog su mobile), i due
rimandi `?chat=1`, pannello admin con anteprima e conferma, impostazione
`ai_vision_model`, sezione in `DESIGN.md`, note nel `README`.

**Fase 2**, se serve dopo l'uso reale:

- **Streaming**. Prima richiesta non-streamed (i `tool_calls` in streaming
  arrivano a delta ed è codice noioso), risposta finale in SSE. Al tavolo la
  latenza percepita conta, ma non blocca la consegna.
- **Embedding**. Solo se le risposte risultano deboli *e* si configura un
  provider con `/embeddings`: colonna vettori su `manual_chunk`, modello
  registrato accanto ai vettori, fusione reciprocal-rank con BM25. Il tool
  non cambia firma, quindi né prompt né UI si toccano.

## 12. Non-obiettivi

- **OCR locale**: tesseract richiede CGO o un binario esterno, incompatibile
  con distroless static. Le scansioni si leggono col modello multimodale.
- **Rendering di pagine PDF a immagine**: non esiste un renderer pure-Go
  maturo. L'unica via senza CGO sarebbe `klippa-app/go-pdfium` in modalità
  WebAssembly su Wazero, che funziona davvero — ma imbarca un
  `pdfium.wasm` da **5,7 MB**, in un progetto il cui frontend compilato sta
  in 540 KB. Non serve: uno scan ha già le immagini dentro, un PDF con layer
  testo si legge come testo, e i due percorsi si coprono a vicenda. Il caso
  scoperto — un PDF con layer testo di cui servirebbe la *figura* — non si
  gestisce.
- **Chat persistita lato server**: nessuno storico, nessuna sessione per
  utenti anonimi.
- **Ricerca su più giochi**: la domanda è sempre su un gioco.
- **Vector store**: motivato in sezione 2.
- **QR da stampare per il tavolo**: l'indirizzo `/games/:id?chat=1` lo rende
  banale in seguito, ma non è in questa fase.

## 13. Rischi aperti

- **Qualità della trascrizione degli scan.** Un manuale di gioco è
  graficamente denso: colonne, box laterali, icone. Un modello economico può
  restituire testo disordinato. Mitigazione: l'admin vede e corregge prima
  del salvataggio, e il testo resta editabile per sempre. Da valutare sul
  manuale reale come prima verifica dell'implementazione, non dopo.
- **Qualità dell'estrazione dai PDF con layer testo.** `ledongthuc/pdf`
  (scelta motivata in 3.1) non ricostruisce l'impaginazione: su un manuale a
  più colonne le colonne possono uscire interlacciate. Mitigazione a livello
  di disegno, non di dipendenza: quel manuale si manda per il percorso
  vision, che su impaginazioni dense dà comunque risultati migliori di
  qualunque estrattore. Da provare su un PDF reale a più colonne nel piano.
- **Tool calling.** Non è un rischio: l'utente conferma che
  `deepseek-v4-flash` gestisce i tool, già in uso in altri suoi progetti.
  Resta solo la nota che i manuali sotto soglia (6.1) non usano tool affatto,
  quindi quella parte è indipendente dal supporto del modello.
- **Vestizione in shadow DOM.** Se deep-chat si rivelasse ostile ai token del
  progetto, il fallback è un componente scritto in casa (~200 righe): è una
  decisione reversibile e circoscritta a un file.
