# Traduzione AI delle descrizioni BGG — design

Data: 2026-09-03

## Problema

BoardGameGeek serve tutto in inglese. Un gioco importato entra in
catalogo con la sua descrizione originale, e l'interfaccia
dell'associazione è in italiano: la scheda gioco è l'unico punto
dell'app dove il partecipante legge un muro di testo in una lingua che
non ha scelto.

Oggi l'unico rimedio è che l'admin traduca a mano, incollando avanti e
indietro. Su un catalogo di qualche decina di giochi è lavoro che non
si fa, e la descrizione resta in inglese.

## Obiettivo

- Chi crea un gioco da BGG scegliendo l'italiano come lingua base si
  ritrova la descrizione **già in italiano**, senza fare nulla.
- Chi aggiunge una lingua a un gioco esistente ottiene la descrizione
  tradotta in quella lingua.
- L'admin può ritradurre a richiesta: sui giochi già in catalogo, dopo
  aver cambiato modello, o quando la traduzione non gli piace.
- Il testo tradotto resta **editabile a mano** come oggi: l'AI riempie
  un campo, non lo blocca.
- Niente di tutto questo è obbligatorio: senza provider configurato
  l'app si comporta esattamente come prima.

## Decisioni prese in brainstorming

- **Provider AI configurabile, non un servizio di traduzione.** Un
  motore dedicato (DeepL, LibreTranslate) risolve solo la traduzione.
  Un provider generico apre la porta a riassunti, tag normalizzati,
  testi degli eventi — sviluppi che il progetto vuole poter fare senza
  ricablare le impostazioni. Il costo reale è irrilevante: una
  descrizione BGG è 2-4k caratteri, un catalogo intero costa
  centesimi.
- **Formato OpenAI-compatible come astrazione.** Con `base_url`,
  `api_key` e `model` in impostazioni si gira su Google Gemini,
  OpenAI, OpenRouter, Groq o Ollama in locale senza toccare il codice.
  Nel README si consiglia **Gemini via AI Studio**: chiave gratuita
  senza carta e free tier che un'associazione non esaurisce.
- **Client Go, non Vercel AI SDK.** L'SDK è TypeScript e il backend è
  Go. Metterlo nel frontend esporrebbe la chiave API nel browser,
  affiancarlo come processo Node romperebbe il "un solo
  binario/container". Streaming e tool-calling, il valore aggiunto
  dell'SDK, qui non servono: una chiamata non-streaming che torna
  testo. Restano ~120 righe di `net/http` e zero dipendenze nuove.
- **`games.bgg_description` come sorgente unica.** Senza, tradurre in
  italiano al momento della creazione cancellerebbe l'originale
  inglese, e aggiungere l'inglese dopo produrrebbe una ritraduzione
  della traduzione. Conservando il grezzo BGG ogni lingua traduce
  sempre dalla stessa fonte: nessun degrado a catena, e si può
  rigenerare tutto dopo un cambio di modello.
- **Traduzione dentro `POST /games`, server-side.** Il form di
  creazione non mostra la descrizione: manda `bggId` e i dati li
  recupera il backend. Non esiste quindi oggi un punto in cui l'admin
  conferma il testo BGG prima del salvataggio — la revisione avviene
  già nella scheda di dettaglio, dove il testo è editabile. Innestare
  la traduzione lì aderisce al flusso esistente invece di piegarlo;
  l'alternativa (anteprima nel form) snaturerebbe un form volutamente
  minimale e scaricherebbe i dati BGG due volte.
- **La traduzione non può far fallire una scrittura.** Provider giù,
  chiave scaduta, credito finito: il gioco si salva con la descrizione
  originale e l'admin ritraduce col bottone.
- **Il nome del gioco non si traduce.** Un LLM produrrebbe "Apertura
  alare" per Wingspan, che in Italia si chiama Wingspan.
  `game_languages.name` resta il nome BGG, editabile a mano come oggi.
- **Nessun flag "abilitato".** L'AI è attiva quando `base_url`,
  `api_key` e `model` sono tutti valorizzati, lo stesso criterio già
  in vigore per le chiavi opzionali.
- **Solo la traduzione, per ora.** Immagini generate, riassunti e tag
  restano fuori: le copertine vere arrivano da BGG e una copertina
  generata sarebbe semplicemente sbagliata.

## Modello dati

### `app_settings`

```sql
ALTER TABLE app_settings ADD COLUMN ai_base_url TEXT;
ALTER TABLE app_settings ADD COLUMN ai_api_key TEXT;
ALTER TABLE app_settings ADD COLUMN ai_model TEXT;
```

`ai_base_url` è la radice OpenAI-compatible, senza `/chat/completions`
finale (es. `https://generativelanguage.googleapis.com/v1beta/openai`).
Normalizzata come `public_base_url`: URL http(s) assoluto o vuoto, mai
con slash finale.

### `games`

```sql
ALTER TABLE games ADD COLUMN bgg_description TEXT;
```

La descrizione BGG grezza, in inglese, salvata all'import. Non viene
mai mostrata in UI: è la sorgente da cui traducono tutte le lingue.
Resta `NULL` per i giochi inseriti a mano e per quelli già in catalogo
prima di questa migrazione — per quelli non c'è nulla da tradurre.

Migrazione unica: `0011_ai_provider.sql`, con tutti e quattro gli
`ALTER`.

## Il package `ai`

Nuovo `backend/internal/ai`, sul calco di `internal/geocode`:
interfaccia più implementazione HTTP, così `httpapi` inietta un fake
nei test.

```go
type Translator interface {
    Translate(ctx context.Context, text, targetLang string) (string, error)
}

type HTTPClient struct {
    BaseURL, APIKey, Model string
    HTTPClient *http.Client
}
```

`Translate` fa un `POST {BaseURL}/chat/completions` con
`Authorization: Bearer {APIKey}`, `temperature: 0`, due messaggi: un
system prompt che impone *solo il testo tradotto, formattazione e
a capo preservati, nessun commento né preambolo*, e la descrizione come
messaggio utente. Legge `choices[0].message.content`.

Timeout 60s: le descrizioni BGG sono lunghe e i modelli economici non
sono veloci. Nessun retry — l'admin ha il bottone per riprovare.

Errori distinti, perché la UI ci reagisce in modo diverso:

- `ErrNotConfigured` — manca uno dei tre valori. Non è un guasto: è
  l'app senza AI, e chi la usa così non deve vedere errori.
- errore di rete, HTTP non-2xx, risposta senza `choices` — guasto vero,
  loggato, che lascia la descrizione originale al suo posto.

Il client si costruisce **per richiesta** a partire dalle impostazioni
lette dal DB, non una volta all'avvio: l'admin può cambiare provider
senza riavviare il container.

`targetLang` arriva come codice ISO (`it`, `en`) e viene reso in nome
esteso nel prompt: "traduci in italiano" funziona meglio di "traduci in
it" su qualunque modello.

## Regole di dominio

**Traduzione di una lingua.** Un solo punto di verità, in `httpapi`:
data una descrizione sorgente e una lingua di destinazione, se l'AI non
è configurata o la sorgente è vuota restituisce la sorgente
invariata; se la lingua di destinazione è l'inglese restituisce la
sorgente senza spendere una chiamata (l'originale BGG è già inglese);
altrimenti traduce, e in caso di guasto restituisce ancora la sorgente.
Non risale mai al chiamante come errore.

**Punti di innesto.**

| Dove | Comportamento |
|---|---|
| `POST /games` da BGG | salva `bgg_description`, traduce nella lingua base e la scrive in `game_languages.description` |
| `POST /games/{id}/languages` | traduce `games.bgg_description` nella nuova lingua |
| `POST /games/{id}/languages/{lang}/translate` | ritraduce e sovrascrive, a richiesta |

`POST /games` in inserimento manuale non è toccato: non c'è nessun
originale BGG da tradurre, la descrizione la scrive l'admin.

**Ripiego sulla lingua base.** `createLanguageHandler` oggi precompila
la nuova lingua col testo della lingua base, come punto di partenza per
una traduzione a mano. Quel comportamento resta il ripiego per quando
`bgg_description` è `NULL` — giochi manuali e giochi entrati in
catalogo prima della `0011`. Con l'originale BGG disponibile e l'AI
configurata si traduce da lì; altrimenti si copia la base, come oggi.

**Store.** `games.Game` guadagna il campo `BGGDescription`, scritto da
`CreateGame` e letto da `GetGame`/`ListGames` come gli altri campi
opzionali. Non viaggia in nessuna risposta API: fuori esce
solo il booleano `canTranslate`.

**Nessuno stato "tradotto".** Non esiste colonna né flag. Il bottone di
ritraduzione compare quando l'AI è configurata e `bgg_description` non
è vuota, punto. Una colonna di stato andrebbe tenuta sincronizzata con
le modifiche a mano dell'admin, e non servirebbe a nessuna decisione
che l'app deve prendere.

**Ritraduzione distruttiva.** L'endpoint di traduzione sovrascrive la
descrizione della lingua, comprese le correzioni a mano. È il senso del
bottone; la UI lo dice prima di eseguirlo.

## API

- `GET /api/settings` — nuovi campi `aiBaseUrl`, `aiModel` (in chiaro,
  come `publicBaseUrl`: l'admin deve poterli rileggere e verificare),
  `aiApiKeySet` e `aiApiKeyMasked` (la chiave non esce mai in chiaro,
  come `bggApiToken`), e `aiConfigured`, il booleano su cui la UI
  decide se mostrare i comandi di traduzione.
- `PUT /api/settings` — accetta `aiBaseUrl`, `aiApiKey`, `aiModel`.
  `aiBaseUrl` validato come URL http(s) assoluto o vuoto; risposta 400
  altrimenti, con messaggio in italiano. `aiApiKey` segue la stessa
  semantica di `bggApiToken`.
- `POST /api/games/{id}/languages/{lang}/translate` — rotta nuova, solo
  admin. Ritraduce la descrizione di quella lingua da
  `games.bgg_description` e restituisce la `GameLanguage` aggiornata,
  nella stessa forma degli altri handler delle lingue. 404 se il gioco
  o la lingua non esistono; 409 con messaggio italiano se l'AI non è
  configurata o se `bgg_description` è vuota — qui, a differenza degli
  innesti automatici, il guasto **si mostra**: l'admin ha premuto un
  bottone e ha diritto di sapere che non ha funzionato.
- `GET /api/games/{id}` — nuovo campo `canTranslate`, vero quando
  `bgg_description` non è vuota. Un booleano, non il testo: la scheda
  di modifica deve sapere se il bottone ha una sorgente da cui
  tradurre, e la descrizione grezza non ha motivo di viaggiare.
- `POST /api/games` e `POST /api/games/{id}/languages` non cambiano
  forma: la traduzione avviene dentro, invisibile al chiamante.

## UI

**Impostazioni.** Nuovo blocco "Provider AI" con tre campi — URL base,
chiave API (mascherata come quella BGG), modello. Un testo di aiuto
dice a cosa serve (tradurre in automatico le descrizioni BGG) e porta
gli esempi di configurazione per Gemini, OpenAI e Ollama, perché senza
un esempio l'URL base è un campo indovinello. Il blocco chiarisce che
lasciandolo vuoto l'app funziona come prima.

**Nuovo gioco.** Quando l'AI è configurata, l'etichetta del pulsante di
salvataggio in corso diventa "Traduzione in corso…". La POST può durare
cinque o dieci secondi e va detto perché sta succedendo: uno spinner
muto su un salvataggio che di solito è istantaneo si legge come un
blocco.

**Scheda gioco (admin).** Accanto alla descrizione di ogni lingua, un
pulsante *Traduci in italiano* (col nome esteso della lingua di quella
scheda), visibile solo se `aiConfigured` e `canTranslate`.
Avverte che sovrascrive il testo attuale, mostra lo stato di
avanzamento e in caso di errore riporta il messaggio del backend senza
toccare il testo esistente.

Le tre superfici sono pagine admin, usate da desktop; il pass
`/impeccable` su di esse chiude il lavoro, come prescritto da
CLAUDE.md.

## Test

- **`ai`**: client contro `httptest.Server` — richiesta ben formata
  (URL, header di autorizzazione, corpo con modello e messaggi),
  risposta letta correttamente, HTTP non-2xx come errore, corpo senza
  `choices` come errore, timeout onorato, `ErrNotConfigured` con i
  campi vuoti.
- **`settings`**: i tre campi nuovi sopravvivono a un giro
  `Update`/`Get` e sono vuoti dopo la migrazione.
- **`httpapi`**, con un `Translator` fake sul modello di
  `bgg_fake_test.go`: creazione da BGG con AI attiva (descrizione
  tradotta, `bgg_description` col grezzo); con AI non configurata
  (descrizione originale, nessuna chiamata); con traduttore che
  fallisce (gioco creato lo stesso, descrizione originale); lingua
  inglese che non chiama il traduttore; aggiunta di una lingua che
  traduce dal grezzo; endpoint di ritraduzione nei casi 200, 404, 409
  senza AI e 409 senza `bgg_description`; mascheramento della chiave
  in `GET /settings` e validazione dell'URL base in `PUT`.
- Suite backend in Docker e `npm run build` prima di dichiarare
  concluso, come da CLAUDE.md.

## Fuori scopo

- Immagini generate: le copertine arrivano da BGG e una generata
  sarebbe sbagliata. La cover dell'evento è l'unico caso sensato, ed è
  un lavoro a sé.
- Riassunti brevi per le card, normalizzazione di categorie e tag,
  testi degli eventi: il provider una volta configurato li abilita, ma
  ognuno è una feature con la sua UI.
- Traduzione dei media e dei loro titoli.
- Traduzione in blocco di tutto il catalogo con un comando solo.
- Cache o deduplica delle traduzioni: a questi volumi non serve.
