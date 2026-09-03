package httpapi

import (
	"errors"
	"net/http"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/storage"
)

// uploadEventImageHandler attaches the optional event image. It is the twin of
// uploadCoverHandler and shares its storage category: same formats, same 5MB
// ceiling, same content-addressed filenames.
func (s *Server) uploadEventImageHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	if _, err := s.Events.GetEvent(r.Context(), eventID); errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}

	if err := r.ParseMultipartForm(storage.CoverCategory.MaxBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	path, err := s.Storage.Save(storage.CoverCategory, file)
	if errors.Is(err, storage.ErrUnsupportedType) {
		writeError(w, http.StatusBadRequest, "only JPEG, PNG or WebP images are allowed")
		return
	}
	if errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusBadRequest, "file exceeds the 5MB limit")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}

	event, err := s.Events.UpdateImagePath(r.Context(), eventID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update image")
		return
	}
	writeJSON(w, http.StatusOK, toEventSummary(event))
}
