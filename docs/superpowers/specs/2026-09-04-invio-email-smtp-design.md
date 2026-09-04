# Invio email SMTP — design

Data: 2026-09-04
Stato: approvato

## Scopo

Rendere più fluida la gestione di prenotazioni e inviti mandando tre
email: l'invito a un nuovo amministratore, la conferma di una
prenotazione (con il codice e i link diretti per disdire o segnare i
punti) e l'avviso di annullamento.

L'associazione manda una decina di email a serata, non centinaia. Il
design è tarato su quel volume: nessuna coda, nessun retry, nessuno
storico degli invii.

## Invariante: SMTP è opzionale

**L'app senza SMTP configurato funziona esattamente come oggi.** Non è
uno stato degradato né un errore: è una configurazione valida e
supportata, come il provider AI. Ogni scelta di questo design va letta
attraverso questa regola.

| flusso | con SMTP | senza SMTP |
|---|---|---|
| invito admin | mail con il link + link copiabile in `UsersView` | link copiabile in `UsersView`, come oggi |
| prenotazione | mail con codice e link + codice a schermo | codice a schermo, come oggi |
| annullamento (partecipante) | ricevuta via mail | nessuna mail, come oggi |
| annullamento (admin) | avviso al partecipante | nessuna mail, come oggi |
| pannello impostazioni | campi compilati, prova disponibile | campi vuoti, prova disabilitata |

Conseguenze vincolanti:

- `mailer.ErrNotConfigured` non viene mai loggato come errore e non
  raggiunge mai una risposta HTTP: è il caso normale.
- Nessun handler cambia il proprio esito per un problema di posta. Una
  prenotazione, un annullamento e un invito riescono o falliscono per i
  loro motivi, mai per l'SMTP.
- Nessun campo SMTP è obbligatorio in `PUT /api/settings`, e salvare
  una configurazione incompleta o sbagliata è permesso.
- La UI non mostra mai un errore o un avviso perché l'SMTP manca. Dice
  che una mail è partita solo quando è vero (`mailQueued`).

L'unica eccezione è il bottone di prova: lì l'admin ha chiesto
esplicitamente un invio, quindi "non configurato" è una risposta
legittima da mostrare a schermo. È lo stesso taglio già scelto per la
traduzione AI, dove `translateDescription` non fallisce mai e
`translateLanguageHandler` sì.

## Architettura

### `internal/mailer` (nuovo)

Modellato su `internal/ai`: un package che parla un protocollo, con un
`Sender` iniettabile e un `ErrNotConfigured` che significa "l'app va
avanti senza".

```go
var ErrNotConfigured = errors.New("smtp not configured")

type Config struct {
    Host                     string
    Port                     int
    Username, Password       string
    FromAddress, FromName    string
    TLSMode                  string // "starttls" | "tls" | "none"
}

type Message struct {
    To, ToName       string
    Subject          string
    TextBody, HTMLBody string
}

type Sender interface {
    Send(ctx context.Context, m Message) error
}

type SMTPSender struct{ Config }
```

- `configured()` è `Host != "" && Port != 0 && FromAddress != ""`.
  Username e password vuote **non** rendono la configurazione
  incompleta: un relay in LAN senza autenticazione è legittimo.
- `TLSMode` copre i tre casi reali: `starttls` (587, il default e
  quello di Gmail, Mailjet, Brevo), `tls` (465, TLS implicito),
  `none` (relay interno). Un valore ignoto si comporta come
  `starttls`.
- `buildMessage(cfg, msg) ([]byte, error)` è **puro**: header `From`,
  `To`, `Subject`, `Date`, `Message-ID`, `MIME-Version`, e un
  `multipart/alternative` con boundary casuale che porta `text/plain`
  e `text/html` in quoted-printable. L'oggetto passa per la codifica
  RFC 2047 quando non è ASCII.
- `Send` rispetta il `ctx` e ha un timeout di dial proprio.

Si usa `net/smtp` della stdlib: nessuna dipendenza nuova in `go.mod`.
Il package è congelato, quindi STARTTLS, il TLS implicito su 465 e il
MIME sono codice nostro — circa 150 righe, tutte testabili.

### `httpapi/mail.go` (nuovo)

La glue, gemella di `httpapi/translate.go`:

- `func (s *Server) mailer(ctx) mailer.Sender` — restituisce `s.Mail`
  se iniettato (i test), altrimenti costruisce un `SMTPSender` dalle
  impostazioni salvate. Costruirlo per richiesta è ciò che permette di
  cambiare provider senza riavviare il container.
- `func (s *Server) sendMailAsync(m mailer.Message)` — goroutine con
  `context.Background()` e timeout 20s. `ErrNotConfigured` esce in
  silenzio; ogni altro errore va in `log.Printf`. La risposta HTTP non
  aspetta mai l'SMTP.
- `func (s *Server) publicBaseURL(r) string` —
  `settings.PublicBaseURL` se configurato, altrimenti ricavato da `r`
  (scheme + `Host`). Serve perché una mail non ha il
  `window.location.origin` su cui ripiega oggi `UsersView`.

`Server` guadagna il campo `Mail mailer.Sender`, nil in produzione.

### `httpapi/mail_templates.go` (nuovo)

Tre funzioni pure `(dati) → mailer.Message`, testabili sul contenuto
senza toccare la rete. Nessuna logica di invio.

### Migrazione `0012_smtp.sql`

Sette colonne nullable su `app_settings`: `smtp_host`, `smtp_port`,
`smtp_username`, `smtp_password`, `smtp_from_address`,
`smtp_from_name`, `smtp_tls_mode`. Forward-only, come le altre.

`settings.Settings` guadagna i campi corrispondenti, con lo stesso
trattamento `nullIfEmpty` già in uso. `smtp_password` segue
`bgg_api_token` e `ai_api_key`: esce mascherata, e un valore vuoto in
ingresso significa "lascia quella che c'è".

## Agganci ai flussi esistenti

| handler | mail | destinatario |
|---|---|---|
| `createUserHandler` | invito, link `{base}/invito/{token}` | il nuovo admin |
| `createBookingHandler` | codice + link gestione + link punteggio | il partecipante |
| `cancelBookingHandler` | ricevuta di annullamento | il partecipante |
| `adminCancelBookingHandler` | avviso che il posto è stato liberato | il partecipante |

`createBookingHandler` e `createUserHandler` aggiungono `mailQueued`
alla risposta: vero quando l'SMTP è configurato e la mail è stata
affidata all'invio. **Non è una conferma di consegna** — con l'invio
fire-and-forget non esiste al momento della risposta — e serve solo
alla UI per non promettere una mail che non partirà.

### Endpoint nuovo

`POST /api/settings/smtp/test`, dentro il gruppo protetto,
rate-limitato a 5 richieste al minuto.

Invio **sincrono** verso `currentUser(r).Email`. A differenza di tutto
il resto, qui gli errori si vedono: `409` con "SMTP non configurato" se
manca la configurazione, `502` con il messaggio SMTP vero (auth, TLS,
host irraggiungibile) se l'invio fallisce, `200` altrimenti.

## Frontend

### Pannello "Configurazione Email (SMTP)"

Terzo `panel-card` di `SettingsView`, dopo "Provider AI", con la stessa
grammatica: campi, `field-hint`, password mascherata.

Campi: Server, Porta (default 587), Sicurezza
(`STARTTLS` / `TLS` / `nessuna`), Nome utente, Password, Indirizzo
mittente, Nome mittente.

L'hint nomina i due casi reali — Gmail `smtp.gmail.com:587` con una app
password, Mailjet `in-v3.mailjet.com:587` con API key e secret — e dice
esplicitamente che lasciando il pannello vuoto l'app funziona come
prima, col codice di prenotazione solo a schermo.

Il bottone "Invia email di prova" sta sotto i campi e prova la
configurazione **salvata**, non quella nel form: resta disabilitato
finché `smtpConfigured` dal `GET /settings` non è vero, con l'hint
"salva prima di provare". L'esito è inline: `success` con l'indirizzo
raggiunto, o `error` con il messaggio SMTP.

### Rotte pubbliche nuove

```
/prenotazione/:code             mode='manage'
/prenotazione/:code/punteggio   mode='score'
/manage-booking                 invariata, chiede il codice
```

Tutte e tre servite da `ManageBookingView`, entrambe le nuove con
`meta: { public: true }`. Con `:code` presente la vista fa il lookup in
`onMounted`, nasconde il form del codice e, in `mode='score'`, porta il
focus sul blocco punteggio. Senza `:code` il comportamento è quello di
oggi.

Il codice di prenotazione viaggia in chiaro nel path: è una scelta
deliberata, perché è l'unica credenziale che il partecipante ha e
chiedergliela dopo avergli mandato il link vanificherebbe il link.
`LookupBooking` accetta già il solo codice, quindi non serve toccare
l'autorizzazione.

`LookupBooking` filtra `status = 'active'`, quindi un link cliccato
dopo l'annullamento restituisce 404. Su queste rotte il messaggio
diventa "questa prenotazione non è più attiva, o il link non è valido":
l'attuale "prenotazione non trovata" lì suonerebbe come un guasto
nostro.

### Altri ritocchi

- `BookingConfirmation.vue`: aggiunge "Ti abbiamo mandato anche una
  mail con il codice e i link" **solo** con `mailQueued` vero. Il
  commento in testa al componente, che oggi afferma "nessuna email
  parte", va riscritto.
- `UsersView.vue`: dopo l'invito mostra "Invito inviato a `email`"
  quando `mailQueued` è vero, tenendo comunque il link copiabile come
  fallback.

## Contenuti delle mail

`multipart/alternative`: testo semplice più HTML con stili inline e
tabelle, larghezza massima 560px, nessuna immagine e nessun font
esterno — i client di posta sono quello che sono. Palette da
`DESIGN.md`: feltro `#1f4d3a` per l'intestazione, carta `#faf6ec` per
il corpo, accento `#9c2b2b` per i bottoni, codice in mono su
`#f2ead4`. Tono e lingua come il resto dell'app: italiano, asciutto,
"dado & pedina".

| mail | oggetto | corpo |
|---|---|---|
| invito | Il tuo accesso da amministratore | chi ha invitato, bottone "Attiva il tuo accesso", nota che il link è personale e serve a scegliere la password |
| conferma | Prenotazione confermata: {gioco} — {data} | gioco (con `#copia` se l'evento ne porta più di una), evento, data e ora, il codice in evidenza, i due bottoni, la nota sul tavolo condiviso quando ha più posti prenotabili |
| annullamento | Prenotazione annullata | conferma se ha annullato il partecipante, avviso se ha annullato l'organizzazione, e il link all'evento per riprenotare |

## Verifica

- `internal/mailer`: `buildMessage` verificato sui byte prodotti
  (header, boundary, quoted-printable, oggetto non-ASCII); `Send`
  contro un finto server SMTP in-process con `TLSMode: "none"`.
- `httpapi/mail_templates_test.go`: i tre template contengono codice e
  URL attesi, e il link usa `PublicBaseURL` quando è configurato.
- `httpapi`: un `fakeMailer` con canale bufferizzato, così i test non
  dipendono dai tempi della goroutine. Per ogni flusso, due test — con
  mailer configurato e **senza**, a verificare che l'esito HTTP sia
  identico.
- `POST /api/settings/smtp/test`: i tre esiti (409, 502, 200).
- Suite Go completa in Docker; `npm run build` per il type-check.
- Ultimo passo: `/impeccable` su `SettingsView` e `ManageBookingView`.

## Fuori scope

- Coda persistente, retry, storico degli invii.
- Promemoria prima dell'evento, mail all'organizzatore a ogni
  prenotazione, disiscrizione.
- Allegati (per esempio il manuale PDF del gioco).
- Un nome dell'associazione riusabile fuori dalla posta: qui esiste
  solo "Nome mittente", nel pannello SMTP.
