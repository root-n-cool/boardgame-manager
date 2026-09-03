package db_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/db"
)

func TestMigrate_CreatesExpectedTables(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, table := range []string{"users", "sessions", "app_settings"} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("second migrate should be a no-op, got error: %v", err)
	}
}

func TestMigrate_EventGamesHasCopiesAndSeats(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Due copie dello stesso gioco nello stesso evento: era vietato dal
	// vecchio UNIQUE(event_id, game_id), ora è il caso normale.
	if _, err := conn.Exec(`INSERT INTO games (name, seats) VALUES ('D&D', 5)`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO events (title, event_date, start_time) VALUES ('Serata', '2026-10-01', '20:00')`); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO event_games (event_id, game_id, copy_index, seats) VALUES (1, 1, 1, 5), (1, 1, 2, 5)`); err != nil {
		t.Fatalf("insert two copies: %v", err)
	}

	var seats int
	if err := conn.QueryRow(`SELECT seats FROM games WHERE id = 1`).Scan(&seats); err != nil {
		t.Fatalf("read game seats: %v", err)
	}
	if seats != 5 {
		t.Fatalf("expected seats 5, got %d", seats)
	}

	// Il risultato partita ora appartiene alla copia, non alla prenotazione.
	if _, err := conn.Exec(`INSERT INTO match_results (event_game_id) VALUES (1)`); err != nil {
		t.Fatalf("insert match result on event_game: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO match_results (event_game_id) VALUES (1)`); err == nil {
		t.Fatal("expected a UNIQUE violation on event_game_id")
	}
}

func TestMigrate_GamesSeatsDefaultsToOne(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO games (name) VALUES ('Catan')`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	var seats int
	if err := conn.QueryRow(`SELECT seats FROM games WHERE name = 'Catan'`).Scan(&seats); err != nil {
		t.Fatalf("read seats: %v", err)
	}
	if seats != 1 {
		t.Fatalf("expected default seats 1, got %d", seats)
	}
}
