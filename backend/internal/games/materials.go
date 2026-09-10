package games

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// I tetti stanno qui perché lo store è l'unico punto che ogni scrittura
// attraversa, da qualunque parte arrivi: il form dell'admin, la proposta
// del modello, un test. Gli altri pacchetti li importano da qui invece di
// ricopiarne il valore.
const (
	MaxMaterialNameChars = 60
	MaxMaterialQuantity  = 9999
	MaxMaterialsPerGame  = 60
)

// Material è una voce del contenuto della scatola.
type Material struct {
	ID       int64
	GameID   int64
	Name     string
	Quantity int
	Position int
}

// MaterialInput è una voce come la manda l'editor: senza id e senza
// posizione, perché l'ordine è quello della lista che arriva.
type MaterialInput struct {
	Name     string
	Quantity int
}

var (
	ErrMaterialInvalid   = errors.New("material name or quantity is not valid")
	ErrDuplicateMaterial = errors.New("duplicate material name")
	ErrTooManyMaterials  = errors.New("too many materials for one game")
)

// ListMaterials torna le voci nell'ordine deciso dall'admin.
func (s *Store) ListMaterials(ctx context.Context, gameID int64) ([]Material, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, name, quantity, position
		 FROM game_material WHERE game_id = ? ORDER BY position, id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Material{}
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.GameID, &m.Name, &m.Quantity, &m.Position); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMaterials sostituisce l'intera lista di un gioco in una
// transazione.
//
// Sostituzione totale e non CRUD per riga: l'editor è una lista che si
// salva tutta insieme e il riordino tocca comunque quasi tutte le righe,
// quindi tre rotte costringerebbero il frontend a calcolare un diff per poi
// mandare N richieste che possono fallire a metà. Così il DB resta o tutto
// vecchio o tutto nuovo.
//
// Effetto collaterale documentato: gli id cambiano a ogni salvataggio, e
// loan_material_issue.material_id dei prestiti passati diventa NULL. Il
// registro storico sopravvive perché nome e quantità attesa sono copiati
// sulla riga di esito.
func (s *Store) ReplaceMaterials(ctx context.Context, gameID int64, in []MaterialInput) ([]Material, error) {
	clean, err := validateMaterials(in)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM game_material WHERE game_id = ?`, gameID); err != nil {
		return nil, err
	}
	for i, m := range clean {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO game_material (game_id, name, quantity, position) VALUES (?, ?, ?, ?)`,
			gameID, m.Name, m.Quantity, i,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListMaterials(ctx, gameID)
}

// validateMaterials normalizza i nomi e rifiuta ciò che non può stare in
// tabella. Il confronto per il duplicato è lowercase + trim, lo stesso
// criterio con cui la classifica riconosce due volte lo stesso giocatore.
func validateMaterials(in []MaterialInput) ([]MaterialInput, error) {
	if len(in) > MaxMaterialsPerGame {
		return nil, fmt.Errorf("%w: %d voci", ErrTooManyMaterials, len(in))
	}
	seen := make(map[string]bool, len(in))
	out := make([]MaterialInput, 0, len(in))
	for _, m := range in {
		name := strings.TrimSpace(m.Name)
		if name == "" || len([]rune(name)) > MaxMaterialNameChars {
			return nil, fmt.Errorf("%w: nome %q", ErrMaterialInvalid, m.Name)
		}
		if m.Quantity < 1 || m.Quantity > MaxMaterialQuantity {
			return nil, fmt.Errorf("%w: quantità %d per %q", ErrMaterialInvalid, m.Quantity, name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateMaterial, name)
		}
		seen[key] = true
		out = append(out, MaterialInput{Name: name, Quantity: m.Quantity})
	}
	return out, nil
}
