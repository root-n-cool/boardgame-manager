-- Provider AI OpenAI-compatible, opzionale: con questi tre valori
-- valorizzati le descrizioni BGG arrivano tradotte, senza l'app si
-- comporta come prima.
ALTER TABLE app_settings ADD COLUMN ai_base_url TEXT;
ALTER TABLE app_settings ADD COLUMN ai_api_key TEXT;
ALTER TABLE app_settings ADD COLUMN ai_model TEXT;

-- La descrizione BGG grezza, in inglese: la sorgente da cui traduce
-- ogni lingua della scheda. Senza, tradurre in italiano cancellerebbe
-- l'originale e ogni lingua aggiunta dopo sarebbe la traduzione di una
-- traduzione.
ALTER TABLE games ADD COLUMN bgg_description TEXT;
