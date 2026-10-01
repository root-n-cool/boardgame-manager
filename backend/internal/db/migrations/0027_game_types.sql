-- Le tipologie di gioco escono dal codice e diventano una tabella che
-- l'admin gestisce. games.kind resta (le migrazioni vanno solo avanti) ma
-- da qui in poi nessuno la legge: la tipologia è games.game_type_id.
-- Niente REFERENCES su game_type_id: con foreign_keys attivo SQLite non
-- accetta un ADD COLUMN con foreign key e default non NULL. L'integrità
-- la tiene lo store (blocco dell'eliminazione, validazione dell'id).
CREATE TABLE game_types (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  slug       TEXT    NOT NULL UNIQUE,
  bgg_search INTEGER NOT NULL DEFAULT 1,
  position   INTEGER NOT NULL,
  created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO game_types (id, name, slug, bgg_search, position) VALUES
  (1, 'Gioco da tavolo', 'GDT', 1, 1),
  (2, 'Gioco di ruolo',  'GDR', 0, 2);

ALTER TABLE games ADD COLUMN game_type_id INTEGER NOT NULL DEFAULT 1;
UPDATE games SET game_type_id = 2 WHERE kind = 'rpg';
