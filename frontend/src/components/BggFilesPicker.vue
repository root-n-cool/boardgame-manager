<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/client'
import { languageName } from '../utils/game'

/**
 * Indice dei file che BoardGameGeek ha per un gioco, dentro il modale che
 * carica un manuale.
 *
 * BGG tiene i manuali di quasi tutto, spesso tradotti, ma i file stanno
 * dietro il login del sito: scaricarli da qui non si può. Quello che si può
 * fare è togliere la parte noiosa, cioè cercarli. Una riga apre la sua
 * pagina su BGG in una scheda nuova — dove l'admin è già loggato — e
 * intanto scrive il titolo nel form accanto: al ritorno resta da scegliere
 * il PDF appena scaricato e salvare.
 *
 * Sta chiuso a fisarmonica (`<details>` nativo, come la modale è `<dialog>`
 * nativo): aperto si prendeva più di metà del foglio, e il controllo
 * primario — scegli il PDF — finiva compresso in cima. La riga di riepilogo
 * porta già il conteggio, così si sa se vale la pena aprirlo.
 */
export interface BggFile {
  title: string
  filename: string
  /** Il nome inglese con cui BGG etichetta il file: "Italian", "English". */
  language: string
  /** Il codice corrispondente nell'app, vuoto per le lingue fuori mappa. */
  languageCode: string
  positive: number
  sizeBytes: number
  pageUrl: string
}

const props = defineProps<{
  gameId: string
  bggId: string
  lang: string
}>()

const emit = defineEmits<{ pick: [file: BggFile] }>()

/** Il nome della lingua della scheda, per parlarne nelle note. */
const langName = computed(() => languageName(props.lang))

const files = ref<BggFile[]>([])
const languageFiltered = ref(true)
const loading = ref(false)
const error = ref('')

let inFlight: AbortController | undefined

async function load(allLanguages: boolean) {
  inFlight?.abort()
  const controller = new AbortController()
  inFlight = controller
  loading.value = true
  error.value = ''
  try {
    const params = new URLSearchParams({ lang: props.lang })
    if (allLanguages) {
      params.set('all', '1')
    }
    const res = await api.get<{ items: BggFile[]; languageFiltered: boolean }>(
      `/games/${props.gameId}/bgg-files?${params.toString()}`,
      { signal: controller.signal },
    )
    files.value = res.items
    languageFiltered.value = res.languageFiltered
  } catch (e) {
    if (controller.signal.aborted) {
      return
    }
    files.value = []
    // L'API risponde in inglese ("could not list BGG files") e qui parla
    // l'interfaccia: il nome del problema e la via d'uscita, che è il link
    // al sito già presente sotto la lista.
    error.value = 'BoardGameGeek non risponde. Riprova, o cerca il file sul sito.'
  } finally {
    if (!controller.signal.aborted) {
      loading.value = false
      inFlight = undefined
    }
  }
}

function choose(file: BggFile) {
  window.open(file.pageUrl, '_blank', 'noopener')
  emit('pick', file)
}

/** Il nome della lingua in italiano, come nel resto dell'interfaccia: BGG
 *  etichetta i suoi file in inglese, e un file senza lingua è "neutro". */
function fileLanguage(file: BggFile): string {
  if (file.languageCode) {
    return languageName(file.languageCode)
  }
  return file.language || 'neutro'
}

/** I file di BGG vanno dai 30KB di un riassunto ai 20MB di una mappa: la
 *  dimensione è il segnale più rapido di che cosa si sta per scaricare. */
function formatSize(bytes: number): string {
  if (bytes <= 0) {
    return ''
  }
  if (bytes < 1024 * 1024) {
    return `${Math.max(1, Math.round(bytes / 1024))} KB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(1).replace('.', ',')} MB`
}

onMounted(() => void load(false))
onBeforeUnmount(() => inFlight?.abort())
</script>

<template>
  <details class="bgg-files">
    <summary>
      <svg class="bgg-files-caret" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M7 10l5 5 5-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      <span class="bgg-files-summary">
        <span>Cerca il manuale su BoardGameGeek</span>
        <span class="bgg-files-count" role="status" aria-live="polite">
          <template v-if="loading">ricerca in corso…</template>
          <template v-else-if="error"><span class="error-text">{{ error }}</span></template>
          <template v-else-if="files.length === 0 && languageFiltered">
            nessun file in {{ langName }}
          </template>
          <template v-else-if="files.length === 0">nessun file</template>
          <template v-else-if="languageFiltered">
            {{ files.length === 1 ? '1 file' : `${files.length} file` }} in {{ langName }}
          </template>
          <template v-else>
            {{ files.length === 1 ? '1 file' : `${files.length} file` }} in tutte le lingue
          </template>
        </span>
      </span>
    </summary>

    <p class="field-hint">
      Il download chiede il login: la riga apre la pagina del file su BGG in una scheda nuova
      e compila il titolo qui sopra.
    </p>

    <ul v-if="files.length > 0" class="bgg-file-list">
      <li v-for="file in files" :key="file.pageUrl">
        <button type="button" class="bgg-file" @click="choose(file)">
          <span class="bgg-file-text">
            <span class="bgg-name">{{ file.title }}</span>
            <span class="bgg-meta">
              <span class="bgg-filename">{{ file.filename }}</span>
              <template v-if="formatSize(file.sizeBytes)">
                <span class="divider" aria-hidden="true">·</span>
                <span>{{ formatSize(file.sizeBytes) }}</span>
              </template>
              <template v-if="file.positive > 0">
                <span class="divider" aria-hidden="true">·</span>
                <span>{{ file.positive }} voti</span>
              </template>
            </span>
          </span>
          <span v-if="!languageFiltered" class="lang-chip">{{ fileLanguage(file) }}</span>
        </button>
      </li>
    </ul>

    <p v-if="!loading" class="field-hint bgg-files-actions">
      <button v-if="languageFiltered" type="button" class="link-button" @click="load(true)">
        Mostra le altre lingue
      </button>
      <a
        :href="`https://boardgamegeek.com/boardgame/${bggId}/files`"
        target="_blank"
        rel="noopener noreferrer"
      >
        Apri la sezione Files su BGG
      </a>
    </p>
  </details>
</template>
