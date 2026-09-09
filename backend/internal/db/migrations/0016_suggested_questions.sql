-- Le tre domande suggerite mostrate nello stato di riposo della chat.
-- Per GIOCO e non per manuale né per lingua: la chat è per gioco e cerca in
-- tutte le fonti insieme, quindi una domanda suggerita non appartiene a un
-- singolo documento.
CREATE TABLE game_suggested_question (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,

    -- 0, 1, 2: l'ordine in cui compaiono a schermo.
    position INTEGER NOT NULL,

    text TEXT NOT NULL,

    -- 1 = riscritta a mano dall'admin. Una reindicizzazione rigenera solo
    -- le righe con edited = 0: sostituire uno scan brutto con uno buono
    -- aggiorna le domande da sé, senza mai perdere il lavoro manuale.
    edited INTEGER NOT NULL DEFAULT 0,

    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Rende impossibile la posizione 1 due volte per lo stesso gioco, ed è
-- l'indice su cui si appoggia l'upsert di SaveEditedQuestions.
CREATE UNIQUE INDEX idx_suggested_question_game_pos
    ON game_suggested_question(game_id, position);
