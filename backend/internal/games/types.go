package games

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// GameType è una tipologia di gioco (GDT, GDR, MTG...). La gestisce
// l'admin: nome, sigla, se la creazione di un gioco passa dalla ricerca
// BGG, e la posizione che decide l'ordine delle tab.
type GameType struct {
	ID        int64
	Name      string
	Slug      string
	BGGSearch bool
	Position  int
	// GameCount è quanti giochi del catalogo hanno questa tipologia.
	GameCount int
}

type GameTypeInput struct {
	Name      string
	Slug      string
	BGGSearch bool
}

type GameTypeUpdate struct {
	Name      *string
	Slug      *string
	BGGSearch *bool
}

var (
	ErrGameTypeName = errors.New("il nome della tipologia è obbligatorio")
	ErrGameTypeSlug = errors.New("la sigla deve avere da 2 a 6 lettere o cifre")
	ErrSlugTaken    = errors.New("sigla già usata da un'altra tipologia")
	// ErrLastGameType: senza tipologie nessun gioco si potrebbe più
	// creare, perché ogni gioco ne ha una.
	ErrLastGameType = errors.New("serve almeno una tipologia: questa è l'ultima")
)

// GameTypeInUseError blocca l'eliminazione di una tipologia che ha
// ancora giochi: prima si spostano, poi si elimina.
type GameTypeInUseError struct{ Count int }

func (e *GameTypeInUseError) Error() string {
	return fmt.Sprintf("la tipologia ha ancora %d giochi", e.Count)
}

var slugPattern = regexp.MustCompile(`^[A-Z0-9]{2,6}$`)

func normalizeGameType(name, slug string) (string, string, error) {
	name = strings.TrimSpace(name)
	slug = strings.ToUpper(strings.TrimSpace(slug))
	if name == "" {
		return "", "", ErrGameTypeName
	}
	if !slugPattern.MatchString(slug) {
		return "", "", ErrGameTypeSlug
	}
	return name, slug, nil
}

const gameTypeColumns = `t.id, t.name, t.slug, t.bgg_search, t.position,
	(SELECT COUNT(*) FROM games g WHERE g.game_type_id = t.id)`

func scanGameType(row interface{ Scan(...any) error }) (GameType, error) {
	var gt GameType
	err := row.Scan(&gt.ID, &gt.Name, &gt.Slug, &gt.BGGSearch, &gt.Position, &gt.GameCount)
	return gt, err
}

func (s *Store) ListGameTypes(ctx context.Context) ([]GameType, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+gameTypeColumns+` FROM game_types t ORDER BY t.position, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GameType{}
	for rows.Next() {
		gt, err := scanGameType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, gt)
	}
	return out, rows.Err()
}

func (s *Store) GetGameType(ctx context.Context, id int64) (GameType, error) {
	gt, err := scanGameType(s.db.QueryRowContext(ctx, `SELECT `+gameTypeColumns+` FROM game_types t WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return GameType{}, ErrNotFound
	}
	return gt, err
}

// slugTaken confronta già normalizzato (maiuscolo): lo slug salvato lo è
// sempre, quindi basta l'uguaglianza. excludeID è la tipologia che si sta
// modificando, che può tenere la propria sigla.
func (s *Store) slugTaken(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_types WHERE slug = ? AND id <> ?`, slug, excludeID).Scan(&n)
	return n > 0, err
}

func (s *Store) CreateGameType(ctx context.Context, in GameTypeInput) (GameType, error) {
	name, slug, err := normalizeGameType(in.Name, in.Slug)
	if err != nil {
		return GameType{}, err
	}
	if taken, err := s.slugTaken(ctx, slug, 0); err != nil {
		return GameType{}, err
	} else if taken {
		return GameType{}, ErrSlugTaken
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_types (name, slug, bgg_search, position)
		 VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM game_types))`,
		name, slug, in.BGGSearch)
	if err != nil {
		return GameType{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GameType{}, err
	}
	return s.GetGameType(ctx, id)
}

func (s *Store) UpdateGameType(ctx context.Context, id int64, upd GameTypeUpdate) (GameType, error) {
	current, err := s.GetGameType(ctx, id)
	if err != nil {
		return GameType{}, err
	}
	name, slug := current.Name, current.Slug
	if upd.Name != nil {
		name = *upd.Name
	}
	if upd.Slug != nil {
		slug = *upd.Slug
	}
	name, slug, err = normalizeGameType(name, slug)
	if err != nil {
		return GameType{}, err
	}
	if taken, err := s.slugTaken(ctx, slug, id); err != nil {
		return GameType{}, err
	} else if taken {
		return GameType{}, ErrSlugTaken
	}
	bgg := current.BGGSearch
	if upd.BGGSearch != nil {
		bgg = *upd.BGGSearch
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE game_types SET name = ?, slug = ?, bgg_search = ? WHERE id = ?`, name, slug, bgg, id); err != nil {
		return GameType{}, err
	}
	return s.GetGameType(ctx, id)
}

// MoveGameType scambia la posizione con la vicina sopra (up) o sotto.
// Agli estremi non fa niente: la freccia è solo un tocco a vuoto.
func (s *Store) MoveGameType(ctx context.Context, id int64, up bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pos int
	if err := tx.QueryRowContext(ctx, `SELECT position FROM game_types WHERE id = ?`, id).Scan(&pos); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	q := `SELECT id, position FROM game_types WHERE position > ? ORDER BY position LIMIT 1`
	if up {
		q = `SELECT id, position FROM game_types WHERE position < ? ORDER BY position DESC LIMIT 1`
	}
	var otherID int64
	var otherPos int
	if err := tx.QueryRowContext(ctx, q, pos).Scan(&otherID, &otherPos); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_types SET position = ? WHERE id = ?`, otherPos, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_types SET position = ? WHERE id = ?`, pos, otherID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteGameType(ctx context.Context, id int64) error {
	gt, err := s.GetGameType(ctx, id)
	if err != nil {
		return err
	}
	if gt.GameCount > 0 {
		return &GameTypeInUseError{Count: gt.GameCount}
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_types`).Scan(&total); err != nil {
		return err
	}
	if total <= 1 {
		return ErrLastGameType
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM game_types WHERE id = ?`, id)
	return err
}

// DefaultGameTypeID è la tipologia di un gioco creato senza indicarla:
// la prima in ordine, quella della prima tab.
func (s *Store) DefaultGameTypeID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM game_types ORDER BY position, id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (s *Store) GameTypeExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_types WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}
