# BoardGames Manager — Claude Project Guidelines

## Project Overview

Applicazione selfhost per gestire le serate di un'associazione di giochi
da tavolo. Un admin mantiene un catalogo di giochi (arricchito con dati
da BoardGameGeek) e crea eventi; i partecipanti prenotano un gioco per
evento **senza account** (nome + email + telefono) e a fine partita
inseriscono i punteggi, che alimentano una classifica storica per gioco.

Un solo binario/container, SQLite come unico storage, nessun servizio
esterno obbligatorio.

## Technology Stack

```yaml
Backend:    Go 1.25 (monolite), router chi, SQLite cgo-free (modernc.org/sqlite)
Frontend:   Vue 3 (Composition API, <script setup>) + TypeScript + Vite + Pinia
Auth:       sessioni via cookie httpOnly (no JWT), password con bcrypt
Migrazioni: runner custom su embed.FS (backend/internal/db/migrate.go)
Deploy:     Docker/docker-compose, un servizio, volume su /data
Lingua UI:  italiano (nessun i18n, stringhe dirette nei componenti)
```

## File Structure

```
backend/
├── cmd/server/                  # entrypoint del binario
├── internal/
│   ├── auth/                    # password, sessioni, token
│   ├── bgg/                     # client BoardGameGeek (XML API2)
│   ├── db/                      # apertura DB + runner migrazioni
│   │   └── migrations/          # NNNN_nome.sql embeddati, forward-only
│   ├── events/                  # eventi, prenotazioni, match/punteggi
│   ├── games/                   # catalogo giochi, lingue, media
│   ├── httpapi/                 # router, handler, middleware, rate limit
│   ├── leaderboard/             # aggregazione classifiche per gioco
│   ├── settings/                # impostazioni (chiavi API opzionali)
│   ├── storage/                 # upload su filesystem (manuali PDF)
│   ├── users/                   # utenti admin
│   └── webui/                   # embed del build frontend (dist/)
frontend/
├── src/
│   ├── main.ts                  # entrypoint
│   ├── App.vue
│   ├── app.css                  # stili globali + design tokens
│   ├── api/client.ts            # wrapper fetch verso /api
│   ├── router/                  # Vue Router
│   ├── stores/                  # Pinia (auth)
│   ├── components/              # componenti condivisi
│   ├── views/                   # pagine (admin + pubbliche)
│   └── assets/fonts/            # font self-hosted
docs/superpowers/
├── specs/                       # spec di design per fase
└── plans/                       # piani di implementazione
PRODUCT.md                       # utenti, scopo, contesto d'uso
DESIGN.md                        # sistema visivo (colori, tipografia, componenti)
```

Il build del frontend finisce in `backend/internal/webui/dist`
(gitignorato) e viene imbeddato nel binario Go al build successivo.

## Ambiente di sviluppo

- **Il toolchain Go locale è rotto** (binario x86_64 su Mac arm64):
  qualunque `go build`/`go test`/`go run` in locale fallisce o usa il
  binario sbagliato. Esegui **sempre** i comandi Go in Docker,
  riusando i due volumi nominati per la cache moduli/build:

  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```

  Non montare volumi anonimi o nuovi al posto di `bgm-gomodcache` /
  `bgm-gocache`, altrimenti ogni esecuzione riparte da zero. La
  versione Go è quella in `backend/go.mod` (immagine `golang:X.Y`
  corrispondente).
- Per questo i target `backend-*` del `Makefile` (che lanciano `go`
  in locale) **non funzionano su questa macchina**: usa Docker.
- Il frontend gira regolarmente in locale: `npm install`,
  `npm run dev`, `npm run build` in `frontend/` senza Docker.
- App completa: `docker compose up -d --build` → http://localhost:8080
  (dati in `./data`, montata come volume).

## Decisioni chiave già prese

Non ridiscutere questi punti senza un motivo nuovo emerso — sono il
risultato di una sessione di brainstorming con l'utente.

**Architettura & persistenza**

- Migrazioni: nessuna dipendenza esterna (`goose`/`golang-migrate`), ma
  un runner custom (~70 righe). I file `.sql` in
  `backend/internal/db/migrations/` (naming `NNNN_nome.sql`, es.
  `0001_init.sql`) sono embeddati con `//go:embed`, applicati in ordine
  alfabetico all'avvio, ognuno in una transazione, e registrati in
  `schema_migrations` (idempotente). **Solo forward**: nessun
  down/rollback, nessuna modifica a un file già rilasciato — per un
  cambio di schema si aggiunge il file col numero successivo.
- Un solo binario serve API e frontend: niente processi o servizi
  aggiuntivi.

**Auth & ruoli**

- Bootstrap del primo utente come n8n: se non esistono utenti nel DB, il
  primo accesso forza la registrazione del primo admin.
- Un solo ruolo: admin/organizzatore a pieni permessi. Non esiste un
  ruolo "utente normale loggato" — i partecipanti non hanno account.

**Prenotazioni & punteggi**

- Prenotazioni anonime: nome, email, telefono. Vincolo: un solo booking
  attivo per coppia `(event_id, telefono)`.
- Alla prenotazione si genera un `booking_code` mostrato a schermo.
  L'invio email è **opzionale**: configurando un server SMTP nelle
  impostazioni partono la conferma di prenotazione (col codice e i link
  diretti a disdetta e punteggi), l'invito di un amministratore e
  l'avviso di annullamento. Senza SMTP l'app funziona per intero come
  prima — il codice resta a schermo e il link d'invito si copia a mano.
  Vale la stessa regola del provider AI: nessun campo obbligatorio,
  nessun errore in UI perché la posta manca.
- Il `booking_code` da solo permette poi di cancellare la prenotazione
  oppure inserire il punteggio finale (nomi giocatori liberi +
  punteggio numerico per ciascuno, vince il punteggio più alto).
- Classifica per gioco: match sul nome giocatore normalizzato
  (lowercase + trim), nessun registro giocatori separato.
- Un evento può includere più copie dello stesso gioco (quantità
  configurabile per gioco per evento).

**Catalogo giochi & arricchimento**

- Dati base da BoardGameGeek (XML API2, nessuna autenticazione).
  BGG **non** offre un'API autenticata per scaricare file dalla sezione
  "Files" (richiede login sul sito): non provare a scrappare quella
  sezione.
- L'arricchimento automatico (manuale PDF via search API, tutorial via
  YouTube Data API) parte **solo** se l'admin ha configurato le
  relative chiavi nelle impostazioni; altrimenti inserimento o upload
  manuale. L'admin conferma sempre i risultati automatici prima del
  salvataggio.
- Ogni gioco ha una o più `GameLanguage` (lingua base scelta alla
  creazione, altre aggiungibili dopo), ciascuna con una lista di media
  di tipo file/link/youtube. Il gioco ha anche un campo `owner`
  testuale libero.

## General Rules

- Usa `docker compose up -d --build` per avviare l'app completa.
- Comandi Go **solo** in Docker (vedi *Ambiente di sviluppo*);
  `npm` in locale.
- Usa il server MCP **Context7** per recuperare documentazione di
  librerie/framework, salvo indicazione diversa.
- Mantieni le dipendenze al minimo: prima di aggiungerne una nuova
  (Go o npm) chiedi — il progetto preferisce stdlib e codice esplicito.
- Nessun i18n: l'interfaccia è in italiano, stringhe direttamente nei
  componenti.

## Workflow Rules

### Spec → piano → implementazione

- **Brainstorming di default.** Se non specificato diversamente, prima
  di qualsiasi lavoro creativo (nuove feature, componenti, modifiche di
  comportamento) usa lo skill `superpowers:brainstorming` per esplorare
  intento e requisiti **prima** di scrivere codice.
- Le spec di design vivono in `docs/superpowers/specs/`, i piani di
  implementazione in `docs/superpowers/plans/` (creati con la skill
  `writing-plans` dopo l'approvazione della spec). Naming per data:
  `YYYY-MM-DD-nome.md`.
- Sviluppo per fasi separate: fondamenta auth+admin → catalogo
  giochi+BGG → eventi/prenotazioni → punteggi/classifiche. Ogni fase
  può avere il proprio ciclo spec → piano → implementazione.
- Niente tracker esterno (Linear, Basecamp, ecc.): piani e avanzamento
  restano file locali nel repo.

### Test

- TDD dove ha senso: il backend ha test accanto a ogni package
  (`*_test.go`) — mantieni la copertura quando aggiungi handler o logica.
- Prima di dichiarare un lavoro concluso lancia la suite backend in
  Docker e, se hai toccato il frontend, `npm run build` (fa anche il
  type-check con `vue-tsc`). Nessuna affermazione di "funziona" senza
  output di verifica.

### Frontend

- **Pass `impeccable` a fine lavoro.** Ogni intervento che tocca
  frontend/UI deve prevedere, come **ultimo task della todo list**, il
  lancio di `/impeccable` sulla superficie modificata (es. `polish` /
  `audit` / `critique`) prima di considerare il lavoro concluso.
- Il sistema visivo (colori, tipografia, componenti, tono "dado &
  pedina") è documentato in `DESIGN.md`: rispettalo e aggiornalo quando
  introduci nuovi pattern. I token stanno in `frontend/src/app.css`.
- Le pagine pubbliche (prenotazione, gestione booking, inserimento
  punteggi) sono usate da smartphone, in piedi al tavolo: mobile-first
  e pochi passaggi.

### Git

- Branch unico **`main`**: non esiste un branch di integrazione
  separato. Per lavori lunghi o rischiosi usa un worktree/branch
  dedicato e poi merge su `main`.
- Commit e push **solo se richiesti esplicitamente**. Messaggi in
  inglese, stile conventional commits (`feat:`, `fix:`, `docs:`),
  coerenti con la history.

### Documentazione

- `README.md` è rivolto a chi installa/usa l'app: aggiornalo quando
  cambiano avvio, variabili d'ambiente o funzionalità visibili.
- `PRODUCT.md` (utenti e contesto) e `DESIGN.md` (sistema visivo) sono
  la fonte per le decisioni di prodotto/UI.

## Comportamento

Sii laconico. Rispondi in italiano.
