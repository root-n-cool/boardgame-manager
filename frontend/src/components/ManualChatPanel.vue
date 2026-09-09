<script setup lang="ts">
import { computed, ref } from 'vue'

/**
 * Il contenuto della chat, indipendente da dove vive: la sidebar su
 * desktop e il dialog a tutto schermo su mobile montano questo.
 *
 * Nello stato di riposo il markup è nostro: titolo, tre domande suggerite e
 * un finto campo di input. deep-chat si monta al primo gesto — clic sul
 * campo o su una domanda — perché pesa 105 KB gzip, più dell'intera app, e
 * chi apre una scheda gioco per leggerla non deve pagarli. È questo che
 * permette alla sidebar di stare aperta per default.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  /** I titoli di sezione del manuale, per le domande suggerite. */
  headings: string[]
}>()

const started = ref(false)
const failed = ref(false)
const chat = ref<HTMLElement | null>(null)

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

async function start(question = '') {
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
  } else {
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

const submitButtonStyles = { submit: inputIconButton }

// Nessuna chiave: webSpeech usa la Web Speech API del browser. it-IT va
// messo esplicito, il default è en-US. Richiede HTTPS (o localhost): in
// LAN su http il microfono non funziona, ed è documentato nel README.
const speechToText = {
  webSpeech: { language: 'it-IT' },
  // `inside-start` e non il default `outside-end`: il campo qui è largo
  // quanto il pannello, e un bottone *fuori* dal campo finisce oltre il
  // bordo destro dello schermo, tagliato a metà (visto su 390px). Dentro,
  // il microfono sta a sinistra e l'invio a destra, uno per capo.
  button: { default: inputIconButton, position: 'inside-start' },
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
      // I due bottoni stanno *dentro* il campo, uno per capo: senza questi
      // due rientri il testo ci passa sotto (il microfono copriva la prima
      // lettera del placeholder).
      paddingLeft: '2.7em',
      paddingRight: '2.7em',
    },
    focus: { border: '1px solid #9c2b2b', outline: '2px solid #9c2b2b' }, // --accent
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
const messageStyles = {
  default: {
    shared: { bubble: { borderRadius: '10px', fontSize: '0.95rem', maxWidth: '92%' } },
    user: { bubble: { backgroundColor: '#1f4d3a', color: '#f4efe1' } }, // --felt / --felt-text
    ai: { bubble: { backgroundColor: '#f2ead4', color: '#241f18' } }, // --card-alt / --ink
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

const auxiliaryStyle = `
  ::-webkit-scrollbar { width: 8px; }
  ::-webkit-scrollbar-thumb { background-color: #ddd0ab; border-radius: 4px; } /* --card-line */
`
</script>

<template>
  <div class="manual-chat-panel">
    <p class="manual-chat-note">
      Risposte generate dal manuale: controlla sempre la pagina citata.
    </p>

    <div v-if="!started && !failed" class="manual-chat-rest">
      <p class="manual-chat-intro">
        Chiedi una regola di <strong>{{ gameName }}</strong> a parole tue.
      </p>
      <ul class="manual-chat-suggestions">
        <li v-for="q in suggestions" :key="q">
          <button type="button" @click="start(q)">{{ q }}</button>
        </li>
      </ul>
      <button type="button" class="manual-chat-fakeinput" @click="start()">
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
</template>
