package httpapi

import (
	"context"

	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

func toGameSummary(g games.Game) map[string]any {
	return map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "weight": g.Weight,
		"owner": g.Owner, "coverPath": g.CoverPath, "seats": g.Seats,
		// canTranslate dice che esiste un originale BGG da cui ritradurre.
		// Esce il booleano, non il testo: la scheda di modifica deve solo
		// sapere se il bottone ha una sorgente.
		"canTranslate": g.BGGDescription != nil && *g.BGGDescription != "",
	}
}

func toMediaResponse(m games.GameMedia) map[string]any {
	return map[string]any{"id": m.ID, "type": m.Type, "url": m.URLOrPath, "title": m.Title}
}

func (s *Server) toGameDetail(ctx context.Context, g games.Game, langs []games.GameLanguage) (map[string]any, error) {
	langOut := make([]map[string]any, 0, len(langs))
	for _, l := range langs {
		media, err := s.Games.ListMedia(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		mediaOut := make([]map[string]any, 0, len(media))
		for _, m := range media {
			mediaOut = append(mediaOut, toMediaResponse(m))
		}
		langOut = append(langOut, map[string]any{
			"code": l.LanguageCode, "isBaseLanguage": l.IsBaseLanguage,
			"name": l.Name, "description": l.Description, "media": mediaOut,
		})
	}
	detail := toGameSummary(g)
	detail["languages"] = langOut
	// canAsk governa la comparsa della chat sulla scheda pubblica. Vero solo
	// se entrambe le condizioni valgono: provider AI configurato e manuale
	// preparato. In UI non esiste il pulsante disabilitato con la
	// spiegazione: se è falso, la chat non c'è.
	//
	// manualHeadings esce dalla STESSA lettura: sono i titoli di sezione del
	// manuale, da cui la chat costruisce le domande suggerite ("Cosa dice il
	// manuale su «Fase di Upkeep»?"). Chiederli con una seconda query
	// costerebbe un giro in più alla pagina che ogni partecipante apre.
	summary := manuals.ManualSummary{}
	if s.Manuals != nil {
		if got, err := s.Manuals.Summary(ctx, g.ID); err == nil {
			summary = got
		}
	}
	detail["canAsk"] = summary.HasPages && s.aiConfigured(ctx)
	// Sempre un array, mai null: il frontend lo tratta come lista.
	headings := summary.Headings
	if headings == nil {
		headings = []string{}
	}
	detail["manualHeadings"] = headings
	return detail, nil
}
