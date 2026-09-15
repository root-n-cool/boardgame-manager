package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"boardgames-manager/internal/events"
)

type createBookingRequest struct {
	EventGameID   int64  `json:"eventGameId"`
	Name          string `json:"participantName"`
	Email         string `json:"participantEmail"`
	TermsAccepted bool   `json:"termsAccepted"`
}

func (s *Server) createBookingHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || !req.TermsAccepted {
		writeError(w, http.StatusBadRequest, "participantName is required and termsAccepted must be true")
		return
	}

	booking, err := s.Events.CreateBooking(r.Context(), eventID, req.EventGameID, req.Name, time.Now())
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event or game not found")
	case errors.Is(err, events.ErrEventAlreadyStarted):
		writeError(w, http.StatusConflict, "l'evento è già iniziato")
	case errors.Is(err, events.ErrGameSoldOut):
		writeError(w, http.StatusConflict, "non ci sono più posti prenotabili su questa copia")
	case errors.Is(err, events.ErrGameNotBookable):
		writeError(w, http.StatusConflict, "questo gioco non è prenotabile: è a disposizione al tavolo")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create booking")
	default:
		s.sendBookingConfirmation(r, booking, req.Email)
		resp := toBookingResponse(booking)
		// mailQueued dice alla pagina se promettere una mail: vero solo se
		// SMTP è configurato E chi prenota ha lasciato un'email — altrimenti
		// non parte niente, e prometterla sarebbe peggio che non dirla.
		resp["mailQueued"] = s.mailEnabled(r.Context()) && req.Email != ""
		writeJSON(w, http.StatusCreated, resp)
	}
}

type bookingCodeRequest struct {
	BookingCode string `json:"bookingCode"`
}

func (s *Server) lookupBookingHandler(w http.ResponseWriter, r *http.Request) {
	var req bookingCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "bookingCode is required")
		return
	}
	booking, err := s.Events.LookupBooking(r.Context(), req.BookingCode)
	if errors.Is(err, events.ErrInvalidBookingCredentials) {
		writeError(w, http.StatusNotFound, "prenotazione non trovata")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not look up booking")
		return
	}
	resp, err := s.toBookingDetailResponse(r.Context(), booking)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) cancelBookingHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	var req bookingCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "bookingCode is required")
		return
	}
	booking, err := s.Events.CancelBooking(r.Context(), id, req.BookingCode)
	if errors.Is(err, events.ErrInvalidBookingCredentials) || errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "prenotazione non trovata")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel booking")
		return
	}
	resp, err := s.toBookingDetailResponse(r.Context(), booking)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// adminCancelBookingHandler is the organiser's counterpart to the public
// cancel: no booking code, session auth only. It cancels rather than deletes,
// so the effect is exactly the one the participant would get.
func (s *Server) adminCancelBookingHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	_, err = s.Events.AdminCancelBooking(r.Context(), id)
	if errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "booking not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel booking")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
