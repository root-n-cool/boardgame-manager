-- Server SMTP opzionale. Con host, porta e indirizzo mittente valorizzati
-- l'app manda l'invito di un amministratore, la conferma di una
-- prenotazione e l'avviso di annullamento; senza si comporta come prima,
-- col codice di prenotazione solo a schermo.
--
-- smtp_username e smtp_password restano vuote per un relay senza
-- autenticazione: non fanno parte del minimo indispensabile.
ALTER TABLE app_settings ADD COLUMN smtp_host TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_port INTEGER;
ALTER TABLE app_settings ADD COLUMN smtp_username TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_password TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_from_address TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_from_name TEXT;

-- 'starttls' (587), 'tls' (465) o 'none'. Vuoto vale come 'starttls'.
ALTER TABLE app_settings ADD COLUMN smtp_tls_mode TEXT;
