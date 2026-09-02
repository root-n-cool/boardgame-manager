# BoardGames Manager — Design & PRD

Data: 2026-08-31

## 1. Panoramica e obiettivi

Applicazione selfhost, leggera, per la gestione degli eventi di
un'associazione di giochi da tavolo. Un amministratore mantiene un
catalogo di giochi (arricchito con dati da BoardGameGeek) e organizza
eventi; le persone che partecipano si prenotano un gioco per evento
senza bisogno di un account, e a fine partita registrano il punteggio
finale, alimentando una classifica storica per ciascun gioco.

Obiettivi chiave:
- Zero dipendenze pesanti: un solo binario/container, SQLite come
  unico storage, nessun servizio esterno obbligatorio.
- Amministrazione semplice: aggiungere giochi deve richiedere il
  minimo sforzo possibile grazie all'integrazione con BGG e alla
  ricerca automatica (opzionale) di manuali e tutorial.
- Partecipazione agli eventi senza attrito: chi vuole prenotare un
  gioco non deve creare un account.

## 2. Utenti e ruoli

Un solo ruolo applicativo: **admin/organizzatore**, con accesso
completo (gestione utenti, catalogo, eventi). Non esiste un ruolo
"utente normale loggato" — chi partecipa agli eventi non ha mai un
account, si identifica solo tramite email + numero di telefono al
momento della prenotazione, e successivamente tramite email +
`booking_code` per gestire la propria prenotazione.

Il primo accesso all'applicazione (nessun utente nel database) forza
la registrazione del primo admin, come avviene in n8n. Da quel punto
in poi, ogni nuovo admin viene aggiunto da un admin già autenticato
tramite la pagina di gestione utenti.

## 3. Architettura

- **Backend**: Go, monolite, router `chi` (o stdlib `net/http` con i
  pattern di routing di Go 1.22+), driver SQLite cgo-free
  (`modernc.org/sqlite`).
- **Migrazioni**: runner custom invece di una dipendenza esterna come
  `goose` o `golang-migrate`. I file `.sql` numerati
  (`0001_init.sql`, ...) vivono in `backend/internal/db/migrations/`,
  vengono embeddati nel binario con `//go:embed` e applicati in ordine
  alfabetico all'avvio, ognuno dentro una transazione; una tabella
  `schema_migrations` traccia le versioni già applicate e rende
  l'operazione idempotente. Solo migrazioni forward, nessun rollback.
  Il costo è ~70 righe in `backend/internal/db/migrate.go` e in cambio
  il deploy resta un singolo binario senza CLI o step aggiuntivi.
- **Frontend**: Vue 3 + Vite. Il build statico viene embeddato nel
  binario Go tramite `embed.FS`, quindi un solo processo serve sia le
  API sia l'interfaccia.
- **Autenticazione admin**: sessioni via cookie httpOnly (no JWT),
  password hashate con bcrypt/argon2, tabella `sessions` in SQLite
  per poter invalidare lato server.
- **Storage**: file SQLite + directory upload (manuali PDF, copertine
  se non recuperate via URL) su un volume `/data` montato dal
  container.
- **Deployment**: Docker/docker-compose, un solo servizio definito
  nel compose file, immagine finale costruita in multi-stage build
  (build Vue → build Go con embed → immagine minimale).

## 4. Modello dati

```
User (admin/organizzatore)
  - id, email, password_hash, created_at

Session
  - id, user_id, token_hash, expires_at, created_at

Game (catalogo)
  - id, bgg_id (nullable), name, year, cover_image_url_or_path,
    description, min_players, max_players, playtime_minutes,
    owner (testo libero), created_at

GameLanguage
  - id, game_id, language_code, is_base_language (bool)

GameMedia
  - id, game_language_id, type (file | link | youtube),
    url_or_path, title, created_at

Event
  - id, title, description, event_date, start_time, created_at

EventGame (giochi disponibili per un evento, con quantità copie)
  - id, event_id, game_id, quantity

Booking (prenotazione di UNA copia di un gioco per un evento)
  - id, event_game_id, participant_name, participant_email,
    participant_phone, booking_code, status (active | cancelled),
    created_at
  - vincolo: un solo booking con status=active per
    (event_id, participant_phone)

MatchResult (inserito a fine partita tramite booking_code)
  - id, booking_id, submitted_at

MatchPlayerScore
  - id, match_result_id, player_name (testo libero), score (numero)

AppSettings (singola riga / key-value)
  - default_language, youtube_api_key, search_api_key,
    search_api_provider (google | bing)
```

Note:
- La disponibilità residua di un `EventGame` è
  `quantity - COUNT(booking attivi collegati)`.
- La classifica per gioco aggrega tutte le `MatchPlayerScore` di tutte
  le `MatchResult` collegate (tramite `Booking` → `EventGame`) allo
  stesso `game_id`, raggruppando per `player_name` normalizzato
  (lowercase + trim). Nessun registro giocatori separato: possibili
  duplicati per refusi/varianti di nome sono un compromesso accettato.
- Il vincolo "un booking attivo per telefono per evento" si implementa
  con un unique index parziale su `(event_id, participant_phone)
  WHERE status = 'active'` (supportato da SQLite).

## 5. Flussi funzionali

### 5.1 Bootstrap primo utente
Al primo avvio, se non esistono utenti, ogni richiesta viene
reindirizzata a una pagina di setup che chiede email+password per
creare il primo admin. Da quel momento l'app richiede login normale.

### 5.2 Gestione utenti admin
Un admin autenticato può aggiungere altri admin (email+password) e
rimuoverli dalla pagina di gestione utenti. Nessuna distinzione di
permessi tra admin: chiunque abbia un account può fare tutto.

### 5.3 Catalogo giochi e integrazione BGG
1. L'admin cerca un gioco per nome → chiamata a
   `GET /xmlapi2/search?query=...` di BGG, mostra risultati con
   anno/miniatura.
2. Selezionato un risultato → chiamata a `GET /xmlapi2/thing?id=...`
   per i dati completi (nome, descrizione, min/max giocatori, durata,
   copertina) → precompila il form. L'immagine di copertina viene
   scaricata e salvata localmente (stesso approccio dei manuali),
   così l'app non dipende dalla disponibilità del CDN di BGG a
   runtime.
3. L'admin sceglie la lingua base e inserisce l'owner (testo libero),
   conferma → viene creato `Game` + prima `GameLanguage`.
4. Se sono configurate le chiavi API (in `AppSettings`), parte in
   background una ricerca: manuale PDF (search API, query localizzata
   tipo `"<nome gioco>" rulebook filetype:pdf`) e tutorial (YouTube
   Data API, query localizzata). I risultati proposti appaiono in una
   sezione "da confermare" nella scheda del gioco; l'admin per
   ciascuno può confermarlo, sostituirlo con un link diverso, o
   caricare un file al suo posto. Se le chiavi non sono configurate,
   si salta la ricerca e l'admin inserisce/carica tutto a mano.
5. L'admin può ripetere i punti 3-4 per aggiungere altre
   `GameLanguage` allo stesso gioco.

**Vincolo noto**: BGG non offre un'API con autenticazione per
scaricare file dalla sezione "Files" del sito (richiede login utente
normale, non è previsto nella XML API2) — non va tentato uno scraping
di quella sezione. Il manuale va quindi recuperato tramite ricerca web
generica (spesso il PDF è ospitato dall'editore) o inserito/caricato a
mano dall'admin.

### 5.4 Creazione eventi
L'admin crea un evento (titolo, descrizione, data, ora) e seleziona i
giochi disponibili dal catalogo, specificando per ciascuno la
quantità di copie disponibili per quell'evento (`EventGame`).

### 5.5 Prenotazione pubblica
1. La pagina pubblica dell'evento (nessun login) mostra i giochi
   disponibili con copertina, descrizione e media (manuale/tutorial)
   nella/e lingua/e disponibili.
2. Il visitatore clicca "prenota" su un gioco con disponibilità
   residua, inserisce nome, email, telefono.
3. Se esiste già un booking attivo con lo stesso telefono per quello
   stesso evento, la prenotazione viene rifiutata con messaggio
   esplicito (un solo gioco per persona per evento).
4. Alla conferma viene generato un `booking_code` (random,
   crittograficamente sicuro, es. 8 caratteri alfanumerici) e
   mostrato a schermo — nessuna email viene inviata (niente SMTP in
   v1).

### 5.6 Cancellazione prenotazione
Pagina pubblica "gestisci prenotazione": inserendo il `booking_code`
corrispondente a un booking attivo, l'utente può annullarlo,
liberando la disponibilità per l'evento.

### 5.7 Inserimento punteggio
Stesso meccanismo del `booking_code`: se il booking è valido,
form per inserire N giocatori (nome libero) e il punteggio numerico di
ciascuno → crea `MatchResult` + `MatchPlayerScore`. Il vincitore è
implicito (punteggio più alto vince).

### 5.8 Classifica per gioco
Pagina pubblica per gioco che mostra tutte le `MatchResult` storiche
(data evento, giocatori, punteggi) e un aggregato per giocatore
(partite giocate, vittorie, punteggio medio/totale).

## 6. Requisiti non funzionali

**Sicurezza:**
- Password admin hashate con bcrypt/argon2.
- Sessioni via cookie httpOnly + secure (se HTTPS) + SameSite=Lax,
  invalidabili lato server.
- `booking_code` generato con RNG crittografico; rate-limit sui
  tentativi di accesso via booking_code per evitare brute force.
- Chiavi API esterne salvate in DB, mai esposte al frontend pubblico.
- Upload manuali: validazione tipo file (solo PDF) e dimensione
  massima, serviti tramite endpoint controllato (non path statico
  diretto).

**Configurazione esterna (impostazioni admin):**
Pagina "Impostazioni" con: lingua di default del sistema, YouTube
Data API key, Search API key (Google Custom Search o Bing) — tutte
opzionali; se assenti, le funzioni di ricerca automatica sono
disabilitate con messaggio esplicativo, senza bloccare il resto
dell'app.

**Internazionalizzazione:**
- Interfaccia (menu, bottoni) solo in italiano per la v1.
- I contenuti multi-lingua per gioco sono coperti dal modello
  `GameLanguage`/`GameMedia`.

**Testing:**
- Backend Go: unit test su logica di dominio (calcolo disponibilità,
  vincoli booking, normalizzazione nomi per classifica, generazione
  codici) + test di integrazione sugli handler HTTP con SQLite
  in-memory.
- Frontend: verifica manuale mirata sui flussi critici (prenotazione,
  cancellazione, inserimento punteggio) in browser per ogni fase
  completata.

**Deployment:**
Dockerfile multi-stage (build Vue → build Go con embed → immagine
finale minimale), `docker-compose.yml` con volume `./data:/data` per
SQLite + upload, variabili d'ambiente per porta e path dati.

## 7. Fuori scope per la v1

- Invio email (conferma prenotazione, promemoria) — richiederebbe
  configurazione SMTP, deliberatamente rimandato.
- Registro giocatori dedicato per la classifica — si usa match sul
  nome normalizzato, accettando il rischio di duplicati per refusi.
- Internazionalizzazione dell'interfaccia (solo italiano in v1).
- Ruoli/permessi differenziati tra admin (tutti gli admin hanno pieno
  accesso).
- Limite configurabile di prenotazioni per persona diverso da "una
  per evento" (il vincolo è fisso: un telefono, un booking per
  evento).
- Integrazione con tracker esterni (Linear, ecc.): piani e stato
  restano file locali nel repo.

## 8. Roadmap a fasi

Ogni fase è pensata per essere autonoma e dimostrabile, e riceverà il
proprio ciclo spec→piano→implementazione quando verrà affrontata:

1. **Scaffolding** — modulo Go, progetto Vue+Vite, `docker-compose.yml`
   minimale, connessione SQLite + migrazioni, health-check.
2. **Autenticazione e amministrazione** — bootstrap primo utente,
   login/logout, sessioni, CRUD utenti admin, pagina impostazioni
   (lingua default, chiavi API salvate ma non ancora usate).
3. **Catalogo giochi + integrazione BGG** — modelli
   Game/GameLanguage/GameMedia, ricerca/import da BGG, CRUD gioco con
   owner e lingua base, upload manuale/copertina, ricerca automatica
   con conferma admin.
4. **Eventi e prenotazioni** — CRUD evento, associazione giochi con
   quantità, pagina pubblica evento, prenotazione con generazione
   `booking_code`, cancellazione.
5. **Punteggi e classifiche** — inserimento punteggio a fine partita,
   pagina classifica per gioco.
