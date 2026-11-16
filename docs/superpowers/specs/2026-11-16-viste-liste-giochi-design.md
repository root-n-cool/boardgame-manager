# Viste Card / Lista / Miniature per le liste di giochi — design

## Perché

Su telefono le liste di giochi si scorrono male: nel catalogo pubblico
ogni card è alta ~318px (una riga e mezza a schermo, dopo ~470px di
tipologie e filtri); al tavolo di un evento ogni riga è ~130px per i
bottoni impilati, e la locandina occupa il primo schermo. Furt vuole
scorrere più in fretta e poter scegliere come vedere i giochi.

## Obiettivi

- Selettore di vista **Card · Lista · Miniature** sopra il catalogo
  pubblico (`/giochi`, `CatalogView`) e sopra "Al tavolo" della pagina
  evento (`/events/:id`, `EventDetailView`).
- Scelta ricordata nel browser, per pagina (`localStorage`); senza scelta:
  **Lista** sotto i 560px, **Card** sopra.
- **Lista**: riga ~64px — copertina piccola, nome (link alla scheda),
  giocatori · durata · difficoltà; al tavolo, a destra, "Prenota" (44px)
  o la pastiglia "Prenotato". Nessun bottone Dettagli/Mentore: ci si
  arriva dal nome. Le azioni "Annulla prenotazione"/"Aggiungi risultato"
  di chi ha già prenotato restano raggiungibili.
- **Miniature**: tre copertine per riga, nome sotto; al tavolo un
  "Prenota" sotto ogni copertina (44px).
- **Card**: com'è oggi.
- **Filtri su telefono** (<560px): la ricerca resta in vista; i filtri a
  tendina passano dietro un bottone "Filtri (n)" che apre il foglio dal
  basso (`ModalDialog`). Su desktop restano in linea.
- **Locandina dell'evento su telefono**: alta al massimo metà schermo;
  toccandola si apre l'immagine intera.

## Non obiettivi

- Catalogo admin (`GamesView`), elenco eventi in home: invariati.
- Schede tipologia agganciate in alto (si scontrano con la testata a
  scomparsa).
- Nessuna dipendenza nuova; nessun cambio di backend, prenotazione o
  filtri.

## Approcci

1. **Tre viste + filtri compatti** — *scelto da Furt.*
2. Solo Lista su telefono, niente selettore — scartato: si perde la vista
   a copertine.
3. Due viste (Card/Lista) — scartato: le miniature sono il modo più
   veloce di riconoscere le scatole.

## Come

- `src/utils/listView.ts`: tipo `ListView = 'cards' | 'list' | 'thumbs'`,
  `readListView(key)`/`saveListView(key, v)` con try/catch su
  `localStorage` e default per larghezza.
- `src/components/ViewSwitcher.vue`: gruppo di tre bottoni-icona
  (`role="group"`, `aria-pressed`, etichette per screen reader), 44px.
- Stesso markup delle liste; cambia solo la classe sul `<ul>`
  (`is-list`, `is-thumbs`) e alcuni elementi si nascondono via CSS o
  `v-if` sulla vista. La logica di prenotazione e filtri non cambia.
- Filtri: `src/components/FiltersSheet.vue` non serve — il bottone
  "Filtri" apre una `ModalDialog` col contenuto dei filtri; su desktop i
  filtri restano in linea (`matchMedia`).

## Lasciato a chi costruisce

Misure esatte, icone del selettore, come mostrare le azioni di chi ha
già prenotato in Lista/Miniature (purché restino raggiungibili).
