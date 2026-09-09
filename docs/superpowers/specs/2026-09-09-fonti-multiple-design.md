# Fonti multiple per le domande sul manuale — Design

Data: 2026-09-09

Supera in parte `2026-09-08-domande-sul-manuale-design.md`: le sezioni 4
(modello dati), 5 (ingestione), 6.1-6.3 (soglia inline, indice, tool) e 7
(API admin) di quella spec sono sostituite da questa. Il resto — la ricerca
FTS5 con più parole chiave, il loop dell'agente, la chat pubblica — resta in
vigore.

## 1. Panoramica e obiettivi

La feature attuale sa rispondere leggendo **un manuale PDF per gioco**, e
cita la pagina. Due limiti sono emersi appena si guarda oltre:

- **Il PDF non è l'unico formato** in cui esiste un regolamento: ci sono
  `.txt`, `.md`, `.docx`.
- **Il numero di pagina vale meno di quanto pensassi**: il file caricato non
  corrisponde necessariamente al manuale cartaceo che i giocatori hanno sul
  tavolo, e una risposta utile mette insieme **più documenti** — un
  regolamento, un'errata, e in futuro le FAQ di BoardGameGeek.

Da cui l'obiettivo: **generalizzare la fonte**. Non «un manuale paginato»,
ma «un pezzo di testo con un riferimento citabile», qualunque cosa sia la
fonte. Il modello dati e il contratto del tool nascono già capaci di
ospitare le FAQ, che restano da costruire in una fase successiva.

Due decisioni di prodotto prese in fase di brainstorming valgono come
vincoli, non come opzioni:

- **L'inserimento manuale del testo non deve esistere.** Un admin che fa
  copia-incolla del regolamento è un'esperienza da eliminare, non da
  migliorare. L'OCR lo fa il provider.
- **Il testo estratto non si conserva.** Si salvano solo i chunk. Se serve
  ri-chunkare, si ri-estrae — accettando il costo.

## 2. Cosa cambia, e perché

La spec precedente aveva tarato il modello dati su ciò che si è rivelato il
dettaglio meno importante: la pagina. `manual_page` esisteva **solo** per
poter scrivere «pag. 7» e costruire il link `#page=7`. Con più fonti e con
pagine che non corrispondono al cartaceo, quella tabella non guadagna il suo
posto.

Tre conseguenze, tutte volute:

**Il percorso inline sparisce.** L'idea «il modo più efficace di ridurre le
chiamate al tool è non offrirlo» dipendeva dall'avere il testo completo del
manuale per metterlo nel prompt. Senza testo salvato non si può, e
ricostruirlo dai chunk duplicherebbe le sovrapposizioni. Quindi il tool si
dichiara **sempre**, anche per un manuale da 4 pagine, e ogni domanda fa
almeno una ricerca. `InlineCorpusMaxChars`, `AskRequest.CorpusText` e
`FormatCorpus` cadono.

**Non c'è più una via d'uscita.** Togliendo l'inserimento manuale, un
documento che il modello non riesce a leggere significa che **quel gioco non
ha la chat**. È un cambio di comportamento visibile, non un dettaglio
interno: prima l'admin poteva scrivere il testo a mano. Il prezzo si paga
nel messaggio d'errore, che diventa parte del prodotto (sezione 5.4).

**L'indice nel prompt diventa più importante e finalmente funziona.** Con il
tool sempre in uso, l'indice che evita la ricerca esplorativa conta più di
prima. E qui la spec precedente aveva un difetto reale: `DetectHeading`
riceve il testo di una pagina intera ma ne guarda **solo la prima riga**,
quindi produce al massimo **un titolo per pagina**, e solo se la pagina
comincia con un titolo. Una pagina con tre sezioni ne perdeva due. Il
markdown come rappresentazione interna (sezione 4) risolve questo, non solo
i formati nuovi.

## 3. Modello dati

Migrazione `0015_game_sources.sql`. Elimina `manual_page`, `manual_chunk` e
i suoi tre trigger; crea una tabella sola. **Nessun dato da migrare**: le
tabelle sono vuote, nessun manuale è ancora stato preparato — la finestra
per ristrutturare senza costi è adesso.

```sql
DROP TRIGGER IF EXISTS manual_chunk_au;
DROP TRIGGER IF EXISTS manual_chunk_ad;
DROP TRIGGER IF EXISTS manual_chunk_ai;
DROP TABLE IF EXISTS manual_chunk_fts;
DROP TABLE IF EXISTS manual_chunk;
DROP TABLE IF EXISTS manual_page;

CREATE TABLE game_source_chunk (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,

    -- NULL per le fonti che non sono un media caricato (le FAQ di BGG).
    -- Quando c'è, la cascata porta via i chunk se il manuale si cancella:
    -- è l'unico modo di tenere quella garanzia senza costringere una FAQ a
    -- inventarsi una riga in game_media.
    game_media_id INTEGER REFERENCES game_media(id) ON DELETE CASCADE,

    -- Governa come si costruisce il link della citazione: un documento si
    -- apre da /api/uploads, una FAQ è già un URL.
    reference_type TEXT NOT NULL CHECK (reference_type IN ('document', 'faq')),

    -- Ciò che si cita: il nome del file per un documento, l'URL della
    -- conversazione per una FAQ.
    reference TEXT NOT NULL,

    -- Il punto dentro la fonte, e sempre nella forma che va scritta nella
    -- citazione: "pagina 3", 'sezione «Fase di Upkeep»', "commento del
    -- 15/07/2023 08:00". Nullable perché una fonte può non averne.
    reference_detail TEXT,

    -- Il titolo della sezione in cui il chunk cade, dal markdown. Vive solo
    -- qui: alimenta l'indice iniettato nel prompt, e NON esce nella risposta
    -- del tool (sezione 5.2).
    heading TEXT,

    language_code TEXT,       -- NULL per una FAQ
    seq INTEGER NOT NULL,     -- ordinale dentro la fonte, non dentro la pagina
    text TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_source_chunk_game ON game_source_chunk(game_id);
CREATE INDEX idx_source_chunk_ref ON game_source_chunk(game_media_id, seq);

-- FTS5 in external-content, come prima: indicizza senza duplicare il testo.
CREATE VIRTUAL TABLE game_source_chunk_fts USING fts5(
    text, content='game_source_chunk', content_rowid='id'
);
CREATE TRIGGER game_source_chunk_ai AFTER INSERT ON game_source_chunk BEGIN
    INSERT INTO game_source_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER game_source_chunk_ad AFTER DELETE ON game_source_chunk BEGIN
    INSERT INTO game_source_chunk_fts(game_source_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
END;
CREATE TRIGGER game_source_chunk_au AFTER UPDATE ON game_source_chunk BEGIN
    INSERT INTO game_source_chunk_fts(game_source_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
    INSERT INTO game_source_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;
```

**`seq` è l'ordinale nella fonte, non nella pagina.** Nella spec precedente
era per-pagina perché la pagina era l'unità. Con le sezioni, «il chunk
vicino» è quello prima o dopo nel documento — che è ciò che serve quando una
regola continua oltre il taglio.

## 4. Ingestione: il markdown come rappresentazione interna

`storage.ManualCategory` accetta quattro tipi MIME:

| MIME | Estensione |
|---|---|
| `application/pdf` | `.pdf` |
| `text/plain` | `.txt` |
| `text/markdown` | `.md` |
| `application/vnd.openxmlformats-officedocument.wordprocessingml.document` | `.docx` |

Il limite di 20 MB resta.

L'ingestione è **un'azione sola** — nessuna proposta da confermare:

```
file → markdown → sezioni → chunk → indice FTS5
```

### 4.1 Da file a markdown

Quattro strade che convergono su una rappresentazione comune. Questa è
l'idea portante della spec: *chi produce il testo lo produce già come
markdown, e trovare le sezioni diventa una cosa sola per tutte le fonti.*

| Fonte | Come si ottiene il markdown |
|---|---|
| PDF scansionato | `ExtractPageImages` → `Transcribe` per pagina. **Il prompt di trascrizione chiede già markdown**: cambia solo che ora lo parsiamo invece di scartarlo |
| PDF con layer testo | `ExtractText` per pagina → testo piatto → **una chiamata al modello** che aggiunge i titoli |
| `.md` | è già markdown, nessun lavoro |
| `.docx` | `archive/zip` + `encoding/xml` su `word/document.xml`: `w:pStyle` `Heading1/2/3` → `#`/`##`/`###`. **Zero dipendenze nuove** |
| `.txt` | testo piatto → una chiamata al modello che aggiunge i titoli |

**Il PDF con layer testo diventa dipendente dal provider** dove prima non lo
era: l'estrazione è locale, ma la segmentazione no. È il prezzo di avere un
indice che funziona, e con il gate della sezione 6 non introduce un caso
nuovo.

### 4.2 La segmentazione generata dal modello

Serve a `.txt` e ai PDF con layer testo — le fonti che arrivano senza alcuna
struttura. Una chiamata per documento: dentro il testo piatto, fuori lo
stesso testo con i titoli markdown inseriti.

Il prompt chiede di **non riscrivere il contenuto**, solo di inserire titoli
dove una sezione comincia. Un modello che parafrasa il regolamento mentre lo
segmenta introdurrebbe errori invisibili — la stessa ragione per cui il
prompt di trascrizione dice «senza riassumere».

**Documenti lunghi.** Un regolamento da 40 pagine sono ~60.000 token, e
passarlo intero può eccedere la finestra del modello. La soglia si esprime in
**caratteri** e si stima come il resto del progetto (`caratteri / 3`,
conservativo per l'italiano): sopra ~60.000 caratteri — cioè ~20.000 token,
un quinto abbondante di margine su una finestra da 128k — si segmenta a
**finestre sequenziali**, passando a ogni finestra l'ultimo titolo della
precedente come contesto, così la continuità non si rompe al confine. Il
numero è prudente di proposito: sbagliare per eccesso costa una chiamata in
più, sbagliare per difetto fa fallire l'ingestione del documento più
importante che l'associazione possiede. Non è elegante, è prevedibile.

### 4.3 Da markdown a sezioni a chunk

Si parsano i titoli markdown (`#` … `######`); ogni sezione diventa un
blocco `(heading, corpo)`; il corpo si spezza con il chunker esistente
(`MaxChunkChars`, `ChunkOverlapChars`, taglio sui paragrafi e sulle frasi).
Ogni chunk porta il `heading` della sezione in cui cade.

Il testo che precede il primo titolo è una sezione senza `heading` — non un
errore: molti regolamenti aprono con un paragrafo introduttivo.

### 4.4 Pagine e sezioni non coincidono

Un PDF ha entrambe le cose: le pagine servono a `reference_detail`, le
sezioni a `heading`, **e una sezione attraversa le pagine**.

Si trascrive per pagina, quindi si concatena il markdown delle pagine in
ordine tenendo l'offset in cui ognuna comincia; si parsano le sezioni sul
testo unito; poi, per ogni chunk, `reference_detail` è **la pagina in cui il
chunk inizia**. È l'unica risposta onesta a «da quale pagina viene questo
pezzo» e costa un conteggio di offset.

### 4.5 Idempotenza

«Prepara» sostituisce **tutti** i chunk di quella fonte in una transazione:
non esiste uno stato in cui i chunk sono nuovi e l'indice è vecchio, né uno
in cui metà documento è indicizzato.

## 5. Il contratto del tool e le citazioni

### 5.1 La risposta del tool

Il tool si **rinomina `cerca_nelle_fonti`**: continuerà a cercare anche nelle
FAQ, e un nome che dice «manuale» al modello lo porterebbe a non considerare
una fonte che invece ha davanti. Continua ad accettare un **elenco di parole
chiave** (spec precedente, sezione 6.3: è ciò che dà recall semantica senza
embedding, perché il modello fornisce lui i sinonimi). Cambia cosa
restituisce: un **array strutturato** invece di testo formattato.

```json
[
  {
    "reference_type": "document",
    "reference": "manuale_ita.pdf",
    "reference_detail": "pagina 3",
    "text": "Ogni turno il giocatore deve tirare il dado e spostare..."
  },
  {
    "reference_type": "faq",
    "reference": "https://boardgamegeek.com/thread/...",
    "reference_detail": "commento del 15/07/2023 08:00:00",
    "text": "Il giocatore può scegliere in questi casi di ritirare i dadi..."
  }
]
```

`FormatSearchResult` e il suo formato testuale cadono. Costa qualche token
in più del testo piatto, e guadagna la cosa che segue.

### 5.2 `reference_detail` porta sempre la citazione, `heading` resta nel database

`reference_detail` contiene **ciò che va scritto nella citazione**, e per una
fonte senza pagine quella cosa è la sezione:

| Fonte | `reference_detail` |
|---|---|
| PDF | `pagina 3` |
| `.md` / `.docx` / `.txt` | `sezione «Fase di Upkeep»` |
| FAQ | `commento del 15/07/2023 08:00` |

Così **non serve un quinto campo**: `heading` non esce nella risposta del
tool. Per i formati senza pagine sarebbe ridondante — è già dentro
`reference_detail` — e per i PDF il modello può aggiungere la sezione da sé
leggendo il testo che ha ricevuto.

### 5.3 Le citazioni smettono di indovinare

Nella spec precedente `linkifyCitations` cercava `pag. N` con una regex e
provava a dedurre *quale* PDF fosse. Con due manuali per gioco sbagliava — una
citazione del regolamento inglese diventava un link all'italiano — e la
review finale ha imposto di **non linkare affatto** in quel caso, perché un
link sbagliato manda a verificare nel posto sbagliato e fa concludere che
l'assistente ha inventato la regola.

**Da dove viene `reference`.** Non dal nome del file su disco: lo storage è
content-addressed, quindi `game_media.url_or_path` è uno sha256
(`668755…pdf`). Il nome leggibile sta in `game_media.title`
(`Carcassonne_Base_&_Fiume_ITA.pdf`). Quindi `reference` = il titolo del
media, e il link si costruisce dal `url_or_path`.

**E `reference` deve essere unico per gioco**, altrimenti la mappa collide e
si ricade nel bug che questa sezione esiste per chiudere: due media senza
titolo, o con lo stesso titolo, produrrebbero due `reference` identiche e il
link punterebbe al file sbagliato. All'ingestione la `reference` si
disambigua quando serve — il titolo, e se già preso il titolo più la lingua,
e se ancora preso un ordinale. `title` è nullable e testo libero: non si può
assumere che sia distinto.

Ora il server **sa** cosa ha mandato al modello. Tiene per la durata della
richiesta la mappa `reference → url_or_path` delle fonti che ha restituito, e
costruisce il link con certezza:

- `document` → `/api/uploads/<path>#page=3` (la pagina da `reference_detail`)
- `faq` → la `reference` **è già** l'URL

Il bug sparisce per costruzione, e le FAQ avranno citazioni cliccabili senza
codice nuovo. La regex cade.

### 5.4 L'indice nel prompt

Dai `heading` distinti per fonte:

```
Fonti: manuale_ita.pdf — Preparazione · Turno del giocatore · Fase di Upkeep · Fine partita
```

Non più un titolo per pagina quando la pagina cominciava bene, ma **tutte**
le sezioni.

## 6. Il gate sul provider

**Senza provider AI configurato non esiste niente di questa feature.** Una
regola, in un posto solo:

- la chat pubblica non compare (`canAsk`, già così);
- **«Prepara per le domande» non compare**, per **nessun** formato.

`.md` e `.docx` sarebbero tecnicamente indicizzabili senza modello — il
markdown è già lì. La scelta è deliberata: un comportamento che dipende dal
formato costringe l'admin a imparare quali file «funzionano a metà», e
comunque un indice preparato non servirebbe a nessuno finché la chat non
esiste.

Il pannello dice il motivo **pratico**, non tecnico: senza provider la chat
non c'è, quindi indicizzare adesso non serve a nulla.

## 7. UI e API

### 7.1 Le quattro rotte admin diventano due

Senza revisione non c'è più niente da proporre e poi confermare:

| Prima | Ora |
|---|---|
| `POST .../extract` (propone, non salva) | `POST .../index` (estrae **e** indicizza) |
| `GET .../pages` (rilegge) | — (lo stato viaggia sul dettaglio del gioco) |
| `PUT .../pages` (salva) | — |
| `DELETE .../pages` | `DELETE .../index` |

Il dettaglio del gioco espone già `canAsk` e `manualHeadings`; quest'ultimo
si **rinomina `sourceHeadings`**, perché i titoli non vengono più solo da un
manuale. Vi si aggiunge il conteggio dei chunk per fonte, che serve al
pannello.

### 7.2 Il pannello admin si riduce

`ManualPrepPanel.vue` conserva: lo stato («12 sezioni indicizzate» / «non
preparato»), «Prepara per le domande», «Rimuovi indice», gli errori.

Spariscono: la bozza, le textarea per pagina, il `beforeunload`, il confirm
su «Prepara di nuovo» (non c'è più lavoro da perdere), l'etichetta di pagina
`sticky`, il tag sulla pagina vuota. **Gran parte della passata `/impeccable`
del 2026-09-08 su questo componente riguardava la metà che sparisce** e va
rifatta su quel che resta.

### 7.3 Il messaggio d'errore è parte del prodotto

Prima il copia-incolla mascherava qualunque causa: qualcosa non funzionava e
l'admin scriveva il testo. Ora no, quindi l'errore deve dire **cosa** è
andato storto e **cosa fare**:

- modello vision non configurato → nominare il campo nelle impostazioni;
- PDF illeggibile (né testo né immagini estraibili) → suggerire di convertire
  il file;
- docx senza contenuto testuale utile → dirlo, senza fingere un guasto
  generico.

### 7.4 Upload e chat pubblica

Il form di upload dei media accetta quattro tipi invece di uno, e l'`accept`
dell'input si allarga.

La **chat pubblica non cambia**: non le interessa da dove vengono i chunk.
Migliorano due cose per riflesso — le domande suggerite vengono da titoli di
sezione veri su tutti i formati, e i link di citazione tornano corretti anche
con più manuali.

## 8. Non-obiettivi

- **Le FAQ di BGG non si costruiscono qui.** Il modello dati e il contratto
  del tool le ammettono (`reference_type`, `reference` come URL); l'unica
  fonte implementata resta il documento. Le FAQ sono una fase a sé: un
  estrattore nuovo che scrive nelle stesse tabelle, senza toccare l'agente
  né la UI.
- **Nessun inserimento manuale del testo**, in nessuna forma e per nessun
  formato. È il vincolo di prodotto, non un rinvio.
- **Nessuna conservazione del testo estratto.** Solo chunk.
- **Nessun percorso inline.** Il tool si dichiara sempre.
- **Il docx si legge in un sottoinsieme utile** — paragrafi, titoli, elenchi.
  Tabelle, note a piè di pagina e caselle di testo non entrano. Per un
  regolamento è quasi sempre abbastanza; per un docx che tiene le regole in
  tabelle, no, e va detto invece di scoprirlo.
- **Nessun embedding.** I due blocchi di allora sono ancora in piedi:
  `sqlite-vec` non si carica in `modernc.org/sqlite` e il provider
  configurato non espone `/embeddings`.

## 9. Rischi aperti

**La qualità della trascrizione non è mai stata misurata, e questo disegno
ci scommette sopra più del precedente.** Via la correzione manuale, via il
percorso inline, e la segmentazione affidata al modello anche sui PDF
testuali. Se `deepseek-v4-flash-vision-exp` legge male un regolamento denso,
prima l'admin correggeva; ora rilancia o converte il file. **Misurarlo costa
cinque minuti** e non blocca la scrittura del piano: configurare il modello,
preparare il manuale di Carcassonne, leggere il risultato.

**La segmentazione può parafrasare.** Il prompt lo vieta, ma è un vincolo
espresso a parole a un modello economico, non una garanzia meccanica. Un
titolo inventato è innocuo; un paragrafo riscritto introduce una regola che
il manuale non contiene. Va verificato sul primo documento reale, e se
succede la mitigazione è confrontare la lunghezza del testo dentro e fuori e
rifiutare una risposta che se ne discosti oltre una soglia.

**Ogni domanda ora fa almeno una ricerca.** Il Carcassonne da 4 pagine, che
prima entrava intero nel contesto senza chiamare il tool, ora fa un giro di
rete in più, e la risposta dipende da cosa la ricerca trova invece di avere
tutto il testo davanti. È il costo diretto della rinuncia al testo salvato,
e si vedrà nella qualità delle risposte sui manuali corti.

**Non c'è una via d'uscita.** Un documento illeggibile significa nessuna
chat per quel gioco. È voluto, ma è il rischio che l'admin incontra senza
poterlo aggirare, e dipende interamente dalla bontà del messaggio d'errore.
