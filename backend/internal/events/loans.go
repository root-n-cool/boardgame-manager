package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Loan è una copia della serata data in mano a qualcuno. Non c'è una
// colonna di stato: lo stato è la data che manca, ReturnedAt nil
// significa "fuori". Un secondo campo sarebbe una verità duplicata che
// può divergere da questa.
type Loan struct {
	ID          int64
	EventGameID int64
	// BookingID c'è solo quando il prestito nasce da una prenotazione:
	// è l'unico modo di rispondere a "chi ha prenotato e non si è
	// presentato". Nome e telefono si copiano comunque sulla riga, così
	// il registro si legge anche se la prenotazione viene annullata.
	BookingID     *int64
	BorrowerName  string
	BorrowerPhone string
	Notes         *string
	LentAt        time.Time
	ReturnedAt    *time.Time
}

// LoanWithGame è un prestito con l'etichetta del gioco già risolta: il
// banco prestiti mostra righe, non fa join per conto suo.
type LoanWithGame struct {
	Loan
	GameID    int64
	GameName  string
	CopyIndex int
}

// LoanInput è un prestito come lo apre l'organizzatore.
type LoanInput struct {
	EventGameID   int64
	BookingID     *int64
	BorrowerName  string
	BorrowerPhone string
	Notes         *string
}

var (
	ErrCopyAlreadyOut      = errors.New("copy already on loan")
	ErrLoanAlreadyReturned = errors.New("loan already returned")
	ErrBorrowerRequired    = errors.New("borrower name and phone are required")
)

// loanColumns tiene allineate le tre query che leggono un prestito: una
// colonna aggiunta qui e dimenticata in uno Scan è un panic a runtime.
const loanColumns = `l.id, l.event_game_id, l.booking_id, l.borrower_name,
	l.borrower_phone, l.notes, l.lent_at, l.returned_at`

// scanner è soddisfatto sia da *sql.Row sia da *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanLoan(sc scanner, extra ...any) (Loan, error) {
	var l Loan
	var bookingID sql.NullInt64
	var notes, returnedAt sql.NullString
	var lentAt string
	dest := append([]any{
		&l.ID, &l.EventGameID, &bookingID, &l.BorrowerName,
		&l.BorrowerPhone, &notes, &lentAt, &returnedAt,
	}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return Loan{}, err
	}
	if bookingID.Valid {
		id := bookingID.Int64
		l.BookingID = &id
	}
	// Una nota svuotata nel form arriva come stringa vuota: per chi legge
	// è la stessa cosa che non averne, e nil lo dice senza ambiguità.
	if notes.Valid && notes.String != "" {
		text := notes.String
		l.Notes = &text
	}
	// Le date le scrive SQLite con datetime('now'), che è UTC.
	l.LentAt, _ = time.Parse("2006-01-02 15:04:05", lentAt)
	if returnedAt.Valid {
		at, _ := time.Parse("2006-01-02 15:04:05", returnedAt.String)
		l.ReturnedAt = &at
	}
	return l, nil
}

// LendCopy consegna una copia della serata. La copia deve appartenere
// all'evento indicato, e la prenotazione — quando c'è — deve stare sulla
// stessa copia.
//
// Prestare una copia che ha prenotazioni attive a un nome che non è fra
// i prenotati è permesso: alle 21:30 chi non si è presentato non deve
// tenere in ostaggio la scatola. L'avviso lo dà la UI.
func (s *Store) LendCopy(ctx context.Context, eventID int64, in LoanInput) (Loan, error) {
	name := strings.TrimSpace(in.BorrowerName)
	phone := strings.TrimSpace(in.BorrowerPhone)
	if name == "" || phone == "" {
		return Loan{}, ErrBorrowerRequired
	}

	eventGame, err := s.GetEventGame(ctx, in.EventGameID)
	if err != nil {
		return Loan{}, err
	}
	if eventGame.EventID != eventID {
		return Loan{}, ErrNotFound
	}

	if in.BookingID != nil {
		var count int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM bookings
			 WHERE id = ? AND event_game_id = ? AND status = ?`,
			*in.BookingID, in.EventGameID, BookingStatusActive,
		).Scan(&count); err != nil {
			return Loan{}, err
		}
		if count == 0 {
			return Loan{}, ErrNotFound
		}
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_loans (event_game_id, booking_id, borrower_name, borrower_phone, notes)
		 VALUES (?, ?, ?, ?, ?)`,
		in.EventGameID, in.BookingID, name, phone, in.Notes,
	)
	if err != nil {
		// idx_one_open_loan_per_copy è l'unico vincolo UNIQUE su questa
		// tabella: una collisione significa che la copia è già fuori. Il
		// controllo sta nell'indice e non in una lettura preventiva, così
		// due consegne simultanee non possono passare entrambe.
		if isUniqueConstraintErr(err) {
			return Loan{}, ErrCopyAlreadyOut
		}
		return Loan{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Loan{}, err
	}
	return s.getLoanByID(ctx, id)
}

// MaterialCheck è una voce della checklist come la manda la modale di
// riconsegna.
type MaterialCheck struct {
	MaterialID int64
	// Complete è la spunta: è tornata tutta, e Returned si ignora.
	Complete bool
	// Returned ha senso solo con Complete == false: nil significa "non
	// verificata", un numero quante ne sono tornate.
	Returned *int
}

// MaterialIssue è l'esito registrato su una voce che non è tornata intera o
// non è stata controllata. Nome e quantità attesa sono copie del momento
// della riconsegna, non join sul catalogo: la lista del gioco può cambiare,
// il registro di quella sera no.
type MaterialIssue struct {
	LoanID   int64
	Name     string
	Expected int
	// Returned nil = voce non verificata.
	Returned *int
}

// ErrMaterialCheckInvalid è una quantità resa negativa. Non è un caso da
// tollerare in silenzio: significa che la modale ha mandato spazzatura, e
// chiudere il prestito con un esito sbagliato è peggio che non chiuderlo.
var ErrMaterialCheckInvalid = errors.New("returned quantity cannot be negative")

// ReturnLoan chiude un prestito e registra l'esito del controllo dei
// materiali. `notes` nil lascia quelle scritte alla consegna: la modale
// manda il campo solo se l'organizzatore l'ha toccato, e un nil non deve
// cancellare quello che c'era.
//
// `checks` nil significa "nessuna checklist" e lascia il comportamento
// identico a prima: un gioco senza materiali, o un client vecchio, chiude un
// prestito senza scrivere nessun esito. Non è la stessa cosa di una
// checklist con tutte le voci non verificate, che invece le registra.
//
// Tutto in una transazione: prima l'UPDATE faceva una riga sola, ora sono
// una UPDATE più N INSERT, e un prestito chiuso senza il suo esito sarebbe
// peggio di un prestito rimasto aperto.
func (s *Store) ReturnLoan(ctx context.Context, id int64, notes *string, checks []MaterialCheck) (Loan, error) {
	loan, err := s.getLoanByID(ctx, id)
	if err != nil {
		return Loan{}, err
	}
	if notes == nil {
		notes = loan.Notes
	}
	for _, c := range checks {
		if c.Returned != nil && *c.Returned < 0 {
			return Loan{}, fmt.Errorf("%w: %d", ErrMaterialCheckInvalid, *c.Returned)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Loan{}, err
	}
	defer tx.Rollback()

	// `AND returned_at IS NULL` fa il lavoro del controllo: una seconda
	// restituzione non tocca nessuna riga, e lo sappiamo da RowsAffected
	// invece che da una lettura che potrebbe essere già vecchia.
	res, err := tx.ExecContext(ctx,
		`UPDATE game_loans SET returned_at = datetime('now'), notes = ?
		 WHERE id = ? AND returned_at IS NULL`, notes, id,
	)
	if err != nil {
		return Loan{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Loan{}, err
	}
	if affected == 0 {
		return Loan{}, ErrLoanAlreadyReturned
	}

	if checks != nil {
		if err := writeMaterialIssues(ctx, tx, id, checks); err != nil {
			return Loan{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Loan{}, err
	}
	return s.getLoanByID(ctx, id)
}

// writeMaterialIssues scrive una riga per ogni voce che non è tornata
// intera o non è stata verificata. Le voci le rilegge dal catalogo dentro
// la transazione invece di fidarsi di quelle mandate dal client: la modale
// può essere aperta da dieci minuti e la lista essere cambiata nel
// frattempo, e ciò che conta è il contenuto della scatola di adesso.
func writeMaterialIssues(ctx context.Context, tx *sql.Tx, loanID int64, checks []MaterialCheck) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT m.id, m.name, m.quantity
		 FROM game_material m
		 JOIN event_games eg ON eg.game_id = m.game_id
		 JOIN game_loans l ON l.event_game_id = eg.id
		 WHERE l.id = ?
		 ORDER BY m.position, m.id`, loanID)
	if err != nil {
		return err
	}
	type material struct {
		id       int64
		name     string
		quantity int
	}
	var materials []material
	for rows.Next() {
		var m material
		if err := rows.Scan(&m.id, &m.name, &m.quantity); err != nil {
			rows.Close()
			return err
		}
		materials = append(materials, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	byID := make(map[int64]MaterialCheck, len(checks))
	for _, c := range checks {
		byID[c.MaterialID] = c
	}

	for _, m := range materials {
		c, sent := byID[m.id]
		switch {
		case sent && c.Complete:
			continue // spuntata: tornata tutta
		case sent && c.Returned != nil && *c.Returned >= m.quantity:
			continue // contata e non manca niente
		}
		var returned any
		if sent && !c.Complete && c.Returned != nil {
			returned = *c.Returned
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO loan_material_issue (loan_id, material_id, name, expected, returned)
			 VALUES (?, ?, ?, ?, ?)`,
			loanID, m.id, m.name, m.quantity, returned,
		); err != nil {
			return err
		}
	}
	return nil
}

// ListMaterialIssues legge gli esiti di più prestiti in una query sola: il
// banco prestiti ne mostra una lista intera, e una query per riga sarebbe
// una N+1 su una pagina che si ricarica dopo ogni consegna.
func (s *Store) ListMaterialIssues(ctx context.Context, loanIDs []int64) (map[int64][]MaterialIssue, error) {
	out := map[int64][]MaterialIssue{}
	if len(loanIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(loanIDs)), ",")
	args := make([]any, len(loanIDs))
	for i, id := range loanIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT loan_id, name, expected, returned FROM loan_material_issue
		 WHERE loan_id IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var iss MaterialIssue
		var returned sql.NullInt64
		if err := rows.Scan(&iss.LoanID, &iss.Name, &iss.Expected, &returned); err != nil {
			return nil, err
		}
		if returned.Valid {
			v := int(returned.Int64)
			iss.Returned = &v
		}
		out[iss.LoanID] = append(out[iss.LoanID], iss)
	}
	return out, rows.Err()
}

func (s *Store) getLoanByID(ctx context.Context, id int64) (Loan, error) {
	loan, err := scanLoan(s.db.QueryRowContext(ctx,
		`SELECT `+loanColumns+` FROM game_loans l WHERE l.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Loan{}, ErrNotFound
	}
	return loan, err
}

// ListLoansForEvent è tutto il registro della serata, aperti e chiusi
// insieme, dal più recente. Chi chiama separa i due gruppi guardando
// ReturnedAt: una query invece di due, e nessun rischio che le due
// risposte arrivino da istanti diversi.
func (s *Store) ListLoansForEvent(ctx context.Context, eventID int64) ([]LoanWithGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+loanColumns+`, g.id, g.name, eg.copy_index
		 FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 WHERE eg.event_id = ?
		 ORDER BY l.lent_at DESC, l.id DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []LoanWithGame{}
	for rows.Next() {
		var lg LoanWithGame
		loan, err := scanLoan(rows, &lg.GameID, &lg.GameName, &lg.CopyIndex)
		if err != nil {
			return nil, err
		}
		lg.Loan = loan
		out = append(out, lg)
	}
	return out, rows.Err()
}

// openLoanCopies dice quali copie dell'evento sono fuori adesso. Serve
// alla modifica evento, che non deve poter togliere una copia che
// qualcuno ha in mano.
func openLoanCopies(ctx context.Context, q queryer, eventID int64) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT l.event_game_id FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 WHERE eg.event_id = ? AND l.returned_at IS NULL`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]bool{}
	for rows.Next() {
		var eventGameID int64
		if err := rows.Scan(&eventGameID); err != nil {
			return nil, err
		}
		out[eventGameID] = true
	}
	return out, rows.Err()
}

// loanHistoryCopies dice quali copie dell'evento hanno almeno una riga in
// game_loans, aperta o chiusa. Serve a dropCopies per preferire di
// eliminare le copie senza storico: cancellare una copia con storico se
// lo porta via a cascata (game_loans.event_game_id è ON DELETE CASCADE),
// quindi conviene spendere per prima quella che non perde niente.
func loanHistoryCopies(ctx context.Context, q queryer, eventID int64) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT l.event_game_id FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 WHERE eg.event_id = ?`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]bool{}
	for rows.Next() {
		var eventGameID int64
		if err := rows.Scan(&eventGameID); err != nil {
			return nil, err
		}
		out[eventGameID] = true
	}
	return out, rows.Err()
}

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

// GamesMissingPieces torna, per un gruppo di giochi, le voci che risultano
// mancanti e non ancora risolte. Un gioco assente dalla mappa è completo.
//
// Lo stato "incompleto" non è memorizzato da nessuna parte: è questa query.
// Un flag su games sarebbe una seconda verità accanto a
// loan_material_issue, e le due possono divergere — un flag acceso dopo che
// la riga d'esito è sparita con la cancellazione di un prestito, o spento
// perché un percorso di scrittura si è dimenticato di alzarlo.
//
// Una query sola con una IN e non una per gioco: la chiama l'elenco del
// catalogo con tutti i giochi in archivio.
func (s *Store) GamesMissingPieces(ctx context.Context, gameIDs []int64) (map[int64][]MissingPiece, error) {
	out := map[int64][]MissingPiece{}
	if len(gameIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(gameIDs))
	for i, id := range gameIDs {
		args[i] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(gameIDs)), ",")

	// MAX(l.returned_at) con le altre colonne nude è la forma idiomatica di
	// SQLite per "la riga del massimo" (sqlite.org/lang_select.html#bareagg):
	// con un solo aggregato MIN/MAX le colonne nude vengono dalla riga
	// scelta. Serve perché la stessa voce può essere risultata corta in tre
	// serate e la scheda deve dire l'ultimo conteggio, non tre righe.
	//
	// `lmi.returned IS NOT NULL` è la regola centrale della funzione: una
	// voce contata e mancante segnala, una "non verificata" no.
	rows, err := s.db.QueryContext(ctx,
		`SELECT eg.game_id, lmi.name, lmi.expected, lmi.returned, MAX(l.returned_at)
		 FROM loan_material_issue lmi
		 JOIN game_loans l   ON l.id = lmi.loan_id
		 JOIN event_games eg ON eg.id = l.event_game_id
		 JOIN games g        ON g.id = eg.game_id
		 WHERE eg.game_id IN (`+placeholders+`)
		   AND lmi.returned IS NOT NULL
		   AND l.returned_at IS NOT NULL
		   AND (g.materials_checked_at IS NULL OR l.returned_at > g.materials_checked_at)
		 GROUP BY eg.game_id, lmi.name
		 ORDER BY eg.game_id, lmi.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var gameID int64
		var p MissingPiece
		var since string
		if err := rows.Scan(&gameID, &p.Name, &p.Expected, &p.Returned, &since); err != nil {
			return nil, err
		}
		p.Since, _ = time.Parse("2006-01-02 15:04:05", since)
		out[gameID] = append(out[gameID], p)
	}
	return out, rows.Err()
}

// LoanWithEvent è un prestito come lo mostra il log di un gioco: la copia e
// la serata risolte, perché quel log attraversa tutte le serate e una riga
// deve leggersi da sola.
type LoanWithEvent struct {
	Loan
	EventID    int64
	EventTitle string
	EventDate  string
	CopyIndex  int
	// Copies è quante copie di questo gioco aveva quella serata: con una
	// sola, "#1" è rumore e la UI lo nasconde.
	Copies int
}

// ListLoansForGame è tutto il registro di un gioco, aperti e chiusi
// insieme, dal più recente. Chi chiama separa i due gruppi guardando
// ReturnedAt, come già fa il banco prestiti: una query invece di due, e
// nessun rischio che le due risposte arrivino da istanti diversi.
func (s *Store) ListLoansForGame(ctx context.Context, gameID int64) ([]LoanWithEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+loanColumns+`, e.id, e.title, e.event_date, eg.copy_index,
		        (SELECT COUNT(*) FROM event_games x
		          WHERE x.event_id = eg.event_id AND x.game_id = eg.game_id)
		 FROM game_loans l
		 JOIN event_games eg ON l.event_game_id = eg.id
		 JOIN events e       ON e.id = eg.event_id
		 WHERE eg.game_id = ?
		 ORDER BY l.lent_at DESC, l.id DESC`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []LoanWithEvent{}
	for rows.Next() {
		var le LoanWithEvent
		loan, err := scanLoan(rows, &le.EventID, &le.EventTitle, &le.EventDate,
			&le.CopyIndex, &le.Copies)
		if err != nil {
			return nil, err
		}
		le.Loan = loan
		out = append(out, le)
	}
	return out, rows.Err()
}
