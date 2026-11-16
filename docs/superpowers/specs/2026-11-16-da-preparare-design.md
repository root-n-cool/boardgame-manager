# "Da preparare" nella scheda evento admin — design

## Perché

Preparando una serata Furt e Daniele devono sapere, per ciascun
proprietario, quali giochi dell'evento mettere nella busta e quali di
questi sono già prenotati (e da chi). Oggi la scheda evento admin mostra
i giochi senza proprietario e le prenotazioni per copia, senza
proprietario.

## Cosa

Nuova sezione **"Da preparare"** nella scheda evento admin
(`EventAdminDetailView`), sopra "Prenotazioni":

- giochi dell'evento raggruppati per **proprietario** (campo testuale
  `owner` del gioco; raggruppamento senza distinguere maiuscole/spazi,
  si mostra la grafia del primo trovato); gruppi in ordine alfabetico,
  "Senza proprietario" in fondo; nella testata del gruppo il numero di
  giochi;
- ogni copia su una riga (`#n` solo se il gioco ha più copie); se
  prenotata: pastiglia **"Prenotato"** e i nomi di chi ha prenotato, con
  `+N` per i posti in più;
- dentro il gruppo prima i prenotati, poi gli altri, alfabetico;
- in testata della sezione: "N prenotati su M copie" (si contano le copie, cioè i tavoli);
- solo lettura: niente spunte, niente stato salvato, nessun cambio di
  backend; si aggiorna con il `load()` esistente (salvataggio evento,
  annullamento prenotazione).

## Approcci

1. **Calcolo nel frontend dai dati già caricati** (`/events/:id`,
   `/games` con `owner`, `/events/:id/bookings`) — *scelto*: zero backend.
2. Endpoint dedicato — scartato: i dati ci sono già.

## Non obiettivi

Spunte di preparazione, stampa dedicata, filtri.
