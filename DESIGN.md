---
name: BoardGames Manager
description: Dado e pedina su un tavolo in feltro, per una serata di giochi da tavolo
colors:
  felt: "#1f4d3a"
  felt-deep: "#143528"
  card: "#faf6ec"
  card-alt: "#f2ead4"
  card-line: "#ddd0ab"
  ink: "#241f18"
  ink-muted: "#6e6250"
  accent: "#9c2b2b"
  accent-deep: "#7c1f1f"
  accent-bg: "#f6e4e0"
  gold: "#b8842a"
  gold-text: "#8a611c"
  gold-bg: "#f8ecd2"
  danger: "#ad4a22"
  danger-bg: "#f8e3d4"
  success: "#2f6b45"
  success-bg: "#e7f2e9"
  page-bg: "#f1ece0"
typography:
  display:
    fontFamily: "Space Grotesk, system-ui, sans-serif"
    fontWeight: 600
    letterSpacing: "-0.01em"
  body:
    fontFamily: "IBM Plex Sans, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
    fontSize: "15px"
    lineHeight: 1.55
  data:
    fontFamily: "IBM Plex Mono, ui-monospace, monospace"
rounded:
  sm: "6px"
  md: "10px"
spacing:
  sm: "0.5rem"
  md: "1rem"
  lg: "1.5rem"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "#ffffff"
    rounded: "{rounded.sm}"
    padding: "0.55rem 1rem"
  button-primary-hover:
    backgroundColor: "{colors.accent-deep}"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
  button-danger:
    backgroundColor: "transparent"
    textColor: "{colors.danger}"
    rounded: "{rounded.sm}"
  card-surface:
    backgroundColor: "{colors.card}"
    rounded: "{rounded.md}"
---

# Design System: BoardGames Manager

## Overview

**Creative North Star: "Dado & pedina su tavolo in feltro"**

Il catalogo è uno scaffale di scatole di gioco, un evento è un tavolo in
feltro verde con le scatole disponibili: non cartelle e form come ogni
altro pannello admin. Il sistema nasce dal mondo fisico dell'Eurogame —
scatole, feltro, cartoncino, pedine (meeple) e dadi — tradotto in una
grammatica di interfaccia funzionale, senza mai scivolare nel gadget o nel
clip-art giocoso. **Corretto dopo revisione utente**: la prima resa usava
un seme di carte (♠) e una texture a righe da dorso di carta da poker —
letta correttamente come "troppo poker" per un'app di giochi in stile
german. Il marchio e i placeholder ora usano una pedina (meeple) e un
dado, gli unici prestiti iconografici dal mondo fisico del gioco.

Il registro resta **Operate**: l'admin scansiona liste ed evade compiti da
laptop, il partecipante prenota e inserisce un punteggio dal telefono,
spesso in piedi, al tavolo di gioco. Il calore ("caldo e giocoso",
richiesto esplicitamente) vive nella palette e nei dettagli — dado per le
copertine mancanti, medaglie di rango — mai a scapito della velocità con
cui si completa un'azione.

Rifiutato esplicitamente: il default "dashboard SaaS" grigio/blu piatto
(lo stato precedente dell'app), l'iconografia da poker (seme di carte,
dorso a righe), e qualunque emoji al posto di un'icona disegnata (🏆/★).

**Key Characteristics:**
- Feltro verde come fondo strutturale (nav, header di tabella, il "tavolo"
  dove sono disposte le scatole di un evento pubblico).
- Cartoncino avorio come superficie di contenuto, su tutte le schermate.
- Un solo accento caldo (rosso) per le azioni primarie; oro riservato a
  distintivi e medaglie, mai a testo corrente su fondo chiaro.
- Tipografia grottesca per i titoli, mono per i dati (booking code,
  punteggi, date) — mai il default di sistema come voce del brand.
- Marchio e placeholder nel linguaggio dado/pedina — mai carte da gioco,
  mai emoji.

## Colors

Palette "Committed": il feltro verde possiede le zone strutturali a piena
superficie (non è un accento sparso), il rosso seme è l'unico accento per
le azioni, l'oro è riservato a distintivi/medaglie.

### Primary
- **Rosso caldo** (`#9c2b2b`, hover `#7c1f1f`): azione primaria (bottoni
  submit, CTA "Crea evento"/"Aggiungi gioco", link trattati come azione).

### Secondary
- **Oro** (`#b8842a`, testo `#8a611c` su fondo chiaro): distintivi —
  medaglia del podio in classifica, indicatore vincitore nello storico
  partite, badge lingua base, bordo della card del booking code. Non è mai
  usato come testo corrente: a 3.05:1 su cartoncino non supera la soglia
  AA per testo normale, solo per elementi grandi/decorativi.

### Neutral
- **Feltro** (`#1f4d3a`, profondo `#143528`): fondo di nav e header
  pubblico/admin, header delle tabelle, il "tavolo" delle scatole di un
  evento, sfondo delle pagine di autenticazione.
- **Cartoncino** (`#faf6ec`, alternativo `#f2ead4`): superficie di
  contenuto — card, form, tabelle — su ogni schermata.
- **Inchiostro** (`#241f18` testo primario, `#6e6250` testo secondario):
  mai nero puro, per restare coerente col calore del cartoncino.
- **Bordo cartoncino** (`#ddd0ab`): bordi di card, input, tabelle.

### Named Rules
**La regola del feltro strutturale.** Il verde feltro copre solo chrome
strutturale (nav, header tabella, il tavolo di un evento pubblico) — mai il
fondo di una schermata admin densa di dati, dove comprometterebbe la
leggibilità.

**La regola dell'oro non-testo.** L'oro (`#b8842a`) etichetta, non scrive:
usalo per badge, medaglie, bordi — mai come colore di un blocco di testo
corrente.

## Typography

**Display Font:** Space Grotesk (fallback: system-ui, sans-serif)
**Body Font:** IBM Plex Sans (fallback: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif)
**Label/Mono Font:** IBM Plex Mono — riservato ai dati: booking code,
punteggi, date/ore nelle liste, indici di lingua.

**Character:** una grottesca decisa per i titoli, una sans neutra e molto
leggibile per il corpo, un monospazio per tutto ciò che è un dato
misurabile o un codice da copiare a mano.

### Hierarchy
- **Display/H1** (600, 1.7rem, tracking -0.01em): titolo di pagina.
- **H2** (600, 1.1rem): titoli di sezione.
- **Body** (400, 15px/1.55): testo corrente, form, liste.
- **Label** (500, 0.82rem, uppercase nei `legend`): etichette di campo.
- **Data** (mono 400/600): booking code (2rem, tracking 0.14em), punteggi
  in tabella, date/orari nelle card evento.

### Named Rules
**La regola del dato in mono.** Qualunque cifra o codice che l'utente
potrebbe dover leggere a voce alta o ricopiare (booking code, punteggio,
data) è in IBM Plex Mono con cifre tabulari — mai nella sans corrente.

## Layout

Contenitore centrale max 56rem (`.public-page`, `.layout > main`), padding
laterale 1.5rem che scende a 1rem sotto i 640px. Le griglie di card
(catalogo, eventi, tavolo di un evento) usano
`repeat(auto-fill, minmax(Xpx, 1fr))` — 180-220px a seconda della densità
— così il numero di colonne si adatta senza breakpoint espliciti fino al
passaggio a colonna singola su mobile.

**Responsive nav:** sopra i 640px la barra è una riga fissa (3.5rem); sotto
i 640px va in `flex-wrap`, altezza automatica, e il wordmark testuale si
nasconde lasciando solo il marchio (pedina) — la barra non deve mai
costringere un elemento (es. "Esci") fuori dal viewport.

**Tabelle larghe** (classifica) scorrono nel proprio contenitore
(`.table-scroll`, `overflow-x:auto`) invece di spingere l'intera pagina in
scroll orizzontale sotto i 480px.

## Elevation & Depth

Sistema a due livelli, mai decorativo: `--shadow-card` (0 1px 2px al 6% +
0 6px 16px a -8px al 22%) per lo stato a riposo di card/form/tabelle,
`--shadow-lift` (più ampia e più scura) per l'hover di card cliccabili
(griglia giochi, griglia eventi). Le card del tavolo di un evento
(`.event-games li`) non sollevano — sono "posate" sul feltro, non
cliccabili come intere unità (solo il bottone "Prenota" lo è).

### Shadow Vocabulary
- **card** (`0 1px 2px rgba(36,31,24,.06), 0 6px 16px -8px rgba(36,31,24,.22)`): stato a riposo.
- **lift** (`0 2px 4px rgba(36,31,24,.08), 0 14px 28px -12px rgba(36,31,24,.32)`): hover di card cliccabili.

### Named Rules
**La regola del sollevamento cliccabile.** Solo le card che sono un intero
link/azione (griglia giochi, griglia eventi) sollevano al passaggio del
mouse. Le card informative (righe di lista, risultati) restano ferme.

**La regola del segnale senza movimento.** Sotto
`prefers-reduced-motion: reduce` il sollevamento sparisce, non il feedback:
ombra, bordo e fondo continuano a cambiare stato, solo senza spostamento.
Mai azzerare tutte le transizioni in blocco. Applicata su `.game-grid`;
`.event-grid` la eredita quando quella pagina verrà rivista.

## Shapes

Due raggi: `10px` (card, form, tabelle, pillole del tavolo in feltro) e
`6px` (input, bottoni, tab). Le pillole di navigazione (link attivi in
nav) usano `border-radius: 999px`. Nessun bordo colorato laterale sulle
card — il confine è sempre un `1px solid var(--card-line)` uniforme.

## Components

### Buttons
- **Shape:** 6px, padding `0.55rem 1rem`.
- **Primary** (default `<button>`): fondo rosso seme, testo bianco — ogni
  submit e ogni azione costruttiva.
- **`.btn-secondary`:** trasparente, bordo cartoncino, testo inchiostro —
  azioni non distruttive e non primarie ("Aggiungi giocatore", "Crea
  manualmente").
- **`.btn-danger`:** trasparente, testo/bordo color danger (`#ad4a22`) —
  cancellazioni, rimozioni, annullamenti fuori da una lista.
- **Bottoni in una `<li>`:** rosso danger di default (la maggioranza dei
  bottoni in lista sono "Rimuovi"). Una lista che non è fatta di azioni
  distruttive — i risultati BGG, dove la riga *è* la selezione — non mette
  bottoni nelle righe: si sceglie la riga, non un bottone dentro la riga.
- **`.link-button`:** nessun fondo né bordo, testo accento sottolineato —
  una deviazione dal percorso principale citata dentro una nota
  ("Inseriscilo a mano"), dove un bottone vero peserebbe quanto l'azione
  che si sta scavalcando.

### Cards / Containers
- **Corner Style:** 10px.
- **Background:** cartoncino (`#faf6ec`).
- **Shadow Strategy:** vedi Elevation.
- **Border:** `1px solid #ddd0ab`.
- **Cover mancante:** placeholder disegnato (`.cover-placeholder`) — feltro
  verde con trama a puntini e un dado a 5 centrato, mai un riquadro vuoto
  o un'icona generica (e mai iconografia da carte da gioco).
- **Card d'azione** (`.add-card`, ultima cella di una griglia): la stessa
  sagoma delle card vicine, ma **senza fondo proprio** — il cartoncino della
  pagina si vede attraverso — tenuta insieme da un bordo tratteggiato 2px in
  `color-mix(in srgb, var(--ink-muted) 70%, var(--card-line))`, che sta a
  3.1:1 sul fondo e quindi supera la soglia WCAG 1.4.11 per un confine di
  componente (`--card-line` da solo è 1.3:1: invisibile). È il posto dove
  una scatola manca, non una scatola più chiara. Al passaggio del mouse il
  tratteggio **si chiude** (`border-style: solid`, accento), il fondo
  diventa `--card-alt` e la card prende il lift delle vicine: lo slot si fa
  scatola. L'etichetta usa il font Display alla misura di un titolo di card
  ma **non** è un heading (`.add-label`): nomina un'azione.
- **La card d'azione è il link, non lo contiene.** Se il contenitore è la
  card e il link vive dentro, il link non si stira all'altezza della riga di
  griglia e resta una fascia in basso non cliccabile e fuori dall'anello di
  focus (bug reale trovato in audit). L'`<a>` è la cella, in flex-column,
  con lo slot dell'icona in `flex: 1 1 auto`.
- **La card d'azione sta fuori dalla lista.** La `<ul>` dei contenuti porta
  `role="list"` e `display: contents` (`.game-grid-items`), così le sue
  `<li>` restano celle della griglia e la card d'azione le sta accanto senza
  farsi contare come un elemento in più da uno screen reader. Il `role`
  esplicito è obbligatorio: `display: contents` rimuove la semantica di
  lista in Chrome e Safari.

### Testa di pagina (`.page-head`)
- Titolo + `.page-meta` a sinistra, **azione primaria in alto a destra**
  come `.action-link.is-compact` (icona `+` e testo, misura ridotta).
- Serve quando la pagina è una griglia o una lista lunga: la card d'azione
  in fondo alla griglia non basta da sola, perché scorre sotto la piega.
  Le due entrate puntano allo stesso posto ed è voluto.
- Su viewport stretto la riga va a capo e l'azione scende sotto il titolo.
- L'azione compatta si alleggerisce nel testo (0.88rem) e nel padding
  orizzontale, **mai nell'altezza**: `min-height: 44px`, il minimo per un
  bersaglio da dito.

### Scheda gioco (`/games/:id`)
- **La copertina è il controllo di caricamento** (`.cover-uploader`): un
  `<button>` che avvolge l'immagine, con un velo feltro all'88% e l'icona
  upload che compare su hover e su focus da tastiera. Il click apre il file
  picker e al `change` **l'upload parte da solo** — niente form, niente
  bottone "Carica" da premere dopo. Durante il caricamento il velo resta
  fisso su "Caricamento…" e il bottone è `disabled`. Per chi non è loggato
  la copertina torna una `<img>` senza affordance.
- **Dati BGG** (`.game-facts`): coppie etichetta/valore in una riga
  impacchettata a sinistra (`display: flex; flex-wrap: wrap`), **mai** una
  griglia `1fr` che li stira ai bordi della card. L'etichetta è
  maiuscoletto tracciato in `--ink-muted`, il valore è mono (`Data`): è un
  dato, non un titolo. Niente stat-tile a numero grande.
- **Azione distruttiva in testa** (`button.is-compact`): "Elimina" +
  cestino nella `.page-head`, non annegata in una riga di metadati insieme
  ai link di navigazione. Stessa misura di `.action-link.is-compact`, con
  l'icona **dopo** il testo — lì nomina la conseguenza, non anticipa
  l'oggetto.
- **Due fogli, non un foglio diviso** (`.panel-card` × 2): "Scheda" e
  "Media" sono card separate sotto la barra delle lingue, che le governa
  entrambe. Un filetto full-bleed in mezzo a una card sola tagliava il
  foglio invece di articolarlo.
- **Dentro un `.panel-card` il campo è largo quanto il foglio**: il tetto
  di 30rem sui campi vale nella colonna della pagina, dove i form si
  allineano ad altre card; dentro un pannello dedicato lasciava mezza
  superficie vuota.
- **Media come griglia di card** (`.media-grid`, riuso di `.game-grid`):
  ogni media è una tessera 16/9 con preview, titolo e tipo, e chiude la
  griglia lo slot "Aggiungi media". Le miniature YouTube arrivano da
  `img.youtube.com/vi/<id>/hqdefault.jpg`: è 4/3 con due bande nere, e il
  ritaglio a 16/9 con `object-fit: cover` toglie esattamente quelle. Se la
  miniatura non carica (video privato, rimosso, offline) la tessera cade
  sull'icona su feltro come per PDF e link.
- **"Rimuovi" sta fuori dal link**: un `<button>` dentro un `<a>` non è
  markup valido e su tastiera è una trappola. È un fratello in
  `position: absolute` nell'angolo, invisibile finché non serve, sempre
  visibile dove l'hover non esiste (`@media (hover: none)`), con l'area di
  tocco portata a 44px da uno `::after` trasparente.

### Amministratori (`/users`)
- **La riga admin** (`.admin-row`, dentro `.admin-list` in un `.panel-card`):
  la pedina con l'iniziale (`.admin-pawn`) sta dove il catalogo mette la
  copertina, lo `.status-badge` ("Attivo" / "In attesa") dove il catalogo
  mette l'anno. Un admin **in attesa** — invitato ma senza password ancora
  impostata — ha la pedina tratteggiata invece che piena: è una pedina non
  ancora posata sul tavolo, coerente con il tratteggio delle card
  "aggiungi". Le azioni stanno a destra (`.admin-row-actions`), sempre
  "Elimina", più "Copia link invito" quando la riga è in attesa.
- **Il link d'invito è una riga di dato, rientrata** (`.admin-invite`): sotto
  la riga principale, oltre la pedina, in mono e troncato con ellissi — mai
  a spingere il foglio più largo. Compare solo per un admin in attesa e
  resta visibile finché l'invitato non attiva l'invito (impostando la password)
  o la riga non viene eliminata: il token non scade e non si rigenera, quindi
  il link di oggi è quello valido fra una settimana.
- **Lo slot "Aggiungi admin" a riga piena** (`.admin-add`) è la variante
  orizzontale di `.add-card` del catalogo, stessa grammatica: bordo
  tratteggiato 2px senza fondo proprio (il cartoncino della pagina si vede
  attraverso), che **si chiude** in tratto continuo, accento, al passaggio
  del mouse. Cambia solo la sagoma — una riga piena invece di una cella di
  griglia — perché qui non c'è uno scaffale di caselle da chiudere, ma un
  elenco. Al click diventa un form inline (`.admin-add-form`, solo campo
  email) invece di navigare altrove: l'intera creazione di un invito è
  un'azione, non una pagina.
- **Nelle liste il rosso è "Rimuovi"**: la regola generale sui bottoni in
  `<li>` vale anche qui, quindi "Copia link invito" — che non distrugge
  niente — non può ereditarlo. Passa all'oro (`.btn-invite`, testo
  `--gold-text`, hover `--gold-bg`/`--gold`), la stessa famiglia di colore
  delle medaglie e dei distintivi, mai usata altrove per un'azione di
  lista.

### Eventi (`/admin/events` e lista pubblica)
- **Tessere larghe** (`.event-card-grid`, riuso di `.game-grid`): un evento
  non è una scatola, è una serata — l'immagine è **16/9**, le colonne vanno
  a `minmax(260px, 340px)`. Sotto l'immagine: titolo, data e ora, numero di
  giochi al tavolo.
- **L'immagine dell'evento è opzionale** e quando manca vale la stessa
  regola delle copertine: dado su feltro (`.event-image-placeholder`), mai
  un riquadro vuoto. Si carica con lo stesso gesto della copertina di un
  gioco (`EventImagePicker`, che riusa `.cover-uploader`): si clicca
  l'immagine, si sceglie il file, parte da sé. Nel form di creazione
  l'upload aspetta il salvataggio — l'evento deve esistere prima — e intanto
  si vede l'anteprima da un object URL.
- **La data si legge in italiano** (`formatEventDateTime`): "gio 1 ott 2026
  · 21:00", con il giorno della settimana, in mono con l'icona calendario a
  fare da insegna. Mai la data grezza dell'API (`2026-10-01`).
- **Due insiemi, non due filtri** (`.tab-bar`): "In programma" e "Passati"
  sono linguette, non checkbox — se ne guarda uno per volta. In programma
  ordina dal più vicino, i passati dal più recente: in entrambi i casi in
  cima c'è la serata di cui importa.
- **Lo slot "Crea evento" chiude solo la griglia dei futuri**: nell'archivio
  non si aggiunge niente.
- **Il pager sfoglia, non indirizza** (`.pager`): Precedente / posizione /
  Successiva, mai una fila di numeri di pagina. Compare solo quando le
  pagine sono più d'una.

### Copie numerate
Quando un evento porta più copie dello stesso gioco, il nome prende il
suffisso `#1`, `#2`. Il numero compare **solo** se le copie sono più
d'una: su un evento con una copia per gioco sarebbe rumore. I numeri
sono etichette stabili, non posizioni: eliminando una copia di mezzo
resta un buco, perché chi ha letto "#2" al momento di prenotare deve
ritrovare "#2".

### Posti prenotabili
La dicitura è sempre "posti prenotabili", mai "posti" da solo: un
"posto" si confonderebbe con la sedia intorno al tavolo o con il numero
di giocatori del gioco. Una copia con un solo posto prenotabile non
mostra nulla — resta la dicitura di disponibilità di sempre; da due in
su compare `Posti prenotabili liberi: 3 di 5`.

### Scheda evento admin (`/admin/events/:id`) e creazione (`/admin/events/new`)
- **Il titolo dell'evento è il titolo della pagina**, non "Modifica evento":
  la `.page-head` porta il nome salvato, la data formattata come `page-meta`
  e, a destra, `.page-head-actions` — "Vedi pagina pubblica"
  (`.action-link.is-compact`) e "Elimina" (`.btn-danger.is-compact`). È il
  primo posto in cui la testa regge **due** azioni: vanno a capo insieme,
  come blocco, non una per volta.
- **Fogli impilati, non una pagina sola** (`.panel-card`): Dettagli, Giochi
  dell'evento, Prenotazioni, Risultati. Il conteggio di una sezione sta
  nell'intestazione come `.section-count`, mono, all'altro capo del titolo —
  dove nelle altre sezioni stanno le azioni.
- **In un form fatto di pannelli i campi tengono il tetto di 30rem.** La
  regola opposta ("dentro un `.panel-card` il campo è largo quanto il
  foglio") vale quando è il pannello a contenere un form dedicato di un
  campo o due; qui i fogli sono le sezioni di un form lungo, e un titolo
  largo quanto il foglio si legge peggio.
- **Selezione giochi** (`EventGamesPicker`, condiviso tra creazione e
  scheda): i giochi già scelti si staccano in cima su `--card-alt` col
  campo **copie** in mono; sotto, la ricerca per nome (che compare solo
  oltre i 6 giochi in catalogo) e il resto del catalogo in un'area con
  scroll proprio (18rem), perché un form non deve allungarsi a fisarmonica
  quanto è grande lo scaffale. Un gioco con prenotazioni attive mostra
  "N prenotate", non si può togliere e il campo copie non scende sotto
  quel numero: il backend rifiuterebbe, e un campo che non scende è più
  onesto di un 409 dopo il salvataggio.
- **Prenotazioni e risultati riusano la lista di `/users`** (`.admin-list`
  + `.admin-row` + `.admin-pawn`): pedina con l'iniziale, nome e contatti,
  il gioco come pastiglia quieta (`.booking-game` — dice a cosa si
  riferisce la riga, non come sta, quindi non è uno `.status-badge`), e
  l'azione in fondo. I punteggi sono un dato: mono (`.match-scores`).
- **Annullare una prenotazione è l'azione del partecipante, fatta
  dall'admin**: stessa transazione (copia liberata, punteggio eliminato,
  codice non più valido), quindi si chiama "Annulla" e non "Elimina". Il
  bottone eredita il rosso dalla regola dei bottoni in `<li>`, con
  conferma che nomina partecipante e gioco.

### Aggiungi gioco (`/games/new`)
- **Un form solo, due fogli** (`.panel-form` + `.panel-card`): *Gioco* e
  *Dettagli* (lingua base, proprietario), un unico submit in fondo,
  disabilitato finché non c'è un gioco. Stessa impalcatura di
  `/admin/events/new`: `.back-link`, `.page-head`, fogli, `.form-actions`.
- **Cercare su BGG è scrivere, non premere "Cerca"** (`BggSearchSelect`):
  combobox che parte da sola dopo tre caratteri e una pausa di 350ms. Il
  bottone di ricerca era un passo in più per un gesto che si ripete finché
  il nome giusto non compare. Sotto i tre caratteri la nota dice cosa manca,
  durante la chiamata dice che sta cercando: la stessa riga (`.field-hint`
  con `role="status"`) porta tutti gli stati, invece di comparire e sparire.
- **Ogni riga ha la copertina** (`.bgg-thumb`, riquadro fisso 3rem con
  `object-fit: contain` su `--card-alt`): le copertine di BGG arrivano in
  proporzioni qualsiasi e un riquadro fisso tiene ferma la colonna. Manca
  la miniatura, o il browser non la carica: torna il segnaposto a dado.
- **Il peso BGG è una pastiglia oro all'estremo opposto del nome**
  (`.bgg-weight`, mono): si scorre la colonna dei numeri per capire se la
  serata regge il gioco, senza rileggere i titoli. È lo stesso dato che
  nella scheda gioco compare come fact "Complessità" — un decimale, non i
  quattro della media BGG.
- **La lista galleggia** (`position: absolute`) e si ferma alla misura del
  campo, non del pannello: è la continuazione dell'input, e spingendo i
  campi in basso muoverebbe il form a ogni tasto. Tastiera e mouse
  condividono **una sola** riga attiva (`.is-active`): il puntatore la
  sposta invece di accenderne una seconda.
- **Scelto il gioco la ricerca sparisce** e resta la riga di conferma
  (`.bgg-chosen`, `--card-alt`) con copertina, nome, anno, complessità e
  "Cambia": quel che resta da decidere è lingua e proprietario.
- **L'inserimento a mano è la riserva, non un percorso alla pari**: un
  `.link-button` dentro la nota sotto la ricerca, non una scheda o un
  segmented control. Quel che BGG non ha si scrive a mano, ma è
  l'eccezione.

### Griglie su telefono
- Sotto 560px `.game-grid` smette di tenere le celle a 230px fisse e passa
  a `repeat(auto-fill, minmax(140px, 1fr))`: il tetto fisso lasciava una
  colonna sola con mezzo schermo vuoto accanto. `.media-grid` fa eccezione
  e scende a una colonna piena — una miniatura video e un titolo che è un
  URL hanno bisogno di tutta la riga, e lo stesso vale per
  `.event-card-grid`: due tessere 16/9 per riga ridurrebbero l'immagine di
  una serata a una striscia.

### Inputs / Fields
- **Style:** fondo cartoncino, bordo `#ddd0ab`, radius 6px.
- **Focus:** outline 2px accento, offset 2px (mai un semplice cambio di
  bordo).
- **Checkbox inline** (`.checkbox-label`): riga orizzontale
  checkbox+testo, non la colonna verticale di default di `label`.

### Navigation
- Feltro verde, testo cartoncino attenuato di default, cartoncino pieno +
  testo feltro per il link attivo (pillola). Logout/Esci resta un
  bottone-ghost sul feltro, mai un bottone pieno rosso. Sotto 640px: vedi
  Layout.
- **Linguette** (`.tab-bar`): pastiglie mono maiuscoletto su una barra in
  feltro, angoli tondi su tutti i lati (mai linguette tagliate in basso).
  La barra è una sola grammatica per casi diversi — le lingue di un gioco
  (`.language-tabs`, che ci aggiunge solo l'azione "Aggiungi lingua" in
  fondo) e il periodo degli eventi.

### Tabelle / Scoreboard
- **Header:** feltro verde, testo cartoncino uppercase tracciato — lo
  stesso registro delle intestazioni da tabellone torneo.
- **Righe:** cartoncino, bordo inferiore sottile.
- **Podio:** le prime tre righe portano un `.rank-medal` circolare (oro,
  argento, bronzo) — mai un numero nudo per il podio.

### Badge (Signature Component)
- **`.rank-medal`:** cerchio 1.5rem, oro/argento/bronzo per le prime tre
  righe di classifica, grigio neutro altrove.
- **`.win-badge`:** testo oro + icona coppa disegnata (mai emoji 🏆) nello
  storico partite.
- **`.booking-code-card`:** cartoncino con bordo oro, etichetta uppercase
  piccola + codice in mono 2rem tracciato — l'unico elemento della UI che
  usa il bordo oro invece del bordo cartoncino standard, per segnalare
  "questo va conservato".

## Do's and Don'ts

### Do:
- **Do** usare il mono (`IBM Plex Mono`) per ogni dato che l'utente legge
  ad alta voce o ricopia (booking code, punteggi, date in lista).
- **Do** disegnare un'icona propria (coppa, stella, dado, pedina) per ogni
  stato che altrove verrebbe reso con un emoji — coerenza col mondo
  dado/pedina, mai un carattere Unicode improvvisato e mai iconografia da
  carte da gioco/poker.
- **Do** restringere il feltro verde a chrome strutturale e al "tavolo"
  della pagina evento pubblica; le schermate admin dense restano su
  cartoncino/fondo neutro.
- **Do** avvolgere ogni tabella larga in `.table-scroll` prima di
  aggiungere colonne: la pagina non scorre mai in orizzontale.
- **Do** portare l'anello di focus **dentro** la card
  (`outline-offset: -2px`) su ogni contenitore con `overflow: hidden`: la
  regola globale lo disegna 2px fuori dal link, dove viene ritagliato e la
  navigazione da tastiera resta senza indicatore (bug reale trovato in
  audit su `.game-grid`).
- **Do** dare `loading="lazy"` e `decoding="async"` alle copertine in
  griglia, più `width`/`height` intrinseci: il catalogo cresce e non deve
  scaricare l'intero scaffale al primo paint.

### Don't:
- **Don't** usare iconografia da carte da gioco/poker (semi, dorsi a righe,
  "indice d'angolo" come metafora dichiarata) — corretto in sessione dopo
  feedback utente: il motivo proprio dell'app è dado/pedina (Eurogame),
  non poker.
- **Don't** usare l'oro (`#b8842a`) come colore di testo corrente su
  cartoncino: sotto la soglia AA per testo normale (3.05:1).
- **Don't** lasciare un `<li>` di lista ereditare lo stile "Rimuovi" di
  default per un'azione non distruttiva — serve sempre una classe esplicita
  (`.btn-invite` per l'oro, `.link-button` per una deviazione citata).
- **Don't** dare per scontato il `justify-content` di un `<li>`: la regola
  base delle liste li distribuisce agli estremi, e una riga a cui manca
  l'ultimo elemento (una pastiglia opzionale) si ritrova il testo appiccicato
  al bordo destro. Chi costruisce una riga nuova lo dichiara.
- **Don't** dimenticare `display: block` su un `<li>` di griglia-carta:
  eredita altrimenti il flex-row di default della lista base e la card
  collassa (bug reale trovato e corretto in questa sessione, su
  `.event-grid li` ed `.event-games li`).
