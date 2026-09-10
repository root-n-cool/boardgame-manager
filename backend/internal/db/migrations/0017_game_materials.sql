-- Il contenuto della scatola, come lo tiene l'admin nel catalogo. Sta sul
-- gioco e non sulla lingua: le tessere sono le stesse in ogni edizione, e
-- una lista per lingua vorrebbe dire tenerne due allineate a mano.
CREATE TABLE game_material (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id  INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity >= 1),
    -- L'ordine lo decide l'admin: è l'ordine in cui si controlla la
    -- scatola, e non coincide con nessun ordinamento naturale.
    position INTEGER NOT NULL,
    UNIQUE(game_id, name)
);

CREATE INDEX idx_game_material_game ON game_material(game_id, position);

-- L'esito della riconsegna, una riga solo per ciò che NON è tornato intero
-- o non è stato verificato. Un prestito pulito non ne lascia nessuna:
-- questa tabella si legge per rispondere a "cosa manca", non per
-- archiviare i controlli andati bene.
CREATE TABLE loan_material_issue (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    loan_id     INTEGER NOT NULL REFERENCES game_loans(id) ON DELETE CASCADE,

    -- SET NULL e non CASCADE: se domani l'admin cancella la voce dal
    -- catalogo, il fatto che a marzo mancassero sette tessere resta vero.
    material_id INTEGER REFERENCES game_material(id) ON DELETE SET NULL,

    -- Nome e quantità attesa sono COPIATI al momento della riconsegna, non
    -- risolti con una join: la lista del catalogo può cambiare, il registro
    -- di quella sera no.
    name        TEXT NOT NULL,
    expected    INTEGER NOT NULL,

    -- NULL = voce non verificata. Un numero = quante ne sono tornate,
    -- sempre meno di expected (una voce completa non genera riga). Lo stato
    -- vive nel nullable, come returned_at sui prestiti: una colonna di
    -- stato separata sarebbe una verità duplicata che può divergere.
    returned    INTEGER CHECK (returned IS NULL OR returned >= 0),

    UNIQUE(loan_id, material_id)
);

CREATE INDEX idx_loan_material_issue_loan ON loan_material_issue(loan_id);
