package leaderboard_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/leaderboard"
)

func newTestStores(t *testing.T) (*events.Store, *games.Store, *leaderboard.Store) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return events.NewStore(conn), games.NewStore(conn), leaderboard.NewStore(conn)
}

func TestGetLeaderboard_AggregatesWinsAndAveragesAcrossEvents(t *testing.T) {
	eventStore, gameStore, lbStore := newTestStores(t)
	ctx := context.Background()

	game, err := gameStore.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	event1, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata 1",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: game.ID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event 1: %v", err)
	}
	event1Games, _ := eventStore.ListEventGames(ctx, event1.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	booking1, err := eventStore.CreateBooking(ctx, event1.ID, event1Games[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("create booking 1: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, booking1.ID, booking1.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 50}, {Name: "luigi", Score: 30}}); err != nil {
		t.Fatalf("submit match 1: %v", err)
	}

	event2, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata 2",
		EventDate: "2026-11-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: game.ID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event 2: %v", err)
	}
	event2Games, _ := eventStore.ListEventGames(ctx, event2.ID)
	booking2, err := eventStore.CreateBooking(ctx, event2.ID, event2Games[0].ID, "Luigi Verdi", "luigi2@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("create booking 2: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, booking2.ID, booking2.BookingCode,
		[]events.PlayerScore{{Name: "Luigi", Score: 70}, {Name: "Mario", Score: 20}}); err != nil {
		t.Fatalf("submit match 2: %v", err)
	}

	lb, err := lbStore.GetLeaderboard(ctx, game.ID)
	if err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if len(lb.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(lb.Matches))
	}
	if len(lb.Players) != 2 {
		t.Fatalf("expected 2 distinct normalized players (mario/luigi case-insensitive), got %d: %+v", len(lb.Players), lb.Players)
	}

	byName := map[string]leaderboard.PlayerStats{}
	for _, p := range lb.Players {
		byName[strings.ToLower(p.Name)] = p
	}
	mario := byName["mario"]
	if mario.GamesPlayed != 2 || mario.Wins != 1 || mario.TotalScore != 70 {
		t.Fatalf("unexpected mario stats: %+v", mario)
	}
	luigi := byName["luigi"]
	if luigi.GamesPlayed != 2 || luigi.Wins != 1 || luigi.TotalScore != 100 {
		t.Fatalf("unexpected luigi stats: %+v", luigi)
	}

	// Ordered by wins desc, then average score desc; both have 1 win here, so
	// luigi (avg 50) must come before mario (avg 35).
	if lb.Players[0].Name != luigi.Name {
		t.Fatalf("expected luigi first (higher average with same wins), got %+v", lb.Players)
	}
}

func TestGetLeaderboard_TiedTopScoreBothCountAsWinners(t *testing.T) {
	eventStore, gameStore, lbStore := newTestStores(t)
	ctx := context.Background()
	game, err := gameStore.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: game.ID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 40}, {Name: "Luigi", Score: 40}}); err != nil {
		t.Fatalf("submit match: %v", err)
	}

	lb, err := lbStore.GetLeaderboard(ctx, game.ID)
	if err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if len(lb.Matches) != 1 || len(lb.Matches[0].Players) != 2 {
		t.Fatalf("unexpected matches: %+v", lb.Matches)
	}
	for _, p := range lb.Matches[0].Players {
		if !p.IsWinner {
			t.Fatalf("expected both tied players to be winners, got %+v", lb.Matches[0].Players)
		}
	}
	for _, p := range lb.Players {
		if p.Wins != 1 {
			t.Fatalf("expected both players to have 1 win from the tie, got %+v", p)
		}
	}
}

func TestGetLeaderboard_CountsATableOnce(t *testing.T) {
	eventStore, gameStore, lbStore := newTestStores(t)
	ctx := context.Background()

	game, err := gameStore.CreateGame(ctx, games.Game{Name: "D&D", Seats: 5})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: game.ID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// Cinque prenotazioni sullo stesso tavolo, ognuna che manda lo stesso
	// risultato: la partita giocata resta una.
	for i := 0; i < 5; i++ {
		b, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
			fmt.Sprintf("G%d", i), fmt.Sprintf("g%d@example.com", i),
			fmt.Sprintf("33300000%02d", i), now)
		if err != nil {
			t.Fatalf("booking %d: %v", i, err)
		}
		if _, err := eventStore.SubmitMatchResult(ctx, b.ID, b.BookingCode,
			[]events.PlayerScore{{Name: "Mario", Score: 50}, {Name: "Luigi", Score: 30}}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	lb, err := lbStore.GetLeaderboard(ctx, game.ID)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(lb.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lb.Matches))
	}
	for _, p := range lb.Players {
		if p.GamesPlayed != 1 {
			t.Fatalf("expected 1 game for %s, got %d", p.Name, p.GamesPlayed)
		}
	}
}
