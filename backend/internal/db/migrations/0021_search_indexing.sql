-- Il sito si raggiunge per QR code alle serate, non da una ricerca: di
-- default chiede ai motori di ricerca di non indicizzarlo (meta robots
-- nelle pagine, X-Robots-Tag nelle risposte API). L'admin può spegnere
-- il divieto dalle impostazioni se vuole farsi trovare.
ALTER TABLE app_settings ADD COLUMN hide_from_search_engines INTEGER NOT NULL DEFAULT 1;
