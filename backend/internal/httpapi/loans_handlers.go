package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"boardgames-manager/internal/events"
)

// listEventLoansHandler è l'unica lettura del banco prestiti.
func (s *Server) listEventLoansHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	// Un evento inesistente deve dare 404, non una serata vuota: senza
	// questo controllo la risposta sarebbe {copies: [], returned: []}, che
	// è indistinguibile da un evento senza giochi.
	if _, err := s.Events.GetEvent(r.Context(), eventID); errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "questa serata non esiste più")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	payload, err := s.toLoanDeskResponse(r.Context(), eventID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list loans")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

type createLoanRequest struct {
	EventGameID int64 `json:"eventGameId"`
	// BookingID c'è solo quando la consegna parte da una prenotazione.
	BookingID     *int64  `json:"bookingId"`
	BorrowerName  string  `json:"borrowerName"`
	BorrowerPhone string  `json:"borrowerPhone"`
	Notes         *string `json:"notes"`
}

func (s *Server) createLoanHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	var req createLoanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	loan, err := s.Events.LendCopy(r.Context(), eventID, events.LoanInput{
		EventGameID:   req.EventGameID,
		BookingID:     req.BookingID,
		BorrowerName:  req.BorrowerName,
		BorrowerPhone: req.BorrowerPhone,
		Notes:         req.Notes,
	})
	switch {
	case errors.Is(err, events.ErrBorrowerRequired):
		writeError(w, http.StatusBadRequest, "nome e telefono di chi ritira sono obbligatori")
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "copia o prenotazione non più disponibili: ricarica il banco")
	case errors.Is(err, events.ErrCopyAlreadyOut):
		writeError(w, http.StatusConflict, "questa copia è già in prestito")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create loan")
	default:
		writeJSON(w, http.StatusCreated, toLoanResponse(loan))
	}
}

type returnLoanRequest struct {
	// Notes nil lascia quelle scritte alla consegna: un corpo vuoto è il
	// caso normale, si restituisce senza avere niente da segnalare.
	Notes *string `json:"notes"`
}

func (s *Server) returnLoanHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid loan id")
		return
	}
	var req returnLoanRequest
	// Un corpo assente o vuoto è legittimo, quindi l'errore di decodifica
	// non è un errore: si prosegue con Notes nil.
	_ = json.NewDecoder(r.Body).Decode(&req)

	loan, err := s.Events.ReturnLoan(r.Context(), id, req.Notes, nil)
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "questo prestito non esiste più: ricarica il banco")
	case errors.Is(err, events.ErrLoanAlreadyReturned):
		writeError(w, http.StatusConflict, "questo prestito è già stato chiuso")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not return loan")
	default:
		writeJSON(w, http.StatusOK, toLoanResponse(loan))
	}
}
