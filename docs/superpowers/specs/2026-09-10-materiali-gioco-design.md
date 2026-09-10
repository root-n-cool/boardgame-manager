# Materiali del gioco e checklist di riconsegna — Design

Data: 2026-09-10

## 1. Panoramica e obiettivi

Un gioco prestato per una serata torna al banco a fine partita, e il
controllo di quel che c'è dentro la scatola oggi si fa a memoria: si guarda
dentro, si chiude, si spera. Le tessere che mancano si scoprono la volta
dopo, quando ormai non si sa più chi le abbia perse.

Questa fase aggiunge due cose, legate fra loro:

1. **Una lista materiali per gioco** nel catalogo: righe libere di
   `nome` + `quantità`, compilate dall'admin (`tessere 42`, `meeple 40`).
2. **Una checklist alla riconsegna**: la modale di restituzione del banco
   prestiti elenca quelle voci una per una, e chi riceve il gioco le spunta.
   Ciò che non torna intero resta scritto sul prestito.

In più, dove il manuale del gioco è già indicizzato e il provider AI è
configurato, un bottone propone la lista partendo dal *contenuto della
scatola* del regolamento — proposta da confermare, mai un salvataggio
automatico.

**Non è in scope**: un inventario dei pezzi persi da ricomprare, un blocco
sui giochi incompleti, la lista materiali sulla scheda pubblica del gioco.

## 2. Decisioni prese in brainstorming

Queste scelte sono state confermate dall'utente; non vanno ridiscusse senza
un motivo nuovo.

- **Il registro scrive solo quel che non va.** Un prestito riconsegnato
  intero non lascia nessuna riga di esito. La tabella di esito esiste per
  rispondere a "cosa manca", non per archiviare migliaia di "tutto ok".
- **Nessun blocco alla chiusura.** Se manca qualcosa il prestito si chiude
  lo stesso: alle 23:30, in piedi al tavolo, una riga aperta per un meeple è
  un danno più grande del meeple. La modale informa, non impedisce.
- **Tre stati per voce, non due.** Verificata e completa, verificata e
  incompleta (con quante ne sono tornate), oppure **non verificata**. Una
  riga lasciata intatta non è una perdita: è un controllo che non è stato
  fatto, e il registro deve saperlo dire.
- **Le righe partono tutte da spuntare.** Il senso della funzione è forzare
  il controllo voce per voce; partire da "tutto a posto" lo annullerebbe.
- **La generazione AI parte dal manuale già indicizzato**, non da una foto
  né da una nuova pipeline: riusa `game_source_chunk` e il provider già
  configurato. Dove non c'è, il bottone non compare e si compila a mano.
- **Solo admin.** La scheda pubblica del gioco non cambia.

## 3. Modello dati

Migrazione `backend/internal/db/migrations/0017_game_materials.sql`,
forward-only come tutte le altre.

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

-- L'esito della riconsegna, una riga solo per ciò che NON è tornato
-- intero o non è stato verificato. Un prestito pulito non ne lascia
-- nessuna: questa tabella si legge per rispondere a "cosa manca", non per
-- archiviare i controlli andati bene.
CREATE TABLE loan_material_issue (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    loan_id     INTEGER NOT NULL REFERENCES game_loans(id) ON DELETE CASCADE,

    -- SET NULL e non CASCADE: se domani l'admin cancella la voce dal
    -- catalogo, il fatto che a marzo mancassero sette tessere resta vero.
    material_id INTEGER REFERENCES game_material(id) ON DELETE SET NULL,

    -- Nome e quantità attesa sono COPIATI al momento della riconsegna,
    -- non risolti con una join: la lista del catalogo può cambiare, il
    -- registro di quella sera no.
    name        TEXT NOT NULL,
    expected    INTEGER NOT NULL,

    -- NULL = voce non verificata. Un numero = quante ne sono tornate,
    -- sempre < expected (una voce completa non genera riga). Lo stato
    -- vive nel nullable, come returned_at sui prestiti: una colonna di
    -- stato separata sarebbe una verità duplicata che può divergere.
    returned    INTEGER CHECK (returned IS NULL OR returned >= 0),

    UNIQUE(loan_id, material_id)
);

CREATE INDEX idx_loan_material_issue_loan ON loan_material_issue(loan_id);
```

**Perché `returned` nullable invece di una colonna `status`.** Gli stati
possibili sono tre e due di essi portano un numero; un enum più un intero
darebbero quattro combinazioni di cui due impossibili (`status='ok'` con
`returned=3`). Il nullable ne rende rappresentabili esattamente tre. È lo
stesso ragionamento già applicato a `game_loans.returned_at`.

**Perché `UNIQUE(game_id, name)`.** Due righe "meeple" sullo stesso gioco
sono sempre un errore di battitura, e scoprirlo al salvataggio costa molto
meno che scoprirlo al tavolo con due caselle identiche da spuntare.

## 4. Backend

### 4.1 `internal/games` — store dei materiali

Nuovo file `backend/internal/games/materials.go`, stesso pattern di
`media.go`.

```go
type Material struct {
    ID       int64
    GameID   int64
    Name     string
    Quantity int
    Position int
}

// ListMaterials torna le voci in ordine di position.
func (s *Store) ListMaterials(ctx context.Context, gameID int64) ([]Material, error)

// ReplaceMaterials sostituisce l'intera lista di un gioco in una
// transazione. position si riassegna qui da 0 in su seguendo l'ordine
// dell'input: chi chiama manda una lista ordinata, non dei numeri.
func (s *Store) ReplaceMaterials(ctx context.Context, gameID int64, in []MaterialInput) ([]Material, error)
```

`MaterialInput` è `{Name string; Quantity int}`. Validazione nello store,
perché è l'unico punto che tutti attraversano:

- nome: `strings.TrimSpace`, non vuoto, ≤ 60 caratteri
- quantità: 1..9999
- massimo 60 voci per gioco
- nomi duplicati (confronto `lowercase` + `trim`) → `ErrDuplicateMaterial`

Errori sentinella: `ErrMaterialInvalid`, `ErrDuplicateMaterial`,
`ErrTooManyMaterials`.

**Sostituzione totale invece di CRUD per riga.** L'editor è una lista che
si salva tutta insieme, e il riordino cambia comunque quasi tutte le righe:
tre rotte CRUD costringerebbero il frontend a calcolare un diff per poi
mandare N richieste che possono fallire a metà. `ReplaceMaterials` in
transazione lascia il DB o tutto vecchio o tutto nuovo.

Nota: la sostituzione cancella e ricrea le righe, quindi gli `id` cambiano
e `loan_material_issue.material_id` diventa `NULL` per i prestiti passati.
`name` ed `expected` copiati sulla riga di esito sono esattamente ciò che
rende quel `NULL` innocuo — il registro storico resta leggibile.

### 4.2 `internal/events` — esito della riconsegna

In `backend/internal/events/loans.go`:

```go
// MaterialCheck è una voce della checklist come arriva dalla modale.
type MaterialCheck struct {
    MaterialID int64
    // Complete = spunta messa: tornata tutta, e Returned si ignora.
    Complete bool
    // Returned ha senso solo con Complete == false: nil = non
    // verificata, un numero = quante ne sono tornate.
    Returned *int
}
```

`ReturnLoan` guadagna un parametro `checks []MaterialCheck` (nil = nessuna
checklist, comportamento di oggi identico). Dentro la stessa transazione
che chiude il prestito:

1. carica le voci del catalogo del gioco di quel prestito (join
   `event_games → games → game_material`);
2. per ogni voce, risolve lo stato dai `checks` (una voce senza check
   corrispondente è **non verificata**);
3. scrive in `loan_material_issue` solo le voci non-complete;
4. una voce con `Returned >= expected` è completa: non genera riga. Un
   `Returned` maggiore dell'atteso non è un errore da rifiutare (capita di
   ritrovare un pezzo di un'altra scatola), è semplicemente "non manca
   niente".

La transazione è il punto: oggi `ReturnLoan` fa una `UPDATE` sola. Con la
checklist diventano una `UPDATE` più N `INSERT`, e un prestito chiuso senza
il suo esito sarebbe peggio di un prestito non chiuso.

Lettura per il registro: `ListLoansForEvent` carica gli issue dei prestiti
chiusi con **una** query aggiuntiva (`WHERE loan_id IN (...)`), non una per
riga.

### 4.3 `internal/ai` — proposta dal manuale

Nuovo file `backend/internal/ai/materials.go`, gemello di `suggest.go`.

```go
type MaterialLister interface {
    ListMaterials(ctx context.Context, gameName string, passages []string) ([]SuggestedMaterial, error)
}

type SuggestedMaterial struct {
    Name     string
    Quantity int
}

var ErrMaterialsRejected = errors.New("ai materials rejected: no valid line in the response")
```

- **Timeout**: 45s (`materialsTimeout`) — più di `suggestTimeout` perché
  l'input è qualche passaggio di manuale e l'output può essere trenta
  righe, meno di `segmentTimeout`.
- **Prompt di sistema**: chiede righe `nome<TAB>quantità`, una per riga,
  niente preamboli, niente elenchi puntati; nomi in italiano al plurale
  come li direbbe un giocatore; ignorare regolamento, crediti e
  ringraziamenti; una riga per tipo di pezzo, senza raggruppare per colore
  (`meeple 40`, non `meeple rossi 8`) a meno che il manuale non li elenchi
  già così.
- **Parsing rigido** in `parseMaterials`, che è la mitigazione vera (il
  prompt rende il rifiuto raro, non impossibile): scarta le righe senza un
  intero valido, applica gli stessi tetti dello store (60 caratteri, 1–9999,
  60 voci), deduplica per nome normalizzato. Nessuna riga valida →
  `ErrMaterialsRejected`.

I passaggi in ingresso li sceglie l'handler, non il pacchetto `ai`.

### 4.4 `internal/httpapi` — rotte

Nuovo file `backend/internal/httpapi/materials_handlers.go`. Tutte protette
dal middleware admin, registrate nel blocco `protected` di `router.go`:

| Metodo | Rotta | Cosa fa |
|---|---|---|
| `GET` | `/api/games/{id}/materials` | la lista, in ordine |
| `PUT` | `/api/games/{id}/materials` | sostituisce la lista intera |
| `POST` | `/api/games/{id}/materials/suggest` | propone dal manuale, **non salva** |

`POST /api/loans/{id}/return` accetta in più un campo opzionale:

```json
{
  "notes": "…",
  "materials": [
    { "materialId": 7, "complete": true },
    { "materialId": 8, "complete": false, "returned": 35 },
    { "materialId": 9, "complete": false, "returned": null }
  ]
}
```

Il campo assente lascia il comportamento di oggi: il banco prestiti di un
gioco senza materiali manda la stessa richiesta di prima.

La risposta del prestito (`loans_responses.go`) guadagna `materialIssues`
sui prestiti chiusi: `[{name, expected, returned}]` con `returned: null`
per le non verificate.

**`suggest`** riusa la ricerca FTS già in casa:
`s.Manuals.Search(ctx, gameID, preferLang, []string{"contenuto", "componenti", "materiale", "materiali", "scatola"})`,
dove `preferLang` è il codice della lingua base del gioco (`is_base_language`),
la stessa preferenza che usa la chat sul manuale.
Prende al più 6 passaggi, li passa a `ListMaterials`, e torna
`{ "materials": [{name, quantity}] }` senza toccare il DB. Se il gioco non
ha chunk indicizzati risponde `409` con un messaggio che dice di indicizzare
prima un manuale — lo stesso trattamento di `errNoHeadings` per le domande
suggerite, perché l'admin deve leggere "indicizza un manuale", non
"riprova".

Il generatore si costruisce dalle impostazioni a ogni richiesta, con
l'iniezione per i test, come `suggester()` in `questions_handlers.go`.

## 5. Frontend

### 5.1 Editor nel catalogo — `GameAdminDetailView.vue`

Nuova `.panel-card` **"Materiali"**, dopo le lingue. Una riga per voce:

```
[ nome                    ] [ qtà ] [ ↑ ] [ ↓ ] [ 🗑 ]
```

- "Aggiungi voce" in fondo aggiunge una riga vuota e ci mette il focus.
- Un solo "Salva materiali" per tutta la lista (`PUT`).
- Riordino con due frecce, non drag-and-drop: nessuna dipendenza nuova,
  funziona col tocco e da tastiera. Prima riga senza ↑, ultima senza ↓.
- Stato vuoto con una riga che dice a cosa serve: *"Serve alla riconsegna:
  ogni voce diventa una casella da spuntare quando il gioco torna."*
- Il bottone **"Genera dal manuale"** segue la stessa meccanica di
  `SuggestedQuestionsPanel`: c'è sempre, ma è `disabled` quando il provider
  AI non è configurato, con la nota che lo spiega legata al bottone via
  `aria-describedby` (mai un controllo spento e muto — è un *Don't* di
  DESIGN.md). Il caso "nessun manuale indicizzato" lo racconta il `409`
  della rotta, perché è il backend a saperlo con certezza. Il risultato
  **riempie l'editor come proposta non salvata**, con una riga che lo dice
  (*"Proposta dal manuale: correggi quel che serve, poi salva."*); l'admin
  salva o annulla. È la regola già scritta in CLAUDE.md: l'admin conferma
  sempre i risultati automatici.

Il pannello vive in un componente suo, `GameMaterialsPanel.vue`:
`GameAdminDetailView.vue` è già lungo, e questo è un blocco con stato e
rotte proprie.

### 5.2 Checklist di riconsegna — `LoanDeskView.vue`

Nella modale "Restituzione", sopra le note, una sezione **Materiali** con
l'avanzamento in testata (`3 di 7 verificate`). Ogni riga ha esattamente
questa forma, confermata dall'utente:

```
[nome]        [quantità originale]   [input quantità ricevuta]   [checkbox]

tessere              42                      [ — ]                  [x]
carte                40                      [ 35 ]                 [ ]
```

Comportamento:

- **spunta** → l'input si disabilita e mostra `—`: vale "tornate tutte";
- **input compilato senza spunta** → sono tornate quelle;
- **riga intatta** (niente spunta, input vuoto) → non verificata.

Tutte le righe partono da spuntare, con l'input vuoto.

Il banco prestiti si usa in piedi al tavolo, quindi mobile-first: nome
elastico, quantità attesa in mono (regola del dato in mono, DESIGN.md),
input largo 4 caratteri con `inputmode="numeric"`, casella con area di
tocco ≥ 44px e etichetta accessibile (`aria-label="tessere: tutte tornate"`).
La riga regge i 390px senza scroll orizzontale.

Sopra il bottone, un riepilogo quando non è tutto pulito — *"1 voce
incompleta, 3 non verificate"* — che informa e non blocca: il bottone
"Restituito" resta abilitato in ogni caso.

Un gioco senza materiali non mostra la sezione: la modale è quella di oggi.

### 5.3 Registro dei resi

Nella lista "Restituiti" del banco, un prestito con esiti mostra una riga
in più — *"mancano: carte (35/40) · non verificate: dadi"* — così il
problema si vede senza aprire niente. Un prestito pulito resta come oggi.

## 6. Test

**Backend** (in Docker, come da CLAUDE.md):

- `internal/games/materials_test.go`: lista ordinata, sostituzione totale,
  riordino, nomi duplicati, tetti su nome/quantità/numero di voci,
  cancellazione a cascata col gioco.
- `internal/events/loans_test.go`: riconsegna senza checklist (invariata),
  con tutte le voci complete (nessuna riga di esito), con voci incomplete e
  non verificate, con `returned >= expected` (nessuna riga), doppia
  restituzione, atomicità (nessun prestito chiuso senza il suo esito).
- `internal/ai/materials_test.go` + `materials_internal_test.go`: parsing
  di risposte pulite e sporche (numerazione, elenchi puntati, righe senza
  numero, quantità fuori scala, duplicati), rifiuto quando non resta
  niente.
- `internal/httpapi/materials_handlers_test.go`: GET/PUT, 401 senza
  sessione, 409 senza fonti indicizzate, `suggest` che non scrive nel DB,
  `return` con e senza il campo `materials`.

**Frontend**: `npm run build` (che fa anche il type-check con `vue-tsc`) e
verifica visiva con Claude in Chrome della modale a 390px e dell'editor da
desktop.

**Chiusura**: pass `/impeccable` sulle due superfici toccate, come da
CLAUDE.md.

## 7. Rischi e cose da tenere d'occhio

- **La modale cresce.** Un gioco con 30 voci rende la modale di riconsegna
  una pagina lunga su telefono. Accettabile: è esattamente il controllo che
  la funzione vuole imporre, e l'avanzamento in testata dice a che punto
  sei. Se diventasse un problema, la risposta è un tetto ragionevole di
  voci, non una checklist parziale.
- **La qualità della proposta AI dipende dal manuale.** Il "contenuto della
  scatola" di molti regolamenti è un'immagine, non testo: lì la ricerca FTS
  non trova niente e la proposta esce vuota o povera. È il motivo per cui la
  proposta si conferma sempre a mano, e per cui una proposta vuota deve dire
  "non l'ho trovato nel manuale" e non fingere un errore.
- **`ReplaceMaterials` cambia gli id.** Documentato sopra: il registro
  storico sopravvive perché nome e quantità attesa sono copiati sulla riga
  di esito.
