-- La tipologia del gioco (da tavolo, di ruolo, ...) e il flag che lo toglie
-- dal catalogo pubblico. Niente CHECK sui valori di kind: l'elenco delle
-- tipologie ammesse vive in Go (games.Kinds) e crescerà, e in SQLite
-- cambiare un CHECK vuol dire ricostruire la tabella.
ALTER TABLE games ADD COLUMN kind TEXT NOT NULL DEFAULT 'board';
ALTER TABLE games ADD COLUMN hidden_from_catalog INTEGER NOT NULL DEFAULT 0;

-- Il DEFAULT riempie già le righe esistenti: l'UPDATE lo dice in chiaro.
-- Un gioco senza tipologia è sempre stato un gioco da tavolo.
UPDATE games SET kind = 'board' WHERE kind IS NULL OR kind = '';
