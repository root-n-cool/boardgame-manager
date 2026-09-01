package httpapi

import (
	"errors"
	"net/http"

	"boardgames-manager/internal/games"
	"boardgames-manager/internal/storage"
)

func (s *Server) uploadCoverHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}

	if _, err := s.Games.GetGame(r.Context(), gameID); errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
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

	game, err := s.Games.UpdateCoverPath(r.Context(), gameID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update cover")
		return
	}
	writeJSON(w, http.StatusOK, toGameSummary(game))
}
