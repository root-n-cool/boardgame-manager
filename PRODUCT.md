# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **Admin/organizzatore** (unico ruolo autenticato): membro di
  un'associazione ludica amatoriale locale che organizza serate/eventi
  di giochi da tavolo. Usa l'app da laptop/desktop per gestire il
  catalogo giochi, creare eventi e consultare i risultati inseriti,
  probabilmente prima/durante la preparazione dell'evento, seduto a un
  tavolo.
- **Partecipanti** (nessun account): persone della stessa associazione
  o invitate all'evento che prenotano un gioco dal proprio smartphone
  — spesso in piedi, al tavolo di gioco, con poco tempo — e a fine
  partita reinseriscono il punteggio dallo stesso telefono.

## Product Purpose

Strumento selfhost, leggero, per far girare le serate di
un'associazione ludica amatoriale: l'admin tiene un catalogo giochi
(arricchito da BoardGameGeek) e organizza eventi; i partecipanti si
prenotano un gioco senza creare un account e, a fine partita,
registrano il punteggio finale, alimentando una classifica storica
per ciascun gioco. Successo = un evento gestito senza attrito, dalla
prenotazione al punteggio, senza bisogno di account o app da
installare.

## Positioning

A differenza di un foglio di calcolo condiviso o di un form generico,
l'app conosce il dominio (giochi, quantità di copie per evento,
punteggi, classifiche storiche per gioco) e riduce al minimo lo
sforzo di partecipazione: nessun login, solo un booking_code
generato alla prenotazione. Zero dipendenze pesanti: un solo
binario/container, SQLite come unico storage.

## Operating Context

- Le prenotazioni e l'inserimento punteggio avvengono sul posto,
  durante l'evento reale, da smartphone: la UI pubblica deve reggere
  input rapido, luce variabile, poca attenzione disponibile.
- La gestione admin (catalogo, eventi, impostazioni, utenti) avviene
  da laptop/desktop, in sessioni più lunghe e meno frenetiche, prima
  o durante l'evento.
- Nessuno schermo condiviso/proiettato nel flusso attuale — non
  progettare per quella modalità.
- Interfaccia solo in italiano (v1).

## Capabilities and Constraints

- Un solo ruolo autenticato (admin); i partecipanti non hanno mai un
  account, si identificano solo con nome + telefono alla prenotazione
  e successivamente con il solo `booking_code`.
- Catalogo giochi arricchito da BoardGameGeek (copertina, dati base);
  ricerca automatica di manuali/tutorial è opzionale e richiede
  chiavi API configurate dall'admin.
- Un evento può includere più copie dello stesso gioco.
- Classifica per gioco calcolata sul nome giocatore normalizzato,
  nessun registro giocatori separato.
- Email/SMTP opzionale: senza configurazione il `booking_code` resta
  l'unico strumento di gestione post-prenotazione, mostrato a schermo;
  configurando un server SMTP nelle impostazioni l'app manda anche una
  conferma di prenotazione, un avviso di annullamento e l'invito di un
  amministratore.

## Brand Commitments

Nessun nome di associazione o identità di marca confermata: il
progetto resta "BoardGames Manager", nome generico e non vincolante.
Nessun asset di brand (logo, palette, font) esistente da preservare.

## Evidence on Hand

Nessun asset reale (loghi, foto di eventi, copertine) fornito
dall'utente in questa sessione: le copertine dei giochi arrivano da
BoardGameGeek in produzione, ma per il lavoro di design vanno trattate
come dati dinamici, non asset da autorare.

## Product Principles

- Zero attrito per chi partecipa: niente account, azioni minime da
  smartphone, in autonomia decisionale con il gruppo attorno al
  tavolo.
- L'admin lavora in modalità operativa (task, non narrazione): scan
  veloce di liste/stati, azioni chiare, niente ambiguità sui dati.
- Il gioco da tavolo è il soggetto centrale: l'interfaccia può
  prendersi personalità e calore (tono "caldo e giocoso" confermato
  dall'utente) senza mai rallentare il compito.
- Un solo binario/container, nessuna dipendenza pesante: le scelte
  visive non devono richiedere librerie JS pesanti o asset esterni
  non gestiti dal backend.

## Accessibility & Inclusion

Nessun requisito di accessibilità specifico raccolto oltre agli
standard di base (contrasto, focus visibile, target touch adeguati
per l'uso da smartphone dei partecipanti).
