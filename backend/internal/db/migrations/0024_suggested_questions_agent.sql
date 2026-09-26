-- Le domande suggerite diventano per agente: il Manuale ha le sue (generate
-- dai titoli del regolamento), la Strategia le sue (generate da nome e
-- descrizione BGG). Le righe esistenti sono tutte del Manuale.
ALTER TABLE game_suggested_question ADD COLUMN agent TEXT NOT NULL DEFAULT 'rules';

-- L'indice unico si rifà su (gioco, agente, posizione): la posizione 0 esiste
-- una volta per agente. È anche l'indice su cui poggiano gli upsert.
DROP INDEX idx_suggested_question_game_pos;
CREATE UNIQUE INDEX idx_suggested_question_game_agent_pos
    ON game_suggested_question(game_id, agent, position);
