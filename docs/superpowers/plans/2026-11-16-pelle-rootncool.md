# Pelle Root'n'Cool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dare a tutto il manager (pubblico e admin) la pelle del blog Root'n'Cool: tavolo di notte, gesso, pedine, Fraunces + Atkinson, pillole.

**Architecture:** Quasi tutto passa da `frontend/src/app.css` (nessun `<style>` nei componenti): prima token e font, poi la rimappatura dei ruoli tipografici, poi il ripasso delle regole per superficie. Fuori da `app.css`: shadow DOM della chat, tema di md-editor-v3, email in Go, `index.html`/favicon, documentazione.

**Tech Stack:** Vue 3 + Vite (CSS puro), Go 1.25 per le email (test in Docker).

**Spec:** `docs/superpowers/specs/2026-11-16-pelle-rootncool-design.md`

## Global Constraints

- Colori dal blog (`rnc-blog/src/styles/global.css`): nero `#000000`, superficie `#151515`, linea `#2a2a2a`, gesso `#f3efe6`, blu `#035cb6`, arancio `#db8142`, verde `#5c8d2d`, grigio `#aeb2b3`, giallo `#e3b23c`, rosso `#d1453b`.
- Arancio = azione primaria e focus, **testo nero** sopra. Blu mai come colore di testo sul nero, solo riempimento con testo bianco.
- Contrasto testo ≥ 4.5:1 (AA) su ogni superficie in cui appare.
- Fraunces Variable (SOFT 100, 900, line-height 1) su h1/h2/h3, titoli di card, nomi dei giochi; Atkinson Hyperlegible 400/700 per tutto il resto (navigazione ed etichette in 700); IBM Plex Mono **solo** codici e numeri (booking code, punteggi, quantità, date tabellari).
- Bottoni a pillola (`border-radius: 99px`).
- Stampa QR resta chiara (fondo bianco, testo nero).
- Nessuna dipendenza nuova (font copiati da `rnc-blog/node_modules`); niente temi configurabili; niente cambi di layout, flussi o testi.
- Comandi Go solo in Docker (`bgm-gomodcache`/`bgm-gocache`), npm in locale.
- Commit solo se Furt li chiede: i passi "Commit" valgono come punto di controllo, si eseguono solo con il suo via.

## Review Focus

- Campi compilati dall'autofill del browser: niente fondo giallo/bianco con testo gesso illeggibile (`-webkit-autofill` → Task 3).
- Controlli nativi (date, time, select, checkbox, scrollbar) leggibili sul nero: `color-scheme: dark` sulle pagine a schermo e `light` in stampa (Task 1, Task 5).
- Nomi di giochi lunghi in Fraunces 900 (più largo di Space Grotesk) a 390px: niente overflow orizzontale nelle card e nelle teste di pagina (Task 2, verifica Task 8).
- Stati disabilitato, errore, focus visibili sul nero, compreso il focus sui bottoni arancio (outline gesso dove lo sfondo è arancio) (Task 3).
- Mappa evento (Leaflet): controlli e attribuzione leggibili, tile non invertite (Task 4).

---

### Task 1: Font e token di base

**Files:**
- Create: `frontend/src/assets/fonts/fraunces-latin-soft-normal.woff2`, `atkinson-hyperlegible-latin-{400,700}-{normal,italic}.woff2` (copiati da `../rnc-blog/node_modules/@fontsource-variable/fraunces/files/` e `../rnc-blog/node_modules/@fontsource/atkinson-hyperlegible/files/`)
- Delete: `frontend/src/assets/fonts/space-grotesk-variable.woff2`, `ibm-plex-sans-variable.woff2`
- Modify: `frontend/src/app.css` (intestazione, `@font-face`, `:root`, `body`, `h1`/`h2`)
- Modify: `frontend/index.html` (`theme-color` → `#000000`, commento "contratto di direzione" riscritto per "tavolo di notte")
- Modify: `frontend/public/favicon.svg` (pedine del logo su nero al posto del feltro)

**Interfaces:**
- Produces: famiglie `'Titoli'` (Fraunces), `'Testo'` (Atkinson), `'Data'` (Plex Mono, invariata). Token: `--bg #000`, `--card #151515`, `--card-alt` (~`#1f1f1f`), `--card-line #2a2a2a`, `--field-line` (nuovo, bordo campi, ≥3:1 su `--card`), `--ink #f3efe6`, `--ink-muted` (gesso attenuato, ≥4.5:1 su `--card-alt`), `--accent #db8142`, `--accent-ink #000` (nuovo: testo sopra l'accento), `--accent-deep` (arancio più scuro per hover), `--blu`, `--verde`, `--grigio` (nuovi, le pedine), `--gold #e3b23c`, `--danger #d1453b`, `--success #5c8d2d`, e i `*-bg` come tinte scure (colore al 15–20% su `#151515`). I token `--felt*` diventano alias della struttura scura (`--felt: #000`, `--felt-deep: #0a0a0a`, `--felt-line: var(--card-line)`, `--felt-text: var(--ink)`, `--felt-text-muted: var(--ink-muted)`) per non rompere le 70 occorrenze: il Task 3 decide se rinominarli.

- [ ] **Step 1:** Copia i 5 woff2 e cancella i 2 vecchi.
- [ ] **Step 2:** Riscrivi `@font-face` (Fraunces con `font-weight: 100 900` e `font-variation-settings: 'SOFT' 100` applicato su chi lo usa; Atkinson 400/700 normal+italic) e il `:root` coi valori sopra; `color-scheme: dark`; `body` con fondo `--bg` senza trama a puntini; `--shadow-*` ridotte a ombre quasi nulle (la separazione la fanno superfici e linee).
- [ ] **Step 3:** Verifica contrasto dei token con uno script usa-e-getta nello scratchpad (formula WCAG): `--ink` e `--ink-muted` su `--bg`/`--card`/`--card-alt` ≥ 4.5; `--accent-ink` su `--accent` ≥ 4.5; testo dei `*-bg` ≥ 4.5. Expected: tutte le coppie passano, altrimenti ritocca i valori.
- [ ] **Step 4:** `cd frontend && npm run build` → Expected: build ok, nessun errore `vue-tsc`.
- [ ] **Step 5:** `grep -rn "space-grotesk\|ibm-plex-sans\|1f4d3a" frontend/src frontend/index.html frontend/public` → Expected: nessuna riga.
- [ ] **Step 6: Commit** `feat: night palette and blog fonts as base tokens`

### Task 2: Ruoli tipografici

**Files:**
- Modify: `frontend/src/app.css` (le ~14 regole con `'Display'` e le ~40 con `'Data'`)

**Interfaces:**
- Consumes: famiglie del Task 1.

- [ ] **Step 1:** Ogni regola `'Display'`: se è titolo (h1–h3, titolo di card, nome gioco, "Il Mentore") → `'Titoli'` 900 SOFT 100, `line-height` ≤1.1 e `overflow-wrap: anywhere` sui nomi; altrimenti (nav, etichette in "misura titolo di card") → `'Testo'` 700.
- [ ] **Step 2:** Ogni regola `'Data'`: resta mono solo se mostra codici o numeri (booking code, punteggi, quantità, date in tabella, conteggi); le etichette in maiuscoletto/occhielli/metadati → `'Testo'` 700, dimensione ridotta, `letter-spacing` leggero.
- [ ] **Step 3:** `grep -n "'Display'\|'Body'" frontend/src/app.css` → Expected: nessuna riga. `grep -c "'Data'" frontend/src/app.css` → Expected: molto meno di 40, e ogni occorrenza rimasta è su un codice/numero.
- [ ] **Step 4:** `npm run build` → ok.
- [ ] **Step 5: Commit** `feat: Fraunces titles, Atkinson labels, mono only for codes`

### Task 3: Componenti comuni

**Files:**
- Modify: `frontend/src/app.css` (bottoni, campi, card, badge, modali, tab, filtri, tabelle, nav/header, `.page-head`, messaggi di errore/successo, i ~60 colori letterali)

**Interfaces:**
- Consumes: token del Task 1.
- Produces: `.page-head` con barra a sinistra (`border-left: 8px solid var(--segno, var(--ink))`, come `.intestazione` del blog); pedine decorative nell'header (4 pallini 12px blu/arancio/verde/grigio, markup in `App.vue` dove sta il titolo/logo).

- [ ] **Step 1:** Bottoni: tutti a pillola; primario `--accent` con `--accent-ink`, hover `--accent-deep`; secondario contorno gesso trasparente; pericolo `--danger` con testo nero o bianco (quello che passa 4.5:1); disabilitato con opacità e cursore, testo ancora ≥3:1. Focus: `outline: 3px solid var(--accent)`, ma `var(--ink)` sui bottoni arancio.
- [ ] **Step 2:** Campi: fondo `--card`, bordo `--field-line`, testo `--ink`, placeholder ≥4.5:1; regola `input:-webkit-autofill` con `-webkit-box-shadow: 0 0 0 100px var(--card) inset` e `-webkit-text-fill-color: var(--ink)`.
- [ ] **Step 3:** Card, modali, tab, filtri, tabelle (righe alterne `--card-alt`), badge come pillole piene (blu+bianco, verde, arancio+nero, grigio+nero), header/nav sul nero con le pedine.
- [ ] **Step 4:** Sostituisci i colori letterali rimasti (`rgb(36 31 24 …)`, avorio, rosso seme, oro) con token o tinte del gesso (`rgb(243 239 230 / N%)`).
- [ ] **Step 5:** `grep -n "rgb(36 31 24\|#f1ece0\|#faf6ec\|#9c2b2b\|#b8842a" frontend/src/app.css` → Expected: nessuna riga (salvo dentro le regole di stampa del Task 5).
- [ ] **Step 6:** `npm run build` → ok.
- [ ] **Step 7: Commit** `feat: pill buttons and night surfaces for shared components`

### Task 4: Ripasso per schermata

**Files:**
- Modify: `frontend/src/app.css` (regole specifiche di vista), `frontend/src/App.vue` se serve per le pedine

- [ ] **Step 1:** `docker compose up -d --build`, poi con Claude in Chrome apri a 390px e a desktop ogni vista: pubbliche (`/events`, `/events/:id` con modale prenotazione, `/manage-booking`, `/prenotazione/:code`, punteggi, classifica, `/games`, scheda gioco, pagine legali, login/setup/invito) e admin (eventi, scheda evento, nuovo evento, banco prestiti, catalogo, nuovo gioco, scheda gioco admin, tipologie, prestiti, utenti, impostazioni).
- [ ] **Step 2:** Per ognuna correggi ciò che stona: residui del feltro, contrasti bassi, overflow orizzontale, mappa Leaflet (cornice e controlli leggibili, tile come sono). Console senza errori.
- [ ] **Step 3:** `npm run build` → ok.
- [ ] **Step 4: Commit** `fix: per-view adjustments for the night skin`

### Task 5: Stampa QR chiara

**Files:**
- Modify: `frontend/src/app.css` (regole di `QrPrintView`/`QrSheetView` e `@media print`)

- [ ] **Step 1:** Le pagine QR, a schermo e in stampa, sono un "foglio": fondo `#fff`, testo `#000`, `color-scheme: light`, con i nuovi font; `@media print` toglie header/nav scuri e forza `background: #fff` sul `body`.
- [ ] **Step 2:** Anteprima di stampa in Chrome di `/admin/games/qr` e di un cartellino evento → Expected: niente nero di fondo, QR nitidi, nove cartellini per A4 come prima.
- [ ] **Step 3: Commit** `feat: keep QR print sheets on white paper`

### Task 6: Chat del manuale ed editor Markdown

**Files:**
- Modify: `frontend/src/components/ManualChatPanel.vue` (`messageStyles` e stili dell'input: i 9 esadecimali commentati col nome del token)
- Modify: `frontend/src/components/MarkdownEditorImpl.vue:114` (`theme="light"` → `"dark"`) ed eventuali override in `app.css`
- Modify: il commento d'avviso in testa a `:root` di `app.css` (nuovi nomi/valori)

- [ ] **Step 1:** Aggiorna ogni esadecimale al valore attuale del token citato nel commento accanto.
- [ ] **Step 2:** Tema scuro di md-editor-v3; verifica a schermo in Impostazioni (termini/privacy) che fondo e testo seguano la palette.
- [ ] **Step 3:** `npm run build` → ok; apri la chat del manuale in una scheda gioco e verifica bolle leggibili.
- [ ] **Step 4: Commit** `feat: night palette for manual chat and markdown editor`

### Task 7: Email

**Files:**
- Modify: `backend/internal/httpapi/mail_templates.go:23-31` (costanti colore, commento di `mailShell`)
- Test: `backend/internal/httpapi/mail_templates_test.go`

- [ ] **Step 1: Test che fallisce** `TestMailShellUsesNightPalette`: `out := mailShell("Titolo", "<p>x</p>")`; assert `strings.Contains(out, "#000000")` (intestazione), `strings.Contains(out, "#db8142")` (accento), `!strings.Contains(out, "#1f4d3a")`, `!strings.Contains(out, "#9c2b2b")`.
- [ ] **Step 2:** `docker run --rm -v "$(pwd)/backend:/app" -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/ -run TestMailShellUsesNightPalette` → Expected: FAIL.
- [ ] **Step 3:** Costanti: `mailColorHead = "#000000"` (sostituisce `mailColorFelt`), testo intestazione `#f3efe6`, corpo chiaro (`mailColorCard "#ffffff"`, `mailColorCardAlt "#f3efe6"`, `mailColorInk "#111111"`, `mailColorInkMuted "#555555"`, `mailColorLine "#dddddd"`), `mailColorAccent "#db8142"` con testo bottone `#000000`. Il codice prenotazione resta in mono.
- [ ] **Step 4:** Stesso comando con `go test ./...` → Expected: PASS tutto.
- [ ] **Step 5: Commit** `feat: night header and orange buttons in transactional mail`

### Task 8: Documentazione e chiusura

**Files:**
- Modify: `DESIGN.md` (Overview, Colors, Typography, Elevation, Shapes, Buttons, Cards, Inputs, Navigation, Badge, Email; riferimenti a feltro/avorio/oro/rosso nelle sezioni componente)
- Modify: `CLAUDE.md` (riga sul tono "dado & pedina" → "tavolo di notte", stile del blog Root'n'Cool)
- Modify: commento d'intestazione di `app.css`

- [ ] **Step 1:** Aggiorna i documenti; `grep -n -i "feltro\|avorio\|space grotesk\|plex sans" DESIGN.md CLAUDE.md frontend/src/app.css frontend/index.html` → Expected: solo eventuali menzioni storiche esplicite.
- [ ] **Step 2:** `npm run build` e suite backend completa in Docker → Expected: tutto verde.
- [ ] **Step 3:** `/impeccable audit` sulle viste pubbliche e admin (contrasto in primo piano), applica le correzioni, ricontrolla in Chrome a 390px.
- [ ] **Step 4: Commit** `docs: night skin in DESIGN.md and project guidelines`
