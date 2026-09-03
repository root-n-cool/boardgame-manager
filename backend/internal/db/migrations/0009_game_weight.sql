-- BGG's average complexity (1 light .. 5 heavy). NULL means unknown: a game
-- added by hand, or one nobody on BGG has rated yet.
ALTER TABLE games ADD COLUMN weight REAL;
