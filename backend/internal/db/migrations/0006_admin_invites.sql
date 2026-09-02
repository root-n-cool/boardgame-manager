-- Un admin invitato ma non ancora attivo è una riga users con
-- password_hash = '' e invite_token valorizzato. Il token è in chiaro di
-- proposito: il bottone "Copia link invito" deve poter mostrare sempre lo
-- stesso link, e l'unico a poterlo leggere è un admin già autenticato.
-- All'attivazione il token torna NULL, ed è così che il link muore.
ALTER TABLE users ADD COLUMN invite_token TEXT;

CREATE UNIQUE INDEX users_invite_token_unique
    ON users(invite_token) WHERE invite_token IS NOT NULL;
