package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
)

// toMaterialsResponse manda sempre una lista, mai null: una lista vuota che
// arriva come null costringe ogni punto della UI a difendersi.
func toMaterialsResponse(ms []games.Material) map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{"id": m.ID, "name": m.Name, "quantity": m.Quantity})
	}
	return map[string]any{"materials": out}
}

// requireGame risponde 404/500 e dice al chiamante se può proseguire. Le due
// rotte dei materiali cominciano entrambe così: un id inventato deve dare
// 404 prima di qualunque lavoro.
func (s *Server) requireGame(w http.ResponseWriter, r *http.Request, gameID int64) (games.Game, bool) {
	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "gioco non trovato")
		return games.Game{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return games.Game{}, false
	}
	return game, true
}

func (s *Server) listMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	ms, err := s.Games.ListMaterials(r.Context(), gameID)
	if err != nil {
		log.Printf("materials: list for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the materials")
		return
	}
	writeJSON(w, http.StatusOK, toMaterialsResponse(ms))
}

type materialsRequest struct {
	Materials []struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	} `json:"materials"`
}

func (s *Server) putMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	var req materialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo della richiesta non valido")
		return
	}
	in := make([]games.MaterialInput, 0, len(req.Materials))
	for _, m := range req.Materials {
		in = append(in, games.MaterialInput{Name: m.Name, Quantity: m.Quantity})
	}

	ms, err := s.Games.ReplaceMaterials(r.Context(), gameID, in)
	switch {
	case errors.Is(err, games.ErrMaterialInvalid):
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"ogni voce vuole un nome (fino a %d caratteri) e una quantità da 1 a %d",
			games.MaxMaterialNameChars, games.MaxMaterialQuantity))
	case errors.Is(err, games.ErrDuplicateMaterial):
		writeError(w, http.StatusBadRequest, "due voci con lo stesso nome: unisci le righe")
	case errors.Is(err, games.ErrTooManyMaterials):
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"al massimo %d voci per gioco", games.MaxMaterialsPerGame))
	case err != nil:
		log.Printf("materials: save for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not save the materials")
	default:
		writeJSON(w, http.StatusOK, toMaterialsResponse(ms))
	}
}

// materialKeywords sono le parole con cui i regolamenti italiani intitolano
// l'elenco dei pezzi. Una query per parola, come vuole Manuals.Search: con
// un OR unico una parola comune sommergerebbe una rara.
var materialKeywords = []string{"contenuto", "componenti", "materiale", "materiali", "scatola"}

// maxMaterialPassages tiene il prompt corto: l'elenco dei pezzi sta in una
// pagina, e sei passaggi la coprono con margine.
const maxMaterialPassages = 6

// materialLister restituisce il generatore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di suggester() in questions_handlers.go, e per la stessa
// ragione: cambiare modello non deve richiedere un riavvio.
func (s *Server) materialLister(ctx context.Context) ai.MaterialLister {
	if s.MaterialLister != nil {
		return s.MaterialLister
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("materials: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// baseLanguageCode è la lingua da preferire nella ricerca sul manuale: la
// stessa che la chat usa come preferenza.
func (s *Server) baseLanguageCode(ctx context.Context, gameID int64) string {
	langs, err := s.Games.ListLanguages(ctx, gameID)
	if err != nil {
		return ""
	}
	for _, l := range langs {
		if l.IsBaseLanguage {
			return l.LanguageCode
		}
	}
	return ""
}

func (s *Server) suggestMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	game, ok := s.requireGame(w, r, gameID)
	if !ok {
		return
	}

	hits, _, err := s.Manuals.Search(r.Context(), gameID,
		s.baseLanguageCode(r.Context(), gameID), materialKeywords)
	if err != nil {
		log.Printf("materials: search for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not search the manual")
		return
	}
	if len(hits) == 0 {
		// Stesso trattamento di errNoHeadings per le domande suggerite:
		// l'admin deve leggere cosa fare, non "riprova".
		writeError(w, http.StatusUnprocessableEntity,
			"Nel manuale indicizzato non ho trovato l'elenco dei componenti: indicizza un documento nella sezione Chatbot, o scrivi le voci a mano.")
		return
	}
	if len(hits) > maxMaterialPassages {
		hits = hits[:maxMaterialPassages]
	}
	passages := make([]string, 0, len(hits))
	for _, h := range hits {
		passages = append(passages, h.Text)
	}

	out, err := s.materialLister(r.Context()).ListMaterials(r.Context(), game.Name, passages)
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		writeError(w, http.StatusUnprocessableEntity,
			"Nessun provider AI configurato: controlla le impostazioni.")
		return
	case errors.Is(err, ai.ErrMaterialsRejected):
		writeError(w, http.StatusUnprocessableEntity,
			"Nel manuale non ho trovato un elenco di componenti leggibile: scrivi le voci a mano.")
		return
	case err != nil:
		log.Printf("materials: suggest for game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway, "Il provider AI non ha risposto: riprova.")
		return
	}

	// La proposta non si salva: la conferma è dell'admin, come per ogni
	// altro risultato automatico dell'app.
	rows := make([]map[string]any, 0, len(out))
	for _, m := range out {
		rows = append(rows, map[string]any{"name": m.Name, "quantity": m.Quantity})
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": rows})
}
