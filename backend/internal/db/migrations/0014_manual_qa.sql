-- Trascrizione del manuale, una riga per pagina. È modificabile dall'admin
-- e resta la fonte di verità: non è una cache dell'estrazione, che infatti
-- non viene mai rieseguita da sola.
CREATE TABLE manual_page (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_media_id INTEGER NOT NULL REFERENCES game_media(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL,
    text TEXT NOT NULL,
    -- Il titolo di sezione rilevato, se c'è: alimenta l'indice iniettato
    -- nel contesto del modello.
    heading TEXT,
    source TEXT NOT NULL CHECK (source IN ('pdf_text', 'vision', 'manual')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(game_media_id, page_number)
);

-- I chunk cercabili, derivati dalle pagine: rigenerabili in qualunque
-- momento senza perdere niente di inserito a mano.
CREATE TABLE manual_chunk (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    game_media_id INTEGER NOT NULL REFERENCES game_media(id) ON DELETE CASCADE,
    language_code TEXT NOT NULL,
    page_number INTEGER NOT NULL,
    -- Ordinale del chunk dentro la pagina, per allegare il vicino.
    seq INTEGER NOT NULL,
    text TEXT NOT NULL
);

-- game_id è denormalizzato di proposito: la ricerca filtra sempre per gioco,
-- così è un indice B-tree invece di due join.
CREATE INDEX idx_manual_chunk_game ON manual_chunk(game_id);
CREATE INDEX idx_manual_chunk_page ON manual_chunk(game_media_id, page_number, seq);

-- Indice FTS5 in external-content: indicizza manual_chunk senza duplicarne
-- il testo. I tre trigger lo tengono in pari.
CREATE VIRTUAL TABLE manual_chunk_fts USING fts5(
    text, content='manual_chunk', content_rowid='id'
);
CREATE TRIGGER manual_chunk_ai AFTER INSERT ON manual_chunk BEGIN
    INSERT INTO manual_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER manual_chunk_ad AFTER DELETE ON manual_chunk BEGIN
    INSERT INTO manual_chunk_fts(manual_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
END;
CREATE TRIGGER manual_chunk_au AFTER UPDATE ON manual_chunk BEGIN
    INSERT INTO manual_chunk_fts(manual_chunk_fts, rowid, text)
        VALUES ('delete', old.id, old.text);
    INSERT INTO manual_chunk_fts(rowid, text) VALUES (new.id, new.text);
END;

-- Modello multimodale per la trascrizione dei manuali scansionati. Vuoto =
-- nessuna trascrizione automatica, l'admin scrive il testo a mano. Il
-- modello di chat (ai_model) può essere solo-testo, quindi serve un campo
-- separato: deepseek-v4-flash non accetta immagini, la sua variante
-- deepseek-v4-flash-vision-exp sì.
ALTER TABLE app_settings ADD COLUMN ai_vision_model TEXT;
