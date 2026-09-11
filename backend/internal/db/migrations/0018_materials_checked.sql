-- Quando un admin ha dichiarato che la scatola è di nuovo a posto, e chi.
-- NULL significa "nessuno l'ha mai fatto", che è lo stato di partenza
-- giusto per tutto il catalogo esistente: una segnalazione aperta oggi da
-- un prestito di ieri deve comparire senza bisogno di popolare niente.
ALTER TABLE games ADD COLUMN materials_checked_at TEXT;

-- SET NULL e non CASCADE: se l'utente che ha chiuso la segnalazione viene
-- cancellato, resta vero che la segnalazione è stata chiusa.
ALTER TABLE games ADD COLUMN materials_checked_by INTEGER
    REFERENCES users(id) ON DELETE SET NULL;
