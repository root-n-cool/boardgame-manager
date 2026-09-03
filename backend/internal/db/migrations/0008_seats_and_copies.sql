-- Posti prenotabili per copia. 1 = chi prenota si prende la copia (il
-- comportamento di sempre); più di 1 = tavolo aperto, dove ogni posto si
-- prenota a sé.
ALTER TABLE games ADD COLUMN seats INTEGER NOT NULL DEFAULT 1 CHECK (seats > 0);

-- event_games passa da "una riga con quantità" a "una riga per copia":
-- il vecchio UNIQUE(event_id, game_id) va togliuto, e in SQLite un vincolo
-- di tabella non si rimuove con ALTER. Le prenotazioni e i punteggi finora
-- registrati sono dati di sviluppo, quindi le quattro tabelle della catena
-- si ricreano vuote, in ordine inverso di dipendenza per non innescare
-- cascate su tabelle ancora vive. Gli eventi e il catalogo giochi restano:
-- gli eventi esistenti vanno solo ripopolati di giochi dalla loro scheda.
DROP TABLE match_player_scores;
DROP TABLE match_results;
DROP TABLE bookings;
DROP TABLE event_games;

CREATE TABLE event_games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    game_id INTEGER NOT NULL REFERENCES games(id),
    copy_index INTEGER NOT NULL CHECK (copy_index > 0),
    seats INTEGER NOT NULL CHECK (seats > 0),
    UNIQUE(event_id, game_id, copy_index)
);

CREATE TABLE bookings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    event_game_id INTEGER NOT NULL REFERENCES event_games(id) ON DELETE CASCADE,
    participant_name TEXT NOT NULL,
    participant_email TEXT NOT NULL,
    participant_phone TEXT NOT NULL,
    booking_code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled')),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX idx_one_active_booking_per_phone_per_event
    ON bookings(event_id, participant_phone) WHERE status = 'active';

-- Il risultato è del tavolo, non della prenotazione: con sei prenotazioni
-- sullo stesso tavolo la classifica deve contare una partita, non sei.
CREATE TABLE match_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_game_id INTEGER NOT NULL UNIQUE REFERENCES event_games(id) ON DELETE CASCADE,
    submitted_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE match_player_scores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    match_result_id INTEGER NOT NULL REFERENCES match_results(id) ON DELETE CASCADE,
    player_name TEXT NOT NULL,
    score INTEGER NOT NULL
);
