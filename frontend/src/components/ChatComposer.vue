<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'

/**
 * Il campo con cui si scrive al Mentore: testo sopra, dettatura e invio
 * in una riga sotto, a destra — e, quando c'è più di un agente, il
 * selettore ("Regolamento ▾") a sinistra. È markup nostro e non il campo di
 * deep-chat perché quello posiziona i suoi bottoni in `position: absolute`
 * dentro contenitori a larghezza zero: il microfono accanto all'invio si
 * otteneva solo con scarti misurati a mano, e uno stato "sto ascoltando"
 * che cambia bottone e segnaposto non si poteva fare. deep-chat resta a
 * disegnare la conversazione; questo componente gli passa solo il testo
 * (`send`).
 *
 * Non sa niente della chat: chi lo monta decide cosa fare del testo,
 * quali agenti offrire e quando è `busy` (risposta in arrivo — si può
 * scrivere la prossima domanda, non mandarla).
 */
export type ComposerAgent = { key: string; label: string; tag: string; disabled: boolean }

const props = defineProps<{
  busy?: boolean
  /** Le voci del selettore. Meno di due voci = nessun selettore. */
  agents?: ComposerAgent[]
  placeholder?: string
}>()

const emit = defineEmits<{ send: [text: string] }>()

const agent = defineModel<string>('agent', { default: '' })

const draft = ref('')
const field = ref<HTMLTextAreaElement | null>(null)

const canSend = computed(() => !props.busy && draft.value.trim().length > 0)

function send() {
  if (!canSend.value) {
    return
  }
  stopListening()
  emit('send', draft.value.trim())
  draft.value = ''
}

function onKeydown(e: KeyboardEvent) {
  // Invio manda, Maiusc+Invio va a capo. `isComposing`: con le tastiere a
  // composizione (accenti su alcune tastiere, IME) l'Invio conferma la
  // lettera, non la domanda.
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    send()
  }
}

// Il campo cresce con il testo fino a ~5 righe, poi scorre. In JS e non
// con `field-sizing: content`, che Safari non supporta ancora: su iPhone il
// campo resterebbe di una riga.
const MAX_HEIGHT = 128
function autosize() {
  const el = field.value
  if (!el) {
    return
  }
  el.style.height = '0px'
  const h = el.scrollHeight
  el.style.height = `${Math.min(h, MAX_HEIGHT)}px`
  el.style.overflowY = h > MAX_HEIGHT ? 'auto' : 'hidden'
}
watch(draft, () => nextTick(autosize))

/* ---------- Dettatura ----------
 * Web Speech API direttamente, senza deep-chat: nessuna chiave, it-IT
 * esplicito (il default è en-US). Richiede HTTPS o localhost — in LAN su
 * http il bottone c'è ma il browser nega il permesso, e lo diciamo.
 * I tipi non sono in lib.dom: bastano i pochi campi che usiamo. */
type Recognition = {
  lang: string
  interimResults: boolean
  continuous: boolean
  start(): void
  stop(): void
  onresult: ((e: { resultIndex: number; results: ArrayLike<ArrayLike<{ transcript: string }> & { isFinal: boolean }> }) => void) | null
  onerror: ((e: { error: string }) => void) | null
  onend: (() => void) | null
}
type RecognitionCtor = new () => Recognition

const Ctor: RecognitionCtor | undefined =
  typeof window === 'undefined'
    ? undefined
    : ((window as unknown as { SpeechRecognition?: RecognitionCtor }).SpeechRecognition ??
      (window as unknown as { webkitSpeechRecognition?: RecognitionCtor }).webkitSpeechRecognition)

// Senza API il bottone non compare: un microfono che non fa niente è
// peggio di un microfono che manca.
const speechSupported = !!Ctor
const listening = ref(false)
const speechError = ref('')
let recognition: Recognition | null = null

function startListening() {
  if (!Ctor) {
    return
  }
  speechError.value = ''
  // Il testo già scritto resta: la dettatura si accoda, non lo sostituisce.
  const base = draft.value.trimEnd()
  const r = new Ctor()
  r.lang = 'it-IT'
  r.interimResults = true
  r.continuous = false
  r.onresult = (e) => {
    let heard = ''
    for (let i = 0; i < e.results.length; i++) {
      heard += e.results[i][0].transcript
    }
    draft.value = base ? `${base} ${heard.trim()}` : heard.trim()
  }
  r.onerror = (e) => {
    // `no-speech` e `aborted` non sono errori da mostrare: il primo è un
    // silenzio, il secondo lo stop che abbiamo chiesto noi.
    if (e.error === 'not-allowed' || e.error === 'service-not-allowed') {
      speechError.value = 'Il microfono non è permesso su questa pagina. Scrivi la domanda.'
    } else if (e.error !== 'no-speech' && e.error !== 'aborted') {
      speechError.value = 'La dettatura non ha funzionato. Riprova o scrivi la domanda.'
    }
  }
  r.onend = () => {
    listening.value = false
    recognition = null
    field.value?.focus()
  }
  recognition = r
  listening.value = true
  r.start()
}

function stopListening() {
  recognition?.stop()
}

function toggleListening() {
  if (listening.value) {
    stopListening()
  } else {
    startListening()
  }
}

onBeforeUnmount(() => {
  if (recognition) {
    recognition.onend = null
    recognition.stop()
  }
})

function focus() {
  field.value?.focus()
}

defineExpose({ focus })

/* ---------- Selettore dell'agente ----------
 * Sotto le due voci minime resta nascosto: chi monta il componente con un
 * solo agente non vede né bottone né menu, com'era prima di questo task. */
const menuOpen = ref(false)
const menu = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
// Id delle voci unici per istanza: `aria-activedescendant` punta a un id
// del documento, e due composer montati insieme non devono condividerlo.
const uid = useId()
// La voce sotto il cursore o sotto le frecce: l'evidenziazione scivola lì.
const highlighted = ref(0)
const showPicker = computed(() => (props.agents?.length ?? 0) >= 2)
const current = computed(() => props.agents?.find((a) => a.key === agent.value))

// La prossima voce sceglibile nella direzione `dir` (+1 giù, -1 su), con
// wrap: le voci disabilitate si saltano in entrambi i versi — prima ↓ le
// saltava e ↑ no, e l'evidenziazione finiva su una voce che non si sceglie.
function nextEnabled(from: number, dir: 1 | -1): number {
  const list = props.agents ?? []
  for (let step = 1; step <= list.length; step++) {
    const i = (from + dir * step + list.length) % list.length
    if (!list[i].disabled) return i
  }
  return from
}

function openMenu() {
  if (props.busy) return
  const list = props.agents ?? []
  const i = list.findIndex((a) => a.key === agent.value)
  highlighted.value = i >= 0 && !list[i].disabled ? i : nextEnabled(-1, 1)
  menuOpen.value = true
  nextTick(() => menu.value?.focus())
}
function closeMenu(refocus = true) {
  menuOpen.value = false
  if (refocus) trigger.value?.focus()
}
function pick(a: ComposerAgent) {
  if (a.disabled) return
  agent.value = a.key
  closeMenu(false)
  focus()
}
function onMenuKeydown(e: KeyboardEvent) {
  const list = props.agents ?? []
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    highlighted.value = nextEnabled(highlighted.value, e.key === 'ArrowDown' ? 1 : -1)
  } else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    pick(list[highlighted.value])
  } else if (e.key === 'Escape' || e.key === 'Tab') {
    closeMenu(e.key === 'Escape')
  }
}
// Cambiare agente smonta la conversazione in corso: con una risposta in
// arrivo la si perderebbe. Il bottone si spegne finché `busy`, e un menu
// già aperto si chiude.
watch(
  () => props.busy,
  (b) => {
    if (b && menuOpen.value) closeMenu(false)
  },
)
// Clic fuori chiude: pointerdown e non click, così il menu non si
// riapre sotto il dito quando si tocca di nuovo il bottone.
function onDocPointerDown(e: PointerEvent) {
  if (!(e.target as Element).closest('.chat-composer-agent')) closeMenu(false)
}
watch(menuOpen, (open) => {
  if (open) document.addEventListener('pointerdown', onDocPointerDown)
  else document.removeEventListener('pointerdown', onDocPointerDown)
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocPointerDown))
</script>

<template>
  <div class="chat-composer-wrap">
    <!--
      Il clic su tutto il riquadro mette a fuoco il campo: la riga dei
      bottoni sotto il testo è parte del "campo" per l'occhio, e cliccarci
      in un punto vuoto non deve fare niente di diverso.
    -->
    <div class="chat-composer" :class="{ 'is-listening': listening }" @click.self="focus">
      <textarea
        ref="field"
        v-model="draft"
        class="chat-composer-field"
        rows="1"
        enterkeyhint="send"
        aria-label="La tua domanda"
        :placeholder="listening ? 'Sto ascoltando…' : (placeholder ?? 'Chiedi una regola…')"
        @keydown="onKeydown"
      />
      <div class="chat-composer-actions" @click.self="focus">
        <div v-if="showPicker" class="chat-composer-agent">
          <button
            ref="trigger"
            type="button"
            class="chat-composer-agent-trigger"
            aria-haspopup="listbox"
            :aria-expanded="menuOpen"
            :aria-label="`${current?.label ?? 'Agente'}: scegli con chi parlare`"
            :disabled="busy"
            :title="busy ? 'Aspetta la risposta per cambiare' : undefined"
            @click="menuOpen ? closeMenu() : openMenu()"
          >
            {{ current?.label ?? 'Agente' }}
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M6 9l6 6 6-6" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </button>
          <ul
            v-if="menuOpen"
            ref="menu"
            class="chat-composer-agent-menu"
            role="listbox"
            tabindex="-1"
            :aria-activedescendant="`${uid}-opt-${highlighted}`"
            @keydown="onMenuKeydown"
          >
            <li
              v-for="(a, i) in agents"
              :id="`${uid}-opt-${i}`"
              :key="a.key"
              role="option"
              :aria-selected="a.key === agent"
              :aria-disabled="a.disabled"
              :class="{ 'is-highlighted': i === highlighted, 'is-disabled': a.disabled }"
              @pointerenter="!a.disabled && (highlighted = i)"
              @click="pick(a)"
            >
              <span class="chat-composer-agent-name">{{ a.label }}</span>
              <span class="chat-composer-agent-tag">{{ a.disabled ? 'non disponibile per questo gioco' : a.tag }}</span>
              <svg v-if="a.key === agent" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path d="M20 6L9 17l-5-5" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
            </li>
          </ul>
        </div>
        <button
          v-if="speechSupported"
          type="button"
          class="chat-composer-mic"
          :aria-label="listening ? 'Ferma la dettatura' : 'Detta la domanda'"
          :aria-pressed="listening"
          @click="toggleListening"
        >
          <!-- Tre barrette che ballano: si vede da lontano che sta ascoltando. -->
          <span v-if="listening" class="chat-composer-eq" aria-hidden="true">
            <span /><span /><span />
          </span>
          <svg v-else viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 2.5a3 3 0 0 0-3 3v6.5a3 3 0 0 0 6 0V5.5a3 3 0 0 0-3-3z" stroke="currentColor" stroke-width="1.9" stroke-linejoin="round" />
            <path d="M18.5 10.5v1.5a6.5 6.5 0 0 1-13 0v-1.5M12 18.5v3" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" />
          </svg>
        </button>
        <button
          type="button"
          class="chat-composer-send"
          aria-label="Invia"
          :disabled="!canSend"
          @click="send"
        >
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 19V5M5.5 11.5 12 5l6.5 6.5" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </div>
    </div>
    <p v-if="speechError" class="chat-composer-error" role="status">{{ speechError }}</p>
  </div>
</template>
