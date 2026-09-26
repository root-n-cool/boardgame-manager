<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api/client'
import { fileExtensionLabel } from '../utils/game'

/**
 * Una riga della card «Knowledge base» (gruppo «Chatbot», admin): indicizza
 * (o rimuove l'indice di) una fonte per la chat pubblica — una sola
 * richiesta legge il file —
 * PDF, txt, md, docx o la foto di una pagina — lo spezza in sezioni
 * cercabili e le salva. Niente
 * più bozza da correggere a mano: qui si vede solo quanto è stato
 * indicizzato, non il testo.
 *
 * L'avviso sui tempi lunghi e il motivo per cui manca il bottone senza
 * provider AI stanno una volta sola a livello di sezione
 * (`GameAdminDetailView.vue`), non qui: con più righe si ripetevano
 * identici per ogni file.
 */
const props = defineProps<{
  gameId: number
  lang: string
  mediaId: number
  mediaTitle: string
  mediaUrl: string
  /** Chunk indicizzati adesso per questa fonte. 0 = non preparata. */
  indexedChunks: number
  /** Senza provider AI configurato la rotta di indicizzazione risponde 404:
   *  il bottone non compare affatto. */
  aiConfigured: boolean
}>()

// indexedChunks, suggestedQuestions e chat vivono sul gioco intero: dopo
// ogni indicizzazione o rimozione il genitore li ricarica per tutte le fonti.
const emit = defineEmits<{ changed: [] }>()

const busyAction = ref<'prepare' | 'remove' | null>(null)
const busy = computed(() => busyAction.value !== null)
const error = ref('')
const notice = ref('')
// Vero solo per l'ultimo avviso di successo *parziale* (pagine saltate):
// decide se `notice` prende il trattamento oro invece di quello neutro.
const partial = ref(false)

const formatLabel = computed(() => fileExtensionLabel(props.mediaUrl))
const stateLabel = computed(() =>
  props.indexedChunks > 0
    ? props.indexedChunks === 1
      ? '1 sezione indicizzata'
      : `${props.indexedChunks} sezioni indicizzate`
    : 'Non preparato',
)

const base = `/games/${props.gameId}/languages/${props.lang}/media/${props.mediaId}/index`

async function prepare() {
  busyAction.value = 'prepare'
  error.value = ''
  notice.value = ''
  partial.value = false
  try {
    const res = await api.post<{
      reference: string
      chunks: number
      // Additivi e presenti solo quando qualche pagina di uno scansionato è
      // stata saltata per un errore di trascrizione: un'indicizzazione
      // riuscita ma incompleta non deve leggersi come una piena, altrimenti
      // la chat risponde con sicurezza da un manuale a cui mancano pagine
      // senza che nessuno lo sappia.
      pagesIndexed?: number
      pagesSkipped?: number
    }>(base)
    const chunkLabel = res.chunks === 1 ? 'sezione trovata' : 'sezioni trovate'
    if (res.pagesSkipped) {
      partial.value = true
      const pageLabel = res.pagesIndexed === 1 ? 'pagina letta' : 'pagine lette'
      const skipLabel = res.pagesSkipped === 1 ? 'pagina saltata' : 'pagine saltate'
      notice.value =
        `Indicizzazione parziale: ${res.chunks} ${chunkLabel} da ${res.pagesIndexed} ${pageLabel}, ` +
        `${res.pagesSkipped} ${skipLabel} per un errore di lettura. Le domande funzionano già, ma il ` +
        'manuale è incompleto: prepara di nuovo più tardi per recuperare le pagine mancanti.'
    } else {
      notice.value =
        `Manuale indicizzato: ${res.chunks} ${chunkLabel}. ` +
        'Le domande sulla scheda pubblica ora funzionano.'
    }
    emit('changed')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Preparazione non riuscita.'
  } finally {
    busyAction.value = null
  }
}

async function remove() {
  // Ancora un'azione distruttiva, anche senza una bozza da perdere: toglie
  // l'unica cosa che tiene in piedi la chat pubblica per questo gioco.
  if (
    !window.confirm(
      `Rimuovere l'indice di "${props.mediaTitle}"? Le domande su questo gioco smettono di funzionare finché non lo prepari di nuovo.`,
    )
  ) {
    return
  }
  busyAction.value = 'remove'
  error.value = ''
  notice.value = ''
  partial.value = false
  try {
    await api.delete(base)
    notice.value = 'Indice rimosso: la chat non compare più per questo gioco.'
    emit('changed')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Rimozione non riuscita.'
  } finally {
    busyAction.value = null
  }
}
</script>

<template>
  <div class="admin-row manual-doc-row">
    <span class="manual-doc-title">{{ mediaTitle }}</span>
    <span class="lang-chip">{{ formatLabel }}</span>
    <!-- La lingua sta sulla RIGA e non sulla testata della sezione: la
         sezione elenca le fonti di tutte le lingue, perche' la chat cerca
         per gioco e non per lingua. Senza questa chip due manuali intitolati
         entrambi "Regolamento", uno IT e uno EN, sarebbero due righe
         identiche. Il server fa la stessa distinzione da sempre: quando due
         fonti condividono il titolo, `sourceReference` disambigua la
         citazione aggiungendo la lingua. -->
    <span class="lang-chip">{{ lang }}</span>
    <span class="manual-doc-state">{{ stateLabel }}</span>

    <div class="admin-row-actions">
      <button
        v-if="aiConfigured"
        type="button"
        :class="{ 'btn-secondary': indexedChunks > 0 }"
        :disabled="busy"
        @click="prepare"
      >
        {{
          busyAction === 'prepare'
            ? 'Lettura in corso…'
            : indexedChunks > 0
              ? 'Prepara di nuovo'
              : 'Prepara per le domande'
        }}
      </button>
      <button v-if="indexedChunks > 0" type="button" class="btn-danger" :disabled="busy" @click="remove">
        {{ busyAction === 'remove' ? 'Rimozione…' : 'Rimuovi indice' }}
      </button>
    </div>

    <p class="visually-hidden" role="alert" aria-live="assertive">{{ error }}</p>
    <p class="visually-hidden" role="status" aria-live="polite">{{ notice }}</p>
    <p v-if="error" class="manual-doc-message error">{{ error }}</p>
    <p v-else-if="notice" class="manual-doc-message" :class="partial ? 'manual-doc-partial' : 'empty-note'">
      {{ notice }}
    </p>
  </div>
</template>
