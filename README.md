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
- **Luogo dell'evento**: l'indirizzo si cerca su OpenStreetMap mentre lo
  si scrive e la pagina pubblica dell'evento mostra la mappa del posto.
  Non serve nessuna chiave né registrazione: la ricerca passa dal server
  (che si identifica verso Nominatim, come la sua usage policy chiede) e
  le mattonelle della mappa arrivano da openstreetmap.org, quindi serve
  una connessione verso l'esterno. Un indirizzo che la ricerca non trova
  si scrive a mano: resta sull'evento come riga di testo, senza mappa.
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
  sistema genera un link di invito che chi lo riceve apre per scegliere la
  propria password, che chi lo ha invitato non conosce. Senza un server
  SMTP configurato il link va copiato e recapitato a mano; con SMTP
  configurato parte anche una mail con il link pronto. Il link non scade
  e resta valido finché non viene usato o l'invito non viene eliminato.
  Impostazioni per la lingua di default, l'indirizzo pubblico dell'app,
  il token BoardGameGeek e, opzionalmente, un server SMTP per l'invio
  automatico delle email.

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
- **Provider AI**: traduce automaticamente le descrizioni di BGG (arrivano
  solo in inglese) nella lingua della scheda, sia importando un gioco sia
  aggiungendo una lingua a uno esistente. Se assente, l'app funziona come
  prima: descrizioni in inglese, e nella scheda del gioco non compare
  nessun comando di traduzione. Dettagli sotto.
- **Email (SMTP)**: se configurato, l'app manda da sé l'invito di un
  amministratore, la conferma di una prenotazione e l'avviso di
  annullamento. Se assente, l'app funziona esattamente come prima: il
  codice di prenotazione resta a schermo e il link d'invito si copia a
  mano. Dettagli sotto.

### Traduzione automatica delle descrizioni (opzionale)

Va bene qualunque servizio compatibile con le API OpenAI. Servono tre
valori, nella sezione "Provider AI" della pagina Impostazioni:

| Campo | Esempio |
|---|---|
| Indirizzo del provider | `https://generativelanguage.googleapis.com/v1beta/openai` |
| Chiave API | la chiave del provider |
| Modello | `gemini-flash-lite-latest` |

L'indirizzo è quello base, senza `/chat/completions` in fondo: ci pensa
l'app ad aggiungerlo.

Per un'associazione la scelta più semplice è **Google Gemini via AI
Studio**: la chiave è gratuita, non serve una carta di credito, e il
piano gratuito basta ampiamente a tradurre un catalogo. In alternativa
funzionano OpenAI (`https://api.openai.com/v1`), OpenRouter, Groq, o un
Ollama sulla stessa macchina (`http://localhost:11434/v1`).

La descrizione tradotta resta modificabile a mano dalla scheda del gioco.
Il bottone *Traduci* ritraduce sempre dall'originale BoardGameGeek e
sostituisce il testo corrente.

Aggiungendo una lingua si sceglie da quale descrizione partire: l'originale
di BoardGameGeek, oppure una delle lingue già presenti. La seconda opzione
serve quando quella descrizione è stata corretta a mano, ed è quindi un
punto di partenza migliore dell'inglese grezzo. Senza provider configurato
la descrizione scelta viene copiata invariata, pronta da tradurre a mano.

Se il provider è configurato ma non risponde (chiave sbagliata, servizio
non raggiungibile, credito esaurito), l'import automatico fallisce in
silenzio: il gioco entra comunque in catalogo con la descrizione in
inglese, come se nessun provider fosse configurato. Il bottone *Traduci*
invece resta visibile — l'app sa solo che i tre campi sono compilati, non
che funzionano — e se premuto mostra l'errore restituito dal provider
senza toccare la descrizione esistente.

Il link di invito contiene un token che vale un accesso da amministratore:
mandalo su un canale privato, e cancella dalla lista un invito che non
serve più. Come ogni URL, finisce anche nei log del container. Da quando
esiste l'invio via email anche il `booking_code` viaggia in un URL (i link
per gestire la prenotazione o inserire il punteggio): può quindi comparire
nei log di un reverse proxy e nella cronologia del telefono di chi
prenota.

### Email (SMTP) (opzionale)

Senza un server SMTP configurato l'app funziona esattamente come prima:
il `booking_code` resta a schermo dopo la prenotazione e il link
d'invito di un amministratore si copia e recapita a mano. Configurando
un server nella sezione "Configurazione Email (SMTP)" della pagina
Impostazioni partono da sole tre email:

- **invito di un amministratore**, con il link per attivare l'accesso;
- **conferma di prenotazione**, col codice, il link per gestirla o
  disdirla e quello per inserire il punteggio a fine partita;
- **avviso di annullamento**, sia quando è il partecipante a disdire sia
  quando lo fa un organizzatore.

I valori (server, porta, sicurezza, utente, password, mittente) si
inseriscono nella pagina Impostazioni e restano nel database — **non**
ci sono variabili d'ambiente per la posta. I campi:

| Campo | Note |
|---|---|
| Server SMTP | es. `smtp.gmail.com` |
| Porta | default `587` |
| Sicurezza | STARTTLS (587), TLS implicito (465) o nessuna cifratura per un relay in rete locale — lasciato vuoto si comporta come STARTTLS |
| Nome utente / Password | dipendono dal provider, vedi sotto |
| Indirizzo mittente | l'indirizzo che compare come mittente |
| Nome mittente | il nome accanto all'indirizzo, es. "Serate Ludiche" |

Il bottone **"Invia email di prova"** manda un messaggio all'indirizzo
dell'admin con la sessione attiva, usando la configurazione **salvata**
(non quella ancora nel form): salva prima di provare. Se l'invio
fallisce mostra l'errore reale restituito dal server SMTP, utile per
distinguere una porta sbagliata da una password rifiutata.

Fuori da quel bottone, un invio che fallisce davvero (server irraggiungibile,
credenziali scadute dopo la prova) è silenzioso: nessun avviso in pagina, né
per l'admin né per il partecipante, perché l'esito della mail non deve mai
condizionare quello della prenotazione o dell'invito. L'unica traccia è una
riga nel log del container (`docker compose logs`).

I link dentro le email si costruiscono a partire dall'**Indirizzo
pubblico** delle impostazioni; se è vuoto si ripiega sull'host della
richiesta che ha generato la mail, che per un invio in background non è
detto coincida con quello giusto. Su un'installazione raggiungibile da
fuori (dietro un dominio o un reverse proxy) vale la pena impostarlo,
altrimenti i link nelle email possono puntare a un indirizzo che
l'associazione non usa.

**Note sui provider più comuni:**

- **Gmail** (`smtp.gmail.com`, porta 587, STARTTLS): nome utente
  l'indirizzo Gmail, password **non** quella dell'account ma una **app
  password** generata dalle impostazioni Google (richiede la verifica in
  due passaggi attiva). Il piano gratuito regge tranquillamente il
  volume di un'associazione (circa 500 destinatari al giorno).
- **Mailjet** (`in-v3.mailjet.com`, porta 587, STARTTLS): nome utente =
  API key, password = secret key.
- **Brevo** (`smtp-relay.brevo.com`, porta 587, STARTTLS): la password
  **non** è la chiave API `xkeysib-…` (con quella l'autenticazione
  fallisce) ma una **SMTP key** dedicata, nella scheda SMTP del pannello
  "SMTP & API". Su un account gratuito nuovo, spesso quella scheda resta
  bloccata finché Brevo non attiva l'invio transazionale sull'account:
  vale la pena saperlo prima, è un pomeriggio perso altrimenti.
- **Microsoft/Outlook**: da evitare qui — ha dismesso l'autenticazione
  SMTP con sola password a favore di OAuth2, che questa integrazione non
  supporta.

## Documentazione di progetto

- [`PRODUCT.md`](PRODUCT.md) — utenti, scopo, contesto d'uso.
- [`DESIGN.md`](DESIGN.md) — sistema visivo (colori, tipografia,
  componenti).
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — spec di design
  per fase di sviluppo.

## Licenza

[MIT](LICENSE)
