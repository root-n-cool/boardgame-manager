-- Non tutti i giochi di una serata vanno a prenotazione: un filler come
-- Love Letter resta sul tavolo per chi arriva a mani vuote. DEFAULT 1
-- lascia identico tutto quello che c'è già in archivio.
ALTER TABLE event_games ADD COLUMN bookable INTEGER NOT NULL DEFAULT 1
    CHECK (bookable IN (0, 1));

-- Il registro di chi ha in mano cosa. Non c'è una colonna di stato: lo
-- stato è la data che manca, returned_at IS NULL significa "fuori".
-- borrower_name e borrower_phone si copiano sulla riga anche quando il
-- prestito nasce da una prenotazione, così il registro si legge per
-- intero anche se quella prenotazione viene poi annullata.
CREATE TABLE game_loans (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    event_game_id  INTEGER NOT NULL REFERENCES event_games(id) ON DELETE CASCADE,
    booking_id     INTEGER REFERENCES bookings(id) ON DELETE SET NULL,
    borrower_name  TEXT NOT NULL,
    borrower_phone TEXT NOT NULL,
    notes          TEXT,
    lent_at        TEXT NOT NULL DEFAULT (datetime('now')),
    returned_at    TEXT
);

-- Una copia sola può essere fuori una volta sola. Il vincolo sta qui e
-- non nel codice: due consegne simultanee sulla stessa copia non devono
-- poter passare entrambe.
CREATE UNIQUE INDEX idx_one_open_loan_per_copy
    ON game_loans(event_game_id) WHERE returned_at IS NULL;
