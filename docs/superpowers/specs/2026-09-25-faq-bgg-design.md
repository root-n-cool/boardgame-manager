# FAQ dal forum BGG — Design

Data: 2026-09-25

Riprende la spec `2026-09-09-fonti-multiple-design.md`, che aveva lasciato le
FAQ di BoardGameGeek come «fase a sé». Il loop dell'agente, la chat pubblica,
`cerca_nelle_fonti` e l'indicizzazione dei documenti restano come sono.

## 1. Obiettivo

Quando il manuale non basta o è ambiguo, l'agente cerca la risposta nel forum
**Rules** del gioco su BoardGameGeek e la cita con il link al commento.

Vincoli emersi in brainstorming:

- **Niente nel DB.** Le FAQ si cercano e si leggono al volo, a ogni domanda.
  Nessuna tabella, nessuna cache, nessuna scelta dell'admin sui thread.
- **La chat resta legata al manuale indicizzato.** Le FAQ sono un
  complemento: un gioco con solo il `bggId` non ottiene la chat. Il forum è
  materiale della community, a volte contraddittorio, e senza il regolamento
  come base l'agente darebbe per regola un'opinione.
- **Facoltativo come il resto**: senza chiave di ricerca la funzione non
  esiste e non genera errori.

## 2. Perché una web search esterna

L'API BGG non ha un endpoint di ricerca nei forum: elenca i thread (10 per
pagina, solo titoli) e restituisce un thread per id. Il forum Rules di
Wingspan ha 728 thread. Trovare «il thread giusto» è proprio la parte che
manca, e la fa un motore di ricerca.

Alternative scartate, verificate il 2026-09-25:

- **Scraping di un motore** (DuckDuckGo HTML): pagina anti-bot «anomaly» alla
  prima richiesta.
- **Ricerca nei titoli del forum** via `api.geekdo.com/api/forums/threads`:
  funziona, ma vede solo i titoli delle prime pagine «Top».
- **SearXNG self-hosted**: gira anche su Raspberry (immagine arm64/armv7),
  ma è un servizio in più da ospitare e, se l'app sta altrove, da esporre e
  proteggere con un reverse proxy. Troppo oneroso per il beneficio.
- **Brave Search**: niente più free tier puro ($5/mese di credito, carta
  obbligatoria).

Scelta: **Tavily**. 1.000 crediti/mese gratis senza carta, una ricerca
`basic` = 1 credito. Per un'associazione è ampiamente sufficiente.

## 3. Impostazioni

Migrazione `0022_tavily_api_key.sql`: colonna `tavily_api_key TEXT` in
`app_settings`. È un segreto, trattato come `ai_api_key`: non esce mai in
chiaro dall'API, e un campo lasciato vuoto nel form non la cancella (stessa
semantica già in uso).

`SettingsView.vue`: un campo «Chiave Tavily (ricerca nelle FAQ di BGG)» nella
sezione AI, con una riga che spiega a cosa serve e il link a tavily.com.

## 4. Componenti

### 4.1 `internal/websearch` — client Tavily

```go
type Result struct { Title, URL, Content string }
type Searcher interface {
    Search(ctx context.Context, query string, domains []string, max int) ([]Result, error)
}
```

`POST https://api.tavily.com/search`, header `Authorization: Bearer <key>`,
body `{query, search_depth: "basic", max_results, include_domains}`. Base URL
configurabile per i test (`httptest`). Senza chiave: `ErrNotConfigured`.

### 4.2 `internal/bgg` — thread dal JSON geekdo

Si usa `api.geekdo.com`, lo stesso JSON non ufficiale già usato per i Files:
risponde **senza token** e, a differenza dell'XML API2 (che per i thread
vuole il token), dice a quale gioco appartiene il thread.

- `GET /api/threads/<id>` → `source.type`/`source.id` (il gioco), `crumbs`
  (il nome del forum), `subject`.
- `GET /api/articles?threadid=<id>&pageid=1` → `articles[]` con `body`
  (testo semplice), `postdate`, `canonical_link` (link al commento),
  `firstPost`.

```go
type Thread struct {
    ID, Subject, GameID string
    Articles []Article // Body, PostDate time.Time, Link
}
func (c *HTTPClient) Thread(ctx context.Context, threadID string) (Thread, error)
```

Solo la prima pagina di articoli (25): il primo post più le prime risposte è
dove sta quasi sempre la risposta. Il `body` può contenere markup BGG
(`[q]`, `[b]`, `[url=…]`): si tolgono i tag, si tiene il testo.

### 4.3 Il tool `cerca_nelle_faq`

Un tool **separato** da `cerca_nelle_fonti`, perché la query è diversa: il
forum è in inglese e un motore di ricerca vuole una frase, non le varianti
lessicali italiane che servono a FTS5.

```json
{
  "type": "object",
  "properties": {
    "domanda_in_inglese": {
      "type": "string",
      "description": "La domanda sulla regola, tradotta in inglese, breve, con i termini del gioco. Esempio: \"refresh birdfeeder during forest action\"."
    }
  },
  "required": ["domanda_in_inglese"]
}
```

**Perché non un tool unico.** Valutato in brainstorming: `cerca_nelle_fonti`
che interroga sempre anche il forum spenderebbe un credito Tavily e 2-4
chiamate BGG a ogni domanda, anche quando il manuale basta, e metterebbe le
opinioni del forum accanto al regolamento già alla prima lettura; un
parametro facoltativo `domanda_in_inglese` sullo stesso tool è la stessa
scelta con un nome solo, meno esplicita per i modelli economici. Due tool
rendono visibile al modello la regola «prima il manuale».

In `ai` si aggiunge `type FAQSearchFunc func(ctx context.Context, query string) (string, error)`
e il campo `AskRequest.SearchFAQ FAQSearchFunc`; il tool si dichiara solo
se `SearchFAQ != nil`. L'handler lo passa solo se ci sono chiave Tavily e
`bggId`.

Flusso della closure (nell'handler, legata al gioco come `search`):

1. Tavily: query `"<nome gioco>" <domanda_in_inglese> rules`,
   `include_domains: ["boardgamegeek.com/thread"]` (Tavily accetta un path:
   toglie blog e geeklist), `max_results: 8` (circa metà dei
   risultati viene scartata dai filtri del passo 3).
2. Dagli URL si estraggono gli id con `boardgamegeek\.com/thread/(\d+)`,
   deduplicati, in ordine di rilevanza.
3. Per ogni id, `Thread`: si **scarta** se `GameID != bggId` (espansioni,
   giochi omonimi) o se il forum (ultimo `crumbs`) non è `Rules`. Variants
   sono regole della casa, General e Sessions sono rumore. Ci si ferma ai
   primi **2** thread validi.
4. Per ogni thread si prendono gli articoli in ordine fino a un budget di
   **6.000 caratteri** per thread (un articolo non si spezza: se non ci sta,
   si salta e si chiude il thread).
5. Il risultato è lo stesso array strutturato di `cerca_nelle_fonti`:

```json
[
  {
    "reference_type": "faq",
    "reference": "BGG: Refreshing the birdfeeder during an action",
    "reference_detail": "commento del 06/01/2019",
    "text": "What happens when I take a forest action that allows me to take two dice..."
  }
]
```

Nessun thread valido → array vuoto con un messaggio «nessuna discussione
trovata», come fa già `cerca_nelle_fonti` quando non trova nulla.

## 5. Citazioni

La spec del 09-09 prevedeva `reference` = URL per una FAQ. In pratica il
modello scriverebbe l'URL in chiaro nella risposta. Cambia così:

- `reference` = `BGG: <titolo del thread>` — leggibile, e unico per
  richiesta (se due thread hanno lo stesso titolo, si aggiunge `#<id>`).
- `reference_detail` = `commento del DD/MM/YYYY`.
- `citationTarget` guadagna due campi: `url` (il thread) e `commentURLs`
  (`reference_detail` → `canonical_link` del commento). Due commenti dello
  stesso thread citati nella stessa risposta portano ciascuno al suo
  commento; se il modello cita la sola `reference`, o una data che non
  corrisponde, il link va al thread. Due commenti dello stesso giorno nello
  stesso thread: vince il primo.

`linkifyCitations` riconosce, oltre a `, pagina N`, il suffisso
`, commento del DD/MM/YYYY`, e nel caso `faq` usa `commentURLs`/`url`
invece della reference.

## 6. Prompt

Il system prompt, quando `SearchFAQ` è dichiarato, aggiunge:

- il manuale è la fonte primaria: si cerca prima lì;
- `cerca_nelle_faq` si usa quando il manuale non risponde o è ambiguo, o se
  chi chiede nomina un caso specifico che il manuale non copre;
- una risposta presa dal forum si presenta come chiarimento della community
  («sul forum di BGG…»), e se il testo dice che a rispondere è il designer o
  l'editore, lo si dice;
- se manuale e forum si contraddicono, prevale il manuale e si segnala la
  discrepanza.

## 7. Errori e costi

- Tavily o geekdo non rispondono / errore → il tool restituisce «Le FAQ non
  sono disponibili in questo momento.» e l'agente risponde col manuale.
  Nessun errore verso l'utente, log lato server.
- Timeout per singola chiamata esterna: 10 s. `faq.Search` ha anche un
  tetto complessivo di 20 s (la Tavily più le `Thread` che seguono, tutte
  sequenziali): senza, un geekdo lento da solo consumerebbe l'intero
  budget della domanda (`askTimeout`, 60 s) e la risposta finale del
  modello fallirebbe. Se è stato trovato almeno un thread ma NESSUNA
  lettura riesce, il tool segnala un errore, che diventa il messaggio di
  indisponibilità sopra; se qualche lettura riesce ma tutte vengono
  scartate dai filtri del passo 3, il risultato resta un array vuoto senza
  errore.
- Costo: 1 credito Tavily per chiamata al tool; `MaxToolIterations` (5)
  limita il caso peggiore, e l'handler applica anche un tetto proprio di 2
  chiamate a `faq.Search` per domanda (`askMaxFAQSearches`): oltre, il tool
  risponde con un messaggio che invita il modello a rispondere con quello
  che ha, senza consumare altro credito. Nessuna cache: scelta di
  semplicità, da rivedere solo se i crediti finiscono davvero.

## 8. Test

- `websearch`: request (header, body) e parsing risposta con `httptest`;
  errore HTTP; chiave vuota → `ErrNotConfigured`.
- `bgg.Thread`: parsing thread + articoli; pulizia del markup BGG.
- Closure FAQ (nell'handler, con fake `Searcher` e fake BGG): estrazione id
  dagli URL, scarto dei thread di altri giochi, stop a 2 thread, budget di
  caratteri, errore esterno → messaggio di indisponibilità.
- `ai.Ask`: il tool FAQ si dichiara solo con `SearchFAQ`; due tool nello
  stesso loop; prompt coerente con i tool dichiarati.
- `linkifyCitations`: FAQ con `url` per commento; più commenti dello stesso
  thread.
- Settings: la chiave non esce in chiaro; vuoto non la cancella.
- Frontend: `npm run build`, verifica in Chrome, passata `/impeccable` sul
  campo nelle impostazioni.

## 9. Non-obiettivi

- Altri forum oltre a Rules, o altri siti oltre a BGG.
- Altri provider di ricerca (Brave, SearXNG): il `Searcher` è
  un'interfaccia, aggiungerli dopo è possibile, ma non ora.
- Cache dei risultati o persistenza delle FAQ.
- Chat per giochi senza manuale indicizzato.

## 10. Rischi aperti

- **Il path del forum non si può usare come filtro.** Gli URL dei thread
  sono `/thread/<id>/<slug>`, senza gioco né forum; il path del forum Rules
  (`/boardgame/<id>/<slug>/forums/66`) è la pagina elenco, e con quel path
  in `include_domains` Tavily non restituisce nulla (verificato). Gioco e
  forum si controllano quindi per forza dopo, via geekdo.
- **Qualità dei risultati Tavily: misurata il 2026-09-25**, 6 domande
  (4 Wingspan, 1 Carcassonne, 1 Azul), 5 risultati ciascuna. 26 risultati su
  30 erano thread; di questi 22 del gioco giusto, e **17 nel forum Rules e
  pertinenti** (tra cui il thread fissato «[FAQ]» di Wingspan). Cinque
  domande su sei avevano almeno 2 thread Rules utili; quella sullo spareggio
  degli obiettivi di round ne aveva uno solo (da qui `max_results: 8`). Il resto: espansioni o giochi omonimi
  (scartati dal filtro sul gioco), Variants/General/Sessions (scartati dal
  filtro sul forum), blog e geeklist (ora esclusi a monte da
  `boardgamegeek.com/thread`). I filtri del
  passo 3 sono quindi necessari, non prudenza.
- **`api.geekdo.com` non è un'API ufficiale.** Lo usiamo già per i Files:
  se cambia, si rompono entrambe le cose e l'agente degrada al solo manuale.
