-- Il luogo della serata. Tutte le colonne sono nullable: gli eventi già
-- creati non ne hanno uno, e il luogo resta facoltativo anche dopo.
-- Le coordinate arrivano dalla ricerca su OpenStreetMap e possono mancare
-- anche quando l'indirizzo c'è: chi lo scrive a mano ottiene una riga di
-- testo sulla pagina pubblica, senza mappa.
ALTER TABLE events ADD COLUMN venue_name TEXT;
ALTER TABLE events ADD COLUMN venue_address TEXT;
ALTER TABLE events ADD COLUMN venue_lat REAL;
ALTER TABLE events ADD COLUMN venue_lon REAL;
