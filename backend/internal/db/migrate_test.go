package db_test

import (
	"context"
	"database/sql"
	"os"
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

func TestMigration0020_AppliesToAnAlreadyPopulatedBookingsTable(t *testing.T) {
	// sql.Open directly (not db.Open): this test doesn't need foreign_keys
	// enforcement, and skipping it lets the row below use arbitrary
	// event_id/event_game_id values without needing real events/event_games
	// rows — the point of this test is the ALTER TABLE behavior on
	// `bookings` itself, not the rest of the schema.
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	// The bookings schema exactly as migration 0008 left it — the last
	// shape it had before 0020 — with one row already in it, simulating
	// a real installation upgrading to this branch.
	if _, err := conn.Exec(`
		CREATE TABLE bookings (
		    id INTEGER PRIMARY KEY AUTOINCREMENT,
		    event_id INTEGER NOT NULL,
		    event_game_id INTEGER NOT NULL,
		    participant_name TEXT NOT NULL,
		    participant_email TEXT NOT NULL,
		    participant_phone TEXT NOT NULL,
		    booking_code TEXT NOT NULL UNIQUE,
		    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled')),
		    created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		t.Fatalf("create pre-0020 bookings table: %v", err)
	}
	if _, err := conn.Exec(`
		CREATE UNIQUE INDEX idx_one_active_booking_per_phone_per_event
		    ON bookings(event_id, participant_phone) WHERE status = 'active'`); err != nil {
		t.Fatalf("create pre-0020 index: %v", err)
	}
	if _, err := conn.Exec(`
		INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		VALUES (1, 1, 'Mario Rossi', 'mario@example.com', '3331234567', 'ABCD1234', 'active')`); err != nil {
		t.Fatalf("insert pre-existing booking: %v", err)
	}

	migrationSQL, err := os.ReadFile("migrations/0020_prenotazioni_senza_contatti.sql")
	if err != nil {
		t.Fatalf("read migration 0020: %v", err)
	}
	if _, err := conn.Exec(string(migrationSQL)); err != nil {
		t.Fatalf("migration 0020 must apply to a bookings table that already has rows, got: %v", err)
	}

	var name, termsAcceptedAt string
	if err := conn.QueryRow(`SELECT participant_name, terms_accepted_at FROM bookings WHERE booking_code = 'ABCD1234'`).Scan(&name, &termsAcceptedAt); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if name != "Mario Rossi" {
		t.Fatalf("expected the pre-existing row to survive the migration, got name %q", name)
	}
	if termsAcceptedAt == "" {
		t.Fatal("expected terms_accepted_at to be backfilled for a pre-existing row, got empty string")
	}

	// participant_email/phone and the old index must actually be gone.
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('bookings') WHERE name IN ('participant_email', 'participant_phone')`).Scan(&count); err != nil {
		t.Fatalf("check dropped columns: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected participant_email/participant_phone to be dropped, found %d matching columns", count)
	}
}
