package manuals

import (
	"context"
	"database/sql"
	"fmt"
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

// maxSuggestionHeadings è quanti titoli di sezione escono verso la scheda
// pubblica. La chat ne usa tre per costruire le domande suggerite: un
// manuale di quaranta pagine non ha nessun motivo di mandarne quaranta al
// telefono di chi sta al tavolo. Se ne mandano qualcuno in più di tre
// perché il pannello scarta i doppioni (due sezioni con lo stesso titolo, o
// due titoli che la sua tabella mappa sulla stessa domanda) e con
// esattamente tre ripiegherebbe sulle domande fisse al primo doppione.
const maxSuggestionHeadings = 8

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
// configurato, la comparsa della chat), i titoli distinti per la scheda
// pubblica, lo stesso raggruppato per fonte per l'indice nel prompt, e
// quanti chunk ha ciascun media per il pannello admin.
type SourceSummary struct {
	HasChunks bool
	Headings  []string         // distinti, in ordine di seq — per il JSON del frontend
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
		if !seenHeadings[key] && len(out.Headings) < maxSuggestionHeadings {
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
