package httpapi

import (
	"context"
	"log"
	"strings"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// toGameSummary: incomplete è un *bool e non un bool perché la sua assenza
// è significativa. Le rotte dei giochi sono pubbliche, e a un visitatore
// non si racconta che a una scatola mancano pezzi: senza sessione il campo
// non esce affatto, e la risposta resta identica a quella di prima.
func toGameSummary(g games.Game, incomplete *bool) map[string]any {
	out := map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "weight": g.Weight,
		"owner": g.Owner, "coverPath": g.CoverPath, "seats": g.Seats,
		// canTranslate dice che esiste un originale BGG da cui ritradurre.
		// Esce il booleano, non il testo: la scheda di modifica deve solo
		// sapere se il bottone ha una sorgente.
		"canTranslate": g.BGGDescription != nil && *g.BGGDescription != "",
	}
	if incomplete != nil {
		out["incomplete"] = *incomplete
	}
	return out
}

// toMissingPiecesResponse è la forma che l'avviso sulla scheda legge.
// `since` esce in RFC3339 come ogni altra data dell'API.
func toMissingPiecesResponse(pieces []events.MissingPiece) []map[string]any {
	out := make([]map[string]any, 0, len(pieces))
	for _, p := range pieces {
		out = append(out, map[string]any{
			"name": p.Name, "expected": p.Expected, "returned": p.Returned,
			"since": p.Since.Format(isoTime),
		})
	}
	return out
}

// toMediaResponse porta anche indexedChunks, quanti chunk il Task 5 ha
// salvato per questo media: 0 quando il media non è (ancora, o più)
// indicizzato. Il pannello admin lo usa per mostrare se un manuale è
// pronto per la chat senza dover chiamare una rotta a parte per gioco.
func toMediaResponse(m games.GameMedia, chunksByMedia map[int64]int) map[string]any {
	return map[string]any{
		"id": m.ID, "type": m.Type, "url": m.URLOrPath, "title": m.Title,
		"indexedChunks": chunksByMedia[m.ID],
	}
}

func (s *Server) toGameDetail(ctx context.Context, g games.Game, langs []games.GameLanguage, signedIn bool) (map[string]any, error) {
	// summary alimenta chatAvailability sotto (HasChunks) e PerMedia
	// alimenta indexedChunks di ogni media, per il pannello admin.
	// Chiederli con una seconda query costerebbe un giro in più alla
	// pagina che ogni partecipante apre.
	summary := manuals.SourceSummary{}
	if s.Manuals != nil {
		if got, err := s.Manuals.Summary(ctx, g.ID); err == nil {
			summary = got
		}
	}

	langOut := make([]map[string]any, 0, len(langs))
	for _, l := range langs {
		media, err := s.Games.ListMedia(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		mediaOut := make([]map[string]any, 0, len(media))
		for _, m := range media {
			mediaOut = append(mediaOut, toMediaResponse(m, summary.PerMedia))
		}
		langOut = append(langOut, map[string]any{
			"code": l.LanguageCode, "isBaseLanguage": l.IsBaseLanguage,
			"name": l.Name, "description": l.Description, "media": mediaOut,
		})
	}
	detail := toGameSummary(g, nil)
	detail["languages"] = langOut
	// chat governa la comparsa della chat e quali voci del selettore sono
	// attive: la regola è una sola (chatAvailability), la stessa dell'handler.
	rules, strategy := chatAvailability(s.aiConfigured(ctx), summary.HasChunks,
		hasBGGID(g) && s.tavilyConfigured(ctx))
	detail["chat"] = map[string]bool{"rules": rules, "strategy": strategy}
	// missingPieces racconta cosa non è tornato: solo a chi ha una
	// sessione, per lo stesso motivo di incomplete in toGameSummary.
	if signedIn {
		pieces, err := s.Events.GamesMissingPieces(ctx, []int64{g.ID})
		if err != nil {
			return nil, err
		}
		detail["missingPieces"] = toMissingPiecesResponse(pieces[g.ID])
	}
	// Le domande suggerite, già formulate, per agente: sempre due array,
	// mai null. Prima qui uscivano i titoli di sezione (`sourceHeadings`) e
	// il frontend li trasformava in domande con una tabella fissa: quella
	// catena produceva "Cosa dice il manuale su di Klaus-Jürgen Wrede?" su
	// un manuale reale, perché prendeva i primi titoli in ordine di pagina
	// — copertina e contenuto della scatola — e ripiegava su un template
	// per tutto ciò che la tabella non conosceva.
	detail["suggestedQuestions"] = map[string][]string{
		"rules":    s.publicQuestions(ctx, g.ID, manuals.AgentRules),
		"strategy": s.publicQuestions(ctx, g.ID, manuals.AgentStrategy),
	}
	return detail, nil
}

// publicQuestions restituisce le domande NON vuote di un agente, sempre come
// array (mai nil): il pannello pubblico ripiega sulle sue fisse quando la
// lista è vuota. Un errore non costa la scheda: si logga e si ripiega.
func (s *Server) publicQuestions(ctx context.Context, gameID int64, agent string) []string {
	out := []string{}
	if s.Manuals == nil {
		return out
	}
	qs, err := s.Manuals.SuggestedQuestions(ctx, gameID, agent)
	if err != nil {
		log.Printf("game detail: %s questions for game %d: %v", agent, gameID, err)
		return out
	}
	for _, q := range qs {
		if strings.TrimSpace(q.Text) != "" {
			out = append(out, q.Text)
		}
	}
	return out
}
