package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"boardgames-manager/internal/events"
)

func (s *Server) listEventsHandler(w http.ResponseWriter, r *http.Request) {
	includePast := s.hasAdminSession(r)
	list, err := s.Events.ListEvents(r.Context(), includePast, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list events")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		out = append(out, toEventSummary(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getEventHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	event, err := s.Events.GetEvent(r.Context(), id)
	if errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	detail, err := s.toEventDetail(r.Context(), event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

type eventGameRequest struct {
	GameID   int64 `json:"gameId"`
	Quantity int   `json:"quantity"`
}

type eventRequest struct {
	Title       string             `json:"title"`
	Description *string            `json:"description"`
	EventDate   string             `json:"eventDate"`
	StartTime   string             `json:"startTime"`
	Games       []eventGameRequest `json:"games"`
}

func toEventGameInputs(in []eventGameRequest) []events.EventGameInput {
	out := make([]events.EventGameInput, 0, len(in))
	for _, g := range in {
		out = append(out, events.EventGameInput{GameID: g.GameID, Quantity: g.Quantity})
	}
	return out
}

func decodeEventRequest(r *http.Request) (eventRequest, bool) {
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return eventRequest{}, false
	}
	return req, req.Title != "" && req.EventDate != "" && req.StartTime != ""
}
