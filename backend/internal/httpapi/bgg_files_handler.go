package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"boardgames-manager/internal/games"
)

// bggLanguageIDs traduce i codici lingua dell'app negli id lingua di BGG.
// Gli id arrivano da config.languages nella risposta di /api/files, che però
// elenca solo le lingue presenti nei file di quel gioco: questa mappa è
// l'unione raccolta su più giochi, da estendere quando serve una lingua che
// non c'è. Gli id sono globali (l'italiano è 2193 su qualsiasi gioco).
//
// Un codice fuori mappa non è un errore — la lingua di una scheda è testo
// libero: si mostrano tutti i file, con languageFiltered false, così la UI
// può dirlo all'admin.
var bggLanguageIDs = map[string]string{
	"ar": "2178", "ca": "2179", "cs": "2180", "zh": "2181", "da": "2182",
	"nl": "2183", "en": "2184", "et": "2185", "fi": "2186", "fr": "2187",
	"de": "2188", "el": "2189", "he": "2190", "hu": "2191", "it": "2193",
	"ja": "2194", "ko": "2195", "lt": "2197", "no": "2198", "pl": "2199",
	"pt": "2200", "ro": "2201", "ru": "2202", "es": "2203", "sv": "2204",
	"tr": "2349", "hr": "2656", "uk": "2665", "sr": "2681", "th": "2709",
	"vi": "2738", "gl": "2740", "fa": "2756", "ug": "3076",
}

type bggFileItem struct {
	Title    string `json:"title"`
	Filename string `json:"filename"`
	// Language è il nome inglese con cui BGG etichetta il file
	// ("Italian"); LanguageCode è il codice corrispondente nell'app,
	// vuoto per le lingue che non traduciamo e per i file "(neutral)".
	// La UI scrive il nome italiano dal codice e ricade su Language.
	Language     string `json:"language"`
	LanguageCode string `json:"languageCode"`
	Positive     int    `json:"positive"`
	SizeBytes    int64  `json:"sizeBytes"`
	PageURL      string `json:"pageUrl"`
}

type bggFilesResponse struct {
	Items []bggFileItem `json:"items"`
	// LanguageFiltered dice se la lista è ristretta alla lingua chiesta.
	LanguageFiltered bool `json:"languageFiltered"`
}

// listBggFilesHandler elenca i file che BGG ha per un gioco. Serve solo a
// far trovare il manuale all'admin: i byte dei file stanno dietro il login
// di BGG, quindi ogni voce porta il link alla filepage e il download resta
// un gesto manuale dal browser.
func (s *Server) listBggFilesHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	game, err := s.Games.GetGame(r.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	if game.BGGID == nil || *game.BGGID == "" {
		writeError(w, http.StatusConflict, "game is not linked to BGG")
		return
	}

	languageID := ""
	if r.URL.Query().Get("all") != "1" {
		languageID = bggLanguageIDs[strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))]
	}

	files, err := s.BGG.Files(r.Context(), *game.BGGID, languageID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not list BGG files")
		return
	}

	codeByBGGID := make(map[string]string, len(bggLanguageIDs))
	for code, id := range bggLanguageIDs {
		codeByBGGID[id] = code
	}

	items := make([]bggFileItem, 0, len(files))
	for _, f := range files {
		items = append(items, bggFileItem{
			Title:        f.Title,
			Filename:     f.Filename,
			Language:     f.Language,
			LanguageCode: codeByBGGID[f.LanguageID],
			Positive:     f.Positive,
			SizeBytes:    f.SizeBytes,
			PageURL:      f.PageURL,
		})
	}
	writeJSON(w, http.StatusOK, bggFilesResponse{Items: items, LanguageFiltered: languageID != ""})
}
