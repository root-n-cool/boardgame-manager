package manuals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// hitsPerKeyword è quanti chunk risalgono per ogni parola chiave. Due:
// abbastanza perché una parola chiave conti qualcosa nel payload, poco
// abbastanza perché otto parole chiave non producano un muro di testo.
const hitsPerKeyword = 2

// maxKeywords è il tetto alle parole chiave di una singola ricerca. Lo
// schema del tool ne chiede da 3 a 8, ma è una richiesta, non un vincolo:
// nessun provider garantisce di rispettarla, e il payload del risultato
// rientra nel contesto a OGNI iterazione del loop. Quaranta parole chiave
// significherebbero quaranta query FTS, fino a ottanta chunk, due query in
// più per chunk per i vicini, e decine di migliaia di caratteri al posto
// dei 200-600 token previsti — su una rotta pubblica che si paga a token.
// Dodici è largo rispetto alle otto chieste: taglia l'abuso, non l'uso.
//
// Il taglio sta qui e non nel chiamante perché è lo store a dover reggere
// qualunque chiamante, oggi e domani.
const maxKeywords = 12

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

// SourceChunk è un pezzo cercabile di una fonte, pronto per essere salvato.
// ReferenceType e Reference dicono cosa citare, ReferenceDetail dove dentro
// la fonte (pagina, sezione, data del commento); Heading vive solo nel
// database e alimenta l'indice nel prompt, non la risposta del tool.
type SourceChunk struct {
	ReferenceType   string // "document" | "faq"
	Reference       string
	ReferenceDetail string // "" quando la fonte non ne ha
	Heading         string // "" quando la sezione non ha titolo
	LanguageCode    string // "" per una FAQ
	Seq             int
	Text            string
}

// SourceHeadings raggruppa i titoli distinti di UNA fonte, in ordine di
// seq: alimenta l'indice nel prompt, che elenca le sezioni fonte per fonte
// ("Fonti: Carcassonne_ITA.pdf — Preparazione · Turno del giocatore").
type SourceHeadings struct {
	Reference string
	Headings  []string
}

// SourceSummary è quel che la scheda gioco e il prompt devono sapere delle
// fonti di un gioco: se ce n'è almena una (governa, col provider
// configurato, la comparsa della chat), i titoli distinti per generare le
// domande suggerite, lo stesso raggruppato per fonte per l'indice nel
// prompt, e quanti chunk ha ciascun media per il pannello admin.
type SourceSummary struct {
	HasChunks bool
	Headings  []string         // distinti, in ordine di seq — input di ai.SuggestQuestions
	Sources   []SourceHeadings // per l'indice del prompt, raggruppato per fonte
	PerMedia  map[int64]int    // game_media_id → numero di chunk, per il pannello admin
}

// ReplaceSource sostituisce tutti i chunk di UNA fonte (un media caricato, o
// una FAQ quando mediaID è nil) in una transazione: non esiste uno stato in
// cui i chunk sono nuovi e l'indice FTS5 è vecchio, né uno in cui metà
// fonte è indicizzata.
//
// mediaID è un *int64 perché una FAQ non ne ha: è il tipo che rende
// impossibile passare uno zero e credere che significhi "nessun media".
func (s *Store) ReplaceSource(ctx context.Context, gameID int64, mediaID *int64, chunks []SourceChunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if mediaID != nil {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM game_source_chunk WHERE game_media_id = ?`, *mediaID); err != nil {
			return fmt.Errorf("clear chunks: %w", err)
		}
	}

	for _, c := range chunks {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO game_source_chunk
			 (game_id, game_media_id, reference_type, reference, reference_detail, heading, language_code, seq, text)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			gameID, mediaIDArg(mediaID), c.ReferenceType, c.Reference,
			nullIfEmpty(c.ReferenceDetail), nullIfEmpty(c.Heading), nullIfEmpty(c.LanguageCode),
			c.Seq, c.Text); err != nil {
			return fmt.Errorf("insert chunk seq %d: %w", c.Seq, err)
		}
	}

	return tx.Commit()
}

// DeleteSource cancella tutti i chunk di un media caricato. Non serve per
// le FAQ (che non hanno un media_id): quelle si sostituiscono con
// ReplaceSource(ctx, gameID, nil, nil) quando servirà, nella fase che le
// costruisce.
func (s *Store) DeleteSource(ctx context.Context, mediaID int64) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM game_source_chunk WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	return nil
}

// HasChunks è la condizione, insieme al provider configurato, che governa
// la comparsa della chat in UI.
func (s *Store) HasChunks(ctx context.Context, gameID int64) (bool, error) {
	sum, err := s.Summary(ctx, gameID)
	return sum.HasChunks, err
}

// Summary legge in UNA query tutto quel che serve del gioco: se ha chunk, i
// titoli distinti in ordine di seq (scheda pubblica), lo stesso raggruppato
// per fonte (indice nel prompt), e il conteggio per media (pannello admin).
// Una query sola perché la scheda pubblica del gioco è la pagina che ogni
// partecipante apre, e non deve costare un giro per ciascuna di queste tre
// cose.
func (s *Store) Summary(ctx context.Context, gameID int64) (SourceSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT reference, game_media_id, COALESCE(heading, '')
		 FROM game_source_chunk
		 WHERE game_id = ?
		 ORDER BY game_media_id, reference, seq`, gameID)
	if err != nil {
		return SourceSummary{}, fmt.Errorf("source summary: %w", err)
	}
	defer rows.Close()

	out := SourceSummary{PerMedia: map[int64]int{}}
	seenHeadings := map[string]bool{}
	sourceIndex := map[string]int{}
	seenPerSource := map[string]map[string]bool{}
	for rows.Next() {
		var reference string
		var mediaID sql.NullInt64
		var heading string
		if err := rows.Scan(&reference, &mediaID, &heading); err != nil {
			return SourceSummary{}, err
		}
		out.HasChunks = true
		if mediaID.Valid {
			out.PerMedia[mediaID.Int64]++
		}

		heading = strings.TrimSpace(heading)
		if heading == "" {
			continue
		}

		key := strings.ToLower(heading)
		if !seenHeadings[key] {
			seenHeadings[key] = true
			out.Headings = append(out.Headings, heading)
		}

		// Il gruppo per l'indice del prompt è per game_media_id, non per
		// reference: due media distinti possono condividere temporaneamente
		// la stessa reference (la disambiguazione in scrittura è del Task
		// 5, che non è ancora stato fatto quando questo codice gira), e una
		// struttura di lettura non deve dipendere da un invariante
		// mantenuto due task più in là — altrimenti due manuali senza
		// titolo distinto collasserebbero in una voce sola. Una FAQ non ha
		// game_media_id: per quella riga la reference (l'URL della
		// conversazione) fa da chiave, perché è l'unico identificatore che
		// possiede — da cui il criterio doppio.
		sourceKey := "faq:" + reference
		if mediaID.Valid {
			sourceKey = fmt.Sprintf("media:%d", mediaID.Int64)
		}

		i, ok := sourceIndex[sourceKey]
		if !ok {
			out.Sources = append(out.Sources, SourceHeadings{Reference: reference})
			i = len(out.Sources) - 1
			sourceIndex[sourceKey] = i
			seenPerSource[sourceKey] = map[string]bool{}
		}
		if !seenPerSource[sourceKey][key] {
			seenPerSource[sourceKey][key] = true
			out.Sources[i].Headings = append(out.Sources[i].Headings, heading)
		}
	}
	return out, rows.Err()
}

// GamesWithChunks dice, per un gruppo di giochi, quali hanno almeno una
// fonte indicizzata. Una query sola con una IN invece di una HasChunks per
// gioco: la scheda evento la chiama con tutti i giochi della serata.
func (s *Store) GamesWithChunks(ctx context.Context, gameIDs []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(gameIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(gameIDs))
	for _, id := range gameIDs {
		args = append(args, id)
	}
	query := `SELECT DISTINCT game_id FROM game_source_chunk
		 WHERE game_id IN (?` + strings.Repeat(",?", len(gameIDs)-1) + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("games with chunks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func mediaIDArg(mediaID *int64) any {
	if mediaID == nil {
		return nil
	}
	return *mediaID
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// escapeFTS trasforma una parola chiave in una stringa FTS5 sempre legale:
// racchiusa in doppi apici, con i doppi apici interni raddoppiati. Perde gli
// operatori, che è esattamente il compromesso voluto — meglio una ricerca
// letterale che un errore.
func escapeFTS(kw string) string {
	return `"` + strings.ReplaceAll(kw, `"`, `""`) + `"`
}

// SourceHit è un chunk trovato dalla ricerca, nella forma che il modello
// riceve come risultato del tool: i quattro campi del contratto (Task 6
// della spec) più due cose che non escono nel JSON.
//
// MediaPath porta fino al chiamante (l'handler del Task 7) il
// game_media.url_or_path della fonte, così com'è nella join che Search fa
// già: quel chiamante deve costruire una mappa reference → percorso file
// dalle hit che ha DAVVERO ricevuto, e non può ricavarla cercando un media
// per titolo, perché la reference salvata dal Task 5 è disambiguata (puo'
// differire da game_media.title quando due manuali dello stesso gioco
// condividono il titolo). Senza questo campo il link di una citazione può
// puntare al file sbagliato quando un gioco ha più manuali — esattamente
// il bug che il Task 7 esiste per chiudere. Per una FAQ resta "": la
// reference è già l'URL della discussione, non serve nessun file.
type SourceHit struct {
	ReferenceType   string   `json:"reference_type"`
	Reference       string   `json:"reference"`
	ReferenceDetail string   `json:"reference_detail"`
	Text            string   `json:"text"`
	FoundWith       []string `json:"-"` // diagnostica, non esce al modello
	MediaPath       string   `json:"-"` // game_media.url_or_path, "" per una FAQ

	// Non esportati: servono solo a trovare il chunk adiacente (seq ± 1
	// dentro la stessa fonte) e non hanno senso per chi legge il risultato
	// del tool. mediaID è sql.NullInt64 e non int64 perché una FAQ non ha
	// un game_media_id: il tipo rende impossibile confondere "nessun
	// media" con lo zero.
	seq     int
	mediaID sql.NullInt64
}

// scannedHit è una riga letta da searchOne prima della deduplicazione e
// dell'ordinamento per lingua preferita. language vive qui e non su
// SourceHit perché serve solo a decidere l'ordine dentro searchOne: il
// contratto del tool non lo prevede fra i campi restituiti al modello.
type scannedHit struct {
	id       int64
	language string
	h        SourceHit
}

// Search esegue una query FTS5 per ogni parola chiave (al più maxKeywords)
// e unisce i risultati. Restituisce le hit trovate e le parole chiave senza
// nessun risultato: quella seconda lista non è un errore, è ciò che dice al
// modello quali ipotesi lessicali sono cadute, così può riprovare con altre
// parole.
//
// Una query per parola e non un unico OR: con l'OR una parola comune
// sommerge una rara, mentre così ogni variante ha i suoi due posti
// garantiti — ed è quello che rende utile passare i sinonimi tutti insieme.
func (s *Store) Search(ctx context.Context, gameID int64, preferLang string, keywords []string) ([]SourceHit, []string, error) {
	var missing []string
	order := []int64{}
	byID := map[int64]*SourceHit{}

	if len(keywords) > maxKeywords {
		keywords = keywords[:maxKeywords]
	}
	for _, raw := range keywords {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		hits, err := s.searchOne(ctx, gameID, preferLang, kw)
		if err != nil {
			return nil, nil, err
		}
		if len(hits) == 0 {
			missing = append(missing, kw)
			continue
		}
		for _, row := range hits {
			if existing, ok := byID[row.id]; ok {
				existing.FoundWith = append(existing.FoundWith, kw)
				continue
			}
			copied := row.h
			copied.FoundWith = []string{kw}
			byID[row.id] = &copied
			order = append(order, row.id)
		}
	}

	hits := make([]SourceHit, 0, len(order))
	for _, id := range order {
		hits = append(hits, *byID[id])
	}
	if err := s.attachNeighbours(ctx, gameID, hits); err != nil {
		return nil, nil, err
	}
	return hits, missing, nil
}

// searchOne cerca una sola parola chiave. Restituisce una SLICE ordinata e
// non una mappa: l'ordine è il risultato del lavoro qui sotto (rilevanza
// BM25, con la lingua preferita davanti) ed è quello che decide quale
// risultato il modello legge per primo. Restituirlo in una mappa lo
// buttava via — l'ordine di iterazione di una mappa in Go è deliberatamente
// casuale, quindi con due risultati in due lingue la "lingua preferita"
// vinceva a testa o croce. Il chiamante deduplica sull'id, che viaggia
// nella slice insieme all'hit.
//
// L'ordinamento per lingua preferita avviene in Go, non in SQL, e senza
// LIMIT nella query: si leggono tutte le righe ordinate per `rank`
// (rilevanza BM25), poi uno stable sort le riordina mettendo prima la
// lingua preferita — preservando l'ordine di rank dentro ciascun gruppo —
// e solo allora si taglia a hitsPerKeyword. Un LIMIT in SQL prima di quel
// riordino potrebbe scartare una riga nella lingua preferita che il motore
// FTS5 classifica oltre la finestra, prima ancora che il riordino per
// lingua abbia la possibilità di farla emergere.
func (s *Store) searchOne(ctx context.Context, gameID int64, preferLang, keyword string) ([]scannedHit, error) {
	query := func(match string) (*sql.Rows, error) {
		return s.db.QueryContext(ctx,
			`SELECT c.id, c.reference_type, c.reference, COALESCE(c.reference_detail, ''),
			        c.text, c.seq, c.game_media_id, COALESCE(c.language_code, ''),
			        COALESCE(m.url_or_path, '')
			 FROM game_source_chunk_fts f
			 JOIN game_source_chunk c ON c.id = f.rowid
			 LEFT JOIN game_media m ON m.id = c.game_media_id
			 WHERE game_source_chunk_fts MATCH ? AND c.game_id = ?
			 ORDER BY rank`, match, gameID)
	}

	rows, err := query(keyword)
	if err != nil {
		// La sintassi FTS5 si rompe su un apostrofo o una virgoletta. Gli
		// operatori però sono utili (pesc*, "frase esatta"), quindi non si
		// filtrano a monte: si riprova con la parola neutralizzata solo
		// quando la prima forma è illegale.
		rows, err = query(escapeFTS(keyword))
		if err != nil {
			return nil, fmt.Errorf("fts search %q: %w", keyword, err)
		}
	}
	defer rows.Close()

	var all []scannedHit
	for rows.Next() {
		var id int64
		var sh scannedHit
		if err := rows.Scan(&id, &sh.h.ReferenceType, &sh.h.Reference, &sh.h.ReferenceDetail,
			&sh.h.Text, &sh.h.seq, &sh.h.mediaID, &sh.language, &sh.h.MediaPath); err != nil {
			return nil, err
		}
		sh.id = id
		all = append(all, sh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortByPreferredLanguage(all, preferLang)

	if len(all) > hitsPerKeyword {
		all = all[:hitsPerKeyword]
	}
	return all, nil
}

// sortByPreferredLanguage mette il gruppo di preferLang davanti all'altro,
// con uno stable sort: preserva l'ordine di rilevanza (rank) letto dalla
// query dentro ciascun gruppo, sposta solo il gruppo giusto in testa.
// Estratta a parte perché è la funzione che un test unitario deve poter
// richiamare su dati costruiti a mano, senza passare da FTS5/BM25: è lì che
// si è già rotto un riordino per lingua preferita infilato in una mappa
// Go, la cui iterazione è casuale — un test che passasse dalla ricerca vera
// non discriminerebbe con certezza quel bug da un pareggio di rank
// benevolo.
func sortByPreferredLanguage(all []scannedHit, preferLang string) {
	sort.SliceStable(all, func(i, j int) bool {
		iPref := all[i].language == preferLang
		jPref := all[j].language == preferLang
		return iPref && !jPref
	})
}

// attachNeighbours allega al testo di una hit il chunk adiacente (seq ± 1)
// della STESSA fonte, quando il match cade sul primo o sull'ultimo chunk
// della fonte.
//
// Una fonte è un game_media_id per un documento, ma per una FAQ
// game_media_id è NULL — e NULL per OGNI FAQ di OGNI gioco, non solo per
// quella di questa hit. Filtrare solo su "game_media_id IS NULL" farebbe
// combaciare seq con i chunk di FAQ di partite o giochi completamente
// diversi: un vicino preso in prestito da un'altra conversazione. Per
// questo, quando la hit non ha un media, il filtro aggiunge anche game_id e
// reference (l'URL della discussione) — la stessa coppia che Summary usa
// già come chiave per raggruppare le FAQ — che insieme a game_media_id IS
// NULL individuano di nuovo una fonte sola.
func (s *Store) attachNeighbours(ctx context.Context, gameID int64, hits []SourceHit) error {
	for i := range hits {
		h := &hits[i]

		var maxSeq int
		var err error
		if h.mediaID.Valid {
			err = s.db.QueryRowContext(ctx,
				`SELECT COALESCE(MAX(seq), 0) FROM game_source_chunk WHERE game_media_id = ?`,
				h.mediaID.Int64).Scan(&maxSeq)
		} else {
			err = s.db.QueryRowContext(ctx,
				`SELECT COALESCE(MAX(seq), 0) FROM game_source_chunk
				 WHERE game_id = ? AND game_media_id IS NULL AND reference = ?`,
				gameID, h.Reference).Scan(&maxSeq)
		}
		if err != nil {
			return fmt.Errorf("max seq: %w", err)
		}
		if maxSeq == 0 {
			continue // fonte di un chunk solo: non c'è nessun vicino
		}

		neighbour := -1
		after := false
		switch {
		case h.seq == 0:
			neighbour, after = 1, true
		case h.seq == maxSeq:
			neighbour, after = h.seq-1, false
		default:
			continue // non è un bordo: il chunk ha contesto da entrambi i lati
		}

		var text string
		if h.mediaID.Valid {
			err = s.db.QueryRowContext(ctx,
				`SELECT text FROM game_source_chunk WHERE game_media_id = ? AND seq = ?`,
				h.mediaID.Int64, neighbour).Scan(&text)
		} else {
			err = s.db.QueryRowContext(ctx,
				`SELECT text FROM game_source_chunk
				 WHERE game_id = ? AND game_media_id IS NULL AND reference = ? AND seq = ?`,
				gameID, h.Reference, neighbour).Scan(&text)
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("neighbour chunk: %w", err)
		}
		if after {
			h.Text = h.Text + " " + text
		} else {
			h.Text = text + " " + h.Text
		}
	}
	return nil
}

// searchPayload è la forma esatta del JSON che il modello legge come
// risultato del tool: un oggetto e non un array nudo, perché deve portare
// insieme alle hit anche le parole chiave senza risultato — la lista che
// dice al modello quali ipotesi lessicali sono cadute, così può riprovare
// con altre parole invece di concludere che il manuale non lo dice. Un
// array nudo di risultati non avrebbe un posto per quella lista senza un
// secondo valore di ritorno del tool, che i provider OpenAI-compatible non
// supportano (un tool restituisce un contenuto solo).
type searchPayload struct {
	Results []SourceHit `json:"risultati"`
	Missing []string    `json:"nessun_risultato_per,omitempty"`
}

// MarshalHits rende il payload JSON che il modello legge come risultato
// del tool di ricerca.
func MarshalHits(hits []SourceHit, missing []string) (string, error) {
	payload := searchPayload{Results: hits, Missing: missing}
	if payload.Results == nil {
		// Mai null: un array assente costringerebbe il modello a gestire
		// due forme diverse di "nessun risultato" (null contro []).
		payload.Results = []SourceHit{}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal search payload: %w", err)
	}
	return string(b), nil
}
