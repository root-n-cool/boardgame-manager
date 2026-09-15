package httpapi

import (
	"log"
	"net/http"
)

type legalPageResponse struct {
	Markdown string `json:"markdown"`
}

// getTermsHandler e getPrivacyHandler rispondono sempre 200: un testo
// vuoto è uno stato valido (l'admin non ha ancora scritto niente), e la
// view pubblica mostra un messaggio neutro invece di un errore — vedi
// LegalPageView.
func (s *Server) getTermsHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		log.Printf("legal: could not load settings, serving empty terms: %v", err)
		writeJSON(w, http.StatusOK, legalPageResponse{})
		return
	}
	writeJSON(w, http.StatusOK, legalPageResponse{Markdown: cfg.TermsMarkdown})
}

func (s *Server) getPrivacyHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		log.Printf("legal: could not load settings, serving empty privacy: %v", err)
		writeJSON(w, http.StatusOK, legalPageResponse{})
		return
	}
	writeJSON(w, http.StatusOK, legalPageResponse{Markdown: cfg.PrivacyMarkdown})
}
