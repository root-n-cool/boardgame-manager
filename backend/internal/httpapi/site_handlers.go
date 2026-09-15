package httpapi

import (
	"log"
	"net/http"
)

// defaultSiteTitle è il nome che l'app porta finché nessuno lo cambia
// dal pannello impostazioni.
const defaultSiteTitle = "BoardGames Manager"

// siteResponse è quello che l'header (con o senza sessione) legge per
// mostrare il marchio del sito: mai un segreto, sempre leggibile senza
// autenticazione.
type siteResponse struct {
	SiteTitle    string `json:"siteTitle"`
	LogoFilename string `json:"logoFilename"`
}

// getSiteHandler è pubblico apposta: l'header compare anche su /login,
// /setup e le pagine pubbliche, prima che una sessione esista.
func (s *Server) getSiteHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		log.Printf("site: could not load settings, serving defaults: %v", err)
		writeJSON(w, http.StatusOK, siteResponse{SiteTitle: defaultSiteTitle})
		return
	}

	title := cfg.SiteTitle
	if title == "" {
		title = defaultSiteTitle
	}
	writeJSON(w, http.StatusOK, siteResponse{SiteTitle: title, LogoFilename: cfg.LogoFilename})
}
