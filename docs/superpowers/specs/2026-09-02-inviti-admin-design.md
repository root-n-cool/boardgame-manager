# Inviti admin e rifacimento pagina /users — design

Data: 2026-09-02

## Problema

Oggi `/users` è l'ultima pagina non ancora ridisegnata: un `<h1>`, una
`<ul>` di email con un bottone "Rimuovi" testuale, e un form con email
**e password** in chiaro. Quel form ha due difetti:

1. chi crea un amministratore ne conosce la password;
2. la pagina non parla la lingua visiva del resto dell'app
   (`/games`, `/events`).

## Obiettivo

- Un amministratore aggiunge un collega inserendo **solo l'email**.
- Il sistema genera un **link di invito** che chi invita copia e manda a
  mano (WhatsApp, Telegram, voce): nessun invio email, coerente con la
  scelta "niente SMTP in v1".
- Il destinatario apre il link e **sceglie la propria password**: chi lo
  ha invitato non la conosce mai.
- La pagina `/users` adotta il linguaggio visivo del catalogo giochi.

## Decisioni prese in brainstorming

- **Link stabile**: il token è generato una volta alla creazione e
  salvato in chiaro nel DB, così "Copia link invito" mostra sempre lo
  stesso link e si può ricopiare quante volte serve. Nessun bottone
  "rigenera".
- **Nessuna scadenza**: il link vale finché l'admin invitato non
  imposta la password, o finché chi lo ha invitato non elimina la riga.
- **Layout a righe**: un pannello con una riga per admin e una riga
  tratteggiata "Aggiungi admin" in fondo, non una griglia di card.
- **Inserimento inline**: la riga tratteggiata diventa un campo email;
  alla creazione si trasforma nella riga del nuovo admin "In attesa",
  con il link e il bottone "Copia link invito" subito sotto. Nessuna
  modale nel flusso di invito.
- **Login automatico**: impostata la password parte la sessione e il
  nuovo admin arriva su `/users`. Il link, a quel punto, è morto.

## Modello dati

Approccio scelto: **invito inline nella tabella `users`**.

`backend/internal/db/migrations/0006_admin_invites.sql`:

```sql
ALTER TABLE users ADD COLUMN invite_token TEXT;

CREATE UNIQUE INDEX users_invite_token_unique
    ON users(invite_token) WHERE invite_token IS NOT NULL;
```

- Un admin **in attesa** è una riga `users` con `password_hash = ''` e
  `invite_token` valorizzato (32 byte random in hex).
- Un admin **attivo** ha `invite_token IS NULL`.
  `invite_token IS NULL` è **l'unico** criterio di "attivo" usato dal
  codice: `password_hash` resta un dettaglio interno.
- All'attivazione si scrive l'hash bcrypt e si azzera il token, in una
  sola `UPDATE`: il link diventa inutilizzabile senza tabelle di stato.

Perché non una tabella `admin_invites` separata: la lista utenti
diventerebbe una UNION di due fonti, il delete avrebbe due percorsi e
l'unicità dell'email andrebbe verificata su due tabelle — senza alcun
beneficio, dato che non c'è né scadenza né storico inviti da tenere.
`password_hash` resta `NOT NULL` (la sentinella è la stringa vuota)
per evitare la ricostruzione della tabella `users`, che in SQLite
significa maneggiare la foreign key `sessions.user_id`.

## API

### Protette (sessione admin)

- `POST /api/users` — body `{"email": "..."}`. Il campo `password`
  **non è più accettato**: se presente viene ignorato. Genera il token
  con `auth.GenerateToken()`, crea la riga in attesa, risponde `201`
  con `{id, email, pending: true, inviteToken}`. `409` se l'email è
  già in uso (attiva o in attesa), `400` se l'email è vuota.
- `GET /api/users` — ogni elemento aggiunge `pending: bool` e
  `inviteToken: string | null` (`null` per gli attivi). Il token in
  chiaro è visibile solo a un admin autenticato: è esattamente ciò che
  serve al bottone "Copia link invito".
- `DELETE /api/users/{id}` — invariato nell'interfaccia; cambia la
  semantica del guard (vedi sotto).

### Pubbliche (rate-limited, 10 richieste/minuto per IP)

- `GET /api/invites/{token}` — `200 {"email": "..."}` se il token
  esiste, `404` altrimenti (token inesistente o già usato). Serve alla
  pagina di attivazione per validare il link e mostrare a chi
  appartiene.
- `POST /api/invites/{token}` — body `{"password": "..."}`, minimo 8
  caratteri. Scrive l'hash, azzera `invite_token`, apre la sessione con
  `startSession` e risponde `200 {id, email}`. `404` su token non
  valido, `400` su password troppo corta.

L'URL completo del link **non** viene costruito dal backend: il
frontend lo compone come `window.location.origin + '/invito/' + token`.
Così non serve configurare un base-URL per il self-host.

### Login

`loginHandler` rifiuta esplicitamente un utente con
`password_hash == ""` con lo stesso `invalid credentials` degli altri
casi. bcrypt fallirebbe comunque su un hash vuoto, ma il controllo
esplicito rende l'intento leggibile e non dipende dal comportamento
della libreria.

## Store utenti

`backend/internal/users/store.go`:

- `User` guadagna `InviteToken *string` (nil = attivo) e il metodo
  derivato `Pending() bool`.
- `CreateInvite(ctx, email, token) (User, error)` — inserisce con
  `password_hash = ''`; `ErrDuplicateEmail` come `Create`.
- `GetByInviteToken(ctx, token) (User, error)` — `ErrNotFound` se
  assente.
- `Activate(ctx, id, passwordHash) error` — `UPDATE users SET
  password_hash = ?, invite_token = NULL WHERE id = ? AND invite_token
  IS NOT NULL`; `ErrNotFound` se `RowsAffected() == 0`, così due POST
  concorrenti sullo stesso link non impostano due password diverse.
- `Create` resta per il bootstrap del primo admin.
- `DeleteIfNotLast` cambia criterio: nella stessa transazione di prima
  conta gli admin **attivi diversi dal target**
  (`WHERE invite_token IS NULL AND id != ?`) e rifiuta con
  `ErrCannotDeleteLastUser` solo se quel conteggio è `0`. Due
  conseguenze volute: un invito pendente non tiene in vita l'istanza
  (se contasse, si potrebbe cancellare l'unico admin attivo e lasciare
  nessuno in grado di entrare, mentre `POST /api/bootstrap` resterebbe
  chiuso perché guarda `COUNT(*)` totale), e un invito pendente si può
  sempre revocare, anche quando l'admin attivo è uno solo.

## Frontend

### `/users` — `UsersView.vue` riscritta

- `page-head` come `/games`: `<h1>Amministratori</h1>` più
  `page-meta` col conteggio ("1 amministratore" / "N amministratori").
- Un `panel-card` con una riga per admin:
  - pedina tonda con l'iniziale dell'email;
  - email;
  - badge di stato: `● Attivo` / `○ In attesa`;
  - azioni a destra: "Copia link invito" (solo se in attesa) ed
    "Elimina", entrambi con icona (link e cestino).
- Sotto la riga di un admin in attesa, il link in `font-variant-numeric:
  tabular-nums` troncato con ellissi e il bottone di copia
  (`navigator.clipboard.writeText`, con feedback "Copiato" per ~2s e
  fallback: se la clipboard API non è disponibile o rifiuta, il link
  resta selezionabile a mano).
- Ultima riga, tratteggiata: "Aggiungi admin" con l'icona `+`. Al click
  diventa un campo email (autofocus) con "Invita" e "Annulla" (Esc
  annulla). Alla creazione la riga torna al suo stato e il nuovo admin
  appare in lista, in attesa, col link già visibile.
- "Elimina" chiede conferma con `ModalDialog` (il `<dialog>` nativo già
  usato in `GameDetailView`) invece di `window.confirm`.
- Gli errori restano nel `<p class="error">` già previsto: `409` email
  duplicata e `409` ultimo admin hanno messaggi italiani espliciti.

### `/invito/:token` — `InviteAcceptView.vue` nuova

- Route **pubblica** (`meta: { public: true }`), stile `.auth-page`
  come `SetupView`/`LoginView`, mobile-first.
- Al mount chiama `GET /api/invites/{token}`:
  - **valido**: titolo "Imposta la tua password", email in sola lettura
    (`disabled`), campo password (`minlength=8`) e conferma password —
    la conferma è client-side, il backend riceve una sola password;
  - **non valido**: "Questo invito non è più valido" con la
    spiegazione (link già usato o revocato) e un link a `/login`.
- Al submit, `POST /api/invites/{token}`: aggiorna lo store auth con
  l'utente restituito (nuova action `acceptInvite` in
  `stores/auth.ts`, simmetrica a `bootstrap`) e
  `router.push({ name: 'users' })`.
- `client.ts` non va toccato: i token invalidi rispondono `404`, non
  `401`, quindi non scatta il redirect automatico al login.

### Stili

Nuove classi in `frontend/src/app.css`, con i token esistenti: riga
admin, pedina con iniziale, badge di stato, riga "aggiungi"
tratteggiata (riuso del linguaggio di `.add-card`). I nuovi pattern
vanno documentati in `DESIGN.md`.

## Test

Backend (`go test ./...` in Docker):

- `users/store_test.go`: `CreateInvite` + `GetByInviteToken`;
  `Activate` che azzera il token e rende il secondo tentativo
  `ErrNotFound`; `DeleteIfNotLast` che rifiuta la cancellazione
  dell'unico admin **attivo** anche in presenza di inviti pendenti, e
  che permette di cancellare un invito pendente quando c'è un solo
  admin attivo.
- `httpapi/users_handlers_test.go`: `POST /api/users` con sola email
  restituisce il token; email duplicata `409`; `GET /api/users`
  espone `pending`/`inviteToken`.
- `httpapi/invites_handlers_test.go` (nuovo): `GET` valido/invalido;
  `POST` che imposta la password, azzera il token, setta il cookie di
  sessione; `POST` due volte sullo stesso token → `404`; password
  troppo corta → `400`.
- `httpapi/auth_handlers_test.go`: login di un utente in attesa →
  `401`.

Frontend: `npm run build` (include il type-check `vue-tsc`).

Chiusura: `/impeccable polish` su `/users` e sulla pagina di invito,
come ultimo task.

## Fuori scope

- Invio email dell'invito (niente SMTP in v1).
- Scadenza e rigenerazione del link.
- Reset password per un admin già attivo: è una feature diversa, con i
  suoi problemi (chi la chiede? chi la autorizza?). Da valutare a
  parte.
- Ruoli o permessi differenziati: resta un solo ruolo admin.
