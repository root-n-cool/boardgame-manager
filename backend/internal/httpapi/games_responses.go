package httpapi

import (
	"context"

	"boardgames-manager/internal/games"
)

func toGameSummary(g games.Game) map[string]any {
	return map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "owner": g.Owner, "coverPath": g.CoverPath,
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
	return detail, nil
}
