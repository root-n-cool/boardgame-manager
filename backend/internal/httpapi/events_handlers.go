package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"boardgames-manager/internal/events"
)

// pastEventsPageSize is how many past events one page of the archive holds.
const pastEventsPageSize = 12

// listEventsHandler serves both sides of the event list: by default the
// upcoming events, nearest first and unpaged (they are few and the public
// page shows them all); with ?past=true the archive, most recent first and
// paged. The archive is admin-only — an anonymous request asking for it gets
// the upcoming events instead, never a 403 that would leak that it exists.
func (s *Server) listEventsHandler(w http.ResponseWriter, r *http.Request) {
	past := r.URL.Query().Get("past") == "true" && s.hasAdminSession(r)

	params := events.ListEventsParams{Past: past, Now: time.Now()}
	page := 1
	if past {
		page = parsePageParam(r)
		params.Limit = pastEventsPageSize
		params.Offset = (page - 1) * pastEventsPageSize
	}

	list, total, err := s.Events.ListEvents(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list events")
		return
	}
	// Una pagina oltre la fine (link vecchio, archivio rimpicciolito) tornerebbe
	// vuota con un pager che dice "pagina 5 di 2": si ricade sull'ultima piena.
	if past && total > 0 && len(list) == 0 {
		page = (total + pastEventsPageSize - 1) / pastEventsPageSize
		params.Offset = (page - 1) * pastEventsPageSize
		list, total, err = s.Events.ListEvents(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list events")
			return
		}
	}
	items := make([]map[string]any, 0, len(list))
	for _, e := range list {
		items = append(items, toEventListItem(e))
	}
	// pageSize 0 means "everything on one page": the upcoming list is not paged.
	pageSize := 0
	if past {
		pageSize = pastEventsPageSize
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "pageSize": pageSize,
	})
}

func parsePageParam(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
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
	GameID int64 `json:"gameId"`
	// Copies è quante copie del gioco l'evento mette in tavola. I posti
	// prenotabili di ciascuna arrivano dal catalogo, non da qui.
	Copies int `json:"copies"`
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
		out = append(out, events.EventGameInput{GameID: g.GameID, Copies: g.Copies})
	}
	return out
}

func decodeEventRequest(r *http.Request) (eventRequest, bool) {
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return eventRequest{}, false
	}
	if req.Title == "" || req.EventDate == "" || req.StartTime == "" {
		return eventRequest{}, false
	}
	if _, err := time.Parse("2006-01-02", req.EventDate); err != nil {
		return eventRequest{}, false
	}
	if _, err := time.Parse("15:04", req.StartTime); err != nil {
		return eventRequest{}, false
	}
	seenGames := map[int64]bool{}
	for _, g := range req.Games {
		if g.Copies < 1 {
			return eventRequest{}, false
		}
		if seenGames[g.GameID] {
			return eventRequest{}, false
		}
		seenGames[g.GameID] = true
	}
	return req, true
}

func (s *Server) createEventHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeEventRequest(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "title, eventDate and startTime are required")
		return
	}
	event, err := s.Events.CreateEvent(r.Context(), req.Title, req.Description, req.EventDate, req.StartTime, toEventGameInputs(req.Games))
	if errors.Is(err, events.ErrGameNotFound) {
		writeError(w, http.StatusBadRequest, "one of the selected games does not exist")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create event")
		return
	}
	writeJSON(w, http.StatusCreated, toEventSummary(event))
}

func (s *Server) updateEventHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	req, ok := decodeEventRequest(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "title, eventDate and startTime are required")
		return
	}
	event, err := s.Events.UpdateEvent(r.Context(), id, req.Title, req.Description, req.EventDate, req.StartTime, toEventGameInputs(req.Games))
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event not found")
	case errors.Is(err, events.ErrGameNotFound):
		writeError(w, http.StatusBadRequest, "one of the selected games does not exist")
	case errors.Is(err, events.ErrQuantityBelowActiveBookings):
		writeError(w, http.StatusConflict, "fewer copies than the ones with active bookings")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update event")
	default:
		writeJSON(w, http.StatusOK, toEventSummary(event))
	}
}

func (s *Server) deleteEventHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	if err := s.Events.DeleteEvent(r.Context(), id); errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) listEventBookingsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	list, err := s.Events.ListBookingsForEvent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list bookings")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		out = append(out, toBookingAdminResponse(b))
	}
	writeJSON(w, http.StatusOK, out)
}
