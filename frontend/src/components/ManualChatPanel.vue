<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'

/**
 * Il contenuto della chat, indipendente da dove vive: la sidebar su
 * desktop e il dialog a tutto schermo su mobile montano questo.
 *
 * Nello stato di riposo il markup è nostro: titolo, tre domande suggerite e
 * un finto campo di input. deep-chat si monta al primo gesto — clic sul
 * campo o su una domanda — perché pesa 105 KB gzip, più dell'intera app, e
 * chi apre una scheda gioco per leggerla non deve pagarli. È questo che
 * permette alla sidebar di stare aperta per default. L'unica eccezione è
 * chi ha già una conversazione salvata su questo gioco (vedi
 * `hasSavedConversation`): lì il filo si riapre al caricamento, perché
 * quei byte li ha già scaricati e ritrovare la conversazione dov'era vale
 * più del risparmio.
 *
 * La testata (titolo, "nuova conversazione", chiusura) sta qui e non nei
 * due contenitori: prima esisteva solo nel dialog mobile e la sidebar
 * desktop non diceva nemmeno di essere una chat.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  /** I titoli di sezione del manuale, per le domande suggerite. */
  headings: string[]
  /** Nel dialog mobile la testata porta anche la ×; nella sidebar no. */
  closable?: boolean
}>()

const emit = defineEmits<{ close: [] }>()

const started = ref(false)
const failed = ref(false)
const chat = ref<HTMLElement | null>(null)
const fakeInput = ref<HTMLButtonElement | null>(null)

// Una chiave per gioco: le conversazioni di due giochi diversi non si
// mescolano, e chi torna sulla scheda ritrova la sua. deep-chat la
// gestisce da sé (`browserStorage` qui sotto) — scrive a ogni messaggio e
// rilegge al render — quindi non c'è codice nostro che serializza niente.
const storageKey = computed(() => `bgm-chat-${props.gameId}`)

/**
 * Dice se su questo gioco c'è una conversazione da riaprire. Legge la
 * stessa chiave che scrive deep-chat: è l'unico punto dove la tocchiamo
 * noi, e serve solo a decidere se montare il componente subito.
 *
 * try/catch perché in navigazione privata (Safari) il solo accesso a
 * localStorage lancia: in quel caso nessuno storico, stato di riposo, e la
 * chat funziona come sempre.
 */
function hasSavedConversation(): boolean {
  try {
    const raw = localStorage.getItem(storageKey.value)
    if (!raw) {
      return false
    }
    const saved = JSON.parse(raw) as { messages?: unknown }
    return Array.isArray(saved.messages) && saved.messages.length > 0
  } catch {
    return false
  }
}

onMounted(() => {
  if (hasSavedConversation()) {
    // Senza focus: la sidebar è aperta per default e rubare il cursore a
    // chi ha appena aperto la scheda gioco per leggerla sarebbe un agguato.
    start('', false)
  }
})

/**
 * Il ＋ della testata. Non chiama `clearMessages()` di deep-chat: qui il
 * componente viene smontato (`started = false` riporta allo stato di
 * riposo, con le tre domande suggerite), quindi i messaggi in memoria se ne
 * vanno con lui e resta da togliere solo la chiave — che è esattamente
 * quel che `clearMessages()` farebbe, un giro in più per lo stesso effetto.
 */
function newConversation() {
  try {
    localStorage.removeItem(storageKey.value)
  } catch {
    // Storage negato: non c'era niente da cancellare.
  }
  started.value = false
  failed.value = false
  // Il ＋ scompare insieme alla conversazione che ha cancellato: senza
  // questo il fuoco cadrebbe sul body e chi naviga da tastiera
  // ripartirebbe dall'inizio della pagina. Va dove va anche l'occhio, sul
  // campo con cui si ricomincia.
  nextTick(() => fakeInput.value?.focus())
}

// Le domande suggerite: dai titoli del manuale quando ce ne sono almeno
// tre, altrimenti tre domande fisse. Una chat vuota su un telefono non
// suggerisce cosa farne.
const fallbackQuestions = [
  'Come finisce la partita?',
  'In quanti si gioca?',
  'Come si contano i punti?',
]

/** Rende un titolo di sezione come domanda. Tabella fissa, niente magia. */
const headingToQuestion: Record<string, string> = {
  'fine partita': 'Come finisce la partita?',
  'fine della partita': 'Come finisce la partita?',
  preparazione: 'Come si prepara il gioco?',
  punteggio: 'Come si contano i punti?',
  conteggio: 'Come si contano i punti?',
  turno: 'Cosa posso fare nel mio turno?',
  'turno del giocatore': 'Cosa posso fare nel mio turno?',
}

const suggestions = computed<string[]>(() => {
  const fromHeadings = props.headings
    .map((h) => headingToQuestion[h.trim().toLowerCase()] ?? `Cosa dice il manuale su "${h}"?`)
    .filter((q, i, all) => all.indexOf(q) === i)
  return fromHeadings.length >= 3 ? fromHeadings.slice(0, 3) : fallbackQuestions
})

/**
 * deep-chat è un web component, non un componente Vue:
 * defineAsyncComponent non serve. L'import dinamico registra l'elemento
 * personalizzato, e da quel momento <deep-chat> nel template si comporta.
 *
 * Si carica al primo gesto e non al mount perché pesa 105 KB gzip, più
 * dell'intera app: chi apre la scheda gioco per leggerla non deve pagarli.
 */
let loading: Promise<unknown> | null = null
function loadDeepChat() {
  loading ??= import('deep-chat')
  return loading
}

// La domanda da eseguire non appena deep-chat è pronto a riceverla (vuota
// = solo mettere a fuoco l'input, senza inviare nulla).
const pendingQuestion = ref('')
// Se mettere a fuoco l'input all'apertura: sì quando è un gesto (clic sul
// campo finto), no quando è la ripresa automatica di una conversazione.
const focusOnReady = ref(true)

async function start(question = '', focus = true) {
  try {
    await loadDeepChat()
  } catch (e) {
    // Rete andata via a metà, o chunk non raggiungibile: senza questo il
    // pannello resterebbe fermo sul finto input senza dire niente.
    console.error('caricamento della chat', e)
    failed.value = true
    return
  }
  pendingQuestion.value = question
  focusOnReady.value = focus
  started.value = true
}

/**
 * deep-chat spara "render" (una sola volta per istanza, internamente
 * garantito) quando la sua vista interna è pronta: pilotarlo prima —
 * anche dopo nextTick(), che aspetta solo la patch del DOM di Vue — fallisce
 * con l'avviso "please wait for chat view to render before calling this
 * property", perché il componente applica le proprietà e disegna la vista
 * shadow DOM in un passaggio differito rispetto alla connessione al DOM.
 * Verificato in browser: senza questo handler la domanda suggerita non
 * partiva mai.
 */
function onChatReady() {
  const el = chat.value as (HTMLElement & {
    submitUserMessage?: (m: { text: string }) => void
    focusInput?: () => void
  }) | null
  if (!el) {
    return
  }
  if (pendingQuestion.value) {
    el.submitUserMessage?.({ text: pendingQuestion.value })
  } else if (focusOnReady.value) {
    el.focusInput?.()
  }
}

/**
 * Le proprietà si passano come oggetti veri, non stringhe JSON: la doc di
 * deep-chat descrive la via JSON-su-attributo per l'uso vanilla (script
 * caricato prima che l'elemento esista nel markup). Qui invece l'elemento
 * personalizzato è già definito — l'abbiamo atteso in loadDeepChat() prima
 * di montare <deep-chat> — quindi Vue assegna questi bind come proprietà
 * JS dell'elemento (perché la chiave esiste già sul suo prototipo), non
 * come attributi HTML. Passare una stringa JSON in quel percorso rompe
 * deep-chat, che si aspetta l'oggetto vero e prova a mutarlo (verificato
 * in browser: "Cannot create property 'disabled' on string ...").
 */
const connect = computed(() => ({ url: `/api/games/${props.gameId}/ask`, method: 'POST' }))

// Lo storico sta nel localStorage del browser, non nel database: nessuna
// tabella, nessun identificativo da inventare per chi non ha un account, e
// la conversazione resta sul telefono di chi l'ha fatta. `maxMessages` è lo
// stesso tetto di requestBodyLimits — tenere in memoria più turni di
// quanti se ne mandano al provider non servirebbe a niente. La proprietà
// da sola basta: deep-chat salva a ogni messaggio e al render rilegge la
// chiave (solo se non gli si passa `history`, che qui non passiamo).
const browserStorage = computed(() => ({ key: storageKey.value, maxMessages: 40 }))

// maxMessages conta i MESSAGGI, non i turni: 20 turni sono 40 messaggi. E
// senza il campo il default manda solo l'ultimo input, quindi la
// conversazione non arriverebbe affatto al server.
const requestBodyLimits = { maxMessages: 40 }

// I due bottoni del campo (invia, detta) nascono come SVG grigio chiaro su
// bianco. `brightness(0)` li porta al nero pieno — il filtro è la sola via
// che deep-chat offre per il colore di quelle icone — e `scale` ingrandisce
// il disegno dentro il bottone, che resta della sua misura.
//
// La misura del *bersaglio* resta quella di deep-chat, 22px: quei bottoni
// sono in `position: absolute` dentro contenitori a larghezza zero nel suo
// shadow DOM, e imporre 44px dalle proprietà documentate li sposta fuori dal
// campo e taglia il microfono oltre il bordo dello schermo (provato e
// osservato in browser). Chi scrive da telefono manda comunque la domanda
// col tasto invio della tastiera, che è un bersaglio a misura piena.
//
// `top: 50%` + `translateY(-50%)` sul contenitore, non un valore fisso:
// deep-chat ancora i due bottoni al fondo del campo con un margine
// costante (`inset-block-end: .85em`, quindi indipendente dall'altezza
// della riga), e la nostra riga è più alta del default — misurato in
// browser: campo 36,5px, bottoni centrati a 513,4 invece che a 520,4 (il
// centro vero), 7px troppo in alto. Il centraggio percentuale resta
// corretto qualunque altezza prenda la riga in futuro, quel valore fisso
// no. Il contenitore (`#text-input-container`) è `position: relative` nel
// CSS di deep-chat: le percentuali di `top` sono relative alla sua
// altezza, non a quella del bottone.
const inputIconButton = {
  container: {
    default: { top: '50%', transform: 'translateY(-50%)' },
    hover: { backgroundColor: '#f2ead4' }, // --card-alt
  },
  svg: { styles: { default: { filter: 'brightness(0)', transform: 'scale(1.15)' } } },
}

// `inside-end`: invio e microfono stanno dentro il campo, a destra, uno
// accanto all'altro. È lo stesso valore per entrambi — deep-chat mette
// nello stesso contenitore i bottoni che dichiarano la stessa posizione —
// e i nomi ammessi sono solo `inside-start`, `inside-end`, `outside-start`
// e `outside-end`: un valore fuori da questi (provato `inside-right`)
// spinge l'invio fuori dal campo, oltre il bordo destro dello schermo.
// `loading` e `stop` prendono lo stesso centraggio di `submit` ma non il
// filtro sull'SVG: sono i tre puntini dell'attesa e il quadrato di stop,
// che deep-chat disegna già del suo grigio. Senza il centraggio restavano
// 7px più in alto del microfono accanto (lo scarto documentato sopra), e
// l'attesa faceva sobbalzare la riga.
const inputIconPosition = { container: inputIconButton.container }

const submitButtonStyles = {
  submit: inputIconButton,
  loading: inputIconPosition,
  stop: inputIconPosition,
  position: 'inside-end',
}

// Il microfono siede accanto all'invio, non sopra: deep-chat piazza i
// bottoni della stessa posizione tutti allo stesso scarto (`left: -27px`,
// misurato in browser), quindi due `inside-end` finiscono uno sull'altro —
// il microfono disegnato sopra la freccia. `right: 2.2em` con `left: auto`
// lo sposta di un bottone a sinistra, che è l'ordine di ogni app di
// messaggi: dettatura, poi invio.
const micIconButton = {
  ...inputIconButton,
  container: {
    ...inputIconButton.container,
    default: { ...inputIconButton.container.default, left: 'auto', right: '2.2em' },
  },
}

// Nessuna chiave: webSpeech usa la Web Speech API del browser. it-IT va
// messo esplicito, il default è en-US. Richiede HTTPS (o localhost): in
// LAN su http il microfono non funziona, ed è documentato nel README.
const speechToText = {
  webSpeech: { language: 'it-IT' },
  // A destra come l'invio, non più `inside-start`: il microfono a sinistra
  // costringeva il testo a un rientro di 2.7em anche sui browser senza Web
  // Speech, dove quel bottone non esiste nemmeno, e il placeholder partiva
  // da metà campo. Fuori dal campo (`outside-end`) invece finisce oltre il
  // bordo destro dello schermo, tagliato a metà (visto su 390px).
  button: { default: micIconButton, position: 'inside-end' },
}

// Il campo di deep-chat nasce bianco, con ombra propria e un margine che lo
// stacca dai bordi: dentro questa app leggeva come un widget incollato
// sopra il cartoncino. Qui prende la stessa resa degli altri campi — fondo
// carta, bordo `--card-line`, raggio 6px, anello rosso al focus — e tutta
// la larghezza del pannello. Le misure sono generose di proposito: il
// campo si tocca col pollice, in piedi al tavolo.
const textInput = {
  placeholder: { text: 'Chiedi una regola…', style: { color: '#6e6250' } }, // --ink-muted
  styles: {
    container: {
      width: '100%',
      margin: '0',
      backgroundColor: '#faf6ec', // --card
      border: '1px solid #ddd0ab', // --card-line
      borderRadius: '6px',
      boxShadow: 'none',
    },
    text: {
      color: '#241f18', // --ink
      padding: '0.6rem 0.75rem',
      // Rientro solo a destra, dove stanno i due bottoni: senza, il testo
      // ci passa sotto. A sinistra resta il padding normale del campo.
      paddingRight: '4.3em',
    },
    // Un anello solo. Prima erano due, `border: 1px` più `outline: 2px`
    // sovrapposti sullo stesso bordo da 6px di raggio: agli angoli i due
    // tratti si scollavano e il campo sembrava avere gli spigoli
    // smangiati (difetto segnalato dall'utente). Il `box-shadow` segue il
    // raggio esattamente, il bordo tiene il contrasto del focus.
    focus: {
      border: '1px solid #9c2b2b', // --accent
      boxShadow: '0 0 0 3px rgb(156 43 43 / 32%)',
    },
  },
}

const errorMessages = {
  overrides: {
    default: 'Qualcosa non ha funzionato. Riprova.',
    service: 'Non riesco a rispondere in questo momento. Guarda il manuale nella scheda del gioco.',
    speechToText: 'La dettatura non è disponibile su questo browser. Scrivi la domanda.',
  },
}

// deep-chat vive in shadow DOM: i token di app.css (--felt, --felt-text,
// --card-alt, --ink per le bolle, --danger e --danger-bg per la bolla di
// errore qui sotto) non ci cascano dentro, quindi i colori sono ricopiati
// qui come valori letterali. Se uno di questi token cambia in app.css, va
// aggiornato a mano anche qui — è la duplicazione che DESIGN.md documenta
// (task 13). Lo stesso vale per --card, --card-line, --ink-muted e
// --accent più sopra (textInput, inputIconButton) e per --card-line nello
// scrollbar qui sotto.
//
// Le due larghezze non sono uguali di proposito. A 92% per tutti le bolle
// arrivavano quasi da bordo a bordo e la conversazione si leggeva come una
// pila di blocchi centrati, senza un lato di chi parla; ora la risposta
// resta ancorata a sinistra e la domanda si stacca a destra. L'angolo
// basso schiacciato a 3px dal lato del parlante fa da codina, come nel
// dado che parla del bottone tondo su mobile.
const messageStyles = {
  default: {
    shared: { bubble: { borderRadius: '10px', fontSize: '0.95rem' } },
    user: {
      bubble: {
        maxWidth: '82%',
        backgroundColor: '#1f4d3a', // --felt
        color: '#f4efe1', // --felt-text
        borderBottomRightRadius: '3px',
      },
    },
    ai: {
      bubble: {
        maxWidth: '88%',
        backgroundColor: '#f2ead4', // --card-alt
        color: '#241f18', // --ink
        borderBottomLeftRadius: '3px',
      },
    },
  },
  // Senza questa voce l'errore usciva nel rosa di serie di deep-chat: in
  // una palette dove non esiste un rosa era la cosa più fuori posto della
  // pagina. Qui porta gli stessi due colori di `.error` in app.css.
  error: {
    bubble: {
      backgroundColor: '#f8e3d4', // --danger-bg
      color: '#ad4a22', // --danger
      borderRadius: '10px',
      fontSize: '0.95rem',
    },
  },
}

// Il link alla pagina del manuale nasce col blu di serie del browser
// (#0000EE, misurato): l'unica cosa blu in tutta l'app, dentro l'unica
// bolla che cita una fonte. Qui prende il rosso dei link di app.css.
const auxiliaryStyle = `
  ::-webkit-scrollbar { width: 8px; }
  ::-webkit-scrollbar-thumb { background-color: #ddd0ab; border-radius: 4px; } /* --card-line */
  .message-bubble a { color: #9c2b2b; text-underline-offset: 2px; } /* --accent */
`
</script>

<template>
  <div class="manual-chat-panel">
    <!--
      La testata dice di cosa si tratta: nella sidebar desktop, senza,
      restava una colonna di bolle senza nome. "L'Arbitro" è chi risolve le
      dispute al tavolo — e il sottotitolo tiene fermo che risponde dal
      manuale, non a memoria.
    -->
    <!--
      Un div e non un <header>: dentro un <dialog> un <header> prende il
      ruolo `banner`, e un secondo banner di pagina comparirebbe
      nell'elenco dei landmark di uno screen reader (visto nell'albero di
      accessibilità del dialog mobile).
    -->
    <div class="manual-chat-head">
      <div class="manual-chat-head-name">
        <h2>L'Arbitro</h2>
        <p>risposte dal manuale</p>
      </div>
      <div class="manual-chat-head-actions">
        <!--
          Solo a conversazione avviata: nello stato di riposo non c'è niente
          da azzerare, e un ＋ che non fa nulla è peggio di un ＋ che manca.
        -->
        <button
          v-if="started"
          type="button"
          class="manual-chat-new"
          title="Nuova conversazione"
          aria-label="Nuova conversazione"
          @click="newConversation"
        >
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" />
          </svg>
        </button>
        <button
          v-if="closable"
          type="button"
          class="modal-close"
          aria-label="Chiudi"
          @click="emit('close')"
        >
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M6 6l12 12M18 6L6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </div>
    </div>

    <p class="manual-chat-note">Controlla sempre la pagina citata.</p>

    <!--
      Il corpo ha UN figlio solo, ed è voluto: il widget di deep-chat vuole
      un'altezza dichiarata (`height: 100%`) per non piantarsi ai suoi 350px
      di default, e quel 100% è esatto solo se non ha fratelli che si
      prendono una fetta della colonna. La nota sta fuori, sopra, per
      questo.
    -->
    <div class="manual-chat-body">
      <div v-if="!started && !failed" class="manual-chat-rest">
        <p class="manual-chat-intro">
          Chiedi una regola di <strong>{{ gameName }}</strong> a parole tue.
        </p>
        <ul class="manual-chat-suggestions">
          <li v-for="q in suggestions" :key="q">
            <button type="button" @click="start(q)">{{ q }}</button>
          </li>
        </ul>
        <button ref="fakeInput" type="button" class="manual-chat-fakeinput" @click="start()">
          Chiedi una regola…
        </button>
      </div>

      <p v-else-if="failed" class="error">
        La chat non si è caricata. Ricarica la pagina, oppure apri il manuale
        dalla scheda del gioco.
      </p>

      <deep-chat
        v-else
        ref="chat"
        class="manual-chat-widget"
        :connect="connect"
        :browserStorage="browserStorage"
        :requestBodyLimits="requestBodyLimits"
        :speechToText="speechToText"
        :textInput="textInput"
        :submitButtonStyles="submitButtonStyles"
        :errorMessages="errorMessages"
        :messageStyles="messageStyles"
        :auxiliaryStyle="auxiliaryStyle"
        @render="onChatReady"
      />
    </div>
  </div>
</template>
