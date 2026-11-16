# Viste Card / Lista / Miniature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Selettore di vista (Card/Lista/Miniature) e filtri compatti su catalogo pubblico e tavolo dell'evento, più locandina contenuta su telefono.

**Architecture:** Una util per ricordare la vista, un componente selettore, classi `is-list`/`is-thumbs` sulle `<ul>` esistenti, filtri in `ModalDialog` sotto i 560px. Solo frontend.

**Tech Stack:** Vue 3 + TS, CSS in `frontend/src/app.css`.

**Spec:** `docs/superpowers/specs/2026-11-16-viste-liste-giochi-design.md`

## Global Constraints

- Nessuna dipendenza nuova; nessun cambio backend.
- Bersagli touch ≥ 44px; nessuno scroll orizzontale a 390px.
- Pelle "tavolo di notte" (DESIGN.md): pillole, token di `app.css`.
- Il frontend non ha test runner: verifica = `npm run build` (vue-tsc) + prova in Chrome a 390px e desktop.
- Commit solo se Furt li chiede.

## Review Focus

- `localStorage` non disponibile (navigazione privata): la vista deve funzionare comunque col default.
- Chi ha già prenotato un gioco: in Lista e Miniature deve poter ancora annullare e aggiungere il risultato.
- Giochi non prenotabili ("Non si prenotano") e tavoli al completo: niente "Prenota", stato leggibile in tutte le viste.
- Nomi lunghi in Lista e Miniature: nessun overflow.
- Filtri attivi nascosti nel foglio: il bottone deve dire quanti sono, e "Azzera filtri" deve restare raggiungibile.

---

### Task 1: Util e selettore

**Files:**
- Create: `frontend/src/utils/listView.ts` — `export type ListView = 'cards' | 'list' | 'thumbs'`; `readListView(key: string): ListView` (localStorage `bgm.view.<key>`, valore valido o default: `'list'` se `matchMedia('(max-width: 560px)')`, altrimenti `'cards'`; try/catch); `saveListView(key: string, v: ListView): void` (try/catch).
- Create: `frontend/src/components/ViewSwitcher.vue` — prop `modelValue: ListView`, emit `update:modelValue`; tre bottoni-icona in `role="group" aria-label="Vista"`, `aria-pressed`, testo nascosto "Card", "Lista", "Miniature".
- Modify: `frontend/src/app.css` — `.view-switcher` (pillola segmentata, bottoni 44px, attivo gesso con testo nero come `.tab-bar button.active`).

- [ ] Implementa; `npm run build` → ok.

### Task 2: Catalogo pubblico

**Files:** `frontend/src/views/CatalogView.vue`, `frontend/src/app.css`

- [ ] `view = ref(readListView('catalog'))`, `watch` → `saveListView`. `ViewSwitcher` sulla riga dei filtri (a destra).
- [ ] `<ul class="game-grid catalog-grid" :class="{'is-list': view==='list', 'is-thumbs': view==='thumbs'}">`. In Miniature nascondi fatti e difficoltà (CSS).
- [ ] CSS `.catalog-grid.is-list`: una colonna, `li a` in griglia `3rem 1fr`, copertina 3rem×4rem, nome Atkinson 700 1rem, fatti + difficoltà su una riga; separatori `--card-line`. `.catalog-grid.is-thumbs`: `repeat(3, 1fr)` (4–6 su desktop con `auto-fill minmax(110px,1fr)`), nome 0.8rem clamp 2 righe.
- [ ] Filtri: sotto 560px i `SelectFilter` passano in una `ModalDialog` "Filtri" aperta da un bottone `Filtri` + conteggio attivi (`players`, `difficulty`); dentro il foglio anche "Azzera filtri" e "Mostra N giochi" che chiude. Sopra 560px invariati. `isPhone` da `matchMedia` con listener.
- [ ] `npm run build` → ok.

### Task 3: Tavolo dell'evento

**Files:** `frontend/src/views/EventDetailView.vue`, `frontend/src/app.css`

- [ ] `view = ref(readListView('event'))` + `ViewSwitcher` accanto a `GameTypeTabs`/filtri; classi `is-list`/`is-thumbs` su ogni `ul.event-games`.
- [ ] Lista: `li` in griglia `3rem | corpo | azioni`; nome diventa link alla scheda (`router-link` attorno al titolo in tutte le viste è ok, sostituisce "Dettagli" in Lista/Miniature); nascondi `.event-game-links` e la tipologia-pastiglia sulla copertina (in Lista va inline accanto al nome, `.type-chip.is-inline` già esiste); "Prenota" compatto 44px a destra; se prenotato: pastiglia "Prenotato" + le due azioni come link piccoli sotto il nome.
- [ ] Miniature: 3 colonne; copertina, nome 2 righe, sotto "Prenota" o "Prenotato"/"Al completo"; nascondi difficoltà, posti, link.
- [ ] Il filtro "Giocatori" su telefono va nello stesso pattern del catalogo (bottone Filtri + foglio) solo se `showFilters`.
- [ ] Locandina: sotto 560px `.event-poster--banner` max-height 50vh; la `EventPoster` banner su telefono è un link all'immagine (`<a :href="src" target="_blank">`) — aggiungi prop/logica in `EventPoster.vue` solo per la variante banner.
- [ ] `npm run build` → ok.

### Task 4: Verifica, documenti, chiusura

- [ ] `docker compose up -d --build`; Chrome a 390px (iframe) e desktop: catalogo e tavolo nelle tre viste, filtri nel foglio, prenotato/al completo/non prenotabile, nessun overflow, console pulita.
- [ ] `DESIGN.md`: sezione "Viste delle liste di giochi".
- [ ] Revisione indipendente del diff; correggi Important/Critical.
- [ ] `/impeccable audit` sulle due pagine; correggi.
