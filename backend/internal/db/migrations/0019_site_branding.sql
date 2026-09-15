-- Titolo del sito, logo e favicon personalizzabili, e i testi (in
-- Markdown) delle pagine /terms e /privacy. Tutti opzionali: NULL vuol
-- dire "non configurato", non un errore — stessa filosofia di
-- public_base_url.
ALTER TABLE app_settings ADD COLUMN site_title TEXT;
ALTER TABLE app_settings ADD COLUMN logo_filename TEXT;
ALTER TABLE app_settings ADD COLUMN favicon_filename TEXT;
ALTER TABLE app_settings ADD COLUMN terms_markdown TEXT;
ALTER TABLE app_settings ADD COLUMN privacy_markdown TEXT;
