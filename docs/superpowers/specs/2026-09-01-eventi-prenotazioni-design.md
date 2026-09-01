# Fase 3 — Eventi e Prenotazioni — Design

Data: 2026-09-01

## 1. Panoramica e obiettivi

Questa fase aggiunge al BoardGames Manager (Fase 1 auth/admin e Fase 2
catalogo giochi completate) la gestione degli eventi. Un admin crea un
evento (titolo, descrizione, data, ora) e vi associa uno o più giochi dal
catalogo, ciascuno con una quantità di copie disponibili. I partecipanti,
senza bisogno di login, vedono l'elenco pubblico degli eventi futuri,
prenotano una copia di un gioco per un evento non ancora iniziato (nome,
email, telefono), ricevono un `booking_code` a schermo, e possono in
seguito annullare la propria prenotazione tramite email + `booking_code`.

Rispetto alla visione originale (design master, sezioni 5.4–5.6), questa
fase **non copre**:
- L'inserimento del punteggio a fine partita e la classifica per gioco
  (sezioni 5.7–5.8 del design master) — rimandati alla Fase 4 "Punteggi e
  classifiche", che riuserà lo stesso meccanismo email + `booking_code`.
- L'invio di email (conferma, promemoria) — non previsto in v1, come da
  design master.

## 2. Architettura e pacchetti Go

- **`backend/internal/events`**: repository per `Event`, `EventGame`,
  `Booking` — stesso pattern di `internal/games` (store con metodi CRUD,
  errori sentinella, SQLite in-memory nei test).
- **`internal/httpapi`**: nuovi handler per eventi/prenotazioni, in parte
  pubblici (lettura eventi, creazione/lookup/cancellazione booking) e in
  parte protetti da sessione admin (CRUD evento, elenco booking di un
  evento).
- Generazione `booking_code`: 8 caratteri alfanumerici (maiuscoli +
  cifre, esclusi caratteri ambigui come `0/O`, `1/I`), tramite
  `crypto/rand` — stesso livello di rigore già usato per le password.

## 3. Modello dati

Nuova migrazione (`0004_events.sql`):

```sql
Event
  - id, title, description (nullable), event_date (TEXT, es. "2026-10-01"),
    start_time (TEXT, es. "20:00"), created_at

EventGame
  - id, event_id, game_id, quantity (intero > 0)
  - vincolo: un solo EventGame per (event_id, game_id)

Booking
  - id, event_id, event_game_id, participant_name, participant_email,
    participant_phone, booking_code (8 char, univoco), status
    (active | cancelled), created_at
  - vincolo: un solo booking con status='active' per (event_id,
    participant_phone) — unique index parziale
  - event_id è denormalizzato da event_game_id per rendere semplice ed
    efficiente il vincolo sopra e la query "booking attivi per evento"
    nella vista admin
```

Note:
- Nessuno stato esplicito sull'evento: "prenotabile" è calcolato al volo
  confrontando `event_date`+`start_time` con l'istante della richiesta,
  lato server (mai fidarsi di un check lato client).
- Disponibilità residua di un `EventGame` = `quantity - COUNT(booking
  attivi collegati)`, calcolata a ogni lettura (nessuna colonna
  denormalizzata da tenere sincronizzata).
- Aggiornare un evento (`PUT /api/events/{id}`) sostituisce l'intero
  elenco `EventGame`. Sia rimuovere un gioco dall'elenco sia ridurne la
  quantità sotto il numero di booking attivi già presenti per quel gioco
  sono trattati allo stesso modo: l'intera richiesta viene rifiutata
  (409 — è un conflitto con lo stato esistente, non una richiesta
  malformata — nessun aggiornamento parziale) finché quei booking non
  vengono cancellati dai rispettivi partecipanti (o l'evento non viene
  eliminato del tutto).
- Eliminare un evento elimina a cascata `EventGame` e `Booking` (attivi e
  cancellati) — non ci sono vincoli storici da preservare oltre
  all'evento stesso in questa fase (la classifica di Fase 4 leggerà da
  `MatchResult`, non ancora esistente).

## 4. API

**Pubbliche (nessun login):**
- `GET /api/events` — elenco eventi futuri (id, title, event_date,
  start_time), ordinati per data/ora crescente
- `GET /api/events/{id}` — dettaglio evento: dati evento + elenco giochi
  associati (id, nome, copertina, disponibilità residua); include
  l'evento anche se già iniziato (per mostrare la pagina, senza
  possibilità di prenotare — vedi sotto)
- `POST /api/events/{id}/bookings` — crea una prenotazione:
  `{eventGameId, participantName, participantEmail, participantPhone}`.
  In una singola transazione: verifica che l'evento non sia iniziato
  (409 altrimenti), verifica disponibilità residua > 0 per l'
  `eventGameId` (409 "esaurito" altrimenti), verifica assenza di booking
  attivo con lo stesso telefono su quell'evento (409 con messaggio
  esplicito altrimenti), genera `booking_code` univoco, inserisce la riga
  `status='active'`. Risposta: booking creato incluso `bookingCode`.
- `POST /api/bookings/lookup` — `{email, bookingCode}` → dettagli booking
  (evento, gioco, stato) se `status='active'` e email/codice
  corrispondono; 404 generico altrimenti (niente distinzione tra "non
  esiste" e "email sbagliata", per non facilitare enumerazione)
- `POST /api/bookings/{id}/cancel` — `{email, bookingCode}`, riverificati
  server-side; se validi imposta `status='cancelled'` (libera
  disponibilità); 404 generico se non validi

**Protette (sessione admin):**
- `GET /api/events` (stessa route, ma da autenticato include anche eventi
  passati — vedi nota sotto) / `POST /api/events` — crea evento:
  `{title, description, eventDate, startTime, games: [{gameId,
  quantity}]}`
- `GET /api/events/{id}` (stessa route pubblica, dati sufficienti anche
  per l'admin) / `PUT /api/events/{id}` — sostituisce dati evento ed
  elenco `EventGame` come descritto in sezione 3
- `DELETE /api/events/{id}` — elimina evento (cascade)
- `GET /api/events/{id}/bookings` — elenco booking attivi per l'evento:
  nome, email, telefono, gioco prenotato, orario prenotazione (sola
  lettura, nessuna azione admin sui booking altrui in questa fase)

Nota su `GET /api/events`: la versione pubblica filtra solo eventi
futuri; da sessione admin autenticata restituisce anche i passati (serve
per la vista `/admin/events`). Stesso handler, comportamento diverso in
base a `middleware_auth` — non due route separate.

## 5. Frontend

- **`EventsView.vue`** (route pubblica `/`, home): elenco eventi futuri
  (titolo, data/ora), link al dettaglio.
- **`EventDetailView.vue`** (route pubblica `/events/{id}`): dati evento,
  elenco compatto dei giochi associati (copertina, nome, disponibilità
  residua, link a `/games/{id}` già esistente per i dettagli completi),
  form di prenotazione (nome, email, telefono) per gioco con
  disponibilità > 0, visibile solo se l'evento non è ancora iniziato;
  alla conferma mostra il `booking_code` a schermo con invito a salvarlo.
- **`ManageBookingView.vue`** (route pubblica `/manage-booking`): form
  email + booking_code → chiama lookup, mostra dettagli prenotazione e
  pulsante "Annulla prenotazione"; dopo l'annullamento conferma a schermo
  senza richiedere un altro lookup.
- **Admin — riuso del pattern `/games`:**
  - `EventsAdminView.vue` (route `/admin/events`): elenco eventi (inclusi
    passati), pulsante "Crea evento".
  - **Creazione/modifica evento**: form titolo/descrizione/data/ora +
    selezione multipla giochi dal catalogo con campo quantità per
    ciascuno (riusa `GET /api/games` già esistente per popolare la
    selezione).
  - **`EventAdminDetailView.vue`** (route `/admin/events/{id}`): stessi
    campi di modifica + elenco booking attivi in sola lettura.
- Router: la home pubblica (`/`) cambia da eventuale placeholder a
  `EventsView`; le route admin esistenti (`/games`, `/users`,
  `/settings`) restano invariate ma diventano raggiungibili da una
  navigazione "area admin" separata dalla home pubblica.

## 6. Requisiti non funzionali

**Concorrenza:**
- La creazione di un booking verifica la disponibilità residua e i
  vincoli (evento iniziato, telefono duplicato) all'interno della stessa
  transazione DB dell'insert, per evitare race condition su copie
  limitate (due prenotazioni simultanee per l'ultima copia disponibile).

**Sicurezza:**
- `booking_code` generato con `crypto/rand`, mai indovinabile in tempo
  utile per un attaccante con rate-limit in vigore.
- Rate-limit minimale sugli endpoint `POST /api/bookings/lookup` e
  `POST /api/bookings/{id}/cancel`: contatore in-memory per IP (es. max
  10 tentativi/minuto), coerente con il deployment self-host
  single-istanza — non serve uno store distribuito.
- Le risposte di lookup/cancellazione falliti sono generiche (404, nessun
  dettaglio su quale campo è sbagliato) per non facilitare
  l'enumerazione di email o codici validi.

**Testing:**
- Backend: unit test su `internal/events` (SQLite in-memory), inclusi
  test espliciti sui vincoli (booking duplicato per telefono, quantità
  esaurita, evento già iniziato, riduzione quantità sotto booking
  attivi). Test HTTP per gli endpoint pubblici e protetti.
- Frontend: build + verifica manuale in browser (flusso completo:
  creazione evento admin → prenotazione pubblica → verifica
  disponibilità decrementata → cancellazione → verifica disponibilità
  ripristinata), come nelle fasi precedenti.

## 7. Fuori scope per questa fase

- Inserimento punteggio a fine partita e classifica per gioco — Fase 4.
- Invio email (conferma prenotazione, promemoria) — non previsto in v1.
- Azioni admin sui booking di altri (cancellazione per conto terzi) —
  solo vista sola lettura in questa fase.
- Stato manuale dell'evento (bozza/pubblicato/chiuso) — la prenotabilità
  è sempre derivata da data/ora.
- Limite di prenotazioni per persona diverso da "una per evento" (vincolo
  fisso, come da design master).
