package httpapi

import (
	"log"
	"net/http"

	"boardgames-manager/internal/events"
)

// toGameLoanResponse è una riga del log di un gioco. materialIssues ha la
// stessa forma che ha già nel banco prestiti: chi ha scritto un pezzo di UI
// per una delle due schermate riconosce l'altra.
func toGameLoanResponse(l events.LoanWithEvent, issues []events.MaterialIssue) map[string]any {
	rows := make([]map[string]any, 0, len(issues))
	for _, iss := range issues {
		row := map[string]any{"name": iss.Name, "expected": iss.Expected, "returned": nil}
		if iss.Returned != nil {
			row["returned"] = *iss.Returned
		}
		rows = append(rows, row)
	}
	out := map[string]any{
		"id": l.ID, "eventId": l.EventID, "eventTitle": l.EventTitle,
		"eventDate": l.EventDate, "copyIndex": l.CopyIndex, "copies": l.Copies,
		"borrowerName": l.BorrowerName, "borrowerPhone": l.BorrowerPhone,
		"lentAt": l.LentAt.Format(isoTime), "notes": l.Notes,
		"returnedAt": nil, "materialIssues": rows,
	}
	if l.ReturnedAt != nil {
		out["returnedAt"] = l.ReturnedAt.Format(isoTime)
	}
	return out
}

func (s *Server) listGameLoansHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	loans, err := s.Events.ListLoansForGame(r.Context(), gameID)
	if err != nil {
		log.Printf("game loans: list for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not list the loans")
		return
	}

	// Gli esiti di tutti i prestiti in una chiamata sola: il log di un
	// gioco molto prestato non deve diventare una N+1.
	ids := make([]int64, 0, len(loans))
	for _, l := range loans {
		if l.ReturnedAt != nil {
			ids = append(ids, l.ID)
		}
	}
	issues, err := s.Events.ListMaterialIssues(r.Context(), ids)
	if err != nil {
		log.Printf("game loans: issues for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not list the loans")
		return
	}

	out := make([]map[string]any, 0, len(loans))
	for _, l := range loans {
		out = append(out, toGameLoanResponse(l, issues[l.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"loans": out})
}

func (s *Server) resolveMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	user, ok := currentUser(r)
	if !ok {
		// Irraggiungibile: la rotta è nel blocco protetto. Il controllo c'è
		// perché la colonna registra CHI ha chiuso la segnalazione, e uno
		// zero lì dentro sarebbe una bugia silenziosa.
		writeError(w, http.StatusUnauthorized, "sessione richiesta")
		return
	}
	game, err := s.Games.MarkMaterialsChecked(r.Context(), gameID, user.ID)
	if err != nil {
		log.Printf("game loans: resolve for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not resolve")
		return
	}
	langs, err := s.Games.ListLanguages(r.Context(), gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	detail, err := s.toGameDetail(r.Context(), game, langs, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
