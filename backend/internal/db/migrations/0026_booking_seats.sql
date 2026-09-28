-- Quanti posti prenotabili occupa una prenotazione. Su un tavolo aperto una
-- persona può prenderne più d'uno (per sé e per chi arriva con lei) con un
-- codice solo; la capienza si conta come somma di questa colonna, non più
-- come numero di righe. Le prenotazioni esistenti valevano un posto.
ALTER TABLE bookings ADD COLUMN seats_reserved INTEGER NOT NULL DEFAULT 1 CHECK (seats_reserved > 0);
