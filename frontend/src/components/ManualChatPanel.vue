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

// Nessuna chiave: webSpeech usa la Web Speech API del browser. it-IT va
// messo esplicito, il default è en-US. Richiede HTTPS (o localhost): in
// LAN su http il microfono non funziona, ed è documentato nel README.
const speechToText = { webSpeech: { language: 'it-IT' } }

const textInput = { placeholder: { text: 'Chiedi una regola…' } }

const errorMessages = {
  overrides: {
    default: 'Qualcosa non ha funzionato. Riprova.',
    service: 'Non riesco a rispondere in questo momento. Guarda il manuale nella scheda del gioco.',
    speechToText: 'La dettatura non è disponibile su questo browser. Scrivi la domanda.',
  },
}

// deep-chat vive in shadow DOM: i token di app.css (--felt, --felt-text,
// --card-alt, --ink in frontend/src/app.css) non ci cascano dentro, quindi
// i colori sono ricopiati qui come valori letterali. Se --felt o gli altri
// cambiano in app.css, questi vanno aggiornati a mano — è la duplicazione
// che DESIGN.md documenta (task 13).
const messageStyles = {
  default: {
    shared: { bubble: { borderRadius: '10px', fontSize: '0.95rem', maxWidth: '92%' } },
    user: { bubble: { backgroundColor: '#1f4d3a', color: '#f4efe1' } }, // --felt / --felt-text
    ai: { bubble: { backgroundColor: '#f2ead4', color: '#241f18' } }, // --card-alt / --ink
  },
}

const auxiliaryStyle = `
  ::-webkit-scrollbar { width: 8px; }
  ::-webkit-scrollbar-thumb { background-color: #ddd0ab; border-radius: 4px; }
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
      :errorMessages="errorMessages"
      :messageStyles="messageStyles"
      :auxiliaryStyle="auxiliaryStyle"
      @render="onChatReady"
    />
  </div>
</template>
