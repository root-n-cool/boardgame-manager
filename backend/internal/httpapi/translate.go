package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
)

// translator restituisce il traduttore da usare per questa richiesta:
// quello iniettato se c'è (i test), altrimenti uno costruito al volo dalle
// impostazioni salvate. Costruirlo per richiesta è ciò che permette
// all'admin di cambiare provider senza riavviare il container.
func (s *Server) translator(ctx context.Context) ai.Translator {
	if s.AI != nil {
		return s.AI
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		// Senza impostazioni non si traduce; il chiamante tratta la cosa
		// come "AI non configurata", che è l'esito giusto.
		log.Printf("translate: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// translateDescription è l'unico posto che decide se una descrizione va
// tradotta. Non restituisce mai un errore: se non si può tradurre, il
// testo originale è il risultato giusto — un gioco senza descrizione
// tradotta è comunque un gioco in catalogo, e la scheda ha il bottone per
// riprovare.
func (s *Server) translateDescription(ctx context.Context, source, targetLang string) string {
	if strings.TrimSpace(source) == "" {
		return source
	}
	// L'originale BGG è già in inglese: chiedere di tradurlo in inglese
	// costerebbe una chiamata per riavere lo stesso testo, peggiorato.
	if strings.EqualFold(strings.TrimSpace(targetLang), "en") {
		return source
	}

	out, err := s.translator(ctx).Translate(ctx, source, targetLang)
	if errors.Is(err, ai.ErrNotConfigured) {
		return source
	}
	if err != nil {
		log.Printf("translate into %q: %v", targetLang, err)
		return source
	}
	return out
}

// translateLanguageHandler ritraduce a richiesta la descrizione di una
// lingua, sempre dall'originale BGG. Sovrascrive quel che c'è, comprese le
// correzioni a mano: è il senso del bottone, e la UI lo dice prima di
// chiamarlo.
//
// A differenza degli innesti automatici, qui un guasto si mostra: l'admin
// ha premuto un bottone e ha diritto di sapere che non ha funzionato.
func (s *Server) translateLanguageHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	code := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "lang")))
	if code == "" {
		writeError(w, http.StatusBadRequest, "language code is required")
		return
	}

	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}

	lang, err := s.Games.GetLanguage(r.Context(), gameID, code)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "language not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load language")
		return
	}

	if game.BGGDescription == nil || strings.TrimSpace(*game.BGGDescription) == "" {
		writeError(w, http.StatusConflict, "questo gioco non ha una descrizione originale da BoardGameGeek da tradurre")
		return
	}

	translated, err := s.translator(r.Context()).Translate(r.Context(), *game.BGGDescription, code)
	if errors.Is(err, ai.ErrNotConfigured) {
		writeError(w, http.StatusConflict, "il provider AI non è configurato: aggiungilo nelle impostazioni")
		return
	}
	if err != nil {
		log.Printf("translate language %s of game %d: %v", code, gameID, err)
		writeError(w, http.StatusBadGateway, "la traduzione non è riuscita: riprova, o controlla il provider nelle impostazioni")
		return
	}

	updated, err := s.Games.UpdateLanguage(r.Context(), gameID, code, lang.Name, &translated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save the translation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code": updated.LanguageCode, "isBaseLanguage": updated.IsBaseLanguage,
		"name": updated.Name, "description": updated.Description,
	})
}
