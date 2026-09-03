# Prenotazioni multiple sulla stessa copia — design

Data: 2026-09-03

## Problema

Oggi un gioco dentro un evento è una riga `event_games` con una
`quantity`: N copie, e ogni prenotazione ne occupa una. Il modello
regge finché "prenotare un gioco" significa "prendermi la scatola e
portarmi i miei amici", ma non copre il tavolo aperto — una partita a
D&D dove sei persone si iscrivono singolarmente a posti diversi dello
stesso tavolo, ognuna con il proprio codice, ognuna libera di disdire
senza far saltare la serata agli altri.

Due limiti concreti:

1. non esiste un modo di dire "questo gioco ospita 5 persone";
2. `quantity` è un contatore opaco: con 2 copie di Carcassonne, chi
   prenota non sa su quale copia sta finendo, e l'organizzatore non
   sa chi siede dove.

## Obiettivo

- L'admin marca un gioco in catalogo come tavolo indicando quanti
  **posti prenotabili** ha una sua copia.
- Più persone prenotano lo stesso tavolo, fino al massimale, ognuna con
  nome, email, telefono e **codice prenotazione proprio**.
- Ogni prenotazione si disdice singolarmente senza toccare le altre.
- Le copie multiple dello stesso gioco diventano visibili e distinte
  (`Carcassonne #1`, `Carcassonne #2`), così sparisce l'ambiguità su
  quale copia si sta prenotando.
- La classifica continua a contare **una partita una volta sola**,
  anche quando al tavolo ci sono sei prenotazioni.

## Decisioni prese in brainstorming

- **I posti prenotabili stanno in catalogo**, non sull'evento: il gioco
  ha un campo "posti prenotabili per copia", default 1.
- **La dicitura è sempre "posti prenotabili"**, mai "posti" da solo: in
  UI un "posto" si confonderebbe con la sedia intorno al tavolo o con
  il numero di giocatori del gioco. Nel codice la colonna resta
  `seats`.
- **Modello unificato**: niente flag "tavolo" e niente ramo separato per
  i giochi normali. Una copia con `seats = 1` si comporta esattamente
  come oggi; un tavolo è la stessa cosa con `seats = 5`. Una sola
  regola di capienza per tutti.
- **Una riga = una copia**: `event_games` perde `quantity` e guadagna
  `copy_index`. Le copie si numerano in UI solo quando sono più d'una.
- **Fotografia dei posti prenotabili**: `event_games.seats` è copiato
  dal catalogo quando la copia entra nell'evento e non si aggiorna più
  da solo. Portare D&D da 5 a 7 posti prenotabili in catalogo non
  cambia la capienza di una serata già aperta alle prenotazioni.
- **Il punteggio è del tavolo**, non della prenotazione: un risultato
  per copia, che chiunque abbia un codice valido su quel tavolo può
  inserire o correggere.
- **Vincolo telefono invariato**: un solo booking attivo per
  `(evento, telefono)`. Chi vuole portare qualcuno lo prenota con il
  proprio numero; nessuno si prende mezzo tavolo da solo.
- **Migrazione distruttiva**: il DB in uso è di sviluppo, quindi la
  `0008` ricrea le tabelle da `event_games` in giù svuotandole. Il
  catalogo giochi e gli eventi restano.

## Modello dati

### `games`

```sql
ALTER TABLE games ADD COLUMN seats INTEGER NOT NULL DEFAULT 1 CHECK (seats > 0);
```

Posti prenotabili per copia. `1` significa "chi prenota si prende la
copia", come oggi. Il `CHECK` è accettato da `ADD COLUMN` (verificato
su SQLite 3.51); l'handler valida comunque `seats >= 1` per rispondere
400 invece di 500.

### `event_games`

```sql
CREATE TABLE event_games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    game_id INTEGER NOT NULL REFERENCES games(id),
    copy_index INTEGER NOT NULL CHECK (copy_index > 0),
    seats INTEGER NOT NULL CHECK (seats > 0),
    UNIQUE(event_id, game_id, copy_index)
);
```

Una riga per copia. `bookings.event_game_id` continua a puntare qui,
quindi **una prenotazione occupa un posto su una copia specifica**.

### `match_results`

```sql
CREATE TABLE match_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_game_id INTEGER NOT NULL UNIQUE REFERENCES event_games(id) ON DELETE CASCADE,
    submitted_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

`booking_id` → `event_game_id`. È questo cambio che impedisce alla
classifica di contare sei volte la stessa partita di D&D.

### Migrazione `0008_seats_and_copies.sql`

`foreign_keys` è ON e il runner applica ogni file in una transazione,
dove `PRAGMA foreign_keys = off` viene ignorato: non si può togliere il
vincolo `UNIQUE(event_id, game_id)` da `event_games` con un `ALTER`.
Poiché i dati di prenotazione sono di sviluppo, la migrazione droppa e
ricrea, in ordine inverso di dipendenza:

```
DROP TABLE match_player_scores;
DROP TABLE match_results;
DROP TABLE bookings;
DROP TABLE event_games;
-- ricrea le quattro tabelle con il nuovo schema
-- ricrea idx_one_active_booking_per_phone_per_event
ALTER TABLE games ADD COLUMN seats INTEGER NOT NULL DEFAULT 1 CHECK (seats > 0);
```

`events` e il catalogo giochi non vengono toccati; gli eventi esistenti
restano senza giochi, da ripopolare dalla scheda evento.

`bookings` e `match_player_scores` mantengono lo schema attuale: si
ricreano solo perché il drop a cascata le porterebbe via comunque.

## Regole di dominio

**Capienza.** `RemainingCapacity(eventGameID)` diventa
`seats − prenotazioni attive su quella copia`. `CreateBooking` non
cambia struttura: nella `INSERT ... SELECT ... WHERE` atomica il
sottoquery `SELECT quantity FROM event_games` diventa
`SELECT seats FROM event_games`. La protezione contro la corsa
sull'ultimo posto resta quella di oggi, invariata.

**Vincolo telefono.** `idx_one_active_booking_per_phone_per_event`
resta identico: un solo booking attivo per `(evento, telefono)`, tavoli
compresi.

**Composizione dell'evento.** L'API continua ad accettare, per ogni
gioco, un numero di copie; il backend materializza N righe
`event_games` con `copy_index` 1..N e `seats` letto dal catalogo al
momento dell'inserimento.

- Aumentare le copie da 2 a 4: inserisce `copy_index` 3 e 4.
- Ridurre da 4 a 2: si **scende dalla coda saltando le copie
  occupate**. Non è "elimina le più alte": se la #3 ha una prenotazione
  e la #2 no, cade la #2 e la #3 resta col suo numero. Se le copie
  libere non bastano, l'operazione fallisce con
  `ErrQuantityBelowActiveBookings` (già esistente) e nulla viene
  scritto. Attenzione: il 409 di quell'errore ha un messaggio in
  inglese, mostrato grezzo dalla scheda evento — era già così prima di
  questo lavoro e resta un debito noto, non una cosa che questa spec
  può dare per risolta.
- Togliere del tutto un gioco con prenotazioni attive: stesso errore.
- Il `copy_index` non viene rinumerato quando si elimina una copia di
  mezzo: i numeri mostrati sono etichette stabili, non posizioni. Una
  copia eliminata lascia un buco, e va bene così — rinumerare
  sposterebbe sotto i piedi dei partecipanti il "#2" che hanno letto al
  momento di prenotare.

**Punteggi.** `SubmitMatchResult(bookingID, code, players)` mantiene la
firma e la rotta (`POST /api/bookings/{id}/match-result`): risolve il
booking, ne prende la copia e fa l'upsert del risultato **della copia**.
Se un compagno di tavolo aveva già inserito il punteggio, questa
chiamata lo sostituisce — è lo stesso comportamento "il punteggio è
sempre modificabile" già in vigore, esteso da una persona a un tavolo.
La pagina di gestione prenotazione mostra il risultato del tavolo anche
a chi non lo ha inserito.

**Disdetta.** `cancelBooking` oggi cancella sempre il `match_result`
del booking. Nuova regola: cancella il risultato della copia **solo se
dopo la disdetta non resta nessuna prenotazione attiva su quella
copia**. Su un tavolo condiviso, chi si sfila non deve poter azzerare
il punteggio degli altri; su una copia singola il comportamento è
identico a quello attuale.

## API

Nessuna rotta nuova, nessuna rotta rimossa. Cambiano i payload:

- `GET /api/events/{id}` — ogni voce di `games` perde `quantity` e
  guadagna `copyIndex` e `seats`; `remaining` resta e ora vale
  `seats − attive`. Con N copie ci sono N voci per lo stesso `gameId`.
- `POST /api/events`, `PUT /api/events/{id}` — ogni gioco continua a
  portare un numero di copie; il campo si chiama `copies` (era
  `quantity`), perché ora "copie" e "posti prenotabili" sono due cose diverse e
  tenere il vecchio nome confonderebbe.
- `GET /api/events/{id}/bookings` (admin) — ogni prenotazione porta
  anche `eventGameId` e `copyIndex`, per raggruppare per tavolo.
- `POST /api/bookings/lookup` — la risposta porta `copyIndex`, `seats`,
  `gameCopies` e `tableBookings`. Servono due numeri distinti perché
  rispondono a due domande diverse: `gameCopies` dice se la copia va
  numerata (`#2` compare solo con più copie dello stesso gioco),
  `tableBookings` se il punteggio è condiviso. `seats` non basta per
  nessuna delle due: è la capienza di una copia.
- Giochi (`POST`/`PUT /api/games/...`) — nuovo campo `seats`, intero
  ≥ 1, default 1.

## UI

**Scheda gioco (admin).** Un campo numerico "Posti prenotabili per
copia", default 1, accanto agli altri numeri del gioco, con nota
esplicativa: `1` = chi prenota si prende la copia; `più di 1` = tavolo
aperto, dove ci si iscrive uno alla volta (D&D, giochi di ruolo,
tornei).

**`EventGamesPicker`.** Resta il campo "copie". Quando il gioco
selezionato ha `seats > 1` **e non è già nell'evento**, accanto compare
`× 5 posti prenotabili` e la capienza totale della riga
(`2 copie × 5 = 10 posti prenotabili`). Per un gioco già presente il
totale non si mostra: le copie esistenti portano la loro fotografia dei
posti, che può essere diversa dal valore in catalogo, e un totale
calcolato sul catalogo sarebbe un numero falso.
Il minimo del campo copie resta legato alle prenotazioni attive, come
oggi, ma calcolato sul numero di **copie occupate**, non di
prenotazioni.

**Pagina pubblica evento.** Una card per copia. L'intestazione porta il
suffisso `#1`, `#2` solo quando quel gioco ha più di una copia
nell'evento. La riga di disponibilità diventa `Posti prenotabili
liberi: 3 di 5` quando `seats > 1`, e resta la dicitura attuale quando
`seats = 1`. È mobile-first come il resto delle pagine pubbliche.

**Dettaglio evento (admin).** Le prenotazioni si raggruppano per copia,
con intestazione `Nome #k · 3 di 5 posti prenotabili`, così
l'organizzatore legge i tavoli invece di una lista piatta. Il conteggio
dei posti compare **solo con `seats > 1`**, come su tutte le altre
superfici: su un evento ordinario l'intestazione è il solo nome del
tavolo, perché "1 di 1 posti prenotabili" è rumore e in italiano suona
male. Questa regola vale per tutte e quattro le viste, senza
eccezioni.

**Pagina prenotazione (pubblica).** Quando il tavolo ha più di una
prenotazione attiva, il form dei punteggi dice esplicitamente che il
risultato è del tavolo, visibile e modificabile da tutti i compagni.

Il pass `/impeccable` sulle superfici toccate chiude il lavoro, come
prescritto da CLAUDE.md.

## Test

- **`events`**: capienza con `seats > 1` (prenotazioni multiple sulla
  stessa copia fino al massimale, poi `ErrGameSoldOut`); vincolo
  telefono ancora attivo dentro un tavolo; riduzione copie che salta
  quelle occupate; risultato condiviso fra due prenotazioni della
  stessa copia; disdetta parziale che **non** cancella il risultato e
  disdetta dell'ultima prenotazione che lo cancella.
- **`httpapi`**: payload evento con più copie dello stesso gioco;
  `copies` in creazione e aggiornamento; `seats` sul gioco;
  inserimento punteggio da parte del secondo prenotato.
- **`leaderboard`**: un tavolo con cinque prenotazioni e un solo
  risultato conta una partita, non cinque.
- Suite backend in Docker e `npm run build` prima di dichiarare
  concluso, come da CLAUDE.md.

## Fuori scopo

- Assegnare un posto numerato dentro il tavolo (chi siede in che
  sedia): il tavolo è un insieme di posti prenotabili equivalenti.
- Lista d'attesa quando un tavolo è pieno.
- Spostare una prenotazione da una copia a un'altra: si disdice e si
  riprenota.
- Notifiche ai compagni di tavolo quando qualcuno disdice (niente
  SMTP in v1).
