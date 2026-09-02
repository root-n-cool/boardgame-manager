# Fase 4 — Punteggi e Classifiche — Design

Data: 2026-09-02

## 1. Panoramica e obiettivi

Questa fase aggiunge al BoardGames Manager (Fasi 1-3 completate:
auth/admin, catalogo giochi, eventi/prenotazioni) l'inserimento del
punteggio a fine partita e la classifica storica per gioco (sezioni
5.7-5.8 del design master).

Chi ha prenotato un tavolo, tramite il proprio `booking_code`, registra
alla fine della partita i giocatori (nomi liberi) e il punteggio di
ciascuno. Ogni gioco espone poi una pagina pubblica di classifica che
aggrega tutti i risultati storici per giocatore (nome normalizzato) e
mostra lo storico dei match.

Questa fase introduce anche una semplificazione al meccanismo di
identificazione della prenotazione, decisa in fase di brainstorming:
lookup e cancellazione (Fase 3) passano da email+`booking_code` al solo
`booking_code`, coerentemente col nuovo endpoint di inserimento
punteggio.

## 2. Architettura e pacchetti Go

- **`backend/internal/events`**: si estende con `MatchResult` e
  `MatchPlayerScore` (stesso pattern di store già usato per
  `Booking`), dato che entrambi sono scoped a un booking esistente.
- **Nuovo `backend/internal/leaderboard`**: query di sola lettura che
  aggregano `match_results`/`match_player_scores` per `game_id`
  (attraverso `booking → event_game → game`) — separato da `events`
  perché la sua unica responsabilità è l'aggregazione cross-evento per
  gioco, non la gestione dei booking.
- **`internal/httpapi`**: nuovo handler pubblico per il submit
  punteggio e per la classifica; nuovo handler admin di sola lettura
  per i risultati di un evento; modifica dei body di lookup/cancel
  esistenti.

## 3. Modello dati

Nuova migrazione (`0005_scores.sql`):

```sql
MatchResult
  - id, booking_id (univoco — un solo risultato per booking),
    submitted_at

MatchPlayerScore
  - id, match_result_id (FK ON DELETE CASCADE), player_name (testo
    libero), score (intero)
```

Note:
- `booking_id UNIQUE` implementa "il punteggio è sempre modificabile":
  il submit fa upsert, cancellando e reinserendo le righe
  `match_player_scores` del `MatchResult` esistente (e aggiornando
  `submitted_at`) invece di creare righe duplicate.
- Minimo 1 giocatore per match (anche punteggio "solitario"); nessun
  massimo imposto lato server oltre a un limite ragionevole (es. 20) a
  scopo anti-abuso.
- Vince chi ha il punteggio più alto nel match; in caso di pareggio al
  punteggio massimo, tutti i giocatori con quel punteggio sono
  considerati vincitori di quel match — nessuna logica di spareggio.
- Solo un booking con `status='active'` può inserire/modificare un
  punteggio: chi ha cancellato la prenotazione non ha giocato.
- La classifica aggrega per `player_name` normalizzato (lowercase +
  trim), come da design master — nessun registro giocatori separato.
- Eliminare un evento continua a fare cascade su `EventGame`/`Booking`
  (Fase 3): dato che `match_results.booking_id` referenzia `bookings`,
  si aggiunge `ON DELETE CASCADE` anche qui, altrimenti eliminare un
  evento con punteggi già inseriti fallirebbe per violazione FK.

## 4. API

**Modifiche a endpoint esistenti (Fase 3):**
- `POST /api/bookings/lookup` — body diventa `{bookingCode}` (rimosso
  `email`). La risposta include un campo `hasMatchResult: bool` per
  permettere al frontend di distinguere "inserisci" da "modifica"
  punteggio, oltre ai campi già esistenti.
- `POST /api/bookings/{id}/cancel` — body diventa `{bookingCode}`
  (rimosso `email`).
- Motivazione della rimozione di `email`: `booking_code` è già
  generato con `crypto/rand` su un alfabeto di 33 caratteri (8
  posizioni, oltre 10^12 combinazioni) ed è protetto dallo stesso
  rate-limit per IP già in vigore su questi endpoint — l'email non
  aggiunge protezione significativa contro il brute force e la sua
  rimozione semplifica il flusso utente.

**Nuove pubbliche (nessun login):**
- `POST /api/bookings/{id}/match-result` — body `{bookingCode, players:
  [{name, score}, ...]}` (min 1 elemento, `name` non vuoto, `score`
  intero). Verifica `bookingCode` sul booking `id` indicato e che
  `status='active'` (409 altrimenti, stesso stile di errore delle
  altre azioni su booking); esegue l'upsert descritto in sezione 3.
  Risposta: risultato salvato (players con nome/punteggio).
- `GET /api/games/{id}/leaderboard` — nessuna auth. Risposta:
  - `players`: aggregato per nome normalizzato — nome (visualizzato
    come inserito l'ultima volta), partite giocate, vittorie,
    punteggio medio, punteggio totale — ordinato per vittorie desc,
    poi punteggio medio desc.
  - `matches`: storico — per ciascun `MatchResult`, data/ora
    dell'evento associato e lista `{playerName, score, isWinner}`.

**Nuova protetta (sessione admin):**
- `GET /api/events/{id}/match-results` — stesso pattern di route già
  usato per `GET /api/events/{id}/bookings` (nessun prefisso
  `/admin`, protezione data dal middleware di sessione). Sola lettura:
  per ogni booking dell'evento con un `MatchResult`, nome prenotante,
  gioco, giocatori e punteggi inseriti. Nessuna azione di
  modifica/cancella in questa fase.

Il nuovo endpoint `POST /api/bookings/{id}/match-result` viene
registrato con lo stesso middleware di rate-limit già usato per
`lookup`/`cancel` (`bookingCredentialsLimiter`).

## 5. Frontend

- **`ManageBookingView.vue`**: rimosso il campo email dal form di
  ricerca (resta solo "Codice prenotazione"). Se il booking trovato ha
  `status='active'`, viene mostrata una sezione punteggio con righe
  dinamiche nome+punteggio (aggiungi/rimuovi riga, minimo 1); se
  `hasMatchResult` è vero le righe sono precompilate e il bottone dice
  "Aggiorna punteggio", altrimenti "Invia punteggio".
- **`GameDetailView.vue`**: aggiunto link/sezione verso la classifica
  del gioco.
- **Nuova `GameLeaderboardView.vue`** (route pubblica
  `/games/{id}/leaderboard`): tabella aggregata per giocatore (vedi
  sezione 4) e, sotto, storico match con data evento, giocatori e
  punteggi, vincitore/i evidenziati.
- **Admin — `EventAdminDetailView.vue`**: nuova sezione "Risultati
  inseriti" in sola lettura, sotto l'elenco booking già esistente.

## 6. Requisiti non funzionali

**Sicurezza:**
- Il rate-limit per IP già in vigore su `POST /api/bookings/lookup` e
  `POST /api/bookings/{id}/cancel` si applica anche al nuovo endpoint
  `POST /api/bookings/{id}/match-result`.
- Risposte generiche (404/409 senza dettagli) per `bookingCode`
  mancante o non corrispondente, per non facilitare l'enumerazione.

**Testing:**
- Backend: unit test su normalizzazione nome giocatore
  nell'aggregazione, calcolo vittoria/pareggio, upsert (submit due
  volte sullo stesso booking sostituisce senza duplicare), rifiuto su
  booking `cancelled` (409) o `bookingCode` errato (404). Test HTTP sui
  nuovi endpoint e su lookup/cancel aggiornati.
- Frontend: build + verifica manuale in browser (flusso completo:
  prenotazione → inserimento punteggio → modifica punteggio →
  classifica aggiornata), come nelle fasi precedenti.

## 7. Fuori scope per questa fase

- Modifica/cancellazione dei `MatchResult` da parte dell'admin — solo
  vista di sola lettura.
- Spareggio o classifica di posizione oltre al primo posto (secondo,
  terzo, ...) — si traccia solo chi ha il punteggio più alto nel
  match.
- Un limite massimo di giocatori per match diverso da un tetto
  anti-abuso fisso.
- Registro giocatori dedicato — resta il matching su nome normalizzato
  già deciso nel design master.
