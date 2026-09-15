package httpapi

import (
	"errors"
	"net/http"

	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
)

// uploadBrandingFile è la parte comune a logo e favicon: valida e salva il
// file con storage.CoverCategory, poi scrive il filename risultante nella
// riga di app_settings tramite apply, senza toccare il resto della
// configurazione — current è lo stato letto appena prima, così un campo
// come SMTPPassword non sparisce mai per un upload che non lo riguarda.
func (s *Server) uploadBrandingFile(w http.ResponseWriter, r *http.Request, apply func(current *settings.Settings, filename string)) {
	if err := r.ParseMultipartForm(storage.CoverCategory.MaxBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	filename, err := s.Storage.Save(storage.CoverCategory, file, header.Filename)
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

	current, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	apply(&current, filename)
	if err := s.Settings.Update(r.Context(), current); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) uploadLogoHandler(w http.ResponseWriter, r *http.Request) {
	s.uploadBrandingFile(w, r, func(current *settings.Settings, filename string) {
		current.LogoFilename = filename
	})
}

func (s *Server) uploadFaviconHandler(w http.ResponseWriter, r *http.Request) {
	s.uploadBrandingFile(w, r, func(current *settings.Settings, filename string) {
		current.FaviconFilename = filename
	})
}
