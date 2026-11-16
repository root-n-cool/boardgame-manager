# Pelle Root'n'Cool per il manager — design

## Perché

Il blog di Root'n'Cool (`~/project/rootncool/rnc-blog`) ha un'identità
visiva precisa; il manager, dove si arriva dal blog per prenotare un
tavolo, ne ha un'altra (feltro verde e cartoncino avorio, Space Grotesk +
IBM Plex). Chi passa dall'uno all'altro deve sentirsi nello stesso posto.

Il manager è "teoricamente open source" ma in pratica lo usa solo
Root'n'Cool, in self-hosting con Docker: non serve tenerlo neutro né
renderlo tematizzabile. Si sceglie l'identità del blog, fissa nel codice.

## Obiettivi

- Tutto il manager (pagine pubbliche **e** admin) con la pelle del blog:
  "tavolo di notte", nero del logo, testo gesso, le quattro pedine del
  logo come colori pieni, titoli Fraunces, testo Atkinson Hyperlegible,
  bottoni a pillola.
- Nessuna schermata che resti col vecchio stile: si ripassano tutte le
  viste, non solo i token.
- Leggibilità e contrasto almeno pari a oggi (WCAG AA sul testo).

## Non obiettivi

- Nessun tema configurabile dalle impostazioni, nessun selettore
  chiaro/scuro.
- Nessun cambio di funzionalità, flussi, layout delle pagine o testi
  (salvo dove un testo cita il "feltro").
- Il blog non si tocca.

## Anti-obiettivi

- Un admin scuro che diventa faticoso: tabelle, form e liste dense
  devono restare leggibili quanto oggi (superfici distinte, linee
  visibili, stati di focus/errore chiari).
- Colori del vecchio tema che restano "nascosti" (letterali sparsi,
  shadow DOM della chat, email).

## Approcci considerati

1. **Pelle Root'n'Cool fissa nel codice** — *scelto.* Si sostituiscono
   token e font in `app.css` con quelli del blog e si ripassano le
   viste. Più semplice, è ciò che serve.
2. Tema scelto dalle impostazioni ("Feltro"/"Notte"): utile solo a un
   open source vero, scartato.
3. Pelle del blog solo sulle pagine pubbliche: metà lavoro ma due stili
   nella stessa app, scartato.

## Cosa resta com'è

- Titolo del sito caricabile dalle impostazioni. *(Aggiornato su
  richiesta di Furt: logo e favicon sono fissi, presi dal blog; gli upload
  dalle impostazioni non li sostituiscono più. Testata più alta, come nel
  blog.)*
- Struttura di `app.css` (un solo foglio, classi elemento + classi di
  layout) e componenti Vue.

## Design

### Colori (token in `frontend/src/app.css`)

Valori presi da `rnc-blog/src/styles/global.css`:

| Ruolo | Valore | Note |
|---|---|---|
| Fondo (`--bg`, ex feltro) | `#000000` | nero del logo |
| Superficie (card, campi) | `#151515` | come `--superficie` del blog |
| Superficie alternativa | ~`#1f1f1f` | righe alterne, hover, card annidate |
| Linee | `#2a2a2a` | come `--linea` del blog; per i bordi dei campi più chiaro se serve al contrasto (~`#4a4a4a`) |
| Testo | `#f3efe6` | gesso |
| Testo secondario | gesso a ~70% | verificare ≥4.5:1 sul `#151515` |
| Accento / azione primaria | arancio `#db8142` | testo **nero** sopra (7:1); è anche il colore del focus, come nel blog |
| Pedine | blu `#035cb6`, verde `#5c8d2d`, grigio `#aeb2b3` | blu solo come riempimento con testo bianco (sul nero è 3.2:1, mai come colore del testo) |
| Successo | verde `#5c8d2d` | |
| Pericolo | rosso `#d1453b` | |
| Avviso (ex oro) | giallo `#e3b23c` | |

Gli sfondi tenui `*-bg` (danger, success, gold, accent) diventano
versioni scure dello stesso colore (es. colore al 15–20% sul
`#151515`), con testo nel colore pieno o gesso.

`color-scheme: dark`. Si toglie la trama a puntini del fondo. Le ombre
(`--shadow-card`, `--shadow-lift`) sul nero non si vedono: la
separazione la fanno superficie più chiara e linee.

### Tipografia

- Titoli: **Fraunces Variable**, asse `SOFT 100`, peso 900,
  `line-height: 1`, come nel blog. *(Scelta di chi ha scritto la spec.)*
  Va su `h1`/`h2`/`h3` e sui titoli di card e nomi dei giochi, come gli
  `h3` del blog. Le altre regole oggi in `Display`, cioè navigazione,
  etichette piccole e voci dense dell'admin, passano ad
  **Atkinson 700**: Fraunces 900 in piccolo e in lista pesa troppo sul
  nero.
- Testo (ex `Body`): **Atkinson Hyperlegible** 400/700 (+ corsivi).
- Mono (ex `Data`): **IBM Plex Mono** resta **solo per codici e numeri**
  (booking code, punteggi, quantità, date tabellari): serve a leggerli
  senza ambiguità. *(Scelta di chi ha scritto la spec.)* Le etichette
  in maiuscoletto, gli occhielli e i metadati che oggi sono in mono
  "come ogni etichetta del progetto" passano ad Atkinson 700 piccolo:
  il blog non ha nessun mono e quelle etichette sono ovunque.
  `DESIGN.md` va riscritto in questo senso.
- I file `.woff2` si copiano in `frontend/src/assets/fonts/` da
  `rnc-blog/node_modules` (`@fontsource-variable/fraunces` file *soft*
  o *full* latin, `@fontsource/atkinson-hyperlegible` latin 400/700
  normal+italic): nessuna dipendenza npm nuova. Si tolgono Space
  Grotesk e IBM Plex Sans.

### Forme e componenti

- Bottoni a pillola (`border-radius: 99px`), come `.bottone` del blog;
  primario arancio con testo nero, secondario contorno gesso.
- Badge e stati come pillole piene nei colori delle pedine (come il
  `.voto` del blog).
- Teste di pagina: barra colorata a sinistra come `.intestazione` del
  blog dove oggi c'è una testata in feltro.
- Le pedine (quattro pallini blu/arancio/verde/grigio) si possono usare
  come segno decorativo nell'header, come nel blog.

### Superfici con regole proprie

- **Stampa (cartellini e fogli QR, `QrPrintView`, `QrSheetView`)**:
  restano su carta chiara (fondo bianco, testo nero), con i nuovi font.
  Stampare il nero consuma inchiostro e i QR vanno letti.
- **Chat del manuale (`ManualChatPanel.vue`)**: i colori ricopiati a
  mano in `messageStyles` (shadow DOM di deep-chat) vanno aggiornati ai
  nuovi valori, come dice il commento in `app.css`.
- **Email (`mailShell` nel backend)**: intestazione nera col nome
  dell'app, corpo chiaro (i client di posta e le modalità scure fanno
  danni sul fondo scuro), bottoni arancio con testo nero, font di
  sistema. Stessa parte testo di oggi.
- **Mappa dell'evento (`EventMap`)**: le tile restano come sono; si
  adattano solo cornice e controlli.

### Documentazione

- `DESIGN.md`: si riscrivono Overview, Colors, Typography, Elevation,
  Shapes, Buttons, Cards, Inputs e la parte Email; nelle sezioni
  componente si sostituiscono i riferimenti a feltro/avorio/oro/rosso.
  Il concetto passa da "dado & pedina su tavolo in feltro" a "tavolo di
  notte" (con lo stesso tono del blog).
- Il commento "contratto di direzione" in `frontend/index.html` e
  l'intestazione di `app.css` vanno allineati.
- `CLAUDE.md`: la riga sul tono "dado & pedina" si aggiorna.

## Verifica

- `npm run build` (type-check incluso) e suite backend in Docker se si
  tocca `mailShell` (i test sulle mail possono controllare l'HTML).
- Ripasso visivo con Claude in Chrome di **tutte** le viste, a 390px e
  desktop, console pulita; stampa dei QR in anteprima.
- Pass `/impeccable audit` finale (contrasto in particolare).

## Lasciato a chi costruisce

- Valori esatti delle superfici intermedie e dei `*-bg` scuri, purché
  passino il contrasto.
- Come mappare i ~60 colori letterali rimasti in `app.css`.
- Dove usare (o no) le pedine decorative oltre all'header.

## Aggiunte approvate da Furt (stesso giorno)

Proposte dopo il primo giro, approvate tutte ("fai tutto"): riga delle
sezioni a pedine sotto il logo sul telefono, testata che si nasconde
scorrendo, barra colorata accanto al titolo di pagina, piè di pagina con
blog e Instagram, tavolo a righe sul telefono, modale come foglio dal
basso sul telefono. Dettagli in `DESIGN.md`.
