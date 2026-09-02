# BoardGames Manager

Applicazione selfhost, leggera, per gestire le serate di un'associazione
ludica: un admin mantiene un catalogo di giochi (arricchito con dati da
[BoardGameGeek](https://boardgamegeek.com)) e organizza eventi; chi
partecipa prenota un gioco per evento senza bisogno di un account, e a
fine partita registra il punteggio finale, alimentando una classifica
storica per ciascun gioco.

Un solo binario/container, SQLite come unico storage, nessun servizio
esterno obbligatorio.

## Funzionalità

- **Catalogo giochi**: import da BoardGameGeek (nome, anno, numero
  giocatori, copertina) o inserimento manuale; ogni gioco ha una o più
  lingue, ciascuna con i propri media (manuale PDF, link, video YouTube).
- **Eventi**: un evento include più giochi, ciascuno con una quantità di
  copie disponibili.
- **Prenotazioni anonime**: nome, email, telefono — nessun account
  richiesto. Alla prenotazione viene generato un codice che permette in
  seguito di cancellarla o di inserire il punteggio finale.
- **Punteggi e classifiche**: a fine partita si registrano i punteggi dei
  giocatori (nomi liberi); la classifica per gioco aggrega partite
  giocate, vittorie e punteggio medio/totale nel tempo.
- **Amministrazione**: bootstrap del primo admin al primo avvio (come
  n8n); dopo, un admin ne invita un altro inserendo solo l'email — il
  sistema genera un link di invito che l'admin copia e recapita a mano
  (nessuna email viene inviata), e chi lo riceve apre il link e sceglie la
  propria password, che chi lo ha invitato non conosce. Il link non scade
  e resta valido finché non viene usato o l'invito non viene eliminato.
  Impostazioni per la lingua di default, l'indirizzo pubblico dell'app e
  il token BoardGameGeek.

## Stack tecnico

- **Backend**: Go, router [chi](https://github.com/go-chi/chi), SQLite
  cgo-free ([`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)),
  migrazioni imbeddate via `//go:embed` applicate automaticamente
  all'avvio.
- **Frontend**: Vue 3 + Vite, build statico embeddato nel binario Go —
  un solo processo serve sia le API che l'interfaccia.
- **Auth**: sessioni via cookie httpOnly, password con bcrypt.
- **Deployment**: Docker/docker-compose, un solo servizio, volume su
  `/data` per il database SQLite e gli upload.

## Avvio rapido (Docker)

```bash
docker compose up -d --build
```

L'app è disponibile su [http://localhost:8080](http://localhost:8080).
Al primo accesso viene richiesto di creare il primo account admin.

I dati (database SQLite + upload) vivono nella cartella `./data`, montata
come volume nel container — sopravvivono a rebuild e restart.

Variabili d'ambiente (già impostate in `docker-compose.yml`):

| Variabile  | Descrizione                          | Default   |
| ---------- | ------------------------------------- | --------- |
| `PORT`     | Porta HTTP del server                 | `8080`    |
| `DATA_DIR` | Cartella per database SQLite e upload | `/data`   |

## Sviluppo locale

### Backend

```bash
cd backend
go run ./cmd/server
```

Richiede Go 1.25+ (vedi `backend/go.mod`). Il server legge/scrive in
`DATA_DIR` (default: `./data`, creata automaticamente se assente) e
serve le API sotto `/api`.

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Il dev server di Vite gira su una porta separata e fa da proxy verso
`/api` puntando a `http://localhost:8080` (vedi `frontend/vite.config.ts`)
— avvia anche il backend in parallelo per avere l'app funzionante.

Per produrre il build statico che il backend Go serve/imbedda:

```bash
npm run build
```

L'output va in `backend/internal/webui/dist`, dentro il binario Go tramite
`//go:embed` alla compilazione successiva.

### Migrazioni database

I file `.sql` in `backend/internal/db/migrations/` vengono applicati in
ordine alfabetico all'avvio del server, ognuno in una transazione, e
tracciati in una tabella `schema_migrations` (idempotente). Solo
migrazioni forward: per un cambio di schema si aggiunge un nuovo file
numerato, mai si modifica uno esistente.

## Configurazione opzionale

Dalla pagina "Impostazioni" (da admin autenticato):

- **Indirizzo pubblico**: l'URL da cui l'associazione raggiunge l'app, per
  esempio `https://giochi.example.org`. Serve a comporre i link che escono
  dall'app — oggi l'invito di un amministratore — quando chi li genera non
  sta navigando dallo stesso indirizzo (dietro un proxy, o da `localhost`
  mentre gli altri usano il dominio). Se lo lasci vuoto si usa l'indirizzo
  da cui stai navigando, quindi un'installazione locale funziona senza
  configurare niente.
- **BoardGameGeek API token**: per la ricerca e l'import di giochi da BGG.
  Se assente, l'import automatico è disabilitato senza bloccare il resto
  dell'app: i giochi si inseriscono a mano.

Il link di invito contiene un token che vale un accesso da amministratore:
mandalo su un canale privato, e cancella dalla lista un invito che non
serve più. Come ogni URL, finisce anche nei log del container.

## Documentazione di progetto

- [`PRODUCT.md`](PRODUCT.md) — utenti, scopo, contesto d'uso.
- [`DESIGN.md`](DESIGN.md) — sistema visivo (colori, tipografia,
  componenti).
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — spec di design
  per fase di sviluppo.

## Licenza

[MIT](LICENSE)
