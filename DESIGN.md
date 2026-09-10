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

Contenitore centrale max 56rem (`.app-page`, dentro `.app-main`), padding
laterale 1.5rem che scende a 1rem sotto i 640px. Le griglie di card
(catalogo, eventi, tavolo di un evento) usano
`repeat(auto-fill, minmax(Xpx, 1fr))` — 180-220px a seconda della densità
— così il numero di colonne si adatta senza breakpoint espliciti fino al
passaggio a colonna singola su mobile.

**Shell:** una sola shell (`AppShell.vue`) per tutta l'app — topbar in
feltro alta 3.25rem, sticky, con hamburger + marchio a sinistra e menù
utente a destra; sidebar in feltro profondo larga 15rem, chiusa per
default. Da 900px è una colonna `sticky` nel flusso e il contenuto le fa
spazio da sé — chiusa esce dal flusso e il passaggio è istantaneo: nessuna
larghezza o padding animato, che sarebbe un reflow a ogni frame. Sotto i
900px è un drawer `fixed` che entra in `transform` sopra la pagina, con
overlay in dissolvenza, scroll del body bloccato
(`body.has-open-drawer`) e contenuto reso `inert`, così il Tab non esce dal
menù per finire in una pagina che non si vede. Lo stato aperto/chiuso è una
preferenza da desktop salvata in `localStorage` (`bgm.sidebar`): il drawer
su telefono parte sempre chiuso e non la sporca. Sotto i 520px il wordmark
si nasconde lasciando la sola carta-marchio (pedina), perché hamburger,
marchio e pallina utente devono stare su una riga sola. Setup, login e
accettazione invito (`meta.bare`) restano pagine a schermo pieno senza
shell.

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
`6px` (input, bottoni, tab, voci di sidebar). Restano tondi a `999px` solo
gli elementi che sono davvero pastiglie: linguette e trigger del menù
utente. Nessun bordo colorato laterale sulle
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
- **`.btn-with-icon`:** bottone in `inline-flex` con l'icona (1.05rem,
  tratto 1.7, mai emoji) **prima** dell'etichetta e 0.45rem di respiro. Si
  combina con le varianti sopra (`.btn-danger.btn-with-icon`). L'icona
  nomina la conseguenza — dischetto per salvare, cestino per rimuovere,
  × cerchiata per annullare, lente per cercare — e serve dove l'azione va
  riconosciuta di colpo, in piedi al tavolo. Diverso da `.is-compact`, che
  rimpicciolisce un'azione di testa: qui cambia il contenuto, non la
  misura.
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
- **Gruppo di card** (`.section-group`): quando due o più `.panel-card`
  dipendono dalla stessa cosa — la lingua scelta, la chat — un tappeto di
  feltro (`--felt`, raggio 10px, padding 0.4rem) le tiene insieme sotto una
  testata (`.section-group-head`). **Non è una card**: niente cartoncino,
  bordo né ombra, perché una `.panel-card` dentro una `.panel-card` raddoppia
  bordo e padding, e il sistema la rifiuta ovunque. Le figlie perdono i
  margini di colonna e restano separate da 0.4rem: il filo di feltro fra due
  fogli della stessa mano. Il nome del gruppo è un'etichetta, non un titolo di
  foglio — maiuscoletto mono in `--felt-text` — e resta un `h2` mentre i
  titoli delle card figlie scendono a `h3` (`.section-head h3` ha la stessa
  resa di `h2`: cambia il posto nella gerarchia, non il peso sulla pagina).
  Serve a rispondere a "di chi è figlia questa card": card in fila non lo
  dicono, e una barra di linguette in cima sembra governare tutto quel che la
  segue.
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

### Rimando indietro (`.back-link`)
- Freccia + una parola che nomina **dove** si torna ("← Catalogo",
  "← Eventi"), sopra il titolo, in `--ink-muted`. Mai "← Torna alla pagina
  precedente": la parola è il posto, non il gesto.
- **`min-height: 44px`** come le azioni compatte, con `padding` orizzontale
  e un `margin-left` negativo che lo compensa: il bersaglio è da dito
  (queste pagine si aprono da telefono) e il testo resta otticamente
  allineato alla colonna.
- Due rese, una sola misura: `router-link` quando la destinazione è un
  posto fisso, `<button>` quando torna nella **storia del browser** — è il
  caso della scheda gioco pubblica, dove si arriva dalla serata o da un
  link condiviso, e l'etichetta è allora la sola generica ammessa
  ("← Indietro") perché la destinazione non è nota. Senza storia (scheda
  aperta da un QR) il bottone porta al calendario invece di non fare
  nulla.

### Scheda gioco: due pagine, non una con due volti

La scheda gioco è **sdoppiata**, come già gli eventi: `/games/:id` è quel
che vede chi partecipa, `/admin/games/:id` è dove si lavora. Prima era una
pagina sola con dieci rami `v-if="auth.user"` dentro, e le due letture si
ostacolavano: l'admin trovava campi sparsi tra il contenuto, il
partecipante una pagina disegnata attorno a controlli che non vedeva. Quel
che è di entrambe sta in due componenti condivisi — `GameFacts` (i dati
BGG) e `GameMediaList` (la griglia media, con prop `editable`).

#### Scheda pubblica (`/games/:id`)
- **Nessuna azione primaria in testa.** Qui si legge: il titolo non ha
  `.page-head` con bottone a destra, e i rimandi — "Classifica", e
  "Modifica" solo per l'admin già loggato — stanno nella riga
  `.page-meta`. Un bottone accento pieno in testa a una pagina pubblica
  prometterebbe un'azione che il partecipante non deve fare.
- **La copertina è un'immagine e basta**: nessuna affordance di upload,
  nessun velo, nessun bottone attorno.
- **Le linguette lingua restano**, senza l'azione "Aggiungi lingua": la
  lingua del manuale è un'informazione che serve a chi sta al tavolo.
- **Lo stato d'errore parla italiano e offre un'uscita.** Un `/games/:id`
  che non esiste (link vecchio, QR di un evento passato) mostra "Questa
  scheda non è disponibile…" più il rimando ai prossimi eventi — non la
  stringa dell'API (`game not found`) e mai una pagina bianca, che è quel
  che faceva prima con `v-if="game"` e il messaggio d'errore dentro.

#### Chiedi al manuale (`ManualChat.vue`, `ManualChatPanel.vue`)

Sulla scheda pubblica di un gioco con manuale indicizzato compare una chat
che risponde a domande sul regolamento a parole proprie, citando la pagina.
`ManualChat.vue` decide **dove** vive, `ManualChatPanel.vue` **cosa** ci
sta dentro — la stessa divisione di `GameFacts`/`GameMediaList` altrove in
questa scheda.

- **Due forme, una soglia sola.** Da 1100px in su è una barra destra **a
  tutta altezza, ancorata al bordo del viewport** (`.manual-chat-aside`,
  22rem, `position: fixed; right: 0`), sempre aperta; sotto, un bottone
  tondo in basso al centro (`.manual-chat-fab`) che apre un `<dialog>`
  nativo a tutto schermo (`.manual-chat-dialog`). **Perché 1100 e non
  900**: `.app-page` è larga 56rem (896px). Una barra da 22rem dentro
  quello spazio lascerebbe alla colonna di testo
  `56rem − 22rem − gap(1.5rem) ≈ 32.5rem` — sotto la misura leggibile
  (60–75 caratteri, che qui vale circa 34rem). Il breakpoint non è la
  larghezza di `.app-page` stessa: è quella più il margine che serve
  perché la pagina non tocchi i bordi del viewport, da cui i ~1100px
  effettivi invece dei 896px nudi — la somma resta la ragione del
  breakpoint anche ora che i 22rem non sono più una colonna di griglia ma
  uno spazio riservato altrove (vedi sotto). **22rem resta la larghezza**:
  l'ancoraggio al bordo non cambia quel calcolo, e a 22rem deep-chat
  mostra già bolle e campo leggibili — allargarla toglierebbe spazio al
  testo senza un beneficio misurato.
- **La barra tocca il bordo del viewport, non il bordo di `.app-page`.**
  Una prima resa (a tutta altezza ma ancora dentro la griglia di
  `.game-detail-layout`, quindi dentro la scatola centrata di `.app-page`,
  `max-width: 56rem; margin: 0 auto`) lasciava un margine vuoto fra il
  bordo destro della barra e il bordo della finestra — non toccava mai il
  bordo come `.app-sidebar` tocca il suo a sinistra, ed è quel confronto
  che la rendeva "non tutta a destra" (segnalato dall'utente). Ora
  `.manual-chat-aside` è `position: fixed; top: 3.25rem; right: 0; bottom:
  0`: esce dal flusso di `.app-page` e si ancora al viewport, non a una
  scatola centrata al suo interno. **Solo il filetto sul lato che guarda
  il contenuto** (`border-left`), **nessun raggio**: gli angoli tondi a
  sinistra, provati in una prima resa, facevano leggere una barra alta
  quanto il viewport come una card gigante fuori posto (segnalato
  dall'utente). Una barra è una parete, e una parete non ha angoli
  smussati.
- **Lo spazio si riserva su `.app-main`, non su `.app-page`.** Uscita dal
  flusso, la barra non spinge più via nessuno: senza una riserva
  esplicita il contenuto le passerebbe sotto. La riserva **non** può stare
  su `.app-page` (`padding-right` lì sposterebbe il suo stesso centro,
  perché porta `margin: 0 auto` — la colonna di testo resterebbe
  schiacciata a sinistra invece di ricentrarsi in quel che resta) né su
  `.game-detail-layout` (stesso problema, un livello sotto). Sta un
  livello sopra, su `.app-main` (`AppShell.vue`): `.app-main:has(.manual-
  chat-aside) { padding-right: 22rem }` restringe la scatola dentro cui
  `.app-page` si centra, e `.app-page` si ricentra da sé nello spazio che
  resta — lo stesso meccanismo con cui si centra oggi rispetto ad
  `.app-sidebar` a sinistra. `:has()` limita la riserva alle pagine che
  montano davvero la barra: senza, ogni pagina admin/pubblica perderebbe
  22rem a destra per niente.
- **`.manual-chat-panel` porta `flex: 1`**: senza, un figlio senza
  `flex-grow` dentro una colonna flex resta alto quanto il suo contenuto
  — il difetto misurato in una fase precedente, la barra si fermava a
  ~455px con spazio vuoto sotto anche ad altezza piena. Il pannello è
  figlio diretto dell'aside (e del dialog): il vecchio involucro
  intermedio `.manual-chat-aside-body` è stato tolto, non serviva un
  secondo contenitore per portare lo stesso `flex: 1`. **Sopra il
  breakpoint non collassa più**: il toggle (`.manual-chat-toggle`) e lo
  stato `is-collapsed` sono stati rimossi, scelta esplicita — la sidebar
  di sinistra non si nasconde da desktop, e un bottone che facesse
  sparire quella di destra sarebbe un'incoerenza in più da spiegare senza
  un risparmio di spazio che serve davvero. Sotto il breakpoint non
  cambia niente: bottone tondo e dialog a tutto schermo.
- **Il bottone tondo**: `position: fixed`, centrato con
  `left: 50%; transform: translateX(-50%)`, `bottom: calc(1rem +
  env(safe-area-inset-bottom))` — il termine `env()` lo tiene sopra la home
  indicator di iPhone invece che sotto, coperto. **Il padding della pagina
  sotto 1100px deve portare lo stesso termine** (`.game-detail-layout.has-chat
  { padding-bottom: calc(4rem + env(safe-area-inset-bottom)) }`): senza,
  su un device con inset diverso da zero il bottone sale ma la riserva di
  spazio sotto l'ultimo contenuto resta quella vecchia, e il bottone torna a
  coprire quel contenuto. I due valori si muovono insieme — non è successo
  al primo tentativo, va tenuto a mente a ogni modifica dell'uno o
  dell'altro.
- **Il bottone tondo porta il dado che parla**: la sagoma del fumetto con
  dentro tre pip in **diagonale**, la faccia 3 di un dado — la diagonale è la
  firma del dado, tre puntini in fila sarebbero il glifo "chat" di chiunque.
  È la stessa grammatica del segnaposto copertina (rettangolo tondo, pip
  pieni), non un'icona presa a prestito.
- **Sul verde l'anello di focus passa al cartoncino anche qui.** La regola
  generale (vedi Do) vale ora su una superficie in feltro in più oltre a
  topbar e sidebar: la testata della chat, con il suo ＋ e la sua ×. Il bottone tondo fa eccezione
  e tiene il rosso: `outline-offset: 2px` disegna l'anello **fuori** dal
  bottone, sul cartoncino della pagina, dove sta a 6.4:1.
- **deep-chat vive in shadow DOM**: i token di `app.css` (`--felt`,
  `--felt-text`, `--card`, `--card-alt`, `--card-line`, `--ink`,
  `--ink-muted`, `--accent`, `--danger`, `--danger-bg`) non attraversano quel
  confine. Colori e misure si passano invece per proprietà JS
  (`messageStyles`, `textInput`, `submitButtonStyles`, `speechToText.button`,
  `auxiliaryStyle`), il che significa che **i valori di quei token sono
  duplicati a mano, come letterali esadecimali, in
  `frontend/src/components/ManualChatPanel.vue`**: bolla utente =
  `--felt`/`--felt-text`, bolla IA = `--card-alt`/`--ink`, bolla d'errore =
  `--danger-bg`/`--danger` (gli stessi due di `.error`, perché il rosa di
  serie di deep-chat non esiste in questa palette), campo di testo =
  `--card`/`--card-line`/`--ink` col fuoco in `--accent`. Chi cambia uno di
  quei token in `app.css` non vede la chat seguire: deve aprire anche
  `ManualChatPanel.vue` e aggiornare i valori a mano, altrimenti la chat
  resta l'unica isola col colore vecchio, e niente nel codice lo segnala da
  solo.
- **La chat ha un nome e una testata** (`.manual-chat-head`, dentro
  `ManualChatPanel.vue`, quindi la stessa in barra e in dialog): fondo
  feltro, **"L'Arbitro"** in Display — chi risolve le dispute al tavolo —
  con sotto `risposte dal manuale` in `--felt-text-muted`, e a destra il
  ＋ di "Nuova conversazione" più, **solo nel dialog** (prop `closable`,
  evento `close`), la ×. Prima la testata era markup del dialog e la barra
  desktop era una colonna di bolle senza nome, che non diceva nemmeno di
  essere una chat (segnalato dall'utente). Il ＋ compare **solo a
  conversazione avviata**: nello stato di riposo non c'è niente da
  azzerare, e un ＋ che non fa nulla è peggio di un ＋ che manca. Quando
  spariscono il ＋ e la conversazione, il fuoco va sul finto campo
  (`nextTick`), non sul body.
- **Lo storico vive in `localStorage`, non nel database.** Una chiave per
  gioco (`bgm-chat-<gameId>`), gestita da `browserStorage` di deep-chat —
  che scrive a ogni messaggio e rilegge al render, purché non gli si passi
  anche `history` — con lo stesso tetto di `requestBodyLimits`
  (40 messaggi). Nessuna tabella e nessun identificativo da inventare per
  chi non ha un account: la conversazione resta sul telefono di chi l'ha
  fatta. Il ＋ toglie la chiave e smonta il componente; l'unico punto in
  cui la chiave la leggiamo noi è al mount, per decidere se **montare
  deep-chat subito** (c'è una conversazione da riaprire, e quei 457 KB chi
  la ha fatta li ha già scaricati) o restare nello stato di riposo. Ogni
  accesso è in `try/catch`: in navigazione privata su Safari il solo
  toccare `localStorage` lancia, e lì la chat funziona senza storico.
- **Il campo deve dichiarare la sua altezza, non ereditarla.** `flex: 1`
  stira l'elemento `<deep-chat>` ma il suo `#container` interno resta al
  default di 350px e si incolla in cima: il campo di testo finisce a metà
  altezza con una fascia avorio morta sotto (misurato nel dialog: elemento
  738px, container 350px; e segnalato dall'utente sulla barra desktop).
  Serve `height: 100%` sul widget, **in entrambi i contenitori** ora che
  entrambi sono colonne a tutta altezza — la vecchia eccezione «solo nel
  dialog» risaliva a quando la barra era una colonna da 270px dentro la
  griglia della pagina, dove la stessa riga faceva il danno opposto.
  Perché quel 100% sia esatto, **il corpo che lo contiene ha un figlio
  solo** (`.manual-chat-body`): la nota "Controlla sempre la pagina
  citata." sta fuori, sopra, sotto la testata. Un fratello dentro quel
  corpo si prende una fetta della colonna e spinge il campo sotto il
  taglio, perché il corpo è `overflow: hidden` (lo scroll è di deep-chat,
  dentro di sé: senza, attorno alla sua barra ne comparirebbe una
  seconda).
- **Microfono e invio stanno dentro il campo, a destra, uno accanto
  all'altro** (`position: 'inside-end'` per entrambi). I nomi ammessi sono
  solo `inside-start`, `inside-end`, `outside-start`, `outside-end`: un
  valore fuori da questi (provato `inside-right`) spinge l'invio fuori dal
  campo, oltre il bordo destro dello schermo, e lo taglia a metà — che è
  anche quel che fa `outside-end` di suo, con il campo largo quanto il
  pannello. Due bottoni nella **stessa** posizione però deep-chat li
  disegna allo stesso scarto (`left: -27px`, misurato), uno sopra
  l'altro: il microfono prende quindi `left: 'auto'; right: '2.2em'` nel
  suo `container.default` (`micIconButton`) e si sposta di un bottone a
  sinistra, nell'ordine di ogni app di messaggi — dettatura, poi invio.
  **Il rientro del testo è solo a destra** (`paddingRight: '4.3em'`): il
  microfono a sinistra costringeva a un rientro di 2.7em anche sui browser
  senza Web Speech, dove quel bottone non esiste, e il placeholder partiva
  da metà campo (segnalato dall'utente).
- **Un anello di focus solo.** `border: 1px` più `outline: 2px` sullo
  stesso bordo da 6px di raggio si scollavano agli angoli e il campo
  sembrava avere gli spigoli smangiati (segnalato dall'utente). Resta il
  bordo in `--accent` e l'anello passa a
  `box-shadow: 0 0 0 3px rgb(156 43 43 / 32%)`, che segue il raggio
  esattamente.
- **Le bolle non hanno la stessa larghezza.** A 92% per tutte arrivavano
  quasi da bordo a bordo e la conversazione si leggeva come una pila di
  blocchi centrati, senza un lato di chi parla: la domanda sta a 82%, la
  risposta a 88%, e l'angolo basso dal lato del parlante è schiacciato a
  3px — una codina, la stessa idea del dado che parla sul bottone tondo.
  Dentro la bolla il link alla pagina del manuale prende `--accent`
  (`auxiliaryStyle`): di serie era il blu del browser (#0000EE, misurato),
  l'unica cosa blu dell'app, nell'unica bolla che cita una fonte.
- **I due bottoni si centrano da soli, non a un valore fisso.** deep-chat
  li ancora al fondo del campo con un margine costante
  (`inset-block-end: .85em`, indipendente dall'altezza della riga), e la
  nostra riga (dal `padding` di `textInput.styles.text`) è più alta del
  suo default: misurato in browser, campo alto 36,5px con centro a
  520,4px, bottoni alti 22px circa centrati a 513,4px — 7px più in alto
  del centro vero. La correzione sta in `inputIconButton.container.default`
  (`ManualChatPanel.vue`): `top: '50%'` + `transform: 'translateY(-50%)'`
  sull'elemento del bottone stesso (le classi `input-button`/`inside-start`/
  `inside-end` sono tutte sullo stesso nodo). Un valore percentuale e non un
  pixel fisso, perché resta corretto qualunque altezza prenda la riga in
  futuro — un `top` fisso andrebbe ricalcolato a ogni cambio del padding del
  campo. Dopo la correzione i due centri coincidono (misurato: campo e
  bottoni entrambi a 442,7px in un altro punto della pagina), in sidebar,
  nel dialog e sull'hover (che tocca solo `backgroundColor`, non
  `top`/`transform`, perché `default` viene sempre riapplicato prima di
  `hover`). Lo stesso centraggio va passato anche a `loading` e `stop`
  (`inputIconPosition`, il solo `container` e non il filtro sull'SVG, che
  sono i tre puntini dell'attesa e il quadrato di stop, già del grigio
  giusto): senza, durante l'attesa la riga sobbalza di quei 7px.
- **I due bottoni dentro il campo restano bersagli da 22px** ed è un limite
  accettato, non una svista: sono `position: absolute` dentro contenitori a
  larghezza zero nello shadow DOM di deep-chat, e imporre 44px dalle
  proprietà documentate li sposta fuori dal campo (provato e osservato). Chi
  scrive da telefono manda comunque con il tasto invio della tastiera. Tutti
  i bersagli che sono **nostri** — domande suggerite, finto campo, bottone
  tondo, ＋ della testata e × del dialog — stanno a 44px.
- **Lo stato di riposo si ancora in fondo** (`.manual-chat-rest` è
  `flex: 1`, `.manual-chat-intro` porta `margin-top: auto`): introduzione,
  domande e finto campo scendono insieme come un blocco solo. Nel dialog a
  tutto schermo questo li porta dove arriva il pollice di chi sta in piedi
  al tavolo, e lascia sopra lo spazio dove compariranno le risposte — un
  filo di conversazione ancora vuoto, non un buco. Ancorarli in alto
  significava, al primo gesto, veder saltare l'invito dalla cima del
  telefono al fondo, dove deep-chat mette il campo vero. Nella sidebar
  desktop non c'è spazio libero da distribuire e non cambia niente.
- **Scorrono invito e domande, non il finto campo**
  (`.manual-chat-rest-scroll` con `overflow-y: auto`, il finto campo suo
  fratello a `flex: none`). Da quando le tre domande le scrive un modello
  sono frasi di lunghezza variabile fino a 120 caratteri, non più tre righe
  della stessa forma: misurato, in barra desktop (22rem) una domanda lunga
  prende quattro righe e il blocco arriva a 418px. Dove non ci sta —
  telefono in orizzontale, finestra bassa — l'`overflow: hidden` di
  `.manual-chat-body` lo tagliava senza scorrimento, e la prima cosa a
  sparire era proprio il finto campo, cioè l'invito a scrivere. Dove ci sta
  (barra desktop e telefono in verticale, misurati) non compare nessuna
  barra di scorrimento e non cambia niente.
- **Le domande suggerite sono una `<ul>` spogliata** come `.admin-list`:
  senza, la regola globale la veste da card e mette a ogni `li` padding e
  filetto, e tre pastiglie finiscono incorniciate due volte dentro un
  riquadro dentro la card della chat. La pastiglia è `min-height: 44px` e
  **non** `height`, con `overflow-wrap: anywhere`: cresce su più righe, e
  una domanda senza spazi (il server ne limita la lunghezza, non la forma)
  non sfonda la barra.
- **Stato di riposo, non un caricamento silenzioso.** Prima di ogni gesto
  il pannello mostra markup nostro: l'introduzione, tre domande suggerite
  (le manda il server già scritte, `suggestedQuestions` — le genera il
  modello dai titoli di sezione del manuale a ogni indicizzazione e
  l'admin le riscrive a mano nel pannello dedicato, vedi «Domande
  suggerite» più sotto; a lista vuota restano tre domande fisse: come
  finisce la partita, in quanti si gioca, come si contano i punti) e un
  finto campo di input con le stesse misure di
  quello vero. deep-chat si monta solo al primo clic. Il motivo è il peso:
  misurato in build, il chunk `deepChat` pesa **457 KB** non compresso, da
  solo più dell'intero resto del JavaScript dell'app (`index.js`, **228
  KB**). Chi apre una scheda gioco per leggerla — la maggioranza — non deve
  pagare quel download. È anche quello che rende sostenibile tenere la
  sidebar desktop aperta di default: se deep-chat si caricasse al mount
  della pagina, aprirla per tutti costerebbe quel peso a ogni visita.

#### Scheda di modifica (`/admin/games/:id`)
- **La copertina è il controllo di caricamento** (`.cover-uploader`): un
  `<button>` che avvolge l'immagine, con un velo feltro all'88% e l'icona
  upload che compare su hover e su focus da tastiera. Il click apre il file
  picker e al `change` **l'upload parte da solo** — niente form, niente
  bottone "Carica" da premere dopo. Durante il caricamento il velo resta
  fisso su "Caricamento…" e il bottone è `disabled`. Vive solo qui: sulla
  scheda pubblica la copertina è una `<img>`.
- **Dati BGG** (`.game-facts`): coppie etichetta/valore in una riga
  impacchettata a sinistra (`display: flex; flex-wrap: wrap`), **mai** una
  griglia `1fr` che li stira ai bordi della card. L'etichetta è
  maiuscoletto tracciato in `--ink-muted`, il valore è mono (`Data`): è un
  dato, non un titolo. Niente stat-tile a numero grande.
- **Testa a due azioni, identica alla scheda evento admin**
  (`.page-head-actions`): "Vedi scheda pubblica"
  (`.action-link.is-compact`, `target="_blank"`, icona freccia-fuori e
  `aria-label` che dice che apre una scheda nuova) e "Elimina"
  (`button.btn-danger.is-compact`, cestino **dopo** il testo — lì nomina la
  conseguenza, non anticipa l'oggetto). L'azione distruttiva sta in testa,
  non annegata in una riga di metadati insieme ai rimandi.
- **`.back-link` verso il catalogo**, come `/admin/games/new`.
- **Due fogli dentro il gruppo «Lingue»** (`.section-group`): "Scheda" e
  "Media" sono card separate — un filetto full-bleed in mezzo a una card sola
  tagliava il foglio invece di articolarlo — ma stanno **dentro** il tappeto
  di feltro la cui testata porta l'etichetta "Lingue", le linguette e
  "Aggiungi lingua". La barra, che dentro il gruppo *è* la testata, perde il
  proprio fondo (`.section-group-head .tab-bar { background: none }`):
  ripeterlo disegnava due raggi concentrici sullo stesso verde. Prima le
  quattro sezioni della pagina erano in fila e la barra sembrava governarle
  tutte e quattro, comprese quelle che con la lingua non c'entrano.
- **La `.lang-chip` sulle due card resta anche col gruppo.** È ridondante
  rispetto alla linguetta attiva, ma il form della scheda è lungo: a metà
  descrizione la barra è fuori schermo, e la pastiglia è l'unica cosa che
  dice in che lingua si sta scrivendo.
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

#### Indice dei file BGG (`BggFilesPicker`)
- **Un indice, non un download** (`.bgg-files`, dentro il modale "Aggiungi
  media", sotto i campi). BoardGameGeek tiene i manuali di quasi tutto,
  spesso tradotti, ma li serve solo a chi ha fatto login sul sito:
  scaricarli dall'app non si può. Quel che si può togliere è la caccia al
  file. Una riga apre la sua pagina su BGG in una scheda nuova — dove
  l'admin è già loggato — e intanto **scrive il titolo nel campo sopra**:
  al ritorno resta da allegare il PDF e salvare. Nessun media nasce finché
  il file non c'è davvero.
- **Sta chiuso, ed è un `<details>` nativo** — come la modale è un
  `<dialog>` nativo. Aperto si prendeva più di metà del foglio (663px di
  modale su desktop) e comprimeva in cima il controllo primario, che è
  scegliere il PDF. La riga di riepilogo porta già il conteggio in mono
  ("11 file in italiano"), così si sa se vale la pena aprirlo — e un
  "nessun file in italiano" si legge senza un click. Il triangolino di
  serie sparisce a favore del caret disegnato del menù utente, l'unica
  freccia del sistema, che ruota all'apertura.
- **`align-self: stretch`, come `.bgg-select` e `.segmented`.**
  `.app-page form` allinea i figli a `flex-start`: un blocco senza
  larghezza propria si dimensiona sul contenuto, qui il titolo più lungo,
  che è `white-space: nowrap` e non ha un minimo. La modale finiva a
  scorrere in orizzontale invece di troncare i titoli (bug reale). Chi
  aggiunge un blocco a piena riga dentro un form lo dichiara, e mette
  `min-width: 0` dove il contenuto deve poter scendere sotto la sua misura.
- **La lista è quella della lingua aperta.** L'indice parte filtrato sulla
  lingua della scheda; quando non c'è niente, la nota lo dice e offre le due
  vie d'uscita su una riga sola (`.bgg-files-actions`, `.link-button` +
  link): allargare a tutte le lingue, o andare sul sito. Nessun file non è
  un errore, come nessuna mappa su un evento.
- **Le lingue si scrivono in italiano** (`languageName` in `utils/game.ts`,
  condiviso con il bottone "Traduci in…"): BGG etichetta i suoi file in
  inglese ("Italian", "English"), l'interfaccia no. Il backend manda anche
  il codice lingua, e la pastiglia ricade sul nome inglese solo per le
  lingue fuori mappa; un file senza lingua è "neutro".
- **La riga è il bottone** (`.bgg-file`), come al banco prestiti: si sceglie
  la riga, non un bottone dentro la riga. Vale quindi la spoliazione delle
  liste dentro una card — `.bgg-file-list` rinuncia a ombra e margine di
  `ul`, le sue `li` al padding, **ma il filetto tra le righe resta**: la
  scatola scorre, e una riga tagliata a metà dal bordo inferiore si legge
  come "continua" solo se le righe sono visibilmente separate — più due
  rifiuti dell'eredità:
  il rosso che `li button` dà per default ai "Rimuovi" (questo bottone non
  distrugge niente) e l'anello di focus disegnato fuori dal bordo, che una
  scatola con `overflow-y: auto` ritaglierebbe sulla prima e sull'ultima
  riga visibile (`outline-offset: -2px`).
- **Il nome del file è un dato, e si tronca** (`.bgg-filename` in mono
  dentro `.bgg-meta`, con dimensione e voti): è quello che si riconosce
  nella cartella dei download, quindi va troncato lui, non la riga di
  metadati che gli sta accanto.

#### Gruppo «Chatbot»: Knowledge base e Domande suggerite
Il secondo `.section-group` della pagina, con testata "Chatbot" e due card
figlie: "Knowledge base" (le fonti indicizzate) e "Domande suggerite".
**Nel gruppo non compare una sola `.lang-chip`, e l'assenza è
l'informazione**: quel che sta qui è per gioco, non per lingua — la chat
cerca in tutte le fonti insieme e le tre domande sono uniche. Prima le due
card erano in fila sotto la barra delle lingue, che sembrava governare anche
loro: il gruppo esiste per staccarle, e la testata "Chatbot" dice di chi sono
figlie.

#### Knowledge base (`ManualPrepPanel.vue`)
La prima card del gruppo, un tempo la sezione «Chatbot» stessa: sta fuori da
"Media" perché le due rispondono a domande diverse ("che materiali ha il
gioco" contro "quali sono pronti per la chat") e mischiarle confondeva un
pannello alto per file con la griglia dei media. Una riga per fonte
indicizzabile del gioco (PDF, txt, md o docx), riuso dello stesso scaffold di
`.admin-list`/`.admin-row` delle righe amministratori (`/admin/users`):
titolo (`.manual-doc-title`), pastiglia di formato (`.lang-chip`, la
stessa dell'indice lingua "IT" — l'estensione letta da
`fileExtensionLabel` in `utils/game.ts`, condivisa con `GameMediaList.vue`
per non calcolarla due volte con criteri diversi), stato in mono
(`.manual-doc-state`: "3 sezioni indicizzate" / "Non preparato") e azioni
a destra (`.admin-row-actions`). Niente più bozza da correggere: la riga
dice quante sezioni cercabili esistono, non il testo che le compone — il
testo estratto non si conserva da nessuna parte, per scelta.

- **Gli avvisi stanno a livello di sezione, detti una volta.** Un pannello
  per file ripeteva identici, sotto ogni fonte, l'avviso sui tempi lunghi
  e il motivo per cui manca "Prepara" senza provider AI — con più di un
  documento diventava rumore ripetuto. Ora la sezione mostra uno dei due
  avvisi una volta sola, sopra l'elenco: quello sui tempi (se un provider
  c'è) o il motivo pratico (se manca). Nessun documento indicizzabile
  lascia una riga sola in `.empty-note` che dice cosa caricare (PDF, txt,
  md, docx), senza né avviso né elenco vuoto.
- **Rosso pieno solo la prima volta.** "Prepara per le domande" è l'azione
  di una riga finché un indice non c'è; su una fonte già indicizzata
  "Prepara di nuovo" passa a `.btn-secondary`: rifà una lettura lunga e
  sostituisce quel che c'è, e da bottone più acceso della riga pesava più
  del "Salva" della scheda accanto.
- **Senza provider AI la riga non offre "Prepara".** La segmentazione
  di un txt/PDF testuale, la conversione di un docx e l'OCR di uno scan
  passano tutte dal modello configurato nelle impostazioni — anche il caso
  che tecnicamente non ne avrebbe bisogno, un `.md` già segmentato, per
  restare a una sola regola invece che un'eccezione per formato. Senza
  provider la rotta di indicizzazione risponde 404 e la sezione mostra
  solo il motivo: senza chat non c'è niente da preparare, non un errore da
  correggere.
- **Un'indicizzazione può riuscire ma restare incompleta.** Su un PDF
  scansionato una pagina che il modello non riesce a leggere viene
  saltata, non blocca le altre: la risposta resta un successo (200) ma
  porta anche `pagesIndexed`/`pagesSkipped` quando qualche pagina è stata
  persa. Un successo pieno e uno parziale sarebbero altrimenti
  indistinguibili per l'admin — vedrebbe solo "17 sezioni indicizzate"
  senza sapere che al manuale mancano tre pagine, e la chat risponderebbe
  poi con sicurezza da un regolamento incompleto: il guasto peggiore per
  questa funzione, perché una risposta sbagliata sembra identica a una
  giusta. La riga lo dice con un avviso dedicato (`.manual-doc-partial`,
  la stessa resa oro di `.loan-warning`: attenzione, non errore — il rosso
  di `.error` direbbe "non ha funzionato", e ha funzionato) invece che con
  la nota neutra `.empty-note` del successo pieno. È un avviso
  **transitorio**, non uno stato persistito: quel dato arriva solo nella
  risposta di questa chiamata, non nel dettaglio del gioco, quindi sparisce
  al prossimo caricamento della pagina — coerente con la riga, che mostra
  comunque sempre il conteggio corrente in `.manual-doc-state`.
- **Perché non c'è più una bozza da correggere a mano.** Fino alla fase
  precedente il pannello mostrava una casella di testo per pagina — la
  trascrizione di uno scan, da rileggere contro il PDF e correggere prima
  di salvare. È stata **rimossa per scelta di prodotto**, non
  semplificata via: l'app ha smesso di conservare il testo estratto (solo
  i chunk cercabili restano), quindi non esiste più niente da mostrare in
  una casella. La revisione umana pagina per pagina è sostituita da un
  controllo lato server che rifiuta la risposta del modello quando somiglia
  a una riscrittura invece che a una segmentazione col solo aggiunta di
  titoli.

#### Domande suggerite (`SuggestedQuestionsPanel.vue`)
La seconda card del gruppo «Chatbot», sorella di "Knowledge base" e non un
blocco dentro: le tre domande si devono poter scrivere a mano anche su un
gioco senza documenti indicizzati, ed è il motivo per cui il PUT è un upsert.
Il `.panel-card` sta nella vista, quindi il form non porta bordo né ombra
suoi — raddoppiarli sarebbe la `.panel-card` dentro `.panel-card` che il
sistema rifiuta altrove. Il pattern del pannello: una lista ordinata di campi
editabili, ciascuno con una pastiglia di stato, dentro un form annidato in un
foglio.

- **È un `<form>` e la lista è una `<ol>`.** Tre campi etichettati più un
  "Salva" sono un form, non un `<div>` con dei `@click`: dichiararlo
  restituisce l'invio col tasto Invio, che mancava. La `<ol>` resta perché
  la posizione è un dato — le tre domande compaiono nella chat in
  quell'ordine — e porta `role="list"` come ogni lista spogliata del
  progetto.
- **La pastiglia di stato è maiuscoletto tracciato, non sentence-case**
  (`.suggested-questions-badge`, la resa di `.lang-chip`). Il maiuscoletto
  tracciato in mono è la voce delle etichette qui, e in fondo a una riga di
  form è la sola cosa che distingue una pastiglia di stato da un bottone,
  che nel progetto è sempre sentence-case in font di corpo con un
  riempimento o un bordo `--card-line`. La pastiglia è anche legata al suo
  campo da `aria-describedby`: senza, per chi legge con la voce resta un
  testo orfano accanto a un campo, non una proprietà del campo.
- **Un controllo spento dice sempre perché.** "Rigenera" è spento senza
  provider AI, e il motivo sta nel pannello — non nella sezione sopra, che
  lo dice solo quando il gioco ha documenti indicizzabili e quindi taceva
  proprio nella combinazione peggiore (nessun provider *e* nessun
  documento: bottone spento, nessuna spiegazione). Vale anche per "Salva",
  spento finché una delle tre caselle è vuota perché il server la rifiuta:
  la regola si dice prima del click, non con un 400 dopo. La distinzione
  con `ManualPrepPanel`, che invece **nasconde** "Prepara" senza provider,
  è deliberata: si nasconde un'azione quando l'intera riga non ha più
  senso, si spegne con la spiegazione quando il pannello ha ancora un
  lavoro da fare — qui le tre domande si scrivono a mano comunque.
- **Distruttivo e primario non stanno affiancati.** "Rigenera" riscrive
  tutte e tre le domande, comprese quelle a mano, e per scelta di prodotto
  **non ha una conferma**. Sta quindi nella testata del pannello, come
  l'azione di una `.section-head`, e non accanto a "Salva": due bottoni in
  fila si leggono come "scegline uno per finire", e quello che distrugge il
  lavoro appena fatto era a mezzo centimetro da quello che lo salva. Quel
  che altrove sarebbe un modale, qui è distanza più copia adiacente al
  bottone.

### Amministratori (`/admin/users`)
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

### Il luogo di una serata (`VenueSearchSelect`, `EventMap`)
- **Si cerca, non si compila**: il campo Luogo è un combobox gemello di
  `BggSearchSelect` (`.venue-combobox`, `.venue-results`, `.venue-result`
  con `.is-active`) che interroga OpenStreetMap mentre si scrive — tre
  caratteri, pausa di 400ms, richiesta precedente annullata. Una riga è
  l'insegna del posto (o la via) sopra e il resto dell'indirizzo sotto,
  in mono attenuato come ogni dato secondario.
- **La via d'uscita è parte del campo, non un ripiego**: sotto la nota di
  stato c'è "Usa «…» così com'è", perché un circolo che su OpenStreetMap
  non esiste va scritto a mano. La riga di conferma (`.venue-chosen`, su
  `--card-alt` come `.bgg-chosen`) dice sempre quale dei due casi è:
  *Posizione trovata sulla mappa* oppure *Indirizzo scritto a mano:
  nessuna mappa sull'evento*. Nessuna mappa non è un errore.
- **Il nome del luogo è un campo a sé**, sotto l'indirizzo scelto: quello
  che una ricerca restituisce è un indirizzo, l'insegna la sa
  l'organizzatore. Si eredita da OpenStreetMap solo quando è davvero
  un'insegna ("Circolo Arci"), mai quando ripeterebbe la via.
- **La mappina conferma, non naviga** (`.event-map`): 220px di altezza
  sotto la riga del luogo, puntino nel rosso d'accento su bordo cartoncino
  (`.event-map-pin-dot`, non l'icona di serie di Leaflet), attribuzione
  OpenStreetMap come da licenza. Rotella e trascinamento a un dito sono
  spenti: la pagina si legge in piedi al tavolo, e una mappa che cattura
  lo scorrimento è una pagina che si blocca. Chi deve muoversi davvero
  esce da "Apri in mappe".
- **Sulla card di un evento sta l'insegna, non l'indirizzo**
  (`.event-card-venue`, pin come insegna, gemello di `.event-card-date`):
  in un elenco serve riconoscere il posto, non arrivarci.

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

### Scheda evento pubblica (`/events/:id`): il tavolo
Sotto locandina, data, luogo e mappa arriva `Al tavolo` (`h2`), poi il
feltro. La griglia (`.event-games`) è a colonne elastiche
`repeat(auto-fill, minmax(150px, 1fr))`: quattro scatole per riga su
desktop, due sul telefono (`minmax(130px, 1fr)` sotto i 560px). Il minimo
è basso e la colonna è `1fr` di proposito — il feltro si riempie sempre,
invece di lasciare una fascia verde vuota a destra quando le copie non
bastano a fare una riga intera.

La card porta, dall'alto: copertina 3/4, nome (`h3`, con `#2` solo se il
gioco ha più copie), difficoltà, stato dei posti, e in fondo le due
azioni ancorate da `margin-top: auto` — **Prenota** a piena larghezza e,
sotto, **Dettagli →** (`.detail-link`, testo rosso seme) verso la scheda
del gioco con manuali e video. Una card, una sola azione primaria: il
nome non è più un link, perché due bersagli allo stesso peso su una
tessera da 150px si sbagliano col pollice.

### Difficoltà (`GameDifficulty.vue`)
Il peso BGG diventa cinque pip da dado (`.difficulty-pips`, 6px, bordo
oro, riempiti fino a `round(weight)`) più la parola: `<2` Facile, `<3`
Medio, `<4` Impegnativo, `≥4` Esperto. Il decimale esatto vive nel
`title` — chi conosce la scala BGG lo cerca, chi non la conosce non
saprebbe cosa farsene di "3,2/5" mentre sceglie un tavolo. Il testo usa
`--gold-text`, mai `--gold` (vedi Don't sul contrasto). Con `weight`
nullo la riga sparisce: nessun segnaposto per un dato che non c'è.

### La modale scorre nel corpo, non nel foglio (`ModalDialog`)
Il tetto d'altezza sta su `.modal` (`calc(100dvh - 3rem)`), e a scorrere è
`.modal-body`: testa e X restano ferme. Il massimo di serie di `<dialog>`
fa scorrere il dialog intero, e su una finestra bassa il titolo usciva
dalla vista lasciando i bottoni sotto la piega, senza niente che dicesse
che c'era altro (bug reale trovato aprendo l'indice BGG in una finestra da
600px). Il corpo porta un rientro negativo di `0.35rem` compensato dal
padding: un contenitore con `overflow` ritaglia l'anello di focus dei campi
che gli stanno a filo, ed è la stessa regola già applicata alle card con
`overflow: hidden`.

### Prenotazione in modale
Il form di prenotazione vive in `ModalDialog` (`<dialog>` nativo), aperto
dal "Prenota" della card e intitolato `Prenota: <nome copia>`. Non è
scelta di spazio ma di sequenza: in pagina il form restava sotto la
griglia e chi prenotava perdeva di vista quale tavolo aveva scelto.

Il codice di prenotazione compare **due volte**, dallo stesso componente
(`BookingConfirmation.vue`): dentro la modale appena confermata — con un
"Ho segnato il codice" che è l'uscita esplicita, non solo la X — e poi in
cima al tavolo (`.booking-recap`, bordo oro) finché si resta in pagina.
Senza SMTP configurato quel codice si vede solo qui, ed è per questo che
una modale che si chiude d'istinto non basta da sola: da lì il riepilogo
in cima al tavolo. Con SMTP configurato lo stesso codice arriva anche per
email — vedi "Email transazionali" più sotto — ma la UI a schermo resta
la stessa in entrambi i casi: non sa se la mail è partita o no.

Il riepilogo **accumula**: al tavolo un telefono solo prenota per due o tre
persone, quindi ogni conferma si aggiunge invece di sostituire la
precedente, e il titolo passa da "La tua prenotazione" a "Le tue
prenotazioni". La riga "conservalo per..." si dice **una volta sola** sotto
tutti i codici (prop `hint` a `false` nel riepilogo): ripetuta identica
sotto ognuno diventava rumore.

### Gestione prenotazione (`/manage-booking`, `/prenotazione/:code`)
Pagina da telefono, in piedi al tavolo: due sole scatole, e in ognuna
l'azione sta in basso a destra, dove il pollice la trova.
- **Codice e ricerca sulla stessa riga** (`.booking-lookup`): un campo solo
  e corto, largo quanto la card, con "Cerca" (lente) a filo del fondo del
  campo. Sotto i 560px la riga si impila e il bottone prende la larghezza.
- **La disdetta sta dentro la scheda** (`.booking-summary-foot`): stato a
  sinistra, "Annulla prenotazione" (× cerchiata, `.btn-danger`) a destra
  sulla stessa riga. Un'azione che annulla *quella* prenotazione non può
  stare spaiata sotto la scatola che la descrive.
- **La riga dei punti è larga quanto il foglio** (`.player-score-row`):
  nome elastico, punteggio 8rem in mono allineato a destra, "Rimuovi"
  (cestino) in fondo. Le righe non si fermano alla misura dei campi
  (30rem) — qui la lista *è* il contenuto del form.
- **"Aggiungi giocatore" continua la riga**: largo dal bordo del nome al
  bordo di "Rimuovi", come lo slot vuoto in fondo a una lista.
- **L'invio chiude in basso a destra** (`.form-actions`, senza il tetto di
  30rem): dischetto e "Invia punteggio"/"Aggiorna punteggio".
- Su telefono la riga va a capo — nome sopra, punteggio e rimozione sotto —
  e le due linee si stringono a 0.4rem mentre fra un giocatore e l'altro
  resta il passo del form: senza quello stacco sei linee sembrano sei
  righe scollegate invece di tre giocatori.

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
  quanto è grande lo scaffale. Un gioco con copie occupate da prenotazioni
  attive mostra "1 copia occupata" / "N copie occupate" (si contano le
  copie, non le prenotazioni: un tavolo con più prenotati è comunque una
  copia sola), non si può togliere e il campo copie non scende sotto
  quel numero: il backend rifiuterebbe, e un campo che non scende è più
  onesto di un 409 dopo il salvataggio.
- **La spunta "prenotabile" sta accanto al campo copie**
  (`.game-select-bookable`): sono le due cose che si decidono insieme
  mentre si scelgono i giochi. Le prenotazioni attive la bloccano come
  bloccano il resto della riga, e **il blocco si spiega una volta sola**:
  `.game-select-locked` — "Ha prenotazioni attive: annullale nella
  sezione Prenotazioni per togliere il gioco, ridurre le copie o
  renderlo non prenotabile" — nomina la causa, dove si risolve e i tre
  effetti, uno per controllo, e i tre controlli la puntano con
  `aria-describedby`. Una spunta grigia senza spiegazione legata al
  controllo lascia chi usa uno screen reader col solo "disabilitato", e
  chi legge a schermo con una frase scritta per un campo diverso. La
  frase sta su una riga sua (`flex-basis: 100%`, che vuole
  `flex-wrap: wrap` sul contenitore), e la spunta bloccata si smorza a
  `opacity: 0.7` — non oltre: "prenotabile" resta da leggere.
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

### Banco prestiti (`/admin/events/:id/prestiti`)
- **Riga-bersaglio** (`.loan-row`, dentro `.loan-list`): ogni riga è un
  `<button>` intero, alta almeno 3.5rem, testo a sinistra e verbo
  dell'azione a destra (`.loan-row-action`: "Consegna" o "Restituito").
  Il banco si usa in piedi, spesso con una scatola in una mano: un
  bersaglio piccolo, o diviso in più bottoni, costringerebbe a mirare
  due volte. Fuori e Disponibili, le due sezioni della pagina dove c'è
  un gesto da compiere (restituire, consegnare), condividono la stessa
  riga-bottone: cambiano i dati e il verbo, non la forma. Restituiti,
  il registro della serata, non ha un gesto da offrire — riusa invece
  `.admin-list`/`.admin-row`, la stessa riga non interattiva di
  Prenotazioni e Risultati (vedi sopra): una riga bottone su un
  prestito già chiuso avrebbe promesso un'azione che non c'è.
  `.loan-list` e `.loan-booking-picks` si **spogliano** come
  `.admin-list` (fondo, bordo, raggio, ombra, `overflow`, e il padding e
  il filetto che la regola globale mette su ogni `li`): la card è già il
  `.panel-card`, e la riga è il bottone. E `.loan-row` dichiara il fondo
  anche nell'hover: `li button:hover` lo tingeva di `--danger-bg`, il
  colore di "Rimuovi", su un bottone che consegna — la stessa eredità
  che `.loan-booking-picks` doveva già rifiutare.
- **Pastiglia di stato** (`.loan-tag`, gemella di `.seat-state`): un
  gioco senza prenotazione porta la stessa pastiglia quieta, con la
  stessa scritta — "Senza prenotazione" — sia qui sia sulla scheda
  pubblica dell'evento, a dire "questo si gioca ma non si prenota". È la
  stessa informazione letta da due persone diverse — l'organizzatore al
  banco, chi guarda il tavolo — e due forme diverse per lo stesso dato
  l'avrebbero fatto sembrare due fatti distinti. La nota sotto il tavolo
  pubblico ne cita la scritta parola per parola.
- **L'avviso della consegna fuori prenotazione è oro, non rosso**
  (`.loan-warning`): fondo `--gold-bg` e bordo `--gold`, la famiglia dei
  distintivi, con il testo in `--ink` pieno. Consegnare una copia
  prenotata a un altro nome **si fa comunque** — alle 21:30 chi non si è
  presentato non tiene in ostaggio la scatola — quindi `--danger`
  direbbe "non puoi", e `--ink-muted` su fondo neutro lo faceva leggere
  come una postilla: al banco, distratti, è l'unica riga che può fermare
  uno sbaglio. L'oro resta bordo e superficie, mai colore del testo.
- **Avviso ed errore delle due modali si annunciano da regioni live
  montate a permanenza**, come l'esito della prova SMTP (vedi sotto):
  due `p.visually-hidden` dentro il form (`role="status"
  aria-live="polite"` per l'avviso, `role="alert" aria-live="assertive"`
  per l'errore) nascono vuote all'apertura della modale e si riempiono
  dopo, perché una regione live nata già col testo dentro è il caso che
  gli screen reader in pratica non annunciano. L'avviso è `polite`:
  arriva mentre si scrive il nome, non deve interrompere la digitazione
  né rubare il focus. I riquadri visibili restano `v-if` e senza ruoli
  ARIA: sono sola presentazione.
- **Nel registro la nota è testo, il resto è dato**: telefono e orari in
  mono (`.row-meta`), la nota libera con la stessa resa che ha nella
  riga di "Fuori" (`.loan-row-notes`, corsivo nel font di corpo). La
  stessa nota in due font avrebbe fatto sembrare due cose diverse la
  stessa frase.
- **Riga della checklist materiali** (`.material-check-list li`, nella
  modale di restituzione): nome elastico (`.material-check-name`, `flex:
  1`), quantità attesa in mono (`.material-check-expected`, è un dato),
  campo numerico stretto quanto basta a quattro cifre
  (`.material-check-input`) e casella di spunta. La casella nativa in
  Chrome **ignora il `padding`**: gonfiarla per portare l'area di tocco a
  44px lasciava il bersaglio reale a ~20px. La soluzione è avvolgere
  l'`<input>` in un `<label>` (`.material-check-box-wrap`) di 44×44px che
  centra al suo interno una casella piccola quanto il resto della riga
  (`.material-check-box`, 1.35rem): l'area di tocco cresce sul
  contenitore, il disegno del controllo resta fedele alla riga.

### Aggiungi gioco (`/admin/games/new`)
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
- **Sidebar** (`.app-sidebar`): feltro profondo, una voce per riga con
  icona a filo 1.15rem + etichetta, testo cartoncino attenuato di default,
  tessera di cartoncino pieno + testo feltro per la voce corrente. La voce
  resta accesa su tutte le pagine che le discendono (la scheda di un evento
  sta sotto "Eventi"), non solo sul suo indirizzo esatto. Le voci pubbliche
  vengono prima; il gruppo **Gestione** appare solo a sessione aperta,
  separato da un filetto e da un'etichetta mono maiuscoletta. Struttura e
  breakpoint: vedi Layout.
- **Menù utente** (`.user-menu`): pallina 1.95rem in rosso seme con le
  iniziali dell'email, caret, dropdown su cartoncino ancorato a destra con
  l'email in mono e la voce "Esci". È l'unico posto da cui si esce: niente
  bottone "Esci" sciolto nella chrome, e nessun link espone `/login` — chi
  amministra conosce l'indirizzo.
- Nessuna barra di navigazione orizzontale: la vecchia `nav` a pillole
  (fino a cinque voci in riga) è stata sostituita dalla sidebar.
- Ogni bersaglio della chrome (hamburger, voce di sidebar, trigger utente,
  voce del dropdown) sta a **44px** di lato minimo: la sidebar la aprono
  anche i partecipanti col pollice, in piedi al tavolo.
- **Linguette** (`.tab-bar`): pastiglie mono maiuscoletto su una barra in
  feltro, angoli tondi su tutti i lati (mai linguette tagliate in basso).
  La barra è una sola grammatica per casi diversi — le lingue di un gioco
  (`.language-tabs`, che ci aggiunge solo l'azione "Aggiungi lingua" in
  fondo) e il periodo degli eventi. Dentro un `.section-group` la barra è la
  testata del gruppo e rinuncia al fondo, che glielo dà già il tappeto.

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

### Email transazionali
Una superficie nuova, non un'estensione della UI: quel che il sistema
mail visualizza non è il browser dell'associazione ma un client di posta
qualunque, spesso il peggiore possibile in fatto di CSS. Le tre mail
(invito, conferma di prenotazione, annullamento) e quella di prova
condividono una cornice (`mailShell`) fatta di tabelle e stili inline —
niente CSS esterno, niente immagine, niente font caricato da fuori —
larga al massimo 560px, con intestazione in feltro (col nome dell'app),
corpo su cartoncino, bottoni nell'accento rosso e il codice di
prenotazione nello stesso mono con cui compare a schermo
(`.booking-code-card`). È la stessa palette del resto dell'app, non una
sua reinterpretazione.

**La versione in solo testo è di pari rango, non un ripiego.** Ogni mail
esce come `multipart/alternative` con la parte testo scritta per intero
prima di quella HTML: chi legge in solo testo trova comunque il codice e
tutti i link, non un rimando a "vedi la versione HTML".

### Impostazioni SMTP (`.smtp-test`)
Il bottone "Invia email di prova" è un'azione secondaria dentro un form
lungo, non l'ultimo campo prima del submit: sta nel suo blocco
(`.smtp-test`), separato dal `.form-actions` di salvataggio in fondo
alla pagina, con la spiegazione di cosa farà accanto al bottone invece
che sopra o sotto — la stessa riga risponde "cosa succede se premo
questo" mentre lo si guarda. Il bottone resta `.btn-secondary`
disabilitato finché la configurazione non è salvata: prova quella sul
server, non quella ancora nel form, e la spiegazione a fianco lo dice
esplicitamente. `.field-row`, che qui allinea porta e sicurezza sulla
stessa riga, esisteva già (vedi min/max giocatori) e non è un
componente nuovo.

L'esito della prova sdoppia annuncio e riquadro invece di farli coincidere
nello stesso elemento. Una regione live ha bisogno di stare nel DOM
**prima** che il testo cambi — nata già col contenuto dentro (`v-if`) è
il caso che gli screen reader in pratica non annunciano, stesso principio
di `BggSearchSelect`/`VenueSearchSelect` — ma `.panel-card` è un flex
column con `gap`, che non collassa per un elemento a vuoto come farebbe
un margine: un riquadro sempre montato lascerebbe uno spazio morto anche
senza risultato. Perciò due `p.visually-hidden` (`role="status"
aria-live="polite"` per il successo, `role="alert" aria-live="assertive"`
per l'errore) restano montate a permanenza fuori dal flusso visivo — non
essendo elementi di flex, il `gap` non le tocca — e portano solo il testo
per l'annuncio. Il riquadro visibile (`.success`/`.error`, senza ruoli
ARIA: è solo presentazione) torna un semplice `v-if`, esattamente come
prima di questa voce.

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
- **Do** cambiare il colore dell'anello di focus sul feltro
  (`.app-topbar`/`.app-sidebar`/`.manual-chat-head`
  → `outline-color: var(--felt-text)`): il rosso seme della regola globale
  sta a 1.3:1 contro il verde, l'anello esiste ma non si vede. Su cartoncino
  resta il rosso. **Ogni superficie verde nuova va aggiunta a quella lista**:
  non è una regola che si applica da sé, ed è già stata dimenticata due
  volte (la × del dialog della chat disegnava un rettangolo rosso sul feltro,
  trovato in audit).
- **Do** dare a un bottone che non è il rosso primario **anche il suo
  `:hover`**: la regola globale `button:hover` (specificità 0,1,1) batte una
  classe nuda (0,1,0), e dentro una `<li>` batte pure `li button:hover`
  (0,1,2). Un bottone verde feltro senza hover proprio diventa rosso accento
  al passaggio del mouse, una pastiglia in lista diventa `--danger-bg` col
  bordo di "Rimuovi". Bug reale trovato in audit su `.manual-chat-fab`,
  `.manual-chat-fakeinput` e sulle domande suggerite —
  la stessa eredità che `.loan-row` aveva già dovuto rifiutare.
- **Do** tenere ogni bersaglio delle pagine pubbliche a **44px** di lato
  minimo, la × delle modali compresa (`.modal-close`): su un telefono quella
  × è l'unica uscita, l'Esc non c'è, e a 28px si manca. L'icona resta a
  1rem — cresce l'area, non il disegno — e un `margin-right` negativo
  rimette la × otticamente a filo della testata.
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
  `.event-grid li` ed `.event-games li`). Se la card resta flex ma in
  colonna, va dichiarato anche `align-items: stretch`: l'`align-items:
  center` della lista base fa collassare nome e bottoni alla larghezza del
  loro contenuto (bug reale su `.event-games li`).
- **Don't** aggiungere un bordo solo allo stato eccezionale di una card in
  griglia: i 2px in più restringono il contenuto e sfalsano le copertine
  della riga. Il bordo c'è sempre, `transparent` a riposo, e lo stato lo
  colora (`.event-games li.is-full`).
- **Don't** mettere una `<ul>` dentro un `.panel-card` senza spogliarla: la
  regola globale `ul` la veste da card (fondo, bordo, raggio, ombra,
  `overflow: hidden`) e `li` le dà padding e filetto propri, quindi ogni
  riga finisce incorniciata due volte, rientrata, e con l'anello di focus
  ritagliato. Si copia il blocco di `.admin-list` (bug reale su
  `.loan-list` e `.loan-booking-picks`).
- **Don't** usare un `<label>` come riga senza dichiarare
  `flex-direction: row`: la regola globale `label` impila in colonna, e una
  spunta con la sua etichetta finisce con la casella **sopra** la parola
  invece che accanto (bug reale su `.game-select-bookable`; già risolto
  così in `.game-select-copies` e `.checkbox-label`).
- **Don't** dichiarare un `display` su `.modal` (o su qualunque `<dialog>`):
  scavalca il `dialog:not([open]) { display: none }` del browser — una
  regola d'autore vince su quella di serie a qualunque specificità — e ogni
  modale chiusa si disegna in fondo alla pagina (bug reale, introdotto e
  corretto nella stessa sessione). Il `display` che serve al foglio va su
  `.modal[open]`.
- **Don't** mettere due `margin-left: auto` nella stessa
  `.section-head`: conteggio e bottone si spartiscono lo spazio libero e il
  numero galleggia in mezzo alla riga, fuori dalla colonna dove sta nelle
  sezioni accanto. Spinge il gruppo il primo dei due
  (`.section-head .section-count + button { margin-left: 0 }`).
- **Don't** stilare un `<form>` dentro un `.panel-card` con la sola classe:
  `.app-page form` porta `align-items: flex-start` (serve a non stirare i
  submit) e `.panel-card form` non lo tocca, quindi una classe sola non lo
  batte — classe+tipo vince. Senza `align-items: stretch` a specificità
  sufficiente (`form.la-tua-classe`) ogni figlio si stringe sul proprio
  contenuto: bug reale misurato su `.suggested-questions`, la lista dei
  campi larga 305px dentro un foglio da 753px, contro la regola del campo
  largo quanto il foglio.
- **Don't** dare per scontato che `list-style: none` e `padding: 0` basti a
  spogliare una lista: il padding e il `border-bottom` stanno sul `li`, non
  sulla `<ul>`/`<ol>`. Una lista di righe di form che li eredita arriva
  rientrata di 15px e sottolineata da un filo — si legge come una lista di
  dati e non come i campi di un form (bug reale su
  `.suggested-questions-list`, la stessa trappola già annotata per `<ul>`
  qui sopra). Si copia il blocco di `.admin-list`, `li` compreso.
- **Don't** lasciare un controllo `disabled` senza il motivo a schermo:
  `button:disabled` è solo `opacity: 0.55`, quindi un bottone spento non
  comunica nient'altro che di essere spento, ed è un vicolo cieco. Si
  nasconde l'azione quando l'intero contesto non ha senso
  (`ManualPrepPanel` senza provider AI) oppure si spegne **e** si scrive
  accanto perché, legandolo al bottone con `aria-describedby`.
