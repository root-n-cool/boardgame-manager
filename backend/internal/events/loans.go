package events

import (
	"context"
	"database/sql"
	"errors"
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

// ReturnLoan chiude un prestito. `notes` nil lascia quelle scritte alla
// consegna: la modale manda il campo solo se l'organizzatore l'ha
// toccato, e un nil non deve cancellare quello che c'era.
func (s *Store) ReturnLoan(ctx context.Context, id int64, notes *string) (Loan, error) {
	loan, err := s.getLoanByID(ctx, id)
	if err != nil {
		return Loan{}, err
	}
	if notes == nil {
		notes = loan.Notes
	}

	// `AND returned_at IS NULL` fa il lavoro del controllo: una seconda
	// restituzione non tocca nessuna riga, e lo sappiamo da RowsAffected
	// invece che da una lettura che potrebbe essere già vecchia.
	res, err := s.db.ExecContext(ctx,
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
	return s.getLoanByID(ctx, id)
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
