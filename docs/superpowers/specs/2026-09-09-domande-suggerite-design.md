# Domande suggerite generate dal manuale — Design

Data: 2026-09-09

Sostituisce il meccanismo delle domande suggerite introdotto con la chat
pubblica (`2026-09-08-domande-sul-manuale-design.md`). Il resto di quella
spec — la ricerca FTS5, il loop dell'agente, le citazioni — resta in vigore.

## 1. Il problema, misurato

Nello stato di riposo la chat mostra tre domande suggerite. Oggi nascono da
una catena deterministica: l'indicizzazione salva su ogni chunk il titolo
della sezione (`game_source_chunk.heading`), `Summary` ne raccoglie i
distinti **in ordine di documento** e ne manda al massimo otto al frontend
(`maxSuggestionHeadings`), e `ManualChatPanel` passa ciascun titolo per una
tabella fissa di sette voci (`headingToQuestion`), ripiegando su
`Cosa dice il manuale su "<titolo>"?` per tutto il resto. Deduplica, e
mostra i primi tre.

Sul manuale reale del club (Carcassonne, 4 pagine scansionate) l'API manda
questi titoli, in quest'ordine:

```
di Klaus-Jürgen Wrede · Contenuto · Principi generali · Preparazione
Il gioco · Piazzare le tessere · Fine del gioco · Conteggio dei punti
```

e le tre domande a schermo risultano quindi:

```
Cosa dice il manuale su "di Klaus-Jürgen Wrede"?
Cosa dice il manuale su "Contenuto"?
Cosa dice il manuale su "Principi generali"?
```

Il nome dell'autore come domanda suggerita. **Nessuna delle tre è utile**, e
i difetti sono due e indipendenti:

- **La scelta.** Si prendono i primi tre in ordine di pagina, e le prime
  pagine di un regolamento sono copertina, contenuto della scatola e
  premesse — mai le regole.
- **La formulazione.** `Cosa dice il manuale su "X"?` è un template avvolto
  intorno a un titolo di sezione: una query di ricerca travestita da
  domanda. Al tavolo si chiede *"Quando finisce la partita?"*, non *"Cosa
  dice il manuale su Fine del gioco?"*.

Il secondo difetto è la ragione per cui **allargare `headingToQuestion` non
è una soluzione**: qualunque titolo non previsto ricade sul template, quindi
il difetto resta ad aspettare il prossimo manuale. E anche restando su
questo, la tabella mancherebbe comunque il bersaglio — ha la chiave
`fine partita` ma il manuale dice *"Fine del gioco"*, ha `conteggio` ma il
manuale dice *"Conteggio dei punti"*. L'unico titolo che riconoscerebbe è
"Preparazione", che sta in quarta posizione e non viene mai mostrato.

## 2. Obiettivo

Le tre domande le **scrive il modello**, leggendo i titoli di sezione del
manuale vero, e l'admin può **riscriverle a mano** quando una non gli piace.

Tre decisioni di prodotto prese in fase di brainstorming valgono come
vincoli, non come opzioni:

- **`rigenera` sovrascrive anche le domande scritte a mano.** È un pulsante
  che l'admin premette: se lo premi, lo stai chiedendo. Nessuna conferma.
- **`Summary.Headings` si cancella.** Diventerebbe codice morto: nell'API i
  titoli distinti servono *solo* alle domande suggerite.
- **Esattamente tre domande, nessuna scorta.** Non se ne generano cinque per
  mostrarne tre: una domanda che non piace si riscrive, non si scarta.

## 3. Modello dati

Migrazione `0016_suggested_questions.sql`, forward-only come le altre:

```sql
CREATE TABLE game_suggested_question (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,

    -- 0, 1, 2: l'ordine in cui compaiono a schermo.
    position INTEGER NOT NULL,

    text TEXT NOT NULL,

    -- 1 = riscritta a mano dall'admin: una reindicizzazione non la tocca.
    edited INTEGER NOT NULL DEFAULT 0,

    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX idx_suggested_question_game_pos
    ON game_suggested_question(game_id, position);
```

**Per gioco, non per manuale né per lingua**: la chat è per gioco e cerca in
tutte le fonti insieme, quindi una domanda suggerita non appartiene a un
singolo documento. La cascata su `games` porta via le domande col gioco.

**Una riga per domanda, non un campo JSON su `games`**, perché `edited` è
una proprietà della singola domanda: con un blob, "rigenera solo quelle che
non ho toccato" diventerebbe un read-modify-write dell'intero insieme, con
la lettura e la scrittura separate da una chiamata al modello che dura
secondi. L'indice unico su `(game_id, position)` rende impossibile la
posizione 1 due volte.

## 4. Generazione

### 4.1 La firma

```go
func (c *HTTPClient) SuggestQuestions(
    ctx context.Context, gameName string, headings []string,
) ([]string, error)
```

Riceve il nome del gioco e i titoli di sezione distinti che `Summary` già
calcola — circa 200 token, una sola chiamata al **modello di testo**
(`c.Model`), non a quello vision. Il nome del gioco serve al modello per
capire di che gioco si parla quando i titoli sono generici.

L'astrazione segue lo schema di `Segmenter` e `Transcriber`: un'interfaccia
piccola nel package `ai`, implementata da `HTTPClient` e finta nei test.

### 4.2 Il prompt

Chiede di **ignorare i titoli che non sono regole** (nome dell'autore,
contenuto della scatola, indice, ringraziamenti) e di scrivere esattamente
tre domande brevi, in italiano, come le farebbe un giocatore al tavolo, una
per riga, senza numerazione e senza preamboli.

### 4.3 La validazione non è opzionale

Come per `Segment`, il vincolo espresso a parole non è una garanzia: la
mitigazione vera è meccanica. La risposta è accettata solo se sono
**esattamente tre righe non vuote**, ciascuna che **finisce con `?`** e
**sotto i 120 caratteri**. Qualunque altra cosa è `ErrSuggestionsRejected`,
e il chiamante ripiega.

Serve perché questo testo va sulla **scheda pubblica**: un modello che
risponde *"Ecco tre domande:"* non deve poter mettere quella riga a schermo.
Il tetto sui caratteri esiste perché le domande vivono in tre bottoni su
uno schermo di telefono.

Nessun retry: stesso confine di `Segment` (il retry vive in `Transcribe`,
l'unica chiamata che si fa N volte per un documento).

## 5. Innesto sull'indicizzazione

In coda a `indexMediaHandler`, **dopo** che i chunk sono stati salvati, e
**best-effort**:

1. si leggono le domande del gioco; se tutte tre sono `edited`, la chiamata
   al modello si salta del tutto — non c'è niente da riscrivere e non c'è
   motivo di pagarla;
2. altrimenti si genera e si scrivono **solo le posizioni non modificate**;
3. un errore si logga e si ignora.

Il punto 3 è deliberato: la risposta dell'indicizzazione non cambia.
Aggiungere ~2 secondi a un'operazione che sul manuale reale ne dura 145 non
si nota, ma trasformare un'indicizzazione riuscita in un errore per una
domanda suggerita sarebbe fuori scala rispetto al valore della feature.

Da cui il comportamento sul reindex, che è quello scelto in brainstorming:
sostituire uno scan brutto con uno buono **aggiorna le domande da sé**,
mentre il lavoro manuale dell'admin non si perde mai.

## 6. API admin

Tre rotte, protette come le altre rotte admin:

| Rotta | Effetto |
|---|---|
| `GET /api/games/:id/suggested-questions` | le tre righe, ciascuna col suo `edited` |
| `PUT /api/games/:id/suggested-questions` | i tre testi; quelli **diversi** da ciò che era salvato diventano `edited = 1` |
| `POST /api/games/:id/suggested-questions/regenerate` | rigenera tutte e tre, comprese le modificate, e azzera `edited` |

Il `PUT` scrive tutte e tre le posizioni in **upsert**: un gioco che non è
mai stato indicizzato non ha righe, e l'admin deve poter scrivere le sue tre
domande a mano prima (o invece) di qualunque generazione. Le righe create
così nascono `edited = 1`, come qualunque testo scritto a mano.

Il `PUT` marca `edited` confrontando col testo salvato, non con un flag che
arriva dal client: è il server a sapere cosa aveva scritto il modello, e un
client che rimanda invariata una domanda generata non deve poterla
promuovere a "scritta a mano" — altrimenti un salvataggio senza modifiche
congelerebbe le tre domande per sempre.

`regenerate` risponde con un errore leggibile quando il provider non è
configurato o quando la validazione rifiuta la risposta: è un pulsante
premuto a mano, quindi qui il silenzio del punto 3 della sezione 5 non va
bene — chi lo preme deve sapere se ha funzionato. E quando il gioco non ha
ancora nessuna fonte indicizzata non ci sono titoli da cui generare: la
risposta lo dice («indicizza prima un manuale»), non restituisce tre
domande inventate dal nulla.

## 7. Pannello admin

Nella pagina del gioco, accanto al pannello di fonti e indicizzazione: tre
campi di testo con le domande correnti, un pulsante *rigenera*, e un segno
visibile su quali sono scritte a mano — perché è l'informazione che spiega
perché una domanda non è cambiata dopo un reindex.

## 8. Lato pubblico

`sourceHeadings` sparisce da `/api/games/:id`; al suo posto
`suggestedQuestions: string[]`, sempre un array e mai `null`, come già fa
`sourceHeadings` oggi.

`ManualChatPanel` perde `headingToQuestion` e la computed `suggestions`, e
rende direttamente la lista che riceve. Quando arriva vuota — un gioco
indicizzato prima di questa modifica, o una generazione mai riuscita —
ripiega su `fallbackQuestions`, che resta **esattamente come è**: le tre
domande fisse sono l'unica parte della macchina attuale che era già
formulata bene.

## 9. Cosa si cancella

- `headingToQuestion` e la computed `suggestions` (`ManualChatPanel.vue`)
- `maxSuggestionHeadings` (`manuals/store.go`)
- `Summary.Headings` e il campo `sourceHeadings` (`games_responses.go`)
- la prop `headings` lungo la catena `GameDetailView` → `ManualChat` →
  `ManualChatPanel`

Resta `Summary.Sources`, i titoli **raggruppati per fonte**: alimenta
l'indice del prompt (`formatCorpusIndex`) ed è una cosa diversa.

## 10. Test

**`ai`** — tre righe valide passano; due righe, quattro righe, un preambolo
e una riga che non finisce con `?` vengono rifiutate con
`ErrSuggestionsRejected`; la richiesta usa il modello di testo e non quello
vision; senza provider è `ErrNotConfigured`.

**Store** — il test che conta: una rigenerazione **sostituisce le non
modificate e preserva le modificate**. Più la cascata alla cancellazione del
gioco e il vincolo di unicità su `(game_id, position)`.

**`httpapi`** — l'indicizzazione genera quando non c'è niente salvato; con
tutte tre `edited` non fa **nessuna** chiamata AI (verificato sul contatore
del finto, non sul risultato); una generazione fallita lascia
l'indicizzazione a 200 con i suoi chunk; `regenerate` sovrascrive anche le
modificate; `PUT` marca `edited` solo sui testi cambiati; le tre rotte
richiedono l'autenticazione.

**Frontend** — `npm run build` (che fa il type-check con `vue-tsc`), e come
**ultimo task** `/impeccable` sul pannello admin e sullo stato di riposo
della chat, come richiesto da CLAUDE.md per ogni intervento che tocca la UI.

## 11. Non-obiettivi

- **Generare le domande dal testo dei chunk** invece che dai soli titoli.
  Duecento token di titoli bastano a scrivere tre domande buone; mandare il
  manuale intero costerebbe cento volte tanto per un guadagno da dimostrare.
- **Un numero di domande configurabile.** Tre, come oggi: sono tre bottoni
  su uno schermo di telefono.
- **Domande per lingua.** La chat è per gioco e cerca in tutte le fonti
  insieme; separarle per lingua richiederebbe prima di decidere cosa
  significhi la chat in una lingua sola.
- **Tradurre le domande.** Sono in italiano come tutta la UI: nessun i18n.

## 12. Rischi aperti

- **Il modello può scrivere una domanda a cui il manuale non risponde.** La
  chat risponderà "il manuale non ne parla", che è un esito onesto ma non
  brillante come primo contatto. La mitigazione è l'admin che la riscrive:
  è la ragione per cui la modifica manuale è nello scope e non in una fase
  successiva.
- **I titoli di sezione arrivano dal modello** (i `##` della trascrizione
  vision, o quelli inseriti da `Segment`), quindi la qualità delle domande
  eredita quella della trascrizione. Su un manuale letto male le domande
  saranno mediocri — ma su un manuale letto male lo è anche la chat, e il
  problema si affronta là (downscale delle immagini, retry), non qui.
