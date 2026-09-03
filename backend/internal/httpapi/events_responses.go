package httpapi

import (
	"context"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

func toEventSummary(e events.Event) map[string]any {
	return map[string]any{
		"id": e.ID, "title": e.Title, "description": e.Description,
		"eventDate": e.EventDate, "startTime": e.StartTime, "imagePath": e.ImagePath,
	}
}

// toEventListItem is the summary as the list endpoint sends it: same fields
// plus the number of games, which only ListEvents computes.
func toEventListItem(e events.Event) map[string]any {
	item := toEventSummary(e)
	item["gamesCount"] = e.GamesCount
	return item
}

func toEventGameSummary(eventGameID int64, g games.Game, copyIndex, seats, remaining int) map[string]any {
	return map[string]any{
		"eventGameId": eventGameID, "gameId": g.ID, "name": g.Name, "coverPath": g.CoverPath,
		"copyIndex": copyIndex, "seats": seats, "remaining": remaining,
	}
}

func (s *Server) toEventDetail(ctx context.Context, e events.Event) (map[string]any, error) {
	eventGames, err := s.Events.ListEventGames(ctx, e.ID)
	if err != nil {
		return nil, err
	}

	gamesOut := make([]map[string]any, 0, len(eventGames))
	for _, eg := range eventGames {
		game, err := s.Games.GetGame(ctx, eg.GameID)
		if err != nil {
			return nil, err
		}
		remaining, err := s.Events.RemainingCapacity(ctx, eg.ID)
		if err != nil {
			return nil, err
		}
		gamesOut = append(gamesOut, toEventGameSummary(eg.ID, game, eg.CopyIndex, eg.Seats, remaining))
	}

	detail := toEventSummary(e)
	detail["games"] = gamesOut
	return detail, nil
}

func toBookingResponse(b events.Booking) map[string]any {
	return map[string]any{
		"id": b.ID, "eventId": b.EventID, "eventGameId": b.EventGameID,
		"participantName": b.ParticipantName, "bookingCode": b.BookingCode, "status": b.Status,
	}
}

func (s *Server) toBookingDetailResponse(ctx context.Context, b events.Booking) (map[string]any, error) {
	resp := toBookingResponse(b)
	event, err := s.Events.GetEvent(ctx, b.EventID)
	if err != nil {
		return nil, err
	}
	eventGame, err := s.Events.GetEventGame(ctx, b.EventGameID)
	if err != nil {
		return nil, err
	}
	game, err := s.Games.GetGame(ctx, eventGame.GameID)
	if err != nil {
		return nil, err
	}
	resp["eventTitle"] = event.Title
	resp["eventDate"] = event.EventDate
	resp["startTime"] = event.StartTime
	resp["gameName"] = game.Name
	resp["copyIndex"] = eventGame.CopyIndex
	resp["seats"] = eventGame.Seats
	// Whether the copy number is worth showing at all: with one copy of the
	// game in the evening, "#1" would be noise.
	gameCopies, err := s.Events.CountEventGameCopies(ctx, b.EventID, eventGame.GameID)
	if err != nil {
		return nil, err
	}
	resp["gameCopies"] = gameCopies
	// Quante persone siedono a questo tavolo: la pagina pubblica lo usa per
	// dire che il punteggio è condiviso invece di far credere a ognuno di
	// avere il proprio.
	tableBookings, err := s.Events.CountActiveBookingsForEventGame(ctx, b.EventGameID)
	if err != nil {
		return nil, err
	}
	resp["tableBookings"] = tableBookings

	matchResult, err := s.Events.GetMatchResultForEventGame(ctx, b.EventGameID)
	if err != nil {
		return nil, err
	}
	if matchResult == nil {
		resp["matchResult"] = nil
	} else {
		resp["matchResult"] = toMatchResultResponse(*matchResult)
	}
	return resp, nil
}

func toBookingAdminResponse(b events.BookingWithGame) map[string]any {
	return map[string]any{
		"id": b.ID, "eventGameId": b.EventGameID, "gameId": b.GameID, "gameName": b.GameName,
		"copyIndex": b.CopyIndex, "seats": b.Seats,
		"participantName": b.ParticipantName, "participantEmail": b.ParticipantEmail,
		"participantPhone": b.ParticipantPhone,
		"createdAt":        b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toPlayerScores(players []events.PlayerScore) []map[string]any {
	out := make([]map[string]any, 0, len(players))
	for _, p := range players {
		out = append(out, map[string]any{"name": p.Name, "score": p.Score})
	}
	return out
}

func toMatchResultResponse(m events.MatchResult) map[string]any {
	return map[string]any{"players": toPlayerScores(m.Players)}
}

func toEventGameMatchResultResponse(m events.EventGameMatchResult) map[string]any {
	return map[string]any{
		"eventGameId": m.EventGameID, "gameId": m.GameID, "gameName": m.GameName,
		"copyIndex": m.CopyIndex, "players": toPlayerScores(m.Players),
	}
}
