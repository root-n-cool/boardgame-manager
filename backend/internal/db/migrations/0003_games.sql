CREATE TABLE games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bgg_id TEXT,
    name TEXT NOT NULL,
    year INTEGER,
    min_players INTEGER,
    max_players INTEGER,
    playtime_minutes INTEGER,
    owner TEXT,
    cover_path TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE game_languages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    language_code TEXT NOT NULL,
    is_base_language INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT,
    UNIQUE(game_id, language_code)
);

CREATE UNIQUE INDEX idx_one_base_language_per_game
    ON game_languages(game_id) WHERE is_base_language = 1;

CREATE TABLE game_media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_language_id INTEGER NOT NULL REFERENCES game_languages(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('file', 'link', 'youtube')),
    url_or_path TEXT NOT NULL,
    title TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
