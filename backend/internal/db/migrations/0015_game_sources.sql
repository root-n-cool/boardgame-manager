DROP TRIGGER IF EXISTS manual_chunk_au;
DROP TRIGGER IF EXISTS manual_chunk_ad;
DROP TRIGGER IF EXISTS manual_chunk_ai;
DROP TABLE IF EXISTS manual_chunk_fts;
DROP TABLE IF EXISTS manual_chunk;
DROP TABLE IF EXISTS manual_page;

CREATE TABLE game_source_chunk (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,

    -- NULL per le fonti che non sono un media caricato (le FAQ di BGG).
    -- Quando c'è, la cascata porta via i chunk se il manuale si cancella:
    -- è l'unico modo di tenere quella garanzia senza costringere una FAQ a
    -- inventarsi una riga in game_media.
    game_media_id INTEGER REFERENCES game_media(id) ON DELETE CASCADE,

    -- Governa come si costruisce il link della citazione: un documento si
    -- apre da /api/uploads, una FAQ è già un URL.
    reference_type TEXT NOT NULL CHECK (reference_type IN ('document', 'faq')),

    -- Ciò che si cita: il nome del file per un documento, l'URL della
    -- conversazione per una FAQ.
    reference TEXT NOT NULL,

    -- Il punto dentro la fonte, e sempre nella forma che va scritta nella
    -- citazione: "pagina 3", 'sezione «Fase di Upkeep»', "commento del
    -- 15/07/2023 08:00". Nullable perché una fonte può non averne.
    reference_detail TEXT,

    -- Il titolo della sezione in cui il chunk cade, dal markdown. Vive solo
    -- qui: alimenta l'indice iniettato nel prompt, e NON esce nella risposta
    -- del tool (sezione 5.2).
    heading TEXT,

    language_code TEXT,       -- NULL per una FAQ
    seq INTEGER NOT NULL,     -- ordinale dentro la fonte, non dentro la pagina
    text TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_source_chunk_game ON game_source_chunk(game_id);
CREATE INDEX idx_source_chunk_ref ON game_source_chunk(game_media_id, seq);

-- FTS5 in external-content, come prima: indicizza senza duplicare il testo.
CREATE VIRTUAL TABLE game_source_chunk_fts USING fts5(
    text, content='game_source_chunk', content_rowid='id'
);
CREATE TRIGGER game_source_chunk_ai AFTER INSERT ON game_source_chunk BEGIN
    INSERT INTO game_source_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER game_source_chunk_ad AFTER DELETE ON game_source_chunk BEGIN
    INSERT INTO game_source_chunk_fts(game_source_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
END;
CREATE TRIGGER game_source_chunk_au AFTER UPDATE ON game_source_chunk BEGIN
    INSERT INTO game_source_chunk_fts(game_source_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
    INSERT INTO game_source_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;
