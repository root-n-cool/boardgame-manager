package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"boardgames-manager/internal/events"
)

type createBookingRequest struct {
	EventGameID int64  `json:"eventGameId"`
	Name        string `json:"participantName"`
	Email       string `json:"participantEmail"`
	Phone       string `json:"participantPhone"`
}

func (s *Server) createBookingHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Email == "" || req.Phone == "" {
		writeError(w, http.StatusBadRequest, "participantName, participantEmail and participantPhone are required")
		return
	}

	booking, err := s.Events.CreateBooking(r.Context(), eventID, req.EventGameID, req.Name, req.Email, req.Phone, time.Now())
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event or game not found")
	case errors.Is(err, events.ErrEventAlreadyStarted):
		writeError(w, http.StatusConflict, "l'evento è già iniziato")
	case errors.Is(err, events.ErrGameSoldOut):
		writeError(w, http.StatusConflict, "non ci sono più copie disponibili per questo gioco")
	case errors.Is(err, events.ErrDuplicatePhoneBooking):
		writeError(w, http.StatusConflict, "hai già una prenotazione attiva per questo evento")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create booking")
	default:
		writeJSON(w, http.StatusCreated, toBookingResponse(booking))
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
