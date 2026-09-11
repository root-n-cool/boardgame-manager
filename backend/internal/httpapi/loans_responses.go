package httpapi

import (
	"context"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

// isoTime è il formato che l'app usa già per le date in uscita
// (toBookingAdminResponse): RFC3339, che il Date di JavaScript legge
// senza aiuto.
const isoTime = "2006-01-02T15:04:05Z07:00"

func toLoanResponse(l events.Loan) map[string]any {
	out := map[string]any{
		"id": l.ID, "eventGameId": l.EventGameID, "bookingId": l.BookingID,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"notes": l.Notes, "lentAt": l.LentAt.Format(isoTime),
		"returnedAt": nil,
	}
	if l.ReturnedAt != nil {
		out["returnedAt"] = l.ReturnedAt.Format(isoTime)
	}
	return out
}

// toOpenLoanResponse è il prestito come lo vede una riga di copia: il
// gioco lo sa già la riga che lo contiene, quindi qui non si ripete.
func toOpenLoanResponse(l events.LoanWithGame) map[string]any {
	return map[string]any{
		"id": l.ID, "borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "notes": l.Notes,
	}
}

// toReturnedLoanResponse è una riga del log della serata, che si legge da
// sola: il gioco ce l'ha dentro perché il log non è raggruppato per copia.
// issues è ciò che non è tornato intero — quasi sempre vuoto, ed è il punto:
// una riga con qualcosa dentro va guardata.
func toReturnedLoanResponse(l events.LoanWithGame, issues []events.MaterialIssue) map[string]any {
	rows := make([]map[string]any, 0, len(issues))
	for _, iss := range issues {
		row := map[string]any{"name": iss.Name, "expected": iss.Expected, "returned": nil}
		if iss.Returned != nil {
			row["returned"] = *iss.Returned
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"id": l.ID, "eventGameId": l.EventGameID, "gameId": l.GameID,
		"gameName": l.GameName, "copyIndex": l.CopyIndex,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "returnedAt": l.ReturnedAt.Format(isoTime),
		"notes": l.Notes, "materialIssues": rows,
	}
}

// toLoanDeskResponse è tutta la serata in una risposta: le copie con il
// loro prestito aperto (se c'è) e le prenotazioni attive che servono a
// precompilare la consegna, più il log dei prestiti chiusi. La pagina
// ricava "fuori" e "disponibili" dalla presenza di openLoan, senza un
// secondo giro.
func (s *Server) toLoanDeskResponse(ctx context.Context, eventID int64) (map[string]any, error) {
	eventGames, err := s.Events.ListEventGames(ctx, eventID)
	if err != nil {
		return nil, err
	}
	loans, err := s.Events.ListLoansForEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	bookings, err := s.Events.ListBookingsForEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	openByCopy := map[int64]events.LoanWithGame{}
	returned := []map[string]any{}
	closedIDs := []int64{}
	closed := []events.LoanWithGame{}
	for _, l := range loans {
		if l.ReturnedAt == nil {
			openByCopy[l.EventGameID] = l
			continue
		}
		closed = append(closed, l)
		closedIDs = append(closedIDs, l.ID)
	}
	issuesByLoan, err := s.Events.ListMaterialIssues(ctx, closedIDs)
	if err != nil {
		return nil, err
	}
	for _, l := range closed {
		returned = append(returned, toReturnedLoanResponse(l, issuesByLoan[l.ID]))
	}

	bookingsByCopy := map[int64][]map[string]any{}
	for _, b := range bookings {
		bookingsByCopy[b.EventGameID] = append(bookingsByCopy[b.EventGameID], map[string]any{
			"id": b.ID, "name": b.ParticipantName, "phone": b.ParticipantPhone,
		})
	}

	// Quante copie ha ogni gioco: la UI ne ha bisogno per decidere se
	// numerarle, perché con una copia sola "#1" è rumore.
	copiesPerGame := map[int64]int{}
	for _, eg := range eventGames {
		copiesPerGame[eg.GameID]++
	}

	// Il banco lo mostra prima della consegna: chi dà in mano la scatola
	// deve sapere che è già incompleta, e non prendersi la colpa al rientro.
	gameIDs := make([]int64, 0, len(eventGames))
	for _, eg := range eventGames {
		gameIDs = append(gameIDs, eg.GameID)
	}
	missing, err := s.Events.GamesMissingPieces(ctx, gameIDs)
	if err != nil {
		return nil, err
	}

	// Un gioco si legge una volta anche se ha più copie, come in
	// toEventDetail.
	gameCache := map[int64]games.Game{}
	// I materiali si leggono una volta per gioco, non una per copia: una
	// serata con quattro copie di Carcassonne farebbe quattro query uguali.
	materialsCache := map[int64][]map[string]any{}
	copies := make([]map[string]any, 0, len(eventGames))
	for _, eg := range eventGames {
		game, ok := gameCache[eg.GameID]
		if !ok {
			game, err = s.Games.GetGame(ctx, eg.GameID)
			if err != nil {
				return nil, err
			}
			gameCache[eg.GameID] = game
		}
		materials, cached := materialsCache[eg.GameID]
		if !cached {
			ms, err := s.Games.ListMaterials(ctx, eg.GameID)
			if err != nil {
				return nil, err
			}
			materials = make([]map[string]any, 0, len(ms))
			for _, m := range ms {
				materials = append(materials, map[string]any{
					"id": m.ID, "name": m.Name, "quantity": m.Quantity,
				})
			}
			materialsCache[eg.GameID] = materials
		}
		row := map[string]any{
			"eventGameId": eg.ID, "gameId": eg.GameID, "name": game.Name,
			"coverPath": game.CoverPath, "copyIndex": eg.CopyIndex,
			"copies": copiesPerGame[eg.GameID], "bookable": eg.Bookable,
			"seats": eg.Seats, "openLoan": nil,
			"activeBookings": orEmptyRows(bookingsByCopy[eg.ID]),
			"materials":      materials,
			"incomplete":     len(missing[eg.GameID]) > 0,
		}
		if l, out := openByCopy[eg.ID]; out {
			row["openLoan"] = toOpenLoanResponse(l)
		}
		copies = append(copies, row)
	}

	return map[string]any{"copies": copies, "returned": returned}, nil
}

// orEmptyRows manda [] invece di null: una lista vuota che arriva come
// null costringe ogni punto della UI a difendersi.
func orEmptyRows(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}
