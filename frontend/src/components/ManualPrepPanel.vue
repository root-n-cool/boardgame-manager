<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api/client'

/**
 * Indicizza (o rimuove l'indice di) una fonte per la chat pubblica: una
 * sola richiesta legge il file — PDF, txt, md o docx — lo spezza in sezioni
 * cercabili e le salva. Niente più bozza da correggere a mano: qui si vede
 * solo quanto è stato indicizzato, non il testo.
 */
const props = defineProps<{
  gameId: number
  lang: string
  mediaId: number
  mediaTitle: string
  /** Chunk indicizzati adesso per questa fonte. 0 = non preparata. */
  indexedChunks: number
  /** Senza provider AI configurato la rotta di indicizzazione risponde 404:
   *  il bottone non compare affatto. */
  aiConfigured: boolean
}>()

// indexedChunks, sourceHeadings e canAsk vivono sul gioco intero: dopo ogni
// indicizzazione o rimozione il genitore li ricarica per tutte le fonti.
const emit = defineEmits<{ changed: [] }>()

const busyAction = ref<'prepare' | 'remove' | null>(null)
const busy = computed(() => busyAction.value !== null)
const error = ref('')
const notice = ref('')

const base = `/games/${props.gameId}/languages/${props.lang}/media/${props.mediaId}/index`

async function prepare() {
  busyAction.value = 'prepare'
  error.value = ''
  notice.value = ''
  try {
    const res = await api.post<{ reference: string; chunks: number }>(base)
    notice.value =
      `Manuale indicizzato: ${res.chunks} ${res.chunks === 1 ? 'sezione trovata' : 'sezioni trovate'}. ` +
      'Le domande sulla scheda pubblica ora funzionano.'
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
  <div class="manual-prep">
    <h3 class="manual-prep-title">{{ mediaTitle }}</h3>
    <p class="manual-prep-state">
      <template v-if="indexedChunks > 0">
        {{ indexedChunks === 1 ? '1 sezione indicizzata.' : `${indexedChunks} sezioni indicizzate.` }}
      </template>
      <template v-else>Non preparato per le domande.</template>
    </p>

    <!-- Senza provider il motivo è pratico, non tecnico: qui non c'è ancora
         una chat con cui indicizzare avrebbe a che fare. -->
    <p v-if="!aiConfigured" class="empty-note">
      Senza un provider AI configurato nelle impostazioni la chat con le domande non esiste:
      prepararla ora non servirebbe a niente.
    </p>

    <div class="manual-prep-actions">
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

    <!-- Una scansione passa per un modello di visione, una pagina per
         chiamata: la richiesta può durare minuti, va detto prima del click. -->
    <p v-if="aiConfigured" class="field-hint">
      Per un file di testo dura pochi secondi; per una scansione lunga può
      richiedere alcuni minuti, una pagina alla volta — resta su questa
      pagina finché non finisce.
    </p>

    <p class="visually-hidden" role="alert" aria-live="assertive">{{ error }}</p>
    <p class="visually-hidden" role="status" aria-live="polite">{{ notice }}</p>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="empty-note">{{ notice }}</p>
  </div>
</template>
