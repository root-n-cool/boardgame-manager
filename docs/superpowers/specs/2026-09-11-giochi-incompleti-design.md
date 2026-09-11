# Giochi incompleti e log dei prestiti per gioco — Design

Data: 2026-09-11

## 1. Panoramica e obiettivi

La checklist di riconsegna (spec del 2026-09-10) registra cosa non è tornato
dentro una scatola, ma quel dato oggi vive solo nel registro di quella
serata. Chi apre il catalogo la settimana dopo non vede nessuna differenza
fra un gioco integro e uno a cui mancano cinque carte, e chi vuole capire
cos'è successo a una scatola deve aprire evento per evento.

Questa fase aggiunge due cose:

1. **Il marchio "incompleto"** sul gioco: una segnalazione che nasce da sola
   quando una riconsegna rileva una mancanza, e che resta finché un admin
   non dice che è stata sistemata.
2. **Il log dei prestiti per gioco**: tutti i prestiti di quel gioco
   attraverso tutte le serate, con in evidenza quelli tornati incompleti e
   cosa mancava. Raggiungibile dalla scheda del gioco **sempre**, che il
   gioco sia segnalato o no.

**Non è in scope**: un inventario dei pezzi da ricomprare, un blocco sul
prestito di un gioco incompleto, e qualunque comparsa del marchio sulle
pagine pubbliche.

## 2. Decisioni prese in brainstorming

Confermate dall'utente; non vanno ridiscusse senza un motivo nuovo.

- **La segnalazione la chiude un admin**, non il tempo e non la riconsegna
  successiva. Il marchio significa "c'è una cosa da fare", non "è successo
  qualcosa": se lo cancellasse il reso successivo, basterebbe una spunta
  distratta per far sparire una perdita vera.
- **La aprono solo le mancanze vere.** Una voce contata meno dell'attesa
  (carte 35 su 40) segnala il gioco. Una voce lasciata **non verificata**
  no: nessuno ha guardato, e non è una perdita. È la stessa distinzione a
  tre stati della fase precedente, usata qui per decidere cosa merita un
  marchio.
- **Il marchio si vede in tre posti**: elenco giochi, scheda gioco admin,
  banco prestiti. **Non** sulle pagine pubbliche — né sulla scheda del
  gioco né sul tavolo dell'evento.
- **Il log dei prestiti è solo admin** e contiene tutti i prestiti, non solo
  quelli con problemi: contiene nomi e telefoni dei partecipanti.
- **Questa fase ribalta una scelta della precedente.** Nel brainstorming del
  10 settembre l'utente aveva preferito "si registra e si chiude" a "il
  gioco resta segnalato". La seconda opzione è quella che si costruisce ora:
  è un cambio di idea esplicito, non una svista.

## 3. Modello dati

Migrazione `backend/internal/db/migrations/0018_materials_checked.sql`,
forward-only come tutte le altre.

```sql
-- Quando un admin ha dichiarato che la scatola è di nuovo a posto, e chi.
-- NULL significa "nessuno l'ha mai fatto", che è lo stato di partenza
-- giusto per tutto il catalogo esistente: una segnalazione aperta oggi da
-- un prestito di ieri deve comparire senza bisogno di popolare niente.
ALTER TABLE games ADD COLUMN materials_checked_at TEXT;

-- SET NULL e non CASCADE: se l'utente che ha chiuso la segnalazione viene
-- cancellato, resta vero che la segnalazione è stata chiusa.
ALTER TABLE games ADD COLUMN materials_checked_by INTEGER
    REFERENCES users(id) ON DELETE SET NULL;
```

**Lo stato è derivato, non memorizzato.** Non esiste una colonna
`is_incomplete` né una tabella di segnalazioni: un gioco è incompleto se,
*dopo* la sua ultima dichiarazione di completezza, una riconsegna ha
registrato una mancanza.

```sql
-- La regola, scritta una volta sola.
EXISTS (
  SELECT 1 FROM loan_material_issue lmi
  JOIN game_loans l   ON l.id = lmi.loan_id
  JOIN event_games eg ON eg.id = l.event_game_id
  WHERE eg.game_id = games.id
    AND lmi.returned IS NOT NULL        -- contata e mancante, non "non verificata"
    AND l.returned_at IS NOT NULL
    AND (games.materials_checked_at IS NULL
         OR l.returned_at > games.materials_checked_at)
)
```

**Perché derivato e non un flag.** Un booleano su `games` sarebbe una
seconda verità accanto a `loan_material_issue`, e le due possono divergere:
un flag resta acceso dopo che la riga d'esito è sparita con la cancellazione
di un prestito, o resta spento perché un percorso di scrittura si è
dimenticato di alzarlo. Derivandolo, il marchio *è* il registro. In più una
perdita nuova dopo una risoluzione riapre la segnalazione da sé, senza una
riga di codice che ci pensi. È lo stesso ragionamento di
`game_loans.returned_at`: lo stato è il dato che manca, non una colonna che
lo racconta.

Le date in SQLite sono stringhe `YYYY-MM-DD HH:MM:SS` in UTC scritte da
`datetime('now')`, quindi il confronto `>` fra `returned_at` e
`materials_checked_at` è un confronto lessicografico corretto: lo stesso
formato che il resto dello schema usa già.

## 4. Backend

### 4.1 `internal/events` — cosa manca a un gioco

Nuove funzioni in `backend/internal/events/loans.go`.

```go
// MissingPiece è una voce che a un gioco risulta mancante: quante se ne
// aspettano e quante ne sono tornate l'ultima volta che qualcuno le ha
// contate davvero.
type MissingPiece struct {
    Name     string
    Expected int
    Returned int
    // Since è il returned_at del prestito che l'ha rilevata: serve a dire
    // "dalla serata del 7 settembre" invece di un generico "manca".
    Since time.Time
}

// GamesMissingPieces torna, per un gruppo di giochi, le voci mancanti non
// ancora risolte, in ordine di nome. Un gioco assente dalla mappa (o con
// una lista vuota) è completo.
//
// Una query sola con una IN, non una per gioco: la chiama l'elenco del
// catalogo con tutti i giochi in archivio.
func (s *Store) GamesMissingPieces(ctx context.Context, gameIDs []int64) (map[int64][]MissingPiece, error)
```

La query raggruppa per `(game_id, name)` e tiene la rilevazione più recente:
se le stesse carte sono risultate corte in tre serate, la scheda deve dire
l'ultimo conteggio, non tre righe.

```sql
SELECT eg.game_id, lmi.name, lmi.expected, lmi.returned, MAX(l.returned_at)
FROM loan_material_issue lmi
JOIN game_loans l   ON l.id = lmi.loan_id
JOIN event_games eg ON eg.id = l.event_game_id
JOIN games g        ON g.id = eg.game_id
WHERE eg.game_id IN (?, ?, …)
  AND lmi.returned IS NOT NULL
  AND l.returned_at IS NOT NULL
  AND (g.materials_checked_at IS NULL OR l.returned_at > g.materials_checked_at)
GROUP BY eg.game_id, lmi.name
ORDER BY eg.game_id, lmi.name
```

`MAX(l.returned_at)` con le altre colonne nude è la forma idiomatica di
SQLite per "la riga del massimo": documentata e stabile
(`https://sqlite.org/lang_select.html#bareagg`), e più leggibile di una
window function per una query che deve restare comprensibile a chi la
riapre fra un anno. Il comportamento va dichiarato nel commento, perché
altrove sarebbe un bug.

`internal/events` legge la colonna di `games` come già legge `game_material`
in `writeMaterialIssues`: è un monolite con un database solo e il pacchetto
non importa `internal/games`, solo le sue tabelle.

```go
// LoanWithEvent è un prestito come lo mostra il log di un gioco: la copia
// e la serata risolte, perché il log non è raggruppato per evento.
type LoanWithEvent struct {
    Loan
    EventID    int64
    EventTitle string
    EventDate  string
    CopyIndex  int
    Copies     int
}

// ListLoansForGame è tutto il registro di un gioco, aperti e chiusi
// insieme, dal più recente. Chi chiama separa i due gruppi guardando
// ReturnedAt, come già fa il banco prestiti.
func (s *Store) ListLoansForGame(ctx context.Context, gameID int64) ([]LoanWithEvent, error)
```

Gli esiti dei prestiti chiusi si leggono con il `ListMaterialIssues` che
esiste già, in una chiamata sola con tutti gli id: il log di un gioco molto
prestato non deve diventare una N+1.

### 4.2 `internal/games` — chiudere la segnalazione

In `backend/internal/games/materials.go`:

```go
// MarkMaterialsChecked dichiara che la scatola è di nuovo a posto: da
// questo istante le mancanze registrate prima non segnalano più il gioco.
// Le rilevazioni restano nel registro dei prestiti — si archivia la
// segnalazione, non la storia.
func (s *Store) MarkMaterialsChecked(ctx context.Context, gameID, userID int64) (Game, error)
```

`games.Game` guadagna i due campi `MaterialsCheckedAt *time.Time` e
`MaterialsCheckedBy *int64`, letti dalle query esistenti.

### 4.3 `internal/httpapi` — rotte e risposte

| Metodo | Rotta | Cosa fa |
|---|---|---|
| `GET` | `/api/games/{id}/loans` | il log dei prestiti del gioco (admin) |
| `POST` | `/api/games/{id}/materials/resolve` | chiude la segnalazione (admin) |

Entrambe nel blocco `protected`. La seconda risponde con la scheda gioco
aggiornata, così il frontend non deve ricaricare.

**Il marchio non esce mai al pubblico.** `GET /api/games` e
`GET /api/games/{id}` sono rotte pubbliche usate anche dall'admin
(`GamesView.vue` è su `/admin/games` ma legge `/api/games`). La regola:

- `toGameSummary` guadagna `"incomplete": bool` **solo** quando la
  richiesta ha una sessione (`currentUser(r)`);
- `toGameDetail` guadagna `"missingPieces": [{name, expected, returned, since}]`
  con la stessa condizione;
- senza sessione le due risposte restano identiche a oggi, byte per byte, e
  la query aggregata non viene nemmeno eseguita — un visitatore che apre il
  catalogo non deve pagare una join che non vedrà.

La riga di ogni copia in `GET /api/events/{id}/loans` guadagna
`"incomplete": bool` (rotta già protetta), perché il banco lo mostra prima
della consegna.

Il log risponde:

```json
{
  "loans": [
    {
      "id": 12, "eventId": 3, "eventTitle": "Serata di settembre",
      "eventDate": "2026-09-07", "copyIndex": 1, "copies": 2,
      "borrowerName": "Anna Bianchi", "borrowerPhone": "333…",
      "lentAt": "…", "returnedAt": "…", "notes": null,
      "materialIssues": [{"name": "carte", "expected": 40, "returned": 35}]
    }
  ]
}
```

`materialIssues` ha la stessa forma che ha già nel banco prestiti: chi ha
scritto un pezzo di UI per una delle due schermate riconosce l'altra.

## 5. Frontend

### 5.1 Il marchio, tre posti

- **Elenco giochi** (`GamesView.vue`): una pastiglia **"Incompleto"** sulla
  card, in colore d'allarme (`--danger`), sovrapposta in alto a sinistra
  sulla copertina. Parola sola: la card è già densa e il dettaglio sta un
  clic più in là. Non un bordo colorato sul lato, che `DESIGN.md` rifiuta
  esplicitamente sulle card.
- **Scheda gioco admin** (`GameAdminDetailView.vue`): in cima, sopra le
  card, un avviso che dice *cosa* manca e da quando —
  `carte 35 di 40 · segnalini pesce 4 di 6 — dalla serata del 7 settembre` —
  col link al log e il bottone **"Segna come completo"**.
  Nessuna conferma modale: il bottone sta dentro l'avviso che elenca ciò che
  archivia, ed è la stessa scelta già fatta per "Rigenera" nelle domande
  suggerite — la protezione è la distanza e il contesto, non un dialogo.
  Dopo il clic l'avviso sparisce e resta una riga di conferma.
- **Banco prestiti** (`LoanDeskView.vue`): la stessa pastiglia sulla riga
  della copia, accanto al nome del gioco, prima di consegnarla.

La pastiglia è un pattern nuovo: `.state-chip` accanto a `.lang-chip`
(`app.css:1727`), stessa sagoma, colore d'allarme. Va documentata in
`DESIGN.md` fra i componenti.

### 5.2 Il log dei prestiti — `GameLoansView.vue`

Rotta `/admin/games/:id/prestiti`, sorella di `GameLeaderboardView`
(`/games/:id/leaderboard`) ma **protetta**, non `meta.public`.

Tre forme di riga, dal più recente:

```
Serata del 7 set · Anna Bianchi              21:14 → 23:40
  mancano: carte 35/40 · segnalini pesce 4/6

Serata del 31 ago · Marco Neri               20:50 → 23:05

Serata di oggi · Luca Verdi                  fuori dalle 21:20
```

- La riga di un prestito tornato incompleto si distingue **prima di essere
  letta**: fondo `--danger-bg` e l'elenco di ciò che mancava sotto, nella
  stessa forma testuale del registro del banco (`mancano: carte 35/40`).
- Un prestito ancora aperto mostra l'ora di uscita e nessuna di rientro.
- Il numero di copia compare solo se il gioco ne ha più di una, come già fa
  il banco.
- Stato vuoto: *"Questo gioco non è mai stato dato in prestito."*
- Mobile-first come il banco: la riga regge i 390px senza scroll
  orizzontale, orari in mono.

### 5.3 L'accesso al log, "sempre"

Sulla scheda gioco admin, accanto al link "Classifica" che è già lì
(`GameAdminDetailView.vue:359`), un link **"Prestiti"**. Presente sempre,
segnalato o no — è la richiesta esplicita dell'utente. Sulla scheda pubblica
non compare: contiene nomi e telefoni.

## 6. Test

**Backend** (in Docker, come da CLAUDE.md):

- `internal/events`: `GamesMissingPieces` con nessuna mancanza, con una
  mancanza, con una voce solo *non verificata* (non deve segnalare), con due
  rilevazioni della stessa voce in serate diverse (vince la più recente),
  con `materials_checked_at` posteriore alla mancanza (non segnala) e
  anteriore (segnala); `ListLoansForGame` con prestiti su più eventi e su
  più copie, ordine, e prestito ancora aperto.
- `internal/games`: `MarkMaterialsChecked` scrive data e utente; un gioco
  inesistente dà `ErrNotFound`.
- `internal/httpapi`: `GET /api/games` **senza** sessione non contiene
  `incomplete`, con sessione sì; `GET /api/games/{id}/loans` 401 senza
  sessione e 404 su un gioco inesistente; `POST …/materials/resolve` chiude
  la segnalazione e una `GET` successiva non la mostra più.

**Frontend**: `npm run build` (che fa il type-check con `vue-tsc`) e verifica
visiva con Claude in Chrome — il log a 390px e da desktop, la pastiglia nel
catalogo, l'avviso sulla scheda e il bottone di risoluzione.

**Chiusura**: pass `/impeccable` sulle superfici toccate, come da CLAUDE.md.

## 7. Rischi e cose da tenere d'occhio

- **Il marchio può diventare rumore.** Se un'associazione conta i pezzi con
  scrupolo, molti giochi finiranno segnalati e la pastiglia smetterà di
  attirare l'occhio. La mitigazione non è un filtro in più ma la facilità di
  chiudere una segnalazione: un clic dalla scheda, senza dialoghi.
- **La lista materiali cambia sotto la segnalazione.** Nome e quantità sulla
  riga d'esito sono copiati al momento della riconsegna, quindi l'avviso può
  nominare una voce che nel catalogo non esiste più. È voluto: è cosa
  mancava quel giorno. Vale la pena scriverlo nel commento dell'avviso.
- **`GamesMissingPieces` con tutto il catalogo** è una join su tre tabelle
  con una `IN` grande. A questa scala (decine di giochi, centinaia di
  prestiti) è irrilevante; se un domani il catalogo crescesse di un ordine
  di grandezza, la risposta è un indice su `game_loans(returned_at)`, non
  una colonna denormalizzata.
