package httpapi

import (
	"context"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

func toEventSummary(e events.Event) map[string]any {
	return map[string]any{
		"id": e.ID, "title": e.Title, "description": e.Description,
		"eventDate": e.EventDate, "startTime": e.StartTime, "endTime": e.EndTime, "imagePath": e.ImagePath,
		"venue": toVenueResponse(e.Venue),
	}
}

// toVenueResponse manda il luogo, o null se l'evento non ne ha uno. Le
// coordinate restano puntatori: la mappa si disegna solo quando ci sono.
func toVenueResponse(v *events.Venue) map[string]any {
	if v == nil {
		return nil
	}
	return map[string]any{"name": v.Name, "address": v.Address, "lat": v.Lat, "lon": v.Lon}
}

// toEventListItem is the summary as the list endpoint sends it: same fields
// plus the number of games, which only ListEvents computes.
func toEventListItem(e events.Event) map[string]any {
	item := toEventSummary(e)
	item["gamesCount"] = e.GamesCount
	return item
}

func toEventGameSummary(eventGameID int64, g games.Game, copyIndex, seats, remaining int, bookable bool, chat map[string]bool) map[string]any {
	return map[string]any{
		"eventGameId": eventGameID, "gameId": g.ID, "name": g.Name, "coverPath": g.CoverPath,
		"copyIndex": copyIndex, "seats": seats, "remaining": remaining, "weight": g.Weight,
		"bookable": bookable,
		// chat dice se il link "Chiedi al Mentore" ha una chat dietro, e
		// quali agenti: compare se almeno uno dei due è vero. Senza questo
		// il link compariva su ogni gioco e, su uno senza manuale preparato
		// né forum, portava a una scheda dove non succedeva niente: nessun
		// messaggio, nessuna spiegazione. Al tavolo, con le carte in mano, un
		// link che non fa niente si legge come un'app rotta, non come una
		// funzione assente.
		"chat": chat,
	}
}

func (s *Server) toEventDetail(ctx context.Context, e events.Event) (map[string]any, error) {
	eventGames, err := s.Events.ListEventGames(ctx, e.ID)
	if err != nil {
		return nil, err
	}

	// One grouped query for occupancy across every copy, instead of
	// RemainingCapacity called once per copy: this endpoint is the one every
	// participant hits, and a copy is a row, not a game.
	occupied, err := s.Events.ActiveBookingCountsByEventGame(ctx, e.ID)
	if err != nil {
		return nil, err
	}

	// Quali giochi della serata hanno un manuale preparato, in UNA query per
	// tutta la risposta, e il provider AI + Tavily letti una volta sola:
	// sono le condizioni di chatAvailability, e nessuna deve costare una
	// richiesta per gioco.
	aiOK := s.aiConfigured(ctx)
	tavilyOK := s.tavilyConfigured(ctx)
	gameIDs := make([]int64, 0, len(eventGames))
	for _, eg := range eventGames {
		gameIDs = append(gameIDs, eg.GameID)
	}
	withManual := map[int64]bool{}
	if s.Manuals != nil && aiOK {
		if got, err := s.Manuals.GamesWithChunks(ctx, gameIDs); err == nil {
			withManual = got
		}
	}

	// gameCache fetches each distinct game once however many copies it has —
	// two copies of the same game used to mean two GetGame calls for nothing.
	gameCache := map[int64]games.Game{}
	gamesOut := make([]map[string]any, 0, len(eventGames))
	for _, eg := range eventGames {
		game, ok := gameCache[eg.GameID]
		if !ok {
			game, err = s.Games.GetGame(ctx, eg.GameID)
			if err != nil {
				return nil, err
			}
			gameCache[eg.GameID] = game
		}
		remaining := eg.Seats - occupied[eg.ID]
		rules, strategy := chatAvailability(aiOK, withManual[eg.GameID], tavilyOK && hasBGGID(game))
		gamesOut = append(gamesOut, toEventGameSummary(
			eg.ID, game, eg.CopyIndex, eg.Seats, remaining, eg.Bookable,
			map[string]bool{"rules": rules, "strategy": strategy}))
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
	resp["gameId"] = game.ID
	resp["gameName"] = game.Name
	// Stessa regola della scheda evento: il link "Chiedi al Mentore" compare
	// solo se dietro c'è davvero una chat. Qui il gioco è uno solo, quindi
	// basta la condizione presa direttamente.
	aiOK := s.aiConfigured(ctx)
	hasChunks := false
	if s.Manuals != nil && aiOK {
		if has, err := s.Manuals.HasChunks(ctx, game.ID); err == nil {
			hasChunks = has
		}
	}
	rules, strategy := chatAvailability(aiOK, hasChunks, s.tavilyConfigured(ctx) && hasBGGID(game))
	resp["chat"] = map[string]bool{"rules": rules, "strategy": strategy}
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
		"participantName": b.ParticipantName,
		"createdAt":       b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
