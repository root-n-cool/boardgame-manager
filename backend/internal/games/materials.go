package games

import (
	"context"
	"database/sql"
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
// Totale nel risultato ma non nel modo: è un upsert sulla chiave "nome
// normalizzato" (lowercase del nome ripulito, la stessa con cui
// validateMaterials riconosce un duplicato) e non un DELETE seguito da
// N INSERT. Gli id sono ciò con cui la modale di riconsegna, aperta magari
// da mezz'ora, ritrova le voci del catalogo: rigenerarli a ogni
// salvataggio — anche un salvataggio che non cambia niente — significava
// che al ritorno del gioco nessuna spunta combaciava più e tutte le voci
// finivano registrate come "non verificate", con un 200 e niente a
// schermo. Di riflesso resta in piedi anche
// loan_material_issue.material_id dei prestiti passati.
//
// Resta scoperto un caso, ed è accettato: *rinominare* una voce è una
// cancellazione più un inserimento, quindi una modale di riconsegna già
// aperta perde la spunta di quella riga e la registra come non
// verificata. L'errore va sempre dalla parte prudente — non può mai dare
// per presente qualcosa che nessuno ha guardato — e la finestra è quella
// di una serata sola.
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

	keep := make(map[string]bool, len(clean))
	for _, m := range clean {
		keep[materialKey(m.Name)] = true
	}
	reuse, dead, err := splitMaterialRows(ctx, tx, gameID, keep)
	if err != nil {
		return nil, err
	}

	// Le uscite prima di tutto il resto: UNIQUE(game_id, name) conta anche
	// le righe che stanno per sparire, e un UPDATE che riscrive il nome
	// nella grafia appena scelta collide con la riga che sta per andarsene
	// se questa porta lo stesso nome a maiuscole diverse.
	for _, id := range dead {
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_material WHERE id = ?`, id); err != nil {
			return nil, err
		}
	}

	for i, m := range clean {
		if id, ok := reuse[materialKey(m.Name)]; ok {
			// Il nome si riscrive comunque: la chiave è normalizzata, quindi
			// "Tessere" e "tessere" sono la stessa voce e vale l'ultima
			// grafia scelta dall'admin.
			if _, err := tx.ExecContext(ctx,
				`UPDATE game_material SET name = ?, quantity = ?, position = ? WHERE id = ?`,
				m.Name, m.Quantity, i, id,
			); err != nil {
				return nil, err
			}
			continue
		}
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

// splitMaterialRows divide le righe già in tabella fra quelle che la nuova
// lista riusa (per chiave normalizzata) e quelle da cancellare.
//
// La chiave viene rivendicata una volta sola: UNIQUE(game_id, name) è
// case-sensitive, quindi in teoria due righe possono normalizzare allo
// stesso nome (non dalla nostra scrittura, che le rifiuta, ma da una
// migrazione o da SQL a mano) — la prima resta, le altre se ne vanno,
// altrimenti resterebbero in tabella a duplicare una voce.
func splitMaterialRows(ctx context.Context, tx *sql.Tx, gameID int64, keep map[string]bool) (map[string]int64, []int64, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name FROM game_material WHERE game_id = ? ORDER BY position, id`, gameID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	reuse := map[string]int64{}
	var dead []int64
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		key := materialKey(name)
		if _, claimed := reuse[key]; keep[key] && !claimed {
			reuse[key] = id
			continue
		}
		dead = append(dead, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return reuse, dead, nil
}

// materialKey è l'identità di una voce agli occhi dell'editor: due grafie
// che differiscono per spazi o maiuscole sono la stessa riga della scatola.
func materialKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
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
		key := materialKey(name)
		if seen[key] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateMaterial, name)
		}
		seen[key] = true
		out = append(out, MaterialInput{Name: name, Quantity: m.Quantity})
	}
	return out, nil
}

// MarkMaterialsChecked dichiara che la scatola è di nuovo a posto: da
// questo istante le mancanze registrate prima non segnalano più il gioco.
//
// Non cancella niente: le rilevazioni restano nel registro dei prestiti e
// nel log del gioco. Si archivia la segnalazione, non la storia — ed è il
// motivo per cui questa è una data e non un DELETE.
func (s *Store) MarkMaterialsChecked(ctx context.Context, gameID, userID int64) (Game, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE games SET materials_checked_at = datetime('now'), materials_checked_by = ?
		 WHERE id = ?`, userID, gameID)
	if err != nil {
		return Game{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Game{}, err
	}
	if affected == 0 {
		return Game{}, ErrNotFound
	}
	return s.GetGame(ctx, gameID)
}
