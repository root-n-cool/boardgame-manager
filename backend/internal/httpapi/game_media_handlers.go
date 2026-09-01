package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/games"
	"boardgames-manager/internal/storage"
)

func (s *Server) createMediaHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	code := chi.URLParam(r, "lang")
	lang, err := s.Games.GetLanguage(r.Context(), gameID, code)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "language not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load language")
		return
	}

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		s.createFileMediaHandler(w, r, lang)
		return
	}
	s.createLinkMediaHandler(w, r, lang)
}

func (s *Server) createFileMediaHandler(w http.ResponseWriter, r *http.Request, lang games.GameLanguage) {
	if err := r.ParseMultipartForm(storage.ManualCategory.MaxBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	path, err := s.Storage.Save(storage.ManualCategory, file)
	if errors.Is(err, storage.ErrUnsupportedType) {
		writeError(w, http.StatusBadRequest, "only PDF files are allowed")
		return
	}
	if errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusBadRequest, "file exceeds the 20MB limit")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	media, err := s.Games.CreateMedia(r.Context(), games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeFile, URLOrPath: path, Title: &title,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save media")
		return
	}
	writeJSON(w, http.StatusCreated, toMediaResponse(media))
}

type createLinkMediaRequest struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func (s *Server) createLinkMediaHandler(w http.ResponseWriter, r *http.Request, lang games.GameLanguage) {
	var req createLinkMediaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type != games.MediaTypeLink && req.Type != games.MediaTypeYoutube {
		writeError(w, http.StatusBadRequest, "type must be 'link' or 'youtube'")
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeError(w, http.StatusBadRequest, "url must start with http:// or https://")
		return
	}
	if req.Type == games.MediaTypeYoutube && !strings.Contains(req.URL, "youtube.com") && !strings.Contains(req.URL, "youtu.be") {
		writeError(w, http.StatusBadRequest, "youtube url must contain youtube.com or youtu.be")
		return
	}

	var titlePtr *string
	if req.Title != "" {
		titlePtr = &req.Title
	}

	media, err := s.Games.CreateMedia(r.Context(), games.GameMedia{
		GameLanguageID: lang.ID, Type: req.Type, URLOrPath: req.URL, Title: titlePtr,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save media")
		return
	}
	writeJSON(w, http.StatusCreated, toMediaResponse(media))
}

func (s *Server) deleteMediaHandler(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseIDParam(r, "mediaId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid media id")
		return
	}
	if err := s.Games.DeleteMedia(r.Context(), mediaID); errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete media")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) getUploadHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	f, err := s.Storage.Open(filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()

	ext := filepath.Ext(filename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	io.Copy(w, f)
}
