# Prenotazioni senza dati personali persistiti — design

Data: 2026-09-15
Stato: approvato

## Scopo

La prenotazione anonima raccoglie oggi nome, email e telefono, e li
tiene per sempre nel DB: il telefono serve solo a un vincolo server
("una prenotazione attiva per telefono per evento"), l'email a
mandare le mail di conferma/invito/annullamento. Nessuno dei due
è più necessario a quello scopo se si sposta il "sai già di aver
prenotato" sul dispositivo di chi prenota (localStorage) invece che
sul server, e se l'email si usa solo al volo per la mail di conferma
senza salvarla.

Il cambio: eliminare la raccolta del telefono, rendere l'email
facoltativa e non persistita, aggiungere un consenso esplicito a
termini/privacy con prova lato server, e sostituire il vincolo
server-side col riconoscimento "l'ho già prenotato io" nel browser.

## Cosa cambia, in breve

| Prima | Dopo |
|---|---|
| Nome, email, telefono tutti obbligatori | Solo nome obbligatorio; email facoltativa |
| Vincolo DB: una prenotazione attiva per `(event_id, telefono)` | Nessun vincolo di unicità lato server; il browser mostra "già prenotato" via localStorage |
| Email salvata su ogni booking | Email non salvata: usata solo per la mail di conferma al momento della richiesta |
| Mail di annullamento (self e admin) | Rimossa: senza email persistita non c'è più un indirizzo a cui mandarla |
| Nessun consenso richiesto | Checkbox obbligatoria termini/privacy; timestamp di accettazione salvato sul booking |
| Admin vede email · telefono di chi ha prenotato | Admin vede solo il nome |
| Banco prestiti precompila il telefono del prestatario dalla prenotazione | Il telefono va sempre digitato a mano (campo del prestito, non della prenotazione) |

## Migrazione `0020_prenotazioni_senza_contatti.sql`

```sql
DROP INDEX idx_one_active_booking_per_phone_per_event;
ALTER TABLE bookings DROP COLUMN participant_phone;
ALTER TABLE bookings DROP COLUMN participant_email;
ALTER TABLE bookings ADD COLUMN terms_accepted_at TEXT NOT NULL DEFAULT (datetime('now'));
```

Le prenotazioni già esistenti prendono `datetime('now')` come
`terms_accepted_at` di ripiego: non è la data reale del consenso (che
per loro non è mai stato chiesto), ma non c'è modo di ricostruirla, e
non è un dato che serve a fini di query o retention — conta solo la
presenza del campo per le prenotazioni create da qui in avanti.

Il default della colonna (`datetime('now')`) basta anche per le nuove
prenotazioni: nessun codice Go legge mai `terms_accepted_at` indietro
(non è esposto da nessuna risposta API), quindi non serve passarlo
esplicitamente né aggiungerlo alla struct `Booking` — la colonna resta
solo prova di consenso ispezionabile a mano nel DB, popolata dalla
colonna stessa esattamente come `created_at` già fa.

## Backend

### `events.Store.CreateBooking`

Firma senza `phone`:

```go
func (s *Store) CreateBooking(ctx context.Context, eventID, eventGameID int64, name string, now time.Time) (Booking, error)
```

L'INSERT scrive solo `participant_name`, `booking_code`, `status`;
`terms_accepted_at` prende il default della colonna. Il ramo che
mappava una violazione UNIQUE su `ErrDuplicatePhoneBooking` sparisce
insieme all'errore stesso: l'unico UNIQUE rimasto è `booking_code`, la
cui collisione (spazio 33^8) resta un errore generico non gestito
diversamente da oggi. `isUniqueConstraintErr` in `events/store.go`
resta: la usa anche `events/loans.go`, non è specifica di questo
vincolo — si rimuove solo la chiamata dentro `CreateBooking`.

`Booking` perde i campi `ParticipantEmail`/`ParticipantPhone` (nessun
nuovo campo Go: `terms_accepted_at` non è letto da nessun codice
Go). Stesso taglio in `BookingWithGame`, `getBookingByID`,
`LookupBooking`, `ListBookingsForEvent`.

`TestInsertBooking` (store.go) smette di generare/inserire `phone`;
`terms_accepted_at` prende anche qui il default della colonna, senza
bisogno di comparire nell'INSERT.

### Email: solo effimera

L'email non passa più per lo Store. `createBookingHandler` la legge
dal body (`req.Email`, ora facoltativa: nessuna validazione di
presenza) e la passa direttamente a `s.sendBookingConfirmation`,
che oggi ricava `ParticipantEmail` da `b.ParticipantEmail` — va
cambiata per accettare l'email come parametro esplicito invece di
leggerla dal booking:

```go
func (s *Server) sendBookingConfirmation(r *http.Request, b events.Booking, participantEmail string)
```

Se `participantEmail == ""`, la funzione non manda nulla. `mailQueued`
nella risposta di `createBookingHandler` deve riflettere anche questo
caso — non solo `mailEnabled(ctx)` come oggi, ma
`mailEnabled(ctx) && req.Email != ""` — altrimenti una prenotazione
senza email si vedrebbe promettere "ti abbiamo mandato una mail"
(`BookingConfirmation.vue`, prop `mailed`) senza che sia partito
nulla.

`bookingMailDataFor` perde la lettura di `b.ParticipantEmail` (non
esiste più) e riceve l'email come parametro, propagata dentro
`bookingMailData`.

### Consenso ai termini

`createBookingRequest` guadagna `TermsAccepted bool
`json:"termsAccepted"``. Validazione handler: `req.Name == "" ||
!req.TermsAccepted` → 400. Il server è il gate reale (difesa in
profondità): la checkbox nel form è UX, non l'unica barriera.

### Mail di annullamento: rimossa

`sendBookingCancelled`, `bookingCancelledMail` (mail_templates.go) e
le chiamate in `cancelBookingHandler` e `adminCancelBookingHandler`
si eliminano: non c'è più un indirizzo a cui mandarla, né per
l'annullamento fatto dal partecipante né per quello fatto
dall'admin. Il commento sopra `sendBookingCancelled` nella chiamata
admin ("è l'unico modo in cui il partecipante scopre...") va con
loro — non è più vero, e va segnalato come regressione nota (già
approvata) più sotto.

`byAdmin` come parametro/distinzione testuale nel template non serve
più altrove: si rimuove tutto il codice morto associato piuttosto che
lasciarlo raggiungibile da nessun percorso.

### Risposte HTTP

- `toBookingResponse` / `toBookingDetailResponse`: invariate (non
  esponevano già email/telefono).
- `toBookingAdminResponse` (`events_responses.go`): perde
  `"participantEmail"` e `"participantPhone"` dalla mappa.
- `toLoanDeskResponse` (`loans_responses.go`): la riga
  `bookingsByCopy` perde `"phone": b.ParticipantPhone` — resta solo
  `"id"` e `"name"`.

### Test da aggiornare/rimuovere (`backend/internal`)

- `events/bookings_test.go`: rimuovere
  `TestCreateBooking_RejectsDuplicatePhoneForSameEvent`,
  `TestCreateBooking_AllowsSamePhoneAfterCancellation`,
  `TestCreateBooking_PhoneConstraintHoldsInsideATable`,
  `TestCreateBooking_PhoneConstraintHoldsAcrossCopiesOfTheSameEvent`
  (il vincolo che testano non esiste più). Le fixture che passano
  `phone`/`email` a `CreateBooking` vanno adattate alla nuova firma.
- `httpapi/events_bookings_handlers_test.go`: rimuovere
  `TestCancelBooking_EmailsTheParticipantAReceipt`,
  `TestAdminCancelBooking_EmailsTheParticipantThatTheSeatIsFreed`,
  `TestAdminCancelBooking_UnknownSendsNoMail` (asserivano
  sull'invio ora rimosso). `TestCreateBooking_EmailsTheCodeAndBothLinks`
  e simili restano ma passano l'email nel body di richiesta invece che
  aspettarsela persistita; aggiungere un test che una prenotazione
  senza email non manda nulla e comunque va a buon fine, e uno che
  rifiuta la richiesta senza `termsAccepted`.
- `httpapi/loans_handlers_test.go`:
  `TestLoanDesk_ShowsActiveBookingsOfACopy` perde l'assert su
  `row.Phone`.
- `httpapi/mail_templates_test.go`: `testBookingData()` fixture perde
  `ParticipantEmail` come campo del booking persistito (resta come
  parametro passato a mano dove serve) e i test sul contenuto della
  mail di annullamento si rimuovono.
- Ogni `createTestBooking`/fixture condivisa che passa `phone` in
  altri test file (`events_handlers_test.go`, `match_result_handlers_test.go`,
  `ask_handler_test.go`) va aggiornata alla nuova firma/payload.

## Frontend

### `EventDetailView.vue` — form di prenotazione

- Rimuovere `participantPhone` (ref, binding, payload, markup del
  campo "Telefono").
- Campo "Email": togliere `required`; aggiungere sotto una nota
  (`field-hint`, stesso stile delle altre note opzionali del
  progetto): "Facoltativa: verrà usata solo per inviarti la conferma
  della prenotazione."
- Nuovo campo checkbox obbligatorio, dopo l'email:

  ```html
  <label class="booking-consent">
    <input v-model="termsAccepted" type="checkbox" required />
    Accetto i
    <router-link :to="{ name: 'terms' }" target="_blank">termini e condizioni</router-link>
    e ho letto l'
    <router-link :to="{ name: 'privacy' }" target="_blank">informativa privacy</router-link>
  </label>
  ```

  (`target="_blank"` perché siamo dentro un `<dialog>` modale: aprire
  nella stessa scheda perderebbe il form compilato.)
- `submitBooking` invia `{ eventGameId, participantName, participantEmail, termsAccepted }`.
  `startBooking` resetta anche `termsAccepted = false` insieme agli
  altri campi.
- Il commento sul reset del form che cita "un solo booking attivo per
  telefono" va riscritto: il motivo del reset resta valido (un
  form riaperto per un altro tavolo non deve precompilarsi), ma non è
  più legato a un vincolo di telefono.

### Le mie prenotazioni (localStorage)

Nuovo file `frontend/src/utils/myBookings.ts`:

```ts
interface MyBooking {
  id: number
  bookingCode: string
  eventId: number
  eventGameId: number
  gameLabel: string
  multiSeat: boolean
}

const STORAGE_KEY = 'bgm:my-bookings'

export function listMyBookings(): MyBooking[]
export function saveMyBooking(b: MyBooking): void
export function removeMyBooking(id: number): void
```

Tutte le funzioni avvolte in try/catch (localStorage può non essere
disponibile — navigazione privata, storage pieno — e in quel caso il
comportamento degrada a "nessuna prenotazione ricordata", mai a un
errore in pagina).

In `EventDetailView.vue`:
- al mount e dopo ogni `load()`, `listMyBookings()` filtrata sugli
  `eventGameId` di questo evento;
- per un gioco con una mia prenotazione attiva, al posto del bottone
  "Prenota" compare una pastiglia "Prenotato" più due azioni:
  "Annulla prenotazione" (stesso pattern di conferma di
  `ManageBookingView` — `window.confirm` poi
  `POST /bookings/{id}/cancel`, poi `removeMyBooking` e `load()`) e
  "Aggiungi risultato" (`router-link` a `{ name: 'booking-score', params: { code } }`);
- alla conferma di una nuova prenotazione, oltre a `confirmed.value.push(...)`,
  `saveMyBooking(...)` con i dati appena tornati dall'API.

`ManageBookingView.vue` non cambia: resta il percorso di riserva per
chi cambia dispositivo o naviga in incognito.

### `EventAdminDetailView.vue`

- `BookingAdminInfo` perde `participantEmail`/`participantPhone`.
- La riga `<span class="row-meta">{{ b.participantEmail }} · {{ b.participantPhone }}</span>`
  si rimuove; resta solo il nome sulla riga `admin-email booking-who`.

### `LoanDeskView.vue`

- `CopyBooking` perde `phone`.
- `startLending`/`pickBooking` non copiano più `.phone` in
  `borrowerPhone`: con una prenotazione sola precompilano solo il
  nome, `borrowerPhone` resta vuoto e va digitato a mano — come già
  accade oggi quando ci sono più prenotazioni tra cui scegliere.
- Il `<span class="row-meta">{{ b.phone }}</span>` nel picker delle
  prenotazioni si rimuove: resta solo il nome per riga.

## Regressioni note e accettate

Già discusse e approvate esplicitamente in fase di design:

1. **Nessun avviso via mail quando una prenotazione viene
   annullata** — né quando lo fa chi ha prenotato, né quando lo fa
   l'admin. Chi ha prenotato scopre l'annullamento solo tornando
   sulla pagina evento/gestione o controllando la chip
   "Prenotato" nel proprio browser.
2. **L'admin non vede più alcun contatto** (email o telefono) di chi
   ha prenotato, solo il nome.
3. **Il banco prestiti non precompila più il telefono** di chi
   ritira una copia già prenotata: va sempre digitato, anche con una
   sola prenotazione attiva sulla copia.
4. **Nessun vincolo server-side contro doppie prenotazioni**: il
   "già prenotato" è un suggerimento del browser (localStorage), non
   un'enforcement — cancellare i dati del sito, cambiare browser o
   dispositivo permette di riprenotare. Coerente con l'aver tolto
   telefono ed email come identificatori.

## Testing

- Backend: suite Go in Docker (vedi CLAUDE.md) dopo le modifiche a
  migrazioni, `events` e `httpapi`.
- Frontend: `npm run build` (type-check incluso) dopo le modifiche
  alle viste sopra elencate.
- Verifica manuale con Claude in Chrome: prenotare senza email,
  provare a confermare senza spuntare la checkbox (deve restare
  bloccato), prenotare con email e verificare che compaia la chip
  "Prenotato" dopo il reload della pagina evento, annullare da chip e
  verificare che torni "Prenota", controllare `EventAdminDetailView`
  e il banco prestiti senza più contatti.
