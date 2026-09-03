package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// testBookingCounter ensures unique booking_code and participant_phone values
// across repeated TestInsertBooking calls. Without it, multiple insertions with
// the same eventGameID and status would generate identical codes, violating:
// - booking_code UNIQUE constraint
// - idx_one_active_booking_per_phone_per_event partial unique index
// (The brief's original code used only eventGameID*10+len(status), which collides
// when called twice with identical parameters.)
var testBookingCounter int64

type Event struct {
	ID          int64
	Title       string
	Description *string
	EventDate   string
	StartTime   string
	ImagePath   *string
	// Venue è il luogo della serata, nil quando l'admin non l'ha indicato.
	Venue     *Venue
	CreatedAt time.Time
	// GamesCount is filled in by ListEvents only: the detail endpoints send
	// the games themselves, so recomputing it there would be dead weight.
	// It counts distinct games, not copies: two copies of Carcassonne are
	// one game in "N giochi" rendered for a human reading the line-up.
	GamesCount int
}

// Venue è dove si gioca. Address è l'unico campo che c'è sempre: Name è
// l'etichetta che l'admin dà al posto ("Circolo Arci") quando quel nome
// non sta già nell'indirizzo, e le coordinate ci sono solo se il luogo
// arriva dalla ricerca su OpenStreetMap invece che dalla tastiera.
type Venue struct {
	Name    string
	Address string
	Lat     *float64
	Lon     *float64
}

// EventGame è una singola copia di un gioco dentro un evento. Due copie
// dello stesso gioco sono due righe: chi prenota sa su quale sta finendo,
// e l'organizzatore sa chi siede a quale tavolo.
type EventGame struct {
	ID        int64
	EventID   int64
	GameID    int64
	CopyIndex int
	// Seats è la fotografia dei posti prenotabili del gioco al momento in
	// cui la copia è entrata nell'evento: cambiare il catalogo dopo non
	// muove la capienza di una serata già aperta alle prenotazioni.
	Seats int
}

// EventInput è una serata come l'admin la descrive: i suoi dati più i
// giochi che ci saranno. Sta in una struct perché i campi hanno superato
// il numero oltre il quale una lista di parametri posizionali di stringhe
// si scambia senza che il compilatore se ne accorga.
type EventInput struct {
	Title       string
	Description *string
	EventDate   string
	StartTime   string
	Venue       *Venue
	Games       []EventGameInput
}

// EventGameInput è come l'admin descrive un gioco: quante copie ne porta.
// I posti prenotabili non si scelgono qui, si leggono dal catalogo.
type EventGameInput struct {
	GameID int64
	Copies int
}

var (
	ErrNotFound                    = errors.New("not found")
	ErrGameNotFound                = errors.New("referenced game not found")
	ErrQuantityBelowActiveBookings = errors.New("quantity below active bookings")
)

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

// queryer is satisfied by both *sql.DB and *sql.Tx, so read helpers can run
// either against the pool or inside an in-flight transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// execer is satisfied by both *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) CreateEvent(ctx context.Context, in EventInput) (Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO events (title, description, event_date, start_time, venue_name, venue_address, venue_lat, venue_lon)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		append([]any{in.Title, in.Description, in.EventDate, in.StartTime}, venueColumns(in.Venue)...)...,
	)
	if err != nil {
		return Event{}, err
	}
	eventID, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}

	if err := insertEventGames(ctx, tx, eventID, in.Games); err != nil {
		return Event{}, err
	}

	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, eventID)
}

func insertEventGames(ctx context.Context, tx execQueryer, eventID int64, gamesInput []EventGameInput) error {
	for _, g := range gamesInput {
		seats, err := gameSeats(ctx, tx, g.GameID)
		if err != nil {
			return err
		}
		if err := insertCopies(ctx, tx, eventID, g.GameID, seats, 1, g.Copies); err != nil {
			return err
		}
	}
	return nil
}

// insertCopies scrive `count` copie consecutive a partire da firstIndex.
func insertCopies(ctx context.Context, tx execer, eventID, gameID int64, seats, firstIndex, count int) error {
	for i := 0; i < count; i++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO event_games (event_id, game_id, copy_index, seats) VALUES (?, ?, ?, ?)`,
			eventID, gameID, firstIndex+i, seats,
		); err != nil {
			return err
		}
	}
	return nil
}

// gameSeats fa doppio servizio: legge i posti prenotabili del gioco e, se
// il gioco non esiste, è il punto in cui l'input viene rifiutato.
func gameSeats(ctx context.Context, q queryer, gameID int64) (int, error) {
	var seats int
	err := q.QueryRowContext(ctx, `SELECT seats FROM games WHERE id = ?`, gameID).Scan(&seats)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrGameNotFound
	}
	return seats, err
}

// execQueryer is what insertEventGames needs; *sql.Tx satisfies it.
type execQueryer interface {
	execer
	queryer
}

func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	return getEvent(ctx, s.db, id)
}

func getEvent(ctx context.Context, q queryer, id int64) (Event, error) {
	var e Event
	var createdAt string
	var venue venueScan
	err := q.QueryRowContext(ctx,
		`SELECT id, title, description, event_date, start_time, image_path,
		        venue_name, venue_address, venue_lat, venue_lon, created_at
		 FROM events WHERE id = ?`, id,
	).Scan(&e.ID, &e.Title, &e.Description, &e.EventDate, &e.StartTime, &e.ImagePath,
		&venue.name, &venue.address, &venue.lat, &venue.lon, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	e.Venue = venue.venue()
	return e, nil
}

// venueColumns traduce il luogo nei quattro valori delle sue colonne:
// senza luogo sono quattro NULL, ed è così che l'admin lo cancella.
func venueColumns(v *Venue) []any {
	if v == nil {
		return []any{nil, nil, nil, nil}
	}
	var name any
	if v.Name != "" {
		name = v.Name
	}
	return []any{name, v.Address, v.Lat, v.Lon}
}

// venueScan raccoglie le quattro colonne come arrivano dal database.
type venueScan struct {
	name    sql.NullString
	address sql.NullString
	lat     sql.NullFloat64
	lon     sql.NullFloat64
}

// venue ricompone il luogo, o nil se l'evento non ne ha uno. È l'indirizzo
// a decidere: un luogo senza indirizzo non è un luogo.
func (v venueScan) venue() *Venue {
	if !v.address.Valid || v.address.String == "" {
		return nil
	}
	out := &Venue{Name: v.name.String, Address: v.address.String}
	if v.lat.Valid && v.lon.Valid {
		lat, lon := v.lat.Float64, v.lon.Float64
		out.Lat, out.Lon = &lat, &lon
	}
	return out
}

// ListEventsParams describes one page of the event list. The list is split in
// two around Now: the upcoming events, nearest first, and the past ones, most
// recent first. Limit 0 means "no limit" — the upcoming list is short enough
// to send whole, while the past one is paged.
type ListEventsParams struct {
	Past   bool
	Now    time.Time
	Limit  int
	Offset int
}

// ListEvents returns one page of events plus the total number of events on the
// same side of Now, so the caller can render a pager without a second query.
func (s *Store) ListEvents(ctx context.Context, p ListEventsParams) ([]Event, int, error) {
	comparison, order := ">=", "ASC"
	if p.Past {
		comparison, order = "<", "DESC"
	}
	cutoff := p.Now.Format("2006-01-02 15:04")

	var total int
	if err := s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM events WHERE event_date || ' ' || start_time %s ?`, comparison),
		cutoff,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT e.id, e.title, e.description, e.event_date, e.start_time, e.image_path,
				e.venue_name, e.venue_address, e.venue_lat, e.venue_lon, e.created_at,
			(SELECT COUNT(DISTINCT eg.game_id) FROM event_games eg WHERE eg.event_id = e.id)
		 FROM events e
		 WHERE e.event_date || ' ' || e.start_time %s ?
		 ORDER BY e.event_date %s, e.start_time %s, e.id %s`, comparison, order, order, order)
	args := []any{cutoff}
	if p.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, p.Limit, p.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var createdAt string
		var venue venueScan
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.EventDate, &e.StartTime, &e.ImagePath,
			&venue.name, &venue.address, &venue.lat, &venue.lon, &createdAt, &e.GamesCount); err != nil {
			return nil, 0, err
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		e.Venue = venue.venue()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) ListEventGames(ctx context.Context, eventID int64) ([]EventGame, error) {
	return listEventGames(ctx, s.db, eventID)
}

func listEventGames(ctx context.Context, q queryer, eventID int64) ([]EventGame, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, event_id, game_id, copy_index, seats FROM event_games
		 WHERE event_id = ? ORDER BY game_id, copy_index`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventGame
	for rows.Next() {
		var eg EventGame
		if err := rows.Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.CopyIndex, &eg.Seats); err != nil {
			return nil, err
		}
		out = append(out, eg)
	}
	return out, rows.Err()
}

func (s *Store) GetEventGame(ctx context.Context, id int64) (EventGame, error) {
	var eg EventGame
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, game_id, copy_index, seats FROM event_games WHERE id = ?`, id,
	).Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.CopyIndex, &eg.Seats)
	if errors.Is(err, sql.ErrNoRows) {
		return EventGame{}, ErrNotFound
	}
	return eg, err
}

// ActiveBookingCountsByEventGame returns, for every copy of the event, how
// many active bookings sit on it — one grouped query instead of the
// RemainingCapacity-per-copy loop the public event page used to run: an
// evening with 8 games × 2 copies went from ~9 queries to ~33 doing it that
// way. Missing from the map means zero, same as RemainingCapacity's own count.
func (s *Store) ActiveBookingCountsByEventGame(ctx context.Context, eventID int64) (map[int64]int, error) {
	return occupiedCopies(ctx, s.db, eventID)
}

func (s *Store) RemainingCapacity(ctx context.Context, eventGameID int64) (int, error) {
	var remaining int
	err := s.db.QueryRowContext(ctx,
		`SELECT eg.seats - (
			SELECT COUNT(*) FROM bookings b WHERE b.event_game_id = eg.id AND b.status = 'active'
		 ) FROM event_games eg WHERE eg.id = ?`, eventGameID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return remaining, err
}

func (s *Store) UpdateEvent(ctx context.Context, id int64, in EventInput) (Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE events SET title = ?, description = ?, event_date = ?, start_time = ?,
		        venue_name = ?, venue_address = ?, venue_lat = ?, venue_lon = ? WHERE id = ?`,
		append(append([]any{in.Title, in.Description, in.EventDate, in.StartTime}, venueColumns(in.Venue)...), id)...,
	)
	if err != nil {
		return Event{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Event{}, err
	}
	if affected == 0 {
		return Event{}, ErrNotFound
	}

	existing, err := listEventGames(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}
	// Le copie arrivano già ordinate per (game_id, copy_index): raggrupparle
	// per gioco conserva quell'ordine, che è quello in cui si sacrificano
	// dalla coda.
	copiesByGame := map[int64][]EventGame{}
	for _, eg := range existing {
		copiesByGame[eg.GameID] = append(copiesByGame[eg.GameID], eg)
	}

	occupied, err := occupiedCopies(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}

	// Un passaggio a parte per validare tutti i giochi richiesti e leggere
	// i posti prenotabili: se uno non esiste, si esce prima di scrivere.
	seatsByGame := map[int64]int{}
	wanted := map[int64]int{}
	for _, g := range in.Games {
		seats, err := gameSeats(ctx, tx, g.GameID)
		if err != nil {
			return Event{}, err
		}
		seatsByGame[g.GameID] = seats
		wanted[g.GameID] = g.Copies
	}

	// Giochi spariti dalla selezione: via tutte le loro copie, se libere.
	for gameID, copies := range copiesByGame {
		if _, stillWanted := wanted[gameID]; !stillWanted {
			if err := dropCopies(ctx, tx, copies, occupied, len(copies)); err != nil {
				return Event{}, err
			}
		}
	}

	for _, g := range in.Games {
		copies := copiesByGame[g.GameID]
		switch {
		case g.Copies < len(copies):
			if err := dropCopies(ctx, tx, copies, occupied, len(copies)-g.Copies); err != nil {
				return Event{}, err
			}
		case g.Copies > len(copies):
			// I numeri delle copie sono etichette stabili, non posizioni:
			// le nuove partono dopo la più alta esistente, anche se in
			// mezzo c'è un buco lasciato da una copia eliminata.
			next := 1
			if len(copies) > 0 {
				next = copies[len(copies)-1].CopyIndex + 1
			}
			if err := insertCopies(ctx, tx, id, g.GameID, seatsByGame[g.GameID], next, g.Copies-len(copies)); err != nil {
				return Event{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, id)
}

// dropCopies elimina `count` copie partendo dalla più alta, saltando quelle
// con prenotazioni attive. Se le copie libere non bastano l'operazione
// fallisce e la transazione del chiamante viene annullata: meglio un errore
// che una prenotazione cancellata a cascata sotto il naso di chi l'ha fatta.
func dropCopies(ctx context.Context, tx execer, copies []EventGame, occupied map[int64]int, count int) error {
	dropped := 0
	for i := len(copies) - 1; i >= 0 && dropped < count; i-- {
		if occupied[copies[i].ID] > 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_games WHERE id = ?`, copies[i].ID); err != nil {
			return err
		}
		dropped++
	}
	if dropped < count {
		return ErrQuantityBelowActiveBookings
	}
	return nil
}

// occupiedCopies conta le prenotazioni attive di ogni copia dell'evento.
func occupiedCopies(ctx context.Context, q queryer, eventID int64) (map[int64]int, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT eg.id, COUNT(b.id) FROM event_games eg
		 LEFT JOIN bookings b ON b.event_game_id = eg.id AND b.status = 'active'
		 WHERE eg.event_id = ? GROUP BY eg.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]int{}
	for rows.Next() {
		var eventGameID int64
		var count int
		if err := rows.Scan(&eventGameID, &count); err != nil {
			return nil, err
		}
		out[eventGameID] = count
	}
	return out, rows.Err()
}

// UpdateImagePath sets the optional event image. Uploading a new one simply
// replaces the path: there is no removal, the file itself is content-addressed
// and shared, so it is never deleted from disk here.
func (s *Store) UpdateImagePath(ctx context.Context, id int64, path string) (Event, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE events SET image_path = ? WHERE id = ?`, path, id)
	if err != nil {
		return Event{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Event{}, err
	}
	if affected == 0 {
		return Event{}, ErrNotFound
	}
	return s.GetEvent(ctx, id)
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// TestInsertBooking writes a booking row directly, bypassing all of
// CreateBooking's validation. It exists only so tests in this package (and
// the bookings/lookup/cancel tests added in later tasks) can set up booking
// fixtures without a circular dependency on CreateBooking's own tests.
func (s *Store) TestInsertBooking(eventID, eventGameID int64, status string) error {
	// Each call must generate a distinct booking_code and participant_phone.
	// The counter increment ensures uniqueness even when called multiple times with
	// identical eventGameID and status parameters. Without it, the formula
	// (eventGameID*10+len(status)) would collide, violating:
	// (a) booking_code UNIQUE constraint, and
	// (b) idx_one_active_booking_per_phone_per_event partial unique index.
	counter := atomic.AddInt64(&testBookingCounter, 1)
	code := fmt.Sprintf("TEST%04d%d", eventGameID*10+int64(len(status)), counter)
	phone := fmt.Sprintf("TEST%04d%d", eventGameID*10+int64(len(status)), counter)
	_, err := s.db.Exec(
		`INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		 VALUES (?, ?, 'Test Participant', 'test@example.com', ?, ?, ?)`,
		eventID, eventGameID, phone, code, status,
	)
	return err
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
