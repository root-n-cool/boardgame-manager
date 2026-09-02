# CLAUDE.md

## Progetto

BoardGames Manager: applicazione selfhost per la gestione di eventi di
un'associazione di giochi da tavolo. Un admin crea eventi e aggiunge
giochi al catalogo (dati arricchiti da BoardGameGeek), i partecipanti
si prenotano un gioco per evento senza bisogno di login (email +
telefono), e a fine partita inseriscono i punteggi che alimentano una
classifica per gioco nel tempo.

Il design completo vive in
`docs/superpowers/specs/2026-08-31-boardgames-manager-design.md`.

## Stack tecnico

- **Backend**: Go (monolite), router `chi` o stdlib `net/http`,
  SQLite (driver cgo-free `modernc.org/sqlite`), migrazioni con un
  runner custom basato su `embed.FS` (vedi sotto).
- **Frontend**: Vue 3 + Vite, build statico embeddato nel binario Go
  (`embed.FS`) — un solo binario/container serve sia API che frontend.
- **Auth**: sessioni via cookie httpOnly (no JWT), password hashate
  con bcrypt/argon2.
- **Deployment**: Docker/docker-compose, un solo servizio, volume su
  `/data` per il file SQLite e gli upload (manuali PDF).

## Decisioni chiave già prese

Non ridiscutere questi punti senza un motivo nuovo emerso — sono il
risultato di una sessione di brainstorming con l'utente:

- Migrazioni: niente dipendenza esterna (`goose`/`golang-migrate`), ma
  un runner custom in `backend/internal/db/migrate.go` (~70 righe). I
  file `.sql` in `backend/internal/db/migrations/` (naming
  `NNNN_nome.sql`, es. `0001_init.sql`) sono embeddati nel binario con
  `//go:embed`, applicati in ordine alfabetico all'avvio, ognuno in una
  transazione, e registrati in una tabella `schema_migrations` che
  rende l'operazione idempotente. Solo migrazioni forward: non esiste
  il down/rollback. Per aggiungere una migrazione basta creare un
  nuovo file `.sql` con il numero successivo.
- Bootstrap primo utente come n8n: se non esistono utenti nel DB, si
  forza la registrazione del primo admin al primo accesso.
- Un solo ruolo utente: admin/organizzatore a pieni permessi. Non
  esiste un ruolo "utente normale loggato" — i partecipanti agli
  eventi non hanno account.
- Le prenotazioni sono anonime: nome, email, telefono. Vincolo: un
  solo booking attivo per coppia `(event_id, telefono)`.
- Alla prenotazione viene generato un `booking_code` mostrato a
  schermo. Non si invia nessuna email (niente SMTP richiesto in v1).
- Il `booking_code` da solo permette in seguito di: cancellare la
  prenotazione, oppure inserire il punteggio finale (nomi giocatori
  liberi + punteggio numerico per ciascuno, vince chi ha il punteggio
  più alto).
- Classifica per gioco: match sul nome giocatore normalizzato
  (lowercase + trim), nessun registro giocatori separato da gestire.
- Catalogo giochi: dati base da BoardGameGeek (XML API2, nessuna
  autenticazione richiesta). BGG **non** offre un'API con
  autenticazione per scaricare file dalla sezione "Files" (richiede
  login utente sul sito) — non provare a scrappare quella sezione.
- Arricchimento automatico (manuale PDF via search API, tutorial via
  YouTube Data API) parte solo se l'admin ha configurato le relative
  chiavi API nelle impostazioni; altrimenti si passa a inserimento o
  upload manuale. L'admin conferma sempre i risultati trovati
  automaticamente prima che vengano salvati.
- Ogni gioco ha una o più `GameLanguage` (lingua base scelta alla
  creazione, altre aggiungibili dopo), ciascuna con una lista di
  media di tipo file/link/youtube. Il gioco ha anche un campo
  `owner` testuale libero.
- Un evento può includere più copie dello stesso gioco (quantità
  configurabile per gioco per evento).
- Niente tracker esterno (Linear, ecc.): piani e avanzamento restano
  file locali nel repo (spec in `docs/superpowers/specs/`, piano di
  implementazione generato con la skill `writing-plans`).

## Ambiente di sviluppo

- **Il toolchain Go installato in locale su questa macchina è rotto**
  (binario Go x86_64 su Mac arm64): qualunque `go build`/`go test`/`go run`
  lanciato in locale fallisce o usa il binario sbagliato. Esegui SEMPRE i
  comandi Go dentro Docker, esempio:
  ```
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Riusa sempre questi due volumi nominati (`bgm-gomodcache`, `bgm-gocache`)
  per cache moduli/build tra un'esecuzione e l'altra — non montarne di
  nuovi o usare `-v $(pwd)... :/root/...` senza nome, altrimenti si riparte
  da zero ogni volta. La versione Go del progetto è quella in
  `backend/go.mod` (immagine Docker `golang:X.Y` corrispondente).
- Il frontend (`npm`/`node`, `frontend/`) funziona regolarmente in locale:
  non serve Docker per `npm run build`/`npm run dev`.

## Convenzioni di lavoro

- Le spec di design vivono in `docs/superpowers/specs/`.
- I piani di implementazione si creano con la skill `writing-plans`
  dopo l'approvazione della spec.
- Sviluppo per fasi separate: fondamenta auth+admin → catalogo
  giochi+BGG → eventi/prenotazioni → punteggi/classifiche. Ogni fase
  può avere il proprio ciclo spec→piano→implementazione se necessario.
