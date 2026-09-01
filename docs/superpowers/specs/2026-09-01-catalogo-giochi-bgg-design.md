# Fase 2 — Catalogo Giochi + Integrazione BoardGameGeek — Design

Data: 2026-09-01

## 1. Panoramica e obiettivi

Questa fase aggiunge al BoardGames Manager (Fase 1 completa: auth/admin) la
gestione del catalogo giochi. Un admin autenticato può cercare un gioco su
BoardGameGeek (BGG) e importarne i dati base, oppure inserirlo a mano; per
ogni gioco può gestire una o più lingue, ciascuna con un nome/descrizione
tradotti e una lista di media (manuale PDF, link, video YouTube). La lettura
del catalogo (elenco, dettaglio, file) è pubblica, senza login — sarà usata
dalle pagine evento pubbliche nelle fasi successive.

Rispetto alla visione originale della spec di Fase 1 (sezione 5.3), questa
fase **semplifica lo scope**:
- Nessuna ricerca automatica di manuali/tutorial via API esterne (YouTube
  Data API, Google/Bing Search) — l'admin inserisce manuale e tutorial a
  mano (upload file o link). Le chiavi API salvate in Fase 1 restano
  inutilizzate per ora; la ricerca automatica è rimandata a una fase futura.
- L'integrazione BGG richiede un token di autorizzazione (vedi sezione 7 —
  BGG ha introdotto questo requisito dopo la stesura della spec di Fase 1),
  gestito con lo stesso pattern delle altre chiavi API esterne.

## 2. Architettura e pacchetti Go

- **`backend/internal/bgg`**: client per la XML API2 di BGG.
  - `Search(ctx context.Context, token, query string) ([]SearchResult, error)`
  - `GetThing(ctx context.Context, token, id string) (ThingDetail, error)`
  - Parsing con `encoding/xml`. Ogni richiesta include un header
    `Authorization` con il token configurato dall'admin.
  - Il client è definito come interfaccia (`bgg.Client`) per permettere
    l'iniezione di un fake nei test HTTP, evitando dipendenze di rete nei
    test automatici.
- **`backend/internal/games`**: repository per `Game`, `GameLanguage`,
  `GameMedia` — stesso pattern di `internal/users` (store con metodi CRUD,
  errori sentinella).
- **`backend/internal/storage`**: helper per salvare file su disco in
  `/data/uploads/<sha256>.<ext>`, con validazione tipo/dimensione per
  categoria (manuale, copertina).
- **`internal/settings`**: esteso con `BGGAPIToken` (stesso trattamento
  mascherato delle chiavi YouTube/Search già presenti).
- **`internal/httpapi`**: nuovi handler per giochi/lingue/media, montati in
  parte su route pubbliche (sola lettura) e in parte protette (scrittura).

## 3. Modello dati

Nuova migrazione (`0002_games.sql`):

```sql
Game
  - id, bgg_id (nullable), name (nome ufficiale/originale — da BGG o
    inserito a mano, funge da riferimento stabile), year (nullable),
    min_players (nullable), max_players (nullable),
    playtime_minutes (nullable), owner (testo libero, nullable),
    cover_path (nullable, riferimento a un file in /data/uploads),
    created_at

GameLanguage
  - id, game_id, language_code, is_base_language (bool)
  - name: nome tradotto per quella lingua (precompilato dal nome
    BGG/ufficiale alla creazione, modificabile dall'admin)
  - description: descrizione tradotta per quella lingua (precompilata
    dalla descrizione BGG, modificabile dall'admin; vuota se creazione
    manuale senza BGG)
  - vincolo: un solo is_base_language=true per game_id

GameMedia
  - id, game_language_id, type (file | link | youtube), url_or_path,
    title (nullable), created_at

AppSettings (estensione, non nuova tabella)
  - + bgg_api_token (nullable, mascherato in lettura come le altre chiavi)
```

Note:
- Nessuna cache delle risposte BGG: ogni ricerca/import chiama BGG in tempo
  reale (operazione rara, un gioco alla volta, coerente con l'approccio
  sincrono di Fase 2).
- I file sono content-addressed (hash SHA-256 del contenuto): lo stesso file
  caricato due volte riusa il file su disco. Eliminare un gioco/media rimuove
  solo le righe DB, **non** il file fisico — evita bug di reference-counting,
  costo trascurabile di spazio disco per un uso selfhost di piccole
  dimensioni.
- La UI impedisce di eliminare l'unica `GameLanguage` di un gioco (deve
  sempre restarne almeno una); per cambiare la lingua base l'admin ne
  aggiunge prima una nuova.

## 4. API

**Pubbliche (nessun login):**
- `GET /api/games` — elenco giochi (dati base + lingue disponibili,
  sommario)
- `GET /api/games/{id}` — dettaglio gioco con lingue e media
- `GET /api/uploads/{hash}` — serve un file (copertina/manuale) dal content
  store, con il content-type corretto

**Protette (sessione admin):**
- `GET /api/games/search?q=...` — proxy alla ricerca BGG; 409 con messaggio
  esplicito se `bgg_api_token` non è configurato
- `POST /api/games` — crea un gioco:
  - da BGG: `{bgg_id, language_code, owner}` → chiama `GetThing`, scarica
    la copertina, crea `Game` + prima `GameLanguage` precompilata con
    nome/descrizione BGG
  - manuale: `{name, year, min_players, max_players, playtime_minutes,
    owner, language_code, name_translated, description_translated}` senza
    `bgg_id`
- `POST /api/games/{id}/cover` — upload copertina (multipart, immagine),
  usato per creazione manuale o per sostituire quella importata da BGG
- `PATCH /api/games/{id}` — modifica owner e altri campi base
- `DELETE /api/games/{id}` — elimina gioco (cascade su lingue/media)
- `POST /api/games/{id}/languages` — aggiungi lingua `{language_code}`. Se
  il gioco ha un `bgg_id`, precompila nome/descrizione con lo stesso testo
  BGG già salvato sulla lingua base (BGG non fornisce contenuti localizzati
  per lingua nella `thing` API — è solo un punto di partenza da tradurre a
  mano); altrimenti nome/descrizione partono vuoti
- `PATCH /api/games/{id}/languages/{lang}` — modifica nome/descrizione
  tradotti
- `POST /api/games/{id}/languages/{lang}/media` — aggiungi media: file
  (multipart, solo PDF, max 20MB), link, o youtube (URL)
- `DELETE /api/games/{id}/languages/{lang}/media/{media_id}` — rimuovi
  media (solo riga DB, il file su disco resta)

## 5. Frontend

- **`GamesView.vue`** (route `/games`): elenco giochi con copertina, nome,
  owner; pulsante "Aggiungi gioco".
- **Ricerca/creazione gioco**: campo di ricerca che chiama
  `GET /api/games/search`, mostra risultati BGG (copertina piccola, nome,
  anno) selezionabili; link "crea manualmente" per il percorso senza BGG.
- **`GameDetailView.vue`** (route `/games/{id}`): dati base, elenco lingue,
  per ciascuna lingua nome/descrizione modificabili e lista media con
  aggiunta (upload file / link / youtube embeddato) e rimozione; pulsante
  elimina gioco.

## 6. Requisiti non funzionali

**Validazioni:**
- Manuale: solo `application/pdf`, max 20MB.
- Copertina: tipi immagine comuni (jpg/png/webp), max 5MB.
- Media `youtube`: l'URL deve iniziare con `http(s)://` e contenere
  `youtube.com` o `youtu.be`; nessuna verifica oEmbed.

**Gestione errori BGG:**
- Token non configurato: 409 con messaggio esplicito.
- Errore/timeout BGG: 502 con messaggio generico, nessun retry automatico.

**Testing:**
- Backend: unit test su `internal/games` (SQLite in-memory) e
  `internal/storage` (content-addressing); test HTTP con un fake
  `bgg.Client` iniettato (nessuna dipendenza di rete nei test automatici).
- Verifica con BGG reale: la XML API richiede ora un token che l'admin deve
  ottenere manualmente registrandosi sul sito BGG (procedura non
  completamente documentata da BGG stessa, verificata solo parzialmente in
  fase di brainstorming). La verifica end-to-end del client contro l'API
  vera richiede quindi un token reale fornito dall'utente al momento
  opportuno nel piano di implementazione.
- Frontend: build + verifica manuale in browser, come in Fase 1.

## 7. Vincolo noto: autenticazione BGG

A differenza di quanto documentato nella spec di Fase 1 (che indicava la
XML API2 di BGG come pubblica, senza autenticazione), una verifica diretta
in data 2026-09-01 ha mostrato che BGG ora risponde "Unauthorized" alle
chiamate non autenticate su `/xmlapi2/search` e, presumibilmente, sugli
altri endpoint XML API2. Da fonti pubbliche (forum BGG, issue GitHub di
progetti terzi) risulta che BGG ha introdotto un requisito di registrazione
applicazione + token, con l'header `Authorization` da includere nelle
richieste — ma la documentazione ufficiale del formato esatto non è
risultata accessibile in fase di ricerca (pagina bloccata da protezione
anti-bot). Questa fase adotta quindi lo stesso pattern delle altre chiavi
API esterne (token salvato in `AppSettings`, opzionale, funzione disabilitata
se assente) e isola il formato dell'header in un unico punto del client
BGG, da verificare/correggere empiricamente con un token reale ottenuto
dall'utente durante l'implementazione.

## 8. Fuori scope per questa fase

- Ricerca automatica di manuali/tutorial (YouTube Data API, Google/Bing
  Search) — rimandata a una fase futura.
- Cache delle risposte BGG.
- Registro/deduplicazione dei giocatori (non pertinente a questa fase).
- Eventi, prenotazioni, punteggi — fasi successive.
