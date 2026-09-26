<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import ChatComposer, { type ComposerAgent } from './ChatComposer.vue'
import type { ChatAgent, ChatAvailability } from '../utils/game'

/**
 * Il contenuto della chat, indipendente da dove vive: la sidebar su
 * desktop e il dialog a tutto schermo su mobile montano questo.
 *
 * Nello stato di riposo il markup è nostro: titolo e tre domande
 * suggerite. deep-chat si monta al primo gesto — una domanda inviata dal
 * campo o una suggerita — perché pesa 105 KB gzip, più dell'intera app, e
 * chi apre una scheda gioco per leggerla non deve pagarli. È questo che
 * permette alla sidebar di stare aperta per default. L'unica eccezione è
 * chi ha già una conversazione salvata su questo gioco (vedi
 * `hasSavedConversation`): lì il filo si riapre al caricamento, perché
 * quei byte li ha già scaricati e ritrovare la conversazione dov'era vale
 * più del risparmio.
 *
 * Il campo in fondo (`ChatComposer`) è nostro in entrambi gli stati: si
 * scrive subito, senza aspettare deep-chat, e a conversazione avviata gli
 * passa il testo con `submitUserMessage`. Il campo di deep-chat è nascosto
 * (`auxiliaryStyle`): a deep-chat resta solo il filo dei messaggi.
 *
 * La testata (titolo, "nuova conversazione", chiusura) sta qui e non nei
 * due contenitori: prima esisteva solo nel dialog mobile e la sidebar
 * desktop non diceva nemmeno di essere una chat.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  /** Quali agenti ha il gioco (Manuale/regole, Strategia). */
  chat: ChatAvailability
  /** Le tre domande suggerite per agente, già formulate dal modello. Vuota = si usano le fisse. */
  suggestedQuestions: Record<ChatAgent, string[]>
  /** Il gioco ha un manuale indicizzato: cambia il sottotitolo del Manuale. */
  hasManual: boolean
  /** Nel dialog mobile la testata porta anche la ×; nella sidebar no. */
  closable?: boolean
  /** Nella barra desktop la testata porta il bottone per ingrandire in modale. */
  expandable?: boolean
  /** La barra è ingrandita: cambia icona ed etichetta del bottone. */
  expanded?: boolean
}>()

const emit = defineEmits<{ close: []; toggleExpand: [] }>()

const expandButton = ref<HTMLButtonElement | null>(null)

const started = ref(false)
const failed = ref(false)
const chat = ref<HTMLElement | null>(null)
const composer = ref<InstanceType<typeof ChatComposer> | null>(null)

// Per ManualChat: dove mettere il fuoco entrando e uscendo dalla modale.
defineExpose({
  focusComposer: () => composer.value?.focus(),
  focusExpand: () => expandButton.value?.focus(),
})
// Una risposta è in arrivo: il campo accetta la prossima domanda ma non la
// manda, così due richieste non si accavallano sulla stessa conversazione.
const busy = ref(false)

// Una conversazione per agente: chi passa alla Strategia e torna ritrova il
// suo filo sulle regole. L'agente scelto si ricorda per gioco.
const agentKey = computed(() => `bgm-chat-${props.gameId}-agent`)
function initialAgent(): ChatAgent {
  try {
    const saved = localStorage.getItem(agentKey.value)
    if ((saved === 'rules' || saved === 'strategy') && props.chat[saved]) return saved
  } catch {
    // Storage negato: si parte dal default.
  }
  return props.chat.rules ? 'rules' : 'strategy'
}
const agent = ref<ChatAgent>(initialAgent())

// Una chiave per gioco e per agente: le conversazioni di due giochi (o due
// agenti dello stesso gioco) non si mescolano, e chi torna sulla scheda
// ritrova la sua. deep-chat la gestisce da sé (`browserStorage` qui sotto)
// — scrive a ogni messaggio e rilegge al render — quindi non c'è codice
// nostro che serializza niente.
const storageKey = computed(() => `bgm-chat-${props.gameId}-${agent.value}`)

// La chiave di prima (una sola conversazione per gioco) diventa quella del
// Manuale, una volta sola: chi aveva un filo aperto prima dell'aggiornamento
// non lo perde.
function migrateLegacyConversation() {
  try {
    const legacy = `bgm-chat-${props.gameId}`
    const raw = localStorage.getItem(legacy)
    if (raw === null) return
    if (localStorage.getItem(`${legacy}-rules`) === null) {
      localStorage.setItem(`${legacy}-rules`, raw)
    }
    localStorage.removeItem(legacy)
  } catch {
    // Storage negato: niente da migrare.
  }
}

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
  migrateLegacyConversation()
  if (hasSavedConversation()) {
    // Senza focus: la sidebar è aperta per default e rubare il cursore a
    // chi ha appena aperto la scheda gioco per leggerla sarebbe un agguato.
    start('', false)
  }
})

// Cambiare agente smonta deep-chat (`:key` sull'agente) e rimonta la
// conversazione dell'altro, o lo stato di riposo con le sue domande.
watch(agent, (next) => {
  try {
    localStorage.setItem(agentKey.value, next)
  } catch {
    // Storage negato: la scelta vale finché la pagina resta aperta.
  }
  started.value = false
  failed.value = false
  busy.value = false
  pendingQuestion.value = ''
  if (hasSavedConversation()) start('', true)
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
  busy.value = false
  // Il ＋ scompare insieme alla conversazione che ha cancellato: senza
  // questo il fuoco cadrebbe sul body e chi naviga da tastiera
  // ripartirebbe dall'inizio della pagina. Va dove va anche l'occhio, sul
  // campo con cui si ricomincia.
  nextTick(() => composer.value?.focus())
}

// Le voci del selettore: due sempre, quella senza l'agente disponibile
// disabilitata (il selettore stesso resta nascosto sotto i due agenti, ma
// qui i due esistono sempre — è `disabled` a cambiare).
const agents = computed<ComposerAgent[]>(() => [
  { key: 'rules', label: 'Manuale', tag: 'regole', disabled: !props.chat.rules },
  { key: 'strategy', label: 'Strategia', tag: 'consigli', disabled: !props.chat.strategy },
])

// Le domande suggerite: tre domande fisse per agente. Una chat vuota su un
// telefono non suggerisce cosa farne.
const fallbackQuestions: Record<ChatAgent, string[]> = {
  rules: ['Come finisce la partita?', 'In quanti si gioca?', 'Come si contano i punti?'],
  strategy: [
    'Come imposto una buona apertura?',
    'Su cosa conviene puntare a metà partita?',
    'Quali errori fanno i principianti?',
  ],
}

// Le domande suggerite arrivano già formulate dal server, generate dal
// modello sui titoli del manuale vero (per il Manuale) o dal forum di BGG
// (per la Strategia). Prima si costruivano qui da quei titoli con una
// tabella fissa, e su un manuale reale il risultato era «Cosa dice il
// manuale su "di Klaus-Jürgen Wrede"?»: il template non poteva fare di
// meglio, perché una domanda non è un titolo di sezione con un giro di
// frase intorno.
const suggestions = computed<string[]>(() => {
  const qs = props.suggestedQuestions[agent.value] ?? []
  return qs.length >= 3 ? qs.slice(0, 3) : fallbackQuestions[agent.value]
})

// Il sottotitolo dice da dove arrivano le risposte: dal manuale se il gioco
// ne ha uno indicizzato, altrimenti dal forum di BGG (la Strategia parla
// sempre e solo col forum, non ha un manuale da leggere).
const subtitle = computed(() => {
  if (agent.value === 'strategy') return 'consigli dal forum di BGG'
  return props.hasManual ? 'risposte dal manuale' : 'risposte dal forum di BGG'
})
const placeholder = computed(() =>
  agent.value === 'strategy' ? 'Chiedi un consiglio…' : 'Chiedi una regola…',
)

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
// = solo mettere a fuoco il campo, senza inviare nulla).
const pendingQuestion = ref('')
// Se mettere a fuoco il campo all'apertura: sì quando è un gesto, no quando
// è la ripresa automatica di una conversazione.
const focusOnReady = ref(true)

async function start(question = '', focus = true) {
  // `busy` già durante il download: un secondo tap (su una domanda o
  // sull'invio) mentre arrivano i 105 KB non deve partire due volte.
  if (question) {
    busy.value = true
  }
  try {
    await loadDeepChat()
  } catch (e) {
    busy.value = false
    // Rete andata via a metà, o chunk non raggiungibile: senza questo il
    // pannello resterebbe fermo sullo stato di riposo senza dire niente.
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
type DeepChatEl = HTMLElement & { submitUserMessage?: (m: { text: string }) => void }

function onChatReady() {
  if (pendingQuestion.value) {
    submit(pendingQuestion.value)
    pendingQuestion.value = ''
  } else if (focusOnReady.value) {
    composer.value?.focus()
  }
}

function submit(text: string) {
  const el = chat.value as DeepChatEl | null
  if (!el?.submitUserMessage) {
    return
  }
  busy.value = true
  el.submitUserMessage({ text })
}

/**
 * Dal campo: prima domanda → monta deep-chat e la manda al render (vedi
 * `onChatReady`); dopo, dritta a deep-chat.
 */
function onComposerSend(text: string) {
  if (started.value) {
    submit(text)
  } else {
    start(text)
  }
}

// La risposta è arrivata (o è fallita): il campo torna a mandare. I
// messaggi riletti dallo storico al render (`isHistory`) non contano.
function onChatMessage(e: Event) {
  const detail = (e as CustomEvent<{ message: { role?: string }; isHistory: boolean }>).detail
  if (!detail.isHistory && detail.message.role !== 'user') {
    busy.value = false
  }
}

function onChatError() {
  busy.value = false
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
const connect = computed(() => ({
  url: `/api/games/${props.gameId}/ask`,
  method: 'POST',
  additionalBodyProps: { agent: agent.value },
}))

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

const errorMessages = {
  overrides: {
    default: 'Qualcosa non ha funzionato. Riprova.',
    service: 'Non riesco a rispondere in questo momento. Riprova tra poco.',
  },
}

// deep-chat vive in shadow DOM: i token di app.css (--felt, --felt-text,
// --card-alt, --ink per le bolle, --danger e --danger-bg per la bolla di
// errore qui sotto) non ci cascano dentro, quindi i colori sono ricopiati
// qui come valori letterali. Se uno di questi token cambia in app.css, va
// aggiornato a mano anche qui — è la duplicazione che DESIGN.md documenta
// (task 13). Lo stesso vale per --card, --card-line, --ink-muted e
// --card-line nello scrollbar qui sotto.
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
//
// `#input` è il campo di deep-chat: al suo posto c'è ChatComposer, fuori
// dallo shadow DOM.
const auxiliaryStyle = `
  #input { display: none; }
  ::-webkit-scrollbar { width: 8px; }
  ::-webkit-scrollbar-thumb { background-color: #ddd0ab; border-radius: 4px; } /* --card-line */
  .message-bubble a { color: #9c2b2b; text-underline-offset: 2px; } /* --accent */
`
</script>

<template>
  <div class="manual-chat-panel">
    <!--
      La testata dice di cosa si tratta: nella sidebar desktop, senza,
      restava una colonna di bolle senza nome. "Il Mentore" spiega le regole
      e insegna a giocare meglio; il sottotitolo dice da dove arrivano le
      risposte.
    -->
    <!--
      Un div e non un <header>: dentro un <dialog> un <header> prende il
      ruolo `banner`, e un secondo banner di pagina comparirebbe
      nell'elenco dei landmark di uno screen reader (visto nell'albero di
      accessibilità del dialog mobile).
    -->
    <div class="manual-chat-head">
      <div class="manual-chat-head-name">
        <h2>Il Mentore</h2>
        <p>{{ subtitle }}</p>
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
        <!--
          Frecce che si allargano per ingrandire, che rientrano per
          ridurre: lo stesso tratto da 1.9 del ＋ accanto.
        -->
        <button
          v-if="expandable"
          ref="expandButton"
          type="button"
          class="manual-chat-new"
          :title="expanded ? 'Riduci' : 'Ingrandisci'"
          :aria-label="expanded ? 'Riduci la chat' : 'Ingrandisci la chat'"
          @click="emit('toggleExpand')"
        >
          <svg v-if="expanded" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M4 14h6v6M20 10h-6V4M10 14l-6 6M14 10l6-6" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M14 4h6v6M10 20H4v-6M20 4l-6 6M4 20l6-6" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" />
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

    <p class="manual-chat-note">Controlla sempre la fonte citata.</p>

    <!--
      Il corpo ha UN figlio solo, ed è voluto: il widget di deep-chat vuole
      un'altezza dichiarata (`height: 100%`) per non piantarsi ai suoi 350px
      di default, e quel 100% è esatto solo se non ha fratelli che si
      prendono una fetta della colonna. La nota sta fuori, sopra, e il
      campo fuori, sotto, per questo.
    -->
    <div class="manual-chat-body">
      <div v-if="!started && !failed" class="manual-chat-rest">
        <!--
          Invito e domande scorrono, il campo (fuori dal corpo) no: le
          domande sono frasi di lunghezza variabile e su una finestra bassa
          (telefono in orizzontale) il blocco non ci sta. Prima veniva
          tagliato dall'`overflow: hidden` del corpo, e la prima cosa a
          sparire era l'invito a scrivere in fondo.
        -->
        <div class="manual-chat-rest-scroll">
          <p class="manual-chat-intro">
            <template v-if="agent === 'strategy'">Chiedi come giocare meglio a <strong>{{ gameName }}</strong>.</template>
            <template v-else>Chiedi una regola di <strong>{{ gameName }}</strong> a parole tue.</template>
          </p>
          <!--
            `role="list"` come ogni altra lista spogliata del progetto: con
            `list-style: none` Safari/VoiceOver le toglie la semantica di
            lista, e queste tre ora sono domande vere, non tre righe della
            stessa forma da scorrere con l'occhio.

            La chiave è l'indice e non il testo: le domande le scrive un
            modello e due uguali sono possibili — con `:key="q"` sarebbero
            chiavi duplicate.
          -->
          <ul role="list" class="manual-chat-suggestions">
            <li v-for="(q, i) in suggestions" :key="i">
              <button type="button" :disabled="busy" @click="start(q)">{{ q }}</button>
            </li>
          </ul>
        </div>
      </div>

      <!-- Il ripiego del manuale vale solo per il Manuale: la Strategia
           non ha un documento da aprire al posto della risposta. -->
      <p v-else-if="failed" class="error">
        La chat non si è caricata. Ricarica la pagina<template v-if="agent === 'rules'">, oppure apri il manuale
        dalla scheda del gioco</template>.
      </p>

      <deep-chat
        v-else
        :key="agent"
        ref="chat"
        class="manual-chat-widget"
        :connect="connect"
        :browserStorage="browserStorage"
        :requestBodyLimits="requestBodyLimits"
        :errorMessages="errorMessages"
        :messageStyles="messageStyles"
        :auxiliaryStyle="auxiliaryStyle"
        @render="onChatReady"
        @message="onChatMessage"
        @error="onChatError"
      />
    </div>

    <ChatComposer
      v-if="!failed"
      ref="composer"
      v-model:agent="agent"
      class="manual-chat-composer"
      :busy="busy"
      :agents="agents"
      :placeholder="placeholder"
      @send="onComposerSend"
    />
  </div>
</template>
