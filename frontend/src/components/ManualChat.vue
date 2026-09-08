<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import ManualChatPanel from './ManualChatPanel.vue'

/**
 * Decide DOVE vive la chat, non cos'è.
 *
 * Da 1100px in su è una sidebar destra collassabile, sticky, col suo
 * scroll: la colonna di sinistra scorre e la chat resta ferma. La soglia è
 * 1100 e non 900 perché `.app-page` è larga 56rem: una sidebar da 22rem
 * dentro quello spazio lascerebbe al testo 34rem, sotto la misura
 * leggibile.
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
  headings: string[]
}>()

const SIDEBAR_MIN_WIDTH = '(min-width: 1100px)'
const COLLAPSED_KEY = 'manual-chat-collapsed'

const route = useRoute()
const wide = ref(false)
const collapsed = ref(false)
const dialogMounted = ref(false)
const dialog = ref<HTMLDialogElement | null>(null)

let media: MediaQueryList | null = null
function syncWide(e: MediaQueryList | MediaQueryListEvent) {
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

  // Lo stato collassato è una preferenza dell'utente, non del gioco.
  try {
    collapsed.value = window.localStorage.getItem(COLLAPSED_KEY) === '1'
  } catch {
    // Finestra privata o storage bloccato: si parte aperta, che è il
    // default giusto per la scoperta.
  }

  // ?chat=1 arriva dai rimandi della pagina prenotazione e della scheda
  // evento: apre la chat senza far cercare all'utente dove sia.
  if (chatQueryRequested()) {
    collapsed.value = false
    if (!wide.value) {
      openDialog()
    }
  }
})

onBeforeUnmount(() => media?.removeEventListener('change', syncWide))

watch(collapsed, (value) => {
  try {
    window.localStorage.setItem(COLLAPSED_KEY, value ? '1' : '0')
  } catch {
    // Niente da fare: la preferenza vale solo per questa visita.
  }
})

function openDialog() {
  dialogMounted.value = true
  // showModal va chiamato dopo che il <dialog> è nel DOM (al primo tap) o
  // comunque raggiungibile via ref (alle riaperture successive, quando è
  // già montato e semplicemente chiuso).
  requestAnimationFrame(() => dialog.value?.showModal())
}

function closeDialog() {
  if (dialog.value?.open) {
    dialog.value.close()
  }
}
</script>

<template>
  <!-- Desktop: sidebar sticky, collassabile in una barra verticale. -->
  <aside v-if="wide" class="manual-chat-aside" :class="{ 'is-collapsed': collapsed }">
    <!--
      Niente aria-label dinamico: il testo visibile è sempre "Chiedi al
      manuale" e un nome accessibile diverso da quel testo viola il
      criterio WCAG 2.5.3 (Label in Name — chi usa il controllo vocale
      pronuncia quello che vede). aria-expanded basta a comunicare lo stato.
    -->
    <button
      type="button"
      class="manual-chat-toggle"
      :aria-expanded="!collapsed"
      @click="collapsed = !collapsed"
    >
      <span class="manual-chat-toggle-label">Chiedi al manuale</span>
      <span class="manual-chat-toggle-icon" aria-hidden="true">{{ collapsed ? '‹' : '›' }}</span>
    </button>
    <!--
      v-show e non v-if: collassare non deve smontare il pannello,
      altrimenti la conversazione in corso si perde a ogni clic.
    -->
    <div v-show="!collapsed" class="manual-chat-aside-body">
      <ManualChatPanel :game-id="gameId" :game-name="gameName" :headings="headings" />
    </div>
  </aside>

  <!-- Mobile: bottone tondo sempre visibile, e dialog a tutto schermo. -->
  <template v-else>
    <button type="button" class="manual-chat-fab" @click="openDialog">
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          d="M20 12a8 8 0 1 1-3.2-6.4M20 4v4h-4"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
        />
        <circle cx="9" cy="12" r="1.1" fill="currentColor" />
        <circle cx="12.5" cy="12" r="1.1" fill="currentColor" />
        <circle cx="16" cy="12" r="1.1" fill="currentColor" />
      </svg>
      Chiedi al manuale
    </button>

    <!--
      v-if su dialogMounted, non su "è aperto": una volta montato al primo
      tap resta nel DOM. Chiuderlo (Esc, ×, o il backdrop) lo nasconde
      nativamente senza smontare ManualChatPanel, così una conversazione
      in corso sopravvive a "chiudo per guardare il tavolo, riapro dopo".
    -->
    <dialog
      v-if="dialogMounted"
      ref="dialog"
      class="manual-chat-dialog"
      aria-labelledby="manual-chat-dialog-title"
    >
      <div class="manual-chat-dialog-head">
        <h2 id="manual-chat-dialog-title">Chiedi al manuale</h2>
        <button type="button" class="modal-close" aria-label="Chiudi" @click="closeDialog">
          <svg viewBox="0 0 24 24" fill="none">
            <path d="M6 6l12 12M18 6L6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </div>
      <ManualChatPanel
        :game-id="gameId"
        :game-name="gameName"
        :headings="headings"
        @close="closeDialog"
      />
    </dialog>
  </template>
</template>
