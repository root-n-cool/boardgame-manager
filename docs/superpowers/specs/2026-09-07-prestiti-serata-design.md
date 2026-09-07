# Prestiti della serata — design

Data: 2026-09-07

## Problema

Le prenotazioni sono una dichiarazione d'intenti *prima* della serata.
Non dicono nulla su cosa succede *durante*: chi si è presentato
davvero, chi ha in mano la scatola adesso, quale copia è tornata sul
tavolo dell'organizzatore e quale è sparita in fondo alla sala.

Tre casi che oggi non si possono registrare:

1. chi ha prenotato si presenta e **ritira** effettivamente il gioco;
2. dopo la riconsegna, la stessa copia viene **ripresa** da un'altra
   persona (o dalla stessa) nella stessa serata;
3. qualcuno arriva **senza prenotazione** e prende un gioco.

Manca anche il colpo d'occhio che serve all'organizzatore in piedi in
mezzo alla sala: quali giochi sono *fuori* e quali sono disponibili.

E c'è un limite che si farà sentire presto: oggi ogni gioco messo in un
evento è automaticamente prenotabile. In prospettiva solo i giochi
"corposi" andranno a prenotazione, mentre un filler come Love Letter
resterà a disposizione di chi arriva a mani vuote — presente nella
serata, assente dal catalogo prenotabile.

## Obiettivo

- Un **registro prestiti** per evento: chi ha ritirato cosa, quando,
  quando l'ha restituito, con note libere.
- Il prestito può nascere **da una prenotazione** o **da nessuna**.
- Ogni copia della serata è **disponibile** o **fuori**, e l'elenco dei
  disponibili mostra solo ciò che non è in prestito.
- Una copia restituita si può riprestare, e lo storico resta.
- Un gioco può stare in un evento **senza essere prenotabile**.

## Decisioni prese in brainstorming

- **Il pool dei prestiti sono le copie dell'evento**, non il catalogo:
  `event_games` resta l'unico elenco della serata, e ogni gioco vi
  entra con un flag "prenotabile". Love Letter si aggiunge all'evento
  come gioco non prenotabile. Un solo elenco da gestire, e l'admin
  decide serata per serata cosa ha davvero portato.
- **Il flag "prenotabile" si imposta per gioco nell'evento**, non per
  singola copia: tutte le copie di quel gioco lo seguono. La colonna
  sta comunque sulla riga `event_games` (che è per copia), quindi un
  eventuale passaggio al per-copia non richiederà una migrazione.
- **Nome e telefono del mutuatario sono sempre obbligatori**,
  precompilati quando il prestito nasce da una prenotazione. Il
  prestito conserva anche `booking_id` quando c'è: è l'unico modo di
  rispondere a "chi ha prenotato e non si è presentato".
- **Il prestito si chiude nella serata**: punta alla copia della
  serata (`event_game_id`) e la disponibilità si calcola dentro quel
  solo evento. Un prestito aperto a serata finita è un'anomalia
  visibile, non un vincolo sugli eventi successivi.
- **Il banco prestiti è una vista dedicata**, non una sezione della
  scheda admin dell'evento: quella resta la pagina di configurazione,
  già densa fra form, prenotazioni e risultati. Il banco si usa in
  piedi, da telefono.
- **I prestiti non producono punteggi** (fuori scope, sotto).

## Dati

Migrazione `0013_loans.sql`, forward-only:

```sql
ALTER TABLE event_games ADD COLUMN bookable INTEGER NOT NULL DEFAULT 1
    CHECK (bookable IN (0, 1));

CREATE TABLE game_loans (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    event_game_id  INTEGER NOT NULL REFERENCES event_games(id) ON DELETE CASCADE,
    booking_id     INTEGER REFERENCES bookings(id) ON DELETE SET NULL,
    borrower_name  TEXT NOT NULL,
    borrower_phone TEXT NOT NULL,
    notes          TEXT,
    lent_at        TEXT NOT NULL DEFAULT (datetime('now')),
    returned_at    TEXT
);

CREATE UNIQUE INDEX idx_one_open_loan_per_copy
    ON game_loans(event_game_id) WHERE returned_at IS NULL;
```

`DEFAULT 1` su `bookable` lascia identici gli eventi già in
archivio: tutto quello che c'è oggi è prenotabile, come è sempre
stato.

**Non c'è una colonna di stato.** Lo stato è la data che manca:
`returned_at IS NULL` significa fuori. Un secondo campo `status`
sarebbe una verità duplicata che può divergere da `returned_at`.

`booking_id` è nullable perché la maggioranza dei prestiti "leggeri"
non avrà prenotazione, e `ON DELETE SET NULL` perché il registro deve
sopravvivere alla sparizione della prenotazione da cui è nato. Nome e
telefono si copiano comunque sulla riga del prestito, quindi il
registro si legge per intero anche senza seguire la FK.

Il codice va in `backend/internal/events/loans.go`, stesso package di
`bookings.go` e `matches.go` e stesso `Store`: un prestito legge
`event_games` e `bookings`, isolarlo in un package a sé creerebbe una
dipendenza circolare per niente.

## Regole

- Una copia ha **al massimo un prestito aperto**. Il vincolo è
  l'indice parziale, non un controllo applicativo: due consegne
  simultanee sulla stessa copia non possono passare entrambe.
- **Restituzione** = `returned_at = now` più eventuali note. La copia
  torna disponibile e si può riprestare: nuova riga, storico intatto.
- **Le note** si scrivono sia alla consegna che alla restituzione,
  sullo stesso campo: alla restituzione il valore precedente si vede e
  si può integrare.
- Prestare una copia con prenotazioni attive a **un nome non
  prenotato** è permesso, con avviso in UI. Alle 21:30 chi non si è
  presentato non deve tenere in ostaggio la copia.
- **Nessun vincolo un-prestito-per-telefono**: chi ritira per il
  tavolo può ritirare più giochi.
- **Nessun vincolo temporale** sul prestito. Le prenotazioni si
  chiudono all'inizio dell'evento (`ErrEventAlreadyStarted`); i
  prestiti nascono esattamente lì.
- Due nuovi rifiuti sulla modifica evento, gemelli di
  `ErrQuantityBelowActiveBookings`:
  - non si toglie una copia che ha un prestito aperto;
  - non si spegne `bookable` su un gioco con prenotazioni attive.
- Una copia con `bookable = 0` **non si può prenotare**: il rifiuto
  sta nell'endpoint, non solo nella UI.

## API

Un solo GET alimenta la pagina, protetto da sessione:

```
GET /api/events/{id}/loans
→ { copies: [ { eventGameId, gameId, name, coverPath, copyIndex, copies,
                bookable, seats,
                activeBookings: [ { id, name, phone } ],
                openLoan: { id, borrowerName, borrowerPhone, lentAt, notes } | null } ],
    returned: [ { id, eventGameId, gameName, copyIndex,
                  borrowerName, borrowerPhone, lentAt, returnedAt, notes } ] }
```

`copies` è tutta la serata in una risposta: la UI ricava "fuori" e
"disponibili" dalla presenza di `openLoan`, senza un secondo giro.
`activeBookings` serve a precompilare nome e telefono alla consegna.
`copies` (il conteggio delle copie di quel gioco nell'evento) dice
alla UI se numerare la copia: con una copia sola, "#1" è rumore.

Le occupazioni si leggono con query raggruppate, non una per copia —
lo stesso motivo per cui `toEventDetail` usa già
`ActiveBookingCountsByEventGame`.

Scritture:

```
POST /api/events/{id}/loans   { eventGameId, bookingId?, borrowerName, borrowerPhone, notes? }
   → 201 prestito
   → 409 la copia è già fuori
   → 404 la copia non appartiene a questo evento
   → 400 nome o telefono vuoti

POST /api/loans/{id}/return   { notes? }
   → 200 prestito
   → 409 già restituito
   → 404 prestito inesistente
```

Il 409 sulla consegna arriva dalla collisione sull'indice parziale,
non da una lettura preventiva: nessuna finestra fra il controllo e la
scrittura.

Modifiche a quel che c'è:

- `POST /api/events` e `PUT /api/events/{id}`: ogni gioco passa da
  `{ gameId, copies }` a `{ gameId, copies, bookable }`. **`bookable`
  assente significa `true`**, così una chiamata scritta prima di
  questa feature continua a fare quello che faceva.
- `GET /api/events/{id}` (pubblico): ogni copia guadagna `bookable`.
- `POST /api/events/{id}/bookings`: rifiuta con 409 una copia
  `bookable = 0`.

## UI

Nuova vista `LoanDeskView.vue` su `/admin/events/:id/prestiti`,
raggiunta da un bottone nella scheda admin dell'evento.

Ordine in pagina, pensato per il telefono tenuto in una mano:

1. **Fuori (N)** in cima — è la domanda che l'organizzatore si fa
   davvero. Una riga per prestito: gioco, chi l'ha preso, da quanto
   tempo. Tap → modale restituzione, con le note e un solo bottone
   "Restituito".
2. **Disponibili (N)** sotto. Tap su una copia → modale consegna. Se
   la copia ha prenotazioni attive, in cima l'elenco tappabile dei
   prenotati (tap = nome e telefono compilati); sotto i due campi
   comunque modificabili, più le note. Scegliere un nome non prenotato
   su una copia prenotata mostra un avviso e non blocca.
3. **Restituiti (N)** in fondo, collassato: il log della serata.

Le copie con `bookable = 0` portano un badge "senza prenotazione" in
entrambi gli elenchi.

Altrove:

- `EventGamesPicker.vue`: ogni gioco scelto guadagna una spunta
  "prenotabile" accanto al campo copie. Disabilitata, con il motivo
  scritto, quando quel gioco ha prenotazioni attive — come già accade
  al campo copie, per non far arrivare un 409 dopo il salvataggio.
- Pagina pubblica dell'evento: le copie non prenotabili si mostrano
  (è informazione utile: "stasera c'è anche Love Letter") ma senza
  bottone prenota, con la dicitura "disponibile al tavolo, senza
  prenotazione".

Il sistema visivo è quello di `DESIGN.md`; i pattern nuovi (badge
"senza prenotazione", righe di stato disponibile/fuori) vanno
documentati lì se sopravvivono al pass finale.

## Test

Backend, store:

- consegna e restituzione di una copia; `returned_at` passa da NULL a
  una data;
- secondo prestito aperto sulla stessa copia rifiutato dall'indice;
- riprestito dopo restituzione: due righe, entrambe leggibili;
- prestito da prenotazione: `booking_id` valorizzato, nome e telefono
  copiati;
- cancellazione dell'evento: i prestiti scompaiono in cascata;
- i due nuovi rifiuti sulla modifica evento (copia con prestito
  aperto, `bookable` spento con prenotazioni attive).

Backend, handler:

- i tre endpoint nuovi nei loro codici di successo e di errore;
- prenotazione rifiutata su copia `bookable = 0`;
- `bookable` assente nel body di creazione evento = `true`;
- gli endpoint del banco prestiti richiedono la sessione.

Frontend: `npm run build` (che fa anche il type-check con `vue-tsc`).
Suite Go completa in Docker, come da `CLAUDE.md`. Ultimo task del
lavoro: `/impeccable` sul banco prestiti e sul picker.

## Fuori scope

- **I prestiti non producono punteggi.** I punteggi restano legati al
  `booking_code`, quindi un gioco non prenotabile non comparirà in
  classifica. Le tre strade valutate (un codice punteggi sul prestito,
  l'inserimento da parte dell'admin, niente) si scontrano tutte con
  `match_results.event_game_id UNIQUE`: due prestiti della stessa copia
  in una serata non possono avere due risultati distinti. Va deciso a
  parte, con la sua migrazione.
- **Prestiti che scavalcano la serata.** Un prestito aperto non
  influenza gli eventi successivi.
- **Registro prestiti globale** (cross-evento), promemoria di
  restituzione, email al mutuatario.
