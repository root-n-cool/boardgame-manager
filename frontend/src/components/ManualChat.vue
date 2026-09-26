<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ManualChatPanel from './ManualChatPanel.vue'
import type { ChatAgent, ChatAvailability } from '../utils/game'

/**
 * Decide DOVE vive la chat, non cos'è.
 *
 * Da 1100px in su è una sidebar destra sticky, sempre aperta, col suo
 * scroll: la colonna di sinistra scorre e la chat resta ferma. Niente
 * collasso — è una scelta esplicita, non una funzione mancata: la sidebar
 * di navigazione a sinistra (`.app-sidebar`) non si nasconde da desktop, e
 * un bottone che facesse sparire quella di destra sarebbe un'incoerenza in
 * più da spiegare, non un risparmio di spazio che serve davvero (22rem su
 * un desktop). La soglia è 1100 e non 900 perché `.app-page` è larga 56rem:
 * una sidebar da 22rem dentro quello spazio lascerebbe al testo 34rem,
 * sotto la misura leggibile.
 *
 * Da desktop la barra si può anche **ingrandire** in una modale centrata,
 * larga metà schermo: a 22rem una risposta lunga è una colonna stretta da
 * scorrere. La barra contiene un <dialog> che passa da `show()` (barra,
 * nel flusso dell'aside) a `showModal()` (modale, nel top layer) e
 * ritorno: il nodo non si sposta mai, quindi deep-chat non viene
 * scollegato e rimontato, e una risposta in arrivo non si perde. Sfondo,
 * focus trap ed Esc vengono dal <dialog> modale; Esc qui riduce, non
 * chiude (`cancel` intercettato).
 *
 * Sotto 1100px è un bottone tondo in basso al centro che apre un <dialog>
 * nativo a tutto schermo. Il <dialog> regala focus trap, Esc e sfondo
 * inerte: lo stesso motivo per cui ModalDialog.vue lo usa. Si monta al
 * primo tap e da lì in poi resta nel DOM: chiuderlo lo nasconde (nativo,
 * via l'attributo `open`), non lo smonta — altrimenti la conversazione si
 * perderebbe ogni volta che si guarda il tavolo e si riapre la chat.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  chat: ChatAvailability
  suggestedQuestions: Record<ChatAgent, string[]>
  hasManual: boolean
}>()

const SIDEBAR_MIN_WIDTH = '(min-width: 1100px)'

const route = useRoute()
const wide = ref(false)
const dialogMounted = ref(false)
const dialog = ref<HTMLDialogElement | null>(null)
const sheet = ref<HTMLDialogElement | null>(null)
const panel = ref<InstanceType<typeof ManualChatPanel> | null>(null)
const expanded = ref(false)

let media: MediaQueryList | null = null
function syncWide(e: MediaQueryList | MediaQueryListEvent) {
  // Scendendo sotto i 1100px la barra sparisce (v-if) con la modale
  // dentro: `expanded` va azzerato, o risalendo la barra rinascerebbe
  // "ingrandita" senza esserlo davvero.
  if (!e.matches) {
    expanded.value = false
  }
  wide.value = e.matches
}

// vue-router restituisce un array quando lo stesso parametro compare più
// volte (?chat=1&chat=1): tollerarlo evita che quel caso limite faccia
// silenziosamente niente.
function chatQueryRequested(): boolean {
  const value = route.query.chat
  return Array.isArray(value) ? value.includes('1') : value === '1'
}

onMounted(() => {
  media = window.matchMedia(SIDEBAR_MIN_WIDTH)
  syncWide(media)
  media.addEventListener('change', syncWide)

  // ?chat=1 arriva dai rimandi della pagina prenotazione e della scheda
  // evento: apre la chat senza far cercare all'utente dove sia. Da 1100px
  // in su la sidebar è già aperta di suo; serve solo sotto, per aprire il
  // dialog.
  if (chatQueryRequested() && !wide.value) {
    openDialog()
  }
})

onBeforeUnmount(() => media?.removeEventListener('change', syncWide))

function openDialog() {
  dialogMounted.value = true
  // showModal va chiamato dopo che il <dialog> è nel DOM (al primo tap) o
  // comunque raggiungibile via ref (alle riaperture successive, quando è
  // già montato e semplicemente chiuso).
  requestAnimationFrame(() => dialog.value?.showModal())
}

// Il <dialog> della barra nasce aperto non modale: `show()` e non
// l'attributo `open` nel template, perché passare fra i due modi richiede
// close() + show()/showModal(), e un `open` legato da Vue lo rimetterebbe
// a ogni patch.
//
// `show()` però, come `showModal()`, mette il fuoco sul primo elemento
// focalizzabile del dialog: al caricamento della scheda il bottone per
// ingrandire risultava già selezionato (segnalato dall'utente). La barra
// si apre da sola, non per un gesto: il fuoco torna dov'era, o si toglie
// se era sul body.
watch(sheet, (el) => {
  if (el && !el.open) {
    const previous = document.activeElement
    el.show()
    if (previous instanceof HTMLElement && previous !== document.body) {
      previous.focus({ preventScroll: true })
    } else if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur()
    }
  }
})

function setExpanded(value: boolean) {
  const el = sheet.value
  if (!el || value === expanded.value) {
    return
  }
  el.close()
  if (value) {
    el.showModal()
  } else {
    el.show()
  }
  expanded.value = value
  // showModal() mette il fuoco sul primo elemento focalizzabile (il ＋ della
  // testata): nella modale si è lì per scrivere, e il fuoco va sul campo.
  // Riducendo torna sul bottone che ha ingrandito, come ogni modale.
  nextTick(() => (value ? panel.value?.focusComposer() : panel.value?.focusExpand()))
}

// Esc nella modale riduce invece di chiudere: chiudere il <dialog> della
// barra la lascerebbe vuota.
function onSheetCancel(e: Event) {
  e.preventDefault()
  setExpanded(false)
}

// Un clic sullo sfondo (il target è il <dialog> stesso, non il pannello
// che lo riempie) riduce, come in ogni modale. Conta anche dove è partito:
// una selezione di testo iniziata nel pannello e rilasciata fuori genera
// un click sul <dialog>, e ridurre lì butterebbe via la selezione.
let pressedOnBackdrop = false
function onSheetPointerdown(e: PointerEvent) {
  pressedOnBackdrop = e.target === sheet.value
}
function onSheetClick(e: MouseEvent) {
  if (expanded.value && pressedOnBackdrop && e.target === sheet.value) {
    setExpanded(false)
  }
}

function closeDialog() {
  if (dialog.value?.open) {
    dialog.value.close()
  }
}
</script>

<template>
  <!--
    Desktop: sidebar sticky, sempre aperta, a tutta altezza come
    `.app-sidebar` a sinistra. Niente toggle: non collassa più.

    L'aria-label c'è perché un <aside> è una region di landmark: senza nome
    compare nell'elenco dei landmark di uno screen reader come
    "complementary" e basta, e da lì la chat non si trova.
  -->
  <aside v-if="wide" class="manual-chat-aside" aria-label="Il Mentore — chiedi regole e consigli">
    <dialog
      ref="sheet"
      class="manual-chat-sheet"
      :class="{ 'is-expanded': expanded }"
      aria-label="Il Mentore — chiedi regole e consigli"
      @cancel="onSheetCancel"
      @pointerdown="onSheetPointerdown"
      @click="onSheetClick"
    >
      <ManualChatPanel
        ref="panel"
        :game-id="gameId"
        :game-name="gameName"
        :chat="chat"
        :suggested-questions="suggestedQuestions"
        :has-manual="hasManual"
        expandable
        :expanded="expanded"
        @toggle-expand="setExpanded(!expanded)"
      />
    </dialog>
  </aside>

  <!-- Mobile: bottone tondo sempre visibile, e dialog a tutto schermo. -->
  <template v-else>
    <button type="button" class="manual-chat-fab" @click="openDialog">
      <!--
        Il dado che parla: la stessa sagoma tonda del segnaposto copertina
        (rect rx, pip pieni) con la codina di un fumetto, e i pip disposti in
        diagonale come la faccia 3 di un dado — la diagonale è la firma del
        dado, tre puntini in fila sarebbero il glifo "chat" di chiunque.
        Sostituisce il segnaposto precedente, una freccia circolare che si
        leggeva "ricarica".
      -->
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <rect x="2.6" y="3.6" width="18.8" height="13.4" rx="3.4" stroke="currentColor" stroke-width="1.7" />
        <path d="M8.4 17v3.6L12.9 17" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round" />
        <circle cx="7.7" cy="7.6" r="1.25" fill="currentColor" />
        <circle cx="12" cy="10.3" r="1.25" fill="currentColor" />
        <circle cx="16.3" cy="13" r="1.25" fill="currentColor" />
      </svg>
      Chiedi al Mentore
    </button>

    <!--
      v-if su dialogMounted, non su "è aperto": una volta montato al primo
      tap resta nel DOM. Chiuderlo (Esc, che il <dialog> nativo gestisce
      da sé, o il × che chiama closeDialog()) lo nasconde nativamente senza
      smontare ManualChatPanel, così una conversazione in corso sopravvive
      a "chiudo per guardare il tavolo, riapro dopo".
    -->
    <dialog
      v-if="dialogMounted"
      ref="dialog"
      class="manual-chat-dialog"
      aria-label="Il Mentore — chiedi regole e consigli"
    >
      <!--
        La testata (titolo, nuova conversazione, ×) sta dentro il pannello:
        `closable` accende la × e l'evento la ricollega a closeDialog. Prima
        era markup del dialog, e la sidebar desktop — che monta lo stesso
        pannello — restava senza titolo.
      -->
      <ManualChatPanel
        :game-id="gameId"
        :game-name="gameName"
        :chat="chat"
        :suggested-questions="suggestedQuestions"
        :has-manual="hasManual"
        closable
        @close="closeDialog"
      />
    </dialog>
  </template>
</template>
