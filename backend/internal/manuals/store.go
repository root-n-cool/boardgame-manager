package manuals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// hitsPerKeyword è quanti chunk risalgono per ogni parola chiave. Due:
// abbastanza perché una parola chiave conti qualcosa nel payload, poco
// abbastanza perché otto parole chiave non producano un muro di testo.
const hitsPerKeyword = 2

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

// StoredPage è una pagina di manuale come sta nel database: il testo, il
// titolo di sezione rilevato e da dove viene.
type StoredPage struct {
	PageNumber int
	Text       string
	Heading    string
	Source     string
}

// Hit è un chunk trovato dalla ricerca. FoundWith dice con quali parole
// chiave è emerso: è informazione che il modello usa, non decorazione.
type Hit struct {
	PageNumber   int
	LanguageCode string
	ManualTitle  string
	Text         string
	FoundWith    []string

	// Non esportati: servono solo a trovare il chunk adiacente, e non
	// hanno senso per chi legge il risultato.
	seq     int
	mediaID int64
}

// SearchResult tiene insieme quel che si è trovato e quel che non si è
// trovato. Missing non è un errore: è ciò che dice al modello quali
// ipotesi lessicali sono cadute, così può riprovare con altre parole.
type SearchResult struct {
	Hits    []Hit
	Missing []string
}

type ManualText struct {
	Title        string
	LanguageCode string
	// Path è il nome del file su disco (game_media.url_or_path): serve a
	// trasformare "pag. 7" in un link al PDF a quella pagina.
	Path  string
	Pages []StoredPage
}

// Corpus è tutto il testo dei manuali di un gioco. Chars è la misura con
// cui si decide se il manuale entra intero nel contesto del modello,
// rendendo superfluo il tool.
type Corpus struct {
	Chars   int
	Manuals []ManualText
}

// ReplacePages sostituisce le pagine di un manuale e ricostruisce i chunk,
// tutto in una transazione: non esiste uno stato intermedio in cui le
// pagine sono nuove e l'indice è vecchio.
func (s *Store) ReplacePages(ctx context.Context, gameID, mediaID int64, languageCode string, pages []StoredPage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_page WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("clear pages: %w", err)
	}
	// I chunk si cancellano con una DELETE e non con la cascata, perché le
	// pagine e i chunk sono legati al media ma i chunk vanno ricostruiti
	// anche quando le pagine non cambiano (parametri di chunking diversi).
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_chunk WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("clear chunks: %w", err)
	}

	plain := make([]Page, 0, len(pages))
	for _, p := range pages {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			// Una pagina vuota si salva comunque: l'admin deve vedere il
			// buco nell'anteprima e poterlo riempire. Semplicemente non
			// produce chunk.
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO manual_page (game_media_id, page_number, text, heading, source)
				 VALUES (?, ?, '', NULL, ?)`, mediaID, p.PageNumber, p.Source); err != nil {
				return fmt.Errorf("insert empty page %d: %w", p.PageNumber, err)
			}
			continue
		}
		heading := p.Heading
		if heading == "" {
			heading = DetectHeading(text)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO manual_page (game_media_id, page_number, text, heading, source)
			 VALUES (?, ?, ?, ?, ?)`,
			mediaID, p.PageNumber, text, nullIfEmpty(heading), p.Source); err != nil {
			return fmt.Errorf("insert page %d: %w", p.PageNumber, err)
		}
		plain = append(plain, Page{Number: p.PageNumber, Text: text})
	}

	for _, c := range Chunk(plain) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO manual_chunk (game_id, game_media_id, language_code, page_number, seq, text)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			gameID, mediaID, languageCode, c.PageNumber, c.Seq, c.Text); err != nil {
			return fmt.Errorf("insert chunk p%d s%d: %w", c.PageNumber, c.Seq, err)
		}
	}

	return tx.Commit()
}

func (s *Store) ListPages(ctx context.Context, mediaID int64) ([]StoredPage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT page_number, text, COALESCE(heading, ''), source
		 FROM manual_page WHERE game_media_id = ? ORDER BY page_number`, mediaID)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer rows.Close()
	var out []StoredPage
	for rows.Next() {
		var p StoredPage
		if err := rows.Scan(&p.PageNumber, &p.Text, &p.Heading, &p.Source); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeletePages(ctx context.Context, mediaID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_chunk WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_page WHERE game_media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("delete pages: %w", err)
	}
	return tx.Commit()
}

// HasPages è la condizione, insieme al provider configurato, che governa la
// comparsa della chat in UI. Una COUNT invece di Corpus: qui interessa solo
// il sì o no, non il testo.
func (s *Store) HasPages(ctx context.Context, gameID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM manual_page p
		 JOIN game_media m ON m.id = p.game_media_id
		 JOIN game_languages l ON l.id = m.game_language_id
		 WHERE l.game_id = ? AND TRIM(p.text) <> ''`, gameID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("has pages: %w", err)
	}
	return n > 0, nil
}

func (s *Store) Corpus(ctx context.Context, gameID int64) (Corpus, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, COALESCE(m.title, 'Manuale'), m.url_or_path, l.language_code,
		        p.page_number, p.text, COALESCE(p.heading, ''), p.source
		 FROM manual_page p
		 JOIN game_media m ON m.id = p.game_media_id
		 JOIN game_languages l ON l.id = m.game_language_id
		 WHERE l.game_id = ? AND TRIM(p.text) <> ''
		 ORDER BY m.id, p.page_number`, gameID)
	if err != nil {
		return Corpus{}, fmt.Errorf("corpus: %w", err)
	}
	defer rows.Close()

	// Raggruppato per game_media_id, non per (title, language_code): il
	// titolo è un campo libero e opzionale (COALESCE a 'Manuale' quando è
	// NULL), quindi due media distinti nella stessa lingua senza titolo
	// collasserebbero sulla stessa chiave e il secondo Path sparirebbe. Path
	// alimenta il link "pag. 7" del Task 9: un Path sbagliato manda a un
	// file diverso da quello dove quella pagina vive davvero.
	var out Corpus
	index := map[int64]int{}
	for rows.Next() {
		var mediaID int64
		var title, path, lang string
		var p StoredPage
		if err := rows.Scan(&mediaID, &title, &path, &lang, &p.PageNumber, &p.Text, &p.Heading, &p.Source); err != nil {
			return Corpus{}, err
		}
		out.Chars += len(p.Text)
		i, ok := index[mediaID]
		if !ok {
			out.Manuals = append(out.Manuals, ManualText{Title: title, Path: path, LanguageCode: lang})
			i = len(out.Manuals) - 1
			index[mediaID] = i
		}
		out.Manuals[i].Pages = append(out.Manuals[i].Pages, p)
	}
	return out, rows.Err()
}

// Search esegue una query FTS5 per ogni parola chiave e unisce i risultati.
// Una query per parola e non un unico OR: con l'OR una parola comune
// sommerge una rara, mentre così ogni variante ha i suoi due posti
// garantiti — ed è quello che rende utile passare i sinonimi tutti insieme.
func (s *Store) Search(ctx context.Context, gameID int64, preferLang string, keywords []string) (SearchResult, error) {
	var res SearchResult
	order := []int64{}
	byID := map[int64]*Hit{}

	for _, raw := range keywords {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		hits, err := s.searchOne(ctx, gameID, preferLang, kw)
		if err != nil {
			return SearchResult{}, err
		}
		if len(hits) == 0 {
			res.Missing = append(res.Missing, kw)
			continue
		}
		for id, h := range hits {
			if existing, ok := byID[id]; ok {
				existing.FoundWith = append(existing.FoundWith, kw)
				continue
			}
			h.FoundWith = []string{kw}
			copied := h
			byID[id] = &copied
			order = append(order, id)
		}
	}

	for _, id := range order {
		res.Hits = append(res.Hits, *byID[id])
	}
	if err := s.attachNeighbours(ctx, res.Hits); err != nil {
		return SearchResult{}, err
	}
	return res, nil
}

// searchOne cerca una sola parola chiave. Restituisce una mappa id -> Hit
// perché il chiamante deduplica sull'id del chunk.
//
// L'ordinamento per lingua preferita avviene in Go, non in SQL, e senza
// LIMIT nella query: si leggono tutte le righe ordinate per `rank`
// (rilevanza BM25), poi uno stable sort le riordina mettendo prima la
// lingua preferita — preservando l'ordine di rank dentro ciascun gruppo —
// e solo allora si taglia a hitsPerKeyword. È equivalente a fare tutto in
// una query con `ORDER BY (c.language_code = ?) DESC, rank LIMIT ?` (che
// SQLite accetta: non impedisce di mescolare `rank` con un'espressione
// calcolata), ma non dipende da quel comportamento specifico del motore
// FTS5, ed è più facile da leggere e da testare in isolamento.
func (s *Store) searchOne(ctx context.Context, gameID int64, preferLang, keyword string) (map[int64]Hit, error) {
	// Nessun LIMIT qui: il taglio a hitsPerKeyword avviene in Go, dopo il
	// riordino per lingua preferita. Un LIMIT in SQL prima di quel riordino
	// potrebbe scartare una riga nella lingua preferita che il motore FTS5
	// classifica oltre la finestra, prima ancora che il riordino per lingua
	// abbia la possibilità di farla emergere. Un manuale ha al più poche
	// centinaia di chunk per gioco, quindi leggerli tutti per parola chiave
	// non è un problema di scala.
	query := func(match string) (*sql.Rows, error) {
		return s.db.QueryContext(ctx,
			`SELECT c.id, c.page_number, c.language_code, COALESCE(m.title, 'Manuale'), c.text, c.seq, c.game_media_id
			 FROM manual_chunk_fts f
			 JOIN manual_chunk c ON c.id = f.rowid
			 JOIN game_media m ON m.id = c.game_media_id
			 WHERE manual_chunk_fts MATCH ? AND c.game_id = ?
			 ORDER BY rank`, match, gameID)
	}

	rows, err := query(keyword)
	if err != nil {
		// La sintassi FTS5 si rompe su un apostrofo o una virgoletta. Gli
		// operatori però sono utili (pesc*, "frase esatta"), quindi non si
		// filtrano a monte: si riprova con la parola neutralizzata solo
		// quando la prima forma è illegale.
		//
		// Il retry scatta su *qualunque* errore della prima query, non solo
		// su un errore di sintassi FTS5 — inclusi un errore di contesto
		// (timeout, richiesta annullata) o un problema di connessione. Ho
		// considerato di restringerlo controllando il testo dell'errore
		// (driver modernc.org/sqlite riporta "fts5: syntax error" per il
		// caso che interessa), ma è un abbinamento su una stringa che il
		// driver non promette di stabilizzare fra versioni: più fragile del
		// problema che risolverebbe. Il costo del catch largo è concreto ma
		// piccolo — una seconda query quasi identica — e nel caso di un
		// contesto già annullato la seconda query fallisce a sua volta
		// (rientra comunque nell'errore restituito sotto), quindi non si
		// rischia un risultato silenzioso sbagliato: nel peggiore dei casi
		// si perde il messaggio d'errore originale a favore di quello della
		// query di retry.
		rows, err = query(escapeFTS(keyword))
		if err != nil {
			return nil, fmt.Errorf("fts search %q: %w", keyword, err)
		}
	}
	defer rows.Close()

	var all []scannedHit
	for rows.Next() {
		var id int64
		var h Hit
		if err := rows.Scan(&id, &h.PageNumber, &h.LanguageCode, &h.ManualTitle, &h.Text, &h.seq, &h.mediaID); err != nil {
			return nil, err
		}
		all = append(all, scannedHit{id, h})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Stable sort: preserva l'ordine di rilevanza (rank) letto dalla query
	// dentro ciascun gruppo, mette solo il gruppo della lingua preferita
	// davanti all'altro.
	sort.SliceStable(all, func(i, j int) bool {
		iPref := all[i].h.LanguageCode == preferLang
		jPref := all[j].h.LanguageCode == preferLang
		return iPref && !jPref
	})

	out := map[int64]Hit{}
	for i, row := range all {
		if i >= hitsPerKeyword {
			break
		}
		out[row.id] = row.h
	}
	return out, nil
}

// scannedHit è una riga letta da searchOne prima della deduplicazione e
// dell'ordinamento per lingua preferita.
type scannedHit struct {
	id int64
	h  Hit
}

// escapeFTS trasforma una parola chiave in una stringa FTS5 sempre legale:
// racchiusa in doppi apici, con i doppi apici interni raddoppiati. Perde gli
// operatori, che è esattamente il compromesso voluto — meglio una ricerca
// letterale che un errore.
func escapeFTS(kw string) string {
	return `"` + strings.ReplaceAll(kw, `"`, `""`) + `"`
}

// attachNeighbours allega al testo di un hit il chunk adiacente della
// stessa pagina, quando il match cade sul primo o sull'ultimo chunk.
//
// La sovrapposizione di 100 caratteri del chunking copre la frase spezzata;
// questo copre il caso diverso in cui la regola *continua* per un altro
// paragrafo. Costa ~100 token e toglie in radice la seconda chiamata al
// tool del tipo "fammi leggere il resto".
func (s *Store) attachNeighbours(ctx context.Context, hits []Hit) error {
	for i := range hits {
		h := &hits[i]
		var maxSeq int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(seq), 0) FROM manual_chunk
			 WHERE game_media_id = ? AND page_number = ?`,
			h.mediaID, h.PageNumber).Scan(&maxSeq); err != nil {
			return fmt.Errorf("max seq: %w", err)
		}
		if maxSeq == 0 {
			continue // pagina di un chunk solo: non c'è nessun vicino
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
		err := s.db.QueryRowContext(ctx,
			`SELECT text FROM manual_chunk
			 WHERE game_media_id = ? AND page_number = ? AND seq = ?`,
			h.mediaID, h.PageNumber, neighbour).Scan(&text)
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

// FormatSearchResult rende il payload che il modello legge come risultato
// del tool. Testo semplice e non JSON: un LLM lo legge meglio, e costa meno
// token.
func FormatSearchResult(r SearchResult) string {
	var b strings.Builder
	for i, h := range r.Hits {
		fmt.Fprintf(&b, "[%d] %s (%s), pag. %d  ·  trovato con: %s\n%s\n\n",
			i+1, h.ManualTitle, h.LanguageCode, h.PageNumber,
			strings.Join(h.FoundWith, ", "), h.Text)
	}
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "Nessun risultato per: %s\n", strings.Join(r.Missing, ", "))
	}
	out := strings.TrimSpace(b.String())
	if len(r.Hits) == 0 {
		return "Nessun risultato per nessuna parola chiave. " +
			"Riprova con altri termini, oppure di' che il manuale non lo dice.\n" + out
	}
	return out
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// FormatIndex rende l'indice del manuale per il prompt: solo i titoli di
// sezione con la loro pagina, ~200 token. È quel che evita al modello la
// chiamata esplorativa al tool — uno che vede l'indice scrive le parole
// chiave giuste al primo colpo, uno cieco tira a indovinare e richiama.
func FormatIndex(c Corpus) string {
	var parts []string
	for _, m := range c.Manuals {
		for _, p := range m.Pages {
			if strings.TrimSpace(p.Heading) == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s p.%d", p.Heading, p.PageNumber))
		}
	}
	return strings.Join(parts, " · ")
}

// FormatCorpus rende tutto il testo dei manuali, con i marcatori di pagina
// che permettono al modello di citare. Serve al caso sotto soglia, dove il
// manuale entra intero nel contesto e non c'è nessun tool da chiamare.
func FormatCorpus(c Corpus) string {
	var b strings.Builder
	for _, m := range c.Manuals {
		fmt.Fprintf(&b, "=== %s (%s) ===\n\n", m.Title, m.LanguageCode)
		for _, p := range m.Pages {
			fmt.Fprintf(&b, "--- pag. %d ---\n%s\n\n", p.PageNumber, p.Text)
		}
	}
	return strings.TrimSpace(b.String())
}
