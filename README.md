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
  Ogni gioco ha anche un numero di **posti prenotabili per copia**: `1`
  per un gioco da tavolo normale, dove chi prenota si prende la copia e
  si porta i suoi; più di 1 per un tavolo aperto — una partita a D&D, un
  gioco di ruolo, un torneo — dove ci si iscrive uno alla volta, ognuno
  con il proprio codice, e ognuno può disdire senza far saltare la
  serata agli altri.
- **Eventi**: un evento porta più copie dello stesso gioco, ognuna con i
  propri posti prenotabili presi dal catalogo al momento in cui la copia
  entra nell'evento. Nella pagina pubblica le copie compaiono numerate
  (`Carcassonne #1`, `Carcassonne #2`) quando sono più d'una, e si
  prenotano separatamente.
- **Prenotazioni anonime**: nome, email, telefono — nessun account
  richiesto. Alla prenotazione viene generato un codice che permette in
  seguito di cancellarla o di inserire il punteggio finale. Su una copia
  con più posti prenotabili il punteggio è **uno per copia**: lo inserisce
  o lo corregge chiunque abbia prenotato lì, e resta finché la copia ha
  almeno una prenotazione attiva.
- **Punteggi e classifiche**: a fine partita si registrano i punteggi dei
  giocatori (nomi liberi); la classifica per gioco aggrega partite
  giocate, vittorie e punteggio medio/totale nel tempo — ogni copia
  conta come una partita sola, indipendentemente da quanti hanno
  prenotato quel tavolo.
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

## Immagine pubblicata

Ogni tag `vX.Y.Z` pubblica su Docker Hub un'immagine multi-architettura
(`linux/amd64` e `linux/arm64`), quindi gira anche su un Raspberry Pi con
sistema a 64 bit:

```bash
docker run -d --name boardgames-manager \
  -p 8080:8080 \
  -v "$PWD/data:/data" \
  --restart unless-stopped \
  <utente-dockerhub>/boardgame-manager:latest
```

Con docker-compose, basta sostituire `build: .` con l'immagine:

```yaml
services:
  app:
    image: <utente-dockerhub>/boardgame-manager:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    restart: unless-stopped
```

Per pubblicare una nuova versione serve un tag git:

```bash
git tag v1.0.0 && git push origin v1.0.0
```

Il workflow `.github/workflows/docker-publish.yml` costruisce e carica
l'immagine, taggandola `1.0.0`, `1.0`, `1` e `latest`. Richiede due
secret nel repository GitHub (*Settings → Secrets and variables →
Actions*): `DOCKERHUB_USERNAME` e `DOCKERHUB_TOKEN` (un access token
Docker Hub con permessi di scrittura).

Se i due secret non sono configurati il workflow non costruisce nulla: si
chiude senza errori e riporta nel riepilogo del run quali secret mancano.

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

**Attenzione se aggiorni un'installazione esistente**: la migrazione
`0008_seats_and_copies.sql` (posti prenotabili per copia) ricrea vuote le
tabelle di prenotazioni, punteggi e giochi-evento — SQLite non permette
di rimuovere il vecchio vincolo `UNIQUE(event_id, game_id)` con un
`ALTER TABLE`. Il catalogo giochi e gli eventi restano intatti, ma **gli
eventi creati prima dell'aggiornamento perdono i giochi collegati** e
vanno ripopolati dalla pagina dell'evento stesso.

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
