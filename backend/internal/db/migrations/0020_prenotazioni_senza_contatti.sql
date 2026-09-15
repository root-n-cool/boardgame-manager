-- I contatti di chi prenota non servono più a niente lato server: il
-- telefono esisteva solo per il vincolo "una prenotazione attiva per
-- telefono per evento" (sostituito da un promemoria nel browser di chi
-- prenota, non più un vincolo server), e l'email non viene più
-- persistita — si usa solo al volo per la mail di conferma. Il
-- consenso a termini/privacy, prima non richiesto, ora è obbligatorio;
-- la prova resta sul booking come timestamp (nessun codice Go la legge
-- indietro). Il default della colonna è la stringa vuota e non
-- datetime('now'): SQLite rifiuta un default non costante su una ALTER
-- TABLE ADD COLUMN se la tabella ha già righe, quindi le righe
-- preesistenti si riempiono con l'UPDATE qui sotto e le nuove ricevono
-- il timestamp esplicitamente dall'INSERT di CreateBooking.
DROP INDEX idx_one_active_booking_per_phone_per_event;
ALTER TABLE bookings DROP COLUMN participant_phone;
ALTER TABLE bookings DROP COLUMN participant_email;
ALTER TABLE bookings ADD COLUMN terms_accepted_at TEXT NOT NULL DEFAULT '';
UPDATE bookings SET terms_accepted_at = datetime('now') WHERE terms_accepted_at = '';
