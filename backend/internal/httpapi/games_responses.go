package httpapi

import (
	"context"
	"log"
	"strings"

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

func (s *Server) toGameDetail(ctx context.Context, g games.Game, langs []games.GameLanguage) (map[string]any, error) {
	// canAsk governa la comparsa della chat sulla scheda pubblica. Vero solo
	// se entrambe le condizioni valgono: provider AI configurato e almeno
	// una fonte indicizzata. In UI non esiste il pulsante disabilitato con
	// la spiegazione: se è falso, la chat non c'è.
	//
	// PerMedia alimenta indexedChunks di ogni media, per il pannello admin.
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
	detail := toGameSummary(g)
	detail["languages"] = langOut
	detail["canAsk"] = summary.HasChunks && s.aiConfigured(ctx)
	// Le tre domande suggerite, già formulate. Prima qui uscivano i titoli
	// di sezione (`sourceHeadings`) e il frontend li trasformava in domande
	// con una tabella fissa: quella catena produceva "Cosa dice il manuale
	// su di Klaus-Jürgen Wrede?" su un manuale reale, perché prendeva i
	// primi titoli in ordine di pagina — copertina e contenuto della
	// scatola — e ripiegava su un template per tutto ciò che la tabella non
	// conosceva.
	//
	// Solo le domande NON vuote: il pannello pubblico ripiega sulle sue tre
	// domande fisse quando la lista è vuota, e tre stringhe vuote non sono
	// una lista vuota. Sempre un array, mai null.
	questions := []string{}
	if qs, err := s.Manuals.SuggestedQuestions(ctx, g.ID); err != nil {
		// Un errore qui non deve costare la scheda del gioco: senza
		// domande suggerite il frontend usa le sue tre fisse.
		log.Printf("game detail: suggested questions for game %d: %v", g.ID, err)
	} else {
		for _, q := range qs {
			if strings.TrimSpace(q.Text) != "" {
				questions = append(questions, q.Text)
			}
		}
	}
	detail["suggestedQuestions"] = questions
	return detail, nil
}
