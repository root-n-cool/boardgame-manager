-- Chiave dell'API di ricerca Tavily: serve all'agente regole per trovare
-- i thread del forum Rules su BoardGameGeek. Facoltativa: senza, il tool
-- delle FAQ non si dichiara e la chat usa solo i documenti indicizzati.
ALTER TABLE app_settings ADD COLUMN tavily_api_key TEXT;
