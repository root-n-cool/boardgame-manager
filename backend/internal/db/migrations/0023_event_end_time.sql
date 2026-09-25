-- L'orario di fine della serata, facoltativo: gli eventi già creati non ce
-- l'hanno e l'admin può continuare a non indicarlo. Una fine che precede
-- l'inizio (21:00–01:00) cade il giorno dopo: nessuna colonna di data in più.
ALTER TABLE events ADD COLUMN end_time TEXT;
