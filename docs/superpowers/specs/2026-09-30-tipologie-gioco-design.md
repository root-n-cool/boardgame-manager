# Tipologie di gioco gestite da admin — design

Data: 2026-09-30

## Obiettivo

Le tipologie di gioco (oggi GDT e GDR, fisse nel codice) diventano
un'entità gestita dall'admin, così si può aggiungere **Magic: The
Gathering (MTG)** — e altre in futuro — senza toccare codice. Ogni
lista di giochi viene separata in **tab per tipologia**, così i giochi
MTG stanno in una lista a parte. Prenotazioni, punteggi e classifiche
restano identici.

## Stato attuale

- `games.kind` è testo libero (migrazione `0025`), valori ammessi in Go
  (`games.Kinds` = `board`, `rpg`) validati negli handler di creazione e
  modifica gioco.
- Il frontend ha l'elenco speculare in `frontend/src/utils/gameKinds.ts`
  (label, plurale, sigla, `onBgg`), usato da: creazione gioco (salta la
  ricerca BGG se `onBgg` è falso), dettaglio gioco admin (select),
  catalogo pubblico ed evento pubblico (filtro "Tipo" a tendina, visibile
  solo con ≥2 tipologie in lista — `hasKindMix`), pastiglia `.kind-chip`
  (colore d'accento per `kind-rpg`).
- Nessuna logica di prenotazione dipende dalla tipologia.

## Requisiti

1. Una tipologia ha: **nome** (es. "Gioco da tavolo"), **slug** (sigla,
   es. `GDT`), **ricerca BGG** sì/no, **ordine** (posizione delle tab).
2. L'admin crea, modifica, riordina ed elimina le tipologie.
3. Eliminazione **bloccata** se la tipologia ha giochi: l'admin vede
   quanti sono e li sposta prima a mano.
4. Tab per tipologia in **ogni** lista di giochi: catalogo pubblico,
   pagina evento pubblica, catalogo admin, dettaglio evento admin,
   foglio QR del catalogo. Le tab appaiono solo se la lista contiene
   almeno due tipologie; le tipologie senza giochi in quella lista non
   hanno tab.
5. Colore della pastiglia: assegnato dall'app da una palette fissa in
   base all'ordine, non scelto dall'admin.
6. I dati esistenti migrano senza perdite: `board` → GDT, `rpg` → GDR.

## Backend

### Migrazione `0027_game_types.sql`

```sql
CREATE TABLE game_types (
  id          INTEGER PRIMARY KEY,
  name        TEXT    NOT NULL,
  slug        TEXT    NOT NULL UNIQUE,
  bgg_search  INTEGER NOT NULL DEFAULT 1,
  position    INTEGER NOT NULL,
  created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
INSERT INTO game_types (id, name, slug, bgg_search, position) VALUES
  (1, 'Gioco da tavolo', 'GDT', 1, 1),
  (2, 'Gioco di ruolo',  'GDR', 0, 2);

ALTER TABLE games ADD COLUMN game_type_id INTEGER NOT NULL DEFAULT 1;
UPDATE games SET game_type_id = 2 WHERE kind = 'rpg';
```

La colonna `games.kind` resta (migrazioni solo forward) ma non viene più
letta né scritta: il codice Go smette di usarla. Niente `REFERENCES`
sulla nuova colonna: con `foreign_keys(1)` attivo (lo è, `db.Open`)
SQLite rifiuta un `ADD COLUMN` con foreign key e default non NULL.
L'integrità la garantisce lo store: il controllo "ha giochi" prima di
eliminare, e la validazione dell'id in creazione/modifica gioco.

### Store tipologie (`internal/games/types.go`)

Nel package `games`, sullo stesso `Store`: le tipologie cambiano
insieme ai giochi e così non serve un nuovo campo in `Server`.

- `GameType{ID, Name, Slug, BGGSearch, Position, GameCount}`.
- `List` (ordinato per `position`, con `GameCount`), `Get`, `Create`,
  `Update` (campi opzionali), `Delete`, `Move(id, direction)` per
  scambiare la posizione con la vicina.
- Validazione: nome non vuoto; slug normalizzato in maiuscolo, 2–6
  caratteri `[A-Z0-9]`, unico (`ErrSlugTaken`). `Create` mette la nuova
  tipologia in fondo (`MAX(position)+1`).
- `Delete` restituisce `ErrInUse{Count}` se esistono giochi con quella
  tipologia.
- Test `types_test.go` per validazione, unicità slug, ordinamento,
  blocco eliminazione.

### Giochi

- `games.Game` perde `Kind` e guadagna `GameTypeID`; le query leggono e
  scrivono `game_type_id`. Rimossi `Kinds`, `ValidKind`, `KindBoard`,
  `KindRPG`.
- Creazione: `game_type_id` facoltativo; se assente → la prima tipologia
  per `position`. Creazione e modifica rispondono 400 se l'id non
  esiste.
- Le risposte JSON dei giochi (catalogo, dettaglio, giochi di un
  evento, QR) espongono `gameTypeId` (camelCase come il resto
  dell'API) al posto di `kind`; le richieste accettano `gameTypeId`. Il frontend
  risolve nome/slug dalla lista delle tipologie (una richiesta sola,
  niente dati duplicati per gioco).

### API

| Metodo | Percorso | Accesso | Note |
|---|---|---|---|
| GET | `/api/game-types` | pubblico | lista ordinata; `gameCount` solo per l'admin loggato |
| POST | `/api/game-types` | admin | 201; 400 validazione; 409 slug già usato |
| PATCH | `/api/game-types/{id}` | admin | nome, slug, bgg_search |
| POST | `/api/game-types/{id}/move` | admin | `{"direction":"up"\|"down"}` |
| DELETE | `/api/game-types/{id}` | admin | 204; 409 `{"error":…, "games": N}` se in uso |

Test degli handler accanto agli altri in `httpapi`.

## Frontend

### Dati

- Store Pinia `gameTypes` (carica `/api/game-types` una volta, con
  `refresh()` dopo le modifiche admin) sostituisce `utils/gameKinds.ts`.
  Espone `list`, `byId(id)`, `defaultType`, `colorClass(id)`.
- Colore: classe `type-color-N` con N = indice nell'ordine modulo la
  palette. Palette in `app.css`: N=0 lo stile neutro attuale di GDT,
  N=1 l'accento attuale di GDR, più 3–4 colori nuovi coerenti con
  `DESIGN.md`. Un tipo sconosciuto (dati incoerenti) ricade sullo stile
  neutro.

### Pagina admin "Tipologie"

- Nuova rotta `/admin/game-types`, voce nel menu admin accanto a
  "Giochi".
- Lista: nome, slug, "Cerca su BGG" sì/no, numero di giochi, frecce
  su/giù, modifica in riga, elimina.
- Cestino disabilitato con didascalia "Ha N giochi" quando in uso; il
  409 del server viene comunque mostrato se arriva.
- Form di aggiunta in fondo alla lista (nome, slug, checkbox BGG).

### Tab per tipologia

- Nuovo componente `GameTypeTabs` (v-model = id tipologia o `null` per
  "Tutti"): riceve la lista di giochi, calcola le tipologie presenti,
  non si renderizza se sono meno di due. Prima tab "Tutti", poi le
  tipologie nell'ordine admin, ciascuna col conteggio.
- Sostituisce il `SelectFilter` "Tipo" in `CatalogView` ed
  `EventDetailView`; aggiunto in `GamesView` (catalogo admin),
  `EventGamesPicker` (la lista dei giochi da aggiungere, usata da
  creazione e dettaglio evento admin) e `QrSheetView` (in stampa si stampa solo la
  tab selezionata; le tab stesse non vanno in stampa).
- Mobile-first: tab scorrevoli in orizzontale se non stanno in riga.

### Creazione e dettaglio gioco

- `GameNewView`: select tipologia dallo store; se la tipologia scelta
  ha `bgg_search` falso si va diretti all'inserimento manuale (come oggi
  per GDR).
- `GameAdminDetailView`: select tipologia dallo store.
- `.kind-chip` mostra lo slug con la classe colore della tipologia.

## Documentazione

- `DESIGN.md`: pattern tab per tipologia e palette pastiglie.
- `README.md`: tipologie gestibili da admin.
- `CLAUDE.md`: aggiornare la sezione catalogo se serve.

## Fuori scope

- Colori scelti dall'admin, icone per tipologia.
- Regole di prenotazione diverse per tipologia (resta tutto identico).
- Integrazioni con fonti dati MTG (Scryfall ecc.): i giochi MTG si
  inseriscono a mano come i GDR.

## Verifica

- Suite backend in Docker; `npm run build`.
- Nel browser: migrazione su DB esistente (GDT/GDR preservati), crea
  MTG senza BGG, aggiungi un gioco MTG, controlla le tab nelle cinque
  liste (desktop e mobile), tenta di eliminare una tipologia in uso.
- Pass `/impeccable` sulle superfici toccate.
