<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { api } from '../api/client'

/**
 * Scelta di un gioco su BoardGameGeek. La ricerca parte da sé mentre si
 * scrive: il bottone "Cerca" era un passaggio in più per un'operazione che
 * si ripete finché il nome giusto non compare.
 *
 * Due limiti tengono a bada le chiamate a BGG, che non ci appartiene: si
 * parte da tre caratteri (sotto, i risultati sono comunque centinaia) e si
 * aspetta una pausa nella digitazione. La richiesta precedente viene
 * annullata, così l'ultima risposta è sempre quella dell'ultima parola
 * scritta, non quella arrivata per ultima.
 */
export interface BggResult {
  bggId: string
  name: string
  year: number
  thumbnailUrl: string | null
  weight: number | null
}

const MIN_QUERY = 3
const DEBOUNCE_MS = 350

const model = defineModel<BggResult | null>({ required: true })

const baseId = useId()
const listboxId = `${baseId}-listbox`
const optionId = (index: number) => `${baseId}-option-${index}`

const input = ref<HTMLInputElement | null>(null)
const list = ref<HTMLUListElement | null>(null)
const query = ref('')
const results = ref<BggResult[]>([])
const loading = ref(false)
const error = ref('')
const open = ref(false)
const activeIndex = ref(-1)
// Miniature che il browser non è riuscito a caricare: la riga torna al
// segnaposto invece di mostrare l'icona di immagine rotta.
const brokenThumbs = ref(new Set<string>())

let debounceTimer: ReturnType<typeof setTimeout> | undefined
let inFlight: AbortController | undefined

function cancelPending() {
  clearTimeout(debounceTimer)
  inFlight?.abort()
  inFlight = undefined
}

watch(query, (value) => {
  cancelPending()
  error.value = ''
  const needle = value.trim()
  if (needle.length < MIN_QUERY) {
    results.value = []
    loading.value = false
    open.value = false
    return
  }
  // Il caricamento parte subito, non alla fine dell'attesa: chi scrive vede
  // che qualcosa sta succedendo anche durante la pausa.
  loading.value = true
  open.value = true
  debounceTimer = setTimeout(() => void search(needle), DEBOUNCE_MS)
})

async function search(needle: string) {
  const controller = new AbortController()
  inFlight = controller
  try {
    const found = await api.get<BggResult[]>(`/games/search?q=${encodeURIComponent(needle)}`, {
      signal: controller.signal,
    })
    results.value = found
    activeIndex.value = found.length > 0 ? 0 : -1
    open.value = true
  } catch (e) {
    if (controller.signal.aborted) {
      return
    }
    results.value = []
    activeIndex.value = -1
    error.value = (e as Error).message
  } finally {
    if (!controller.signal.aborted) {
      loading.value = false
      inFlight = undefined
    }
  }
}

function choose(result: BggResult) {
  cancelPending()
  model.value = result
  open.value = false
  query.value = ''
  results.value = []
  loading.value = false
}

async function clearChoice() {
  model.value = null
  await nextTick()
  input.value?.focus()
}

function move(step: number) {
  if (results.value.length === 0) {
    return
  }
  open.value = true
  const last = results.value.length - 1
  const next = activeIndex.value + step
  activeIndex.value = next < 0 ? last : next > last ? 0 : next
}

function onEnter(event: KeyboardEvent) {
  const active = results.value[activeIndex.value]
  if (!open.value || !active) {
    // Invio con la lista chiusa non deve inviare il form: qui si sta ancora
    // scegliendo il gioco, e il form non ha nulla da salvare.
    event.preventDefault()
    return
  }
  event.preventDefault()
  choose(active)
}

function onEscape() {
  cancelPending()
  loading.value = false
  // Primo Esc: chiude la lista. Secondo: svuota il campo — come ci si
  // aspetta da un campo di ricerca, e senza dover tenere premuto backspace.
  if (open.value) {
    open.value = false
    return
  }
  query.value = ''
}

function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && (event.currentTarget as HTMLElement).contains(next)) {
    return
  }
  open.value = false
}

function formatWeight(weight: number) {
  return weight.toFixed(1).replace('.', ',')
}

// La lista è più lunga di quanto se ne veda: senza questo, dalla sesta riga
// in giù le frecce spostano una selezione che è fuori dalla finestra.
watch(activeIndex, async () => {
  await nextTick()
  list.value?.querySelector('.is-active')?.scrollIntoView({ block: 'nearest' })
})

onBeforeUnmount(cancelPending)
</script>

<template>
  <div class="bgg-select">
    <div v-if="model" class="bgg-chosen">
      <span class="bgg-thumb">
        <img
          v-if="model.thumbnailUrl && !brokenThumbs.has(model.bggId)"
          :src="model.thumbnailUrl"
          alt=""
          width="48"
          height="48"
          loading="lazy"
          decoding="async"
          referrerpolicy="no-referrer"
          @error="brokenThumbs.add(model.bggId)"
        />
        <svg v-else viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
          <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
          <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
        </svg>
      </span>
      <span class="bgg-chosen-text">
        <span class="bgg-name">{{ model.name }}</span>
        <span class="bgg-meta">
          <span>{{ model.year || 'anno sconosciuto' }}</span>
          <span v-if="model.weight" class="divider" aria-hidden="true">·</span>
          <span v-if="model.weight">Complessità {{ formatWeight(model.weight) }}/5</span>
        </span>
      </span>
      <button type="button" class="btn-secondary is-compact" @click="clearChoice">Cambia</button>
    </div>

    <div v-else class="bgg-combobox" @focusout="onFocusOut">
      <label :for="`${baseId}-input`" class="field-label">Cerca su BoardGameGeek</label>
      <input
        :id="`${baseId}-input`"
        ref="input"
        v-model="query"
        type="text"
        role="combobox"
        autocomplete="off"
        placeholder="Nome del gioco"
        :aria-expanded="open"
        :aria-controls="listboxId"
        aria-autocomplete="list"
        :aria-activedescendant="open && activeIndex >= 0 ? optionId(activeIndex) : undefined"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.enter="onEnter"
        @keydown.esc="onEscape"
      />

      <ul
        v-show="open && results.length > 0"
        :id="listboxId"
        ref="list"
        class="bgg-results"
        role="listbox"
        aria-label="Risultati da BoardGameGeek"
      >
        <li
          v-for="(r, i) in results"
          :id="optionId(i)"
          :key="r.bggId"
          class="bgg-result"
          :class="{ 'is-active': i === activeIndex }"
          role="option"
          :aria-selected="i === activeIndex"
          @mousedown.prevent="choose(r)"
          @mousemove="activeIndex = i"
        >
          <span class="bgg-thumb">
            <img
              v-if="r.thumbnailUrl && !brokenThumbs.has(r.bggId)"
              :src="r.thumbnailUrl"
              alt=""
              width="48"
              height="48"
              loading="lazy"
              decoding="async"
              referrerpolicy="no-referrer"
              @error="brokenThumbs.add(r.bggId)"
            />
            <svg v-else viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
              <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
              <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
            </svg>
          </span>
          <span class="bgg-result-text">
            <span class="bgg-name">{{ r.name }}</span>
            <span class="bgg-meta">
              <span v-if="r.year">{{ r.year }}</span>
              <span v-else>anno sconosciuto</span>
            </span>
          </span>
          <span v-if="r.weight" class="bgg-weight">
            <span class="visually-hidden">Complessità </span>{{ formatWeight(r.weight) }}<span
              class="visually-hidden"
            >
              su 5</span>
          </span>
        </li>
      </ul>

      <p class="field-hint" role="status" aria-live="polite">
        <template v-if="error">
          <span class="error-text">{{ error }}</span>
        </template>
        <template v-else-if="loading">Ricerca su BoardGameGeek…</template>
        <template v-else-if="query.trim().length >= MIN_QUERY && results.length === 0">
          Nessun gioco trovato per “{{ query.trim() }}”.
        </template>
        <template v-else-if="results.length > 0">
          {{ results.length === 1 ? '1 risultato' : `${results.length} risultati` }}, i più
          pertinenti per primi.
        </template>
        <template v-else>Scrivi almeno {{ MIN_QUERY }} caratteri: la ricerca parte da sé.</template>
      </p>
    </div>
  </div>
</template>
