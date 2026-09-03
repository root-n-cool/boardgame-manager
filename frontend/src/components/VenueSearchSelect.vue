<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { api } from '../api/client'

/**
 * Il luogo di una serata: si cerca su OpenStreetMap e si sceglie da un
 * elenco, come si sceglie un gioco su BoardGameGeek. Chi non trova il posto
 * — un circolo che sulla mappa non c'è, una sede senza numero civico — lo
 * scrive a mano: l'indirizzo resta valido lo stesso, semplicemente
 * sull'evento non comparirà la mappa.
 *
 * Il nome resta un campo a sé perché quello che una ricerca restituisce è
 * un indirizzo, non l'insegna: "Circolo Arci" lo sa l'organizzatore.
 */
export interface Venue {
  name: string
  address: string
  lat: number | null
  lon: number | null
}

interface PlaceResult {
  name: string
  address: string
  lat: number
  lon: number
}

const MIN_QUERY = 3
const DEBOUNCE_MS = 400

const model = defineModel<Venue | null>({ required: true })

const baseId = useId()
const listboxId = `${baseId}-listbox`
const optionId = (index: number) => `${baseId}-option-${index}`

const input = ref<HTMLInputElement | null>(null)
const list = ref<HTMLUListElement | null>(null)
const query = ref('')
const results = ref<PlaceResult[]>([])
const loading = ref(false)
const error = ref('')
const open = ref(false)
const activeIndex = ref(-1)

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
  loading.value = true
  open.value = true
  debounceTimer = setTimeout(() => void search(needle), DEBOUNCE_MS)
})

async function search(needle: string) {
  const controller = new AbortController()
  inFlight = controller
  try {
    const found = await api.get<PlaceResult[]>(`/geocode/search?q=${encodeURIComponent(needle)}`, {
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

function choose(place: PlaceResult) {
  cancelPending()
  model.value = {
    // Il nome scelto dall'admin ha la precedenza: si sta cambiando
    // indirizzo, non l'insegna del posto. Quello di OpenStreetMap si
    // eredita solo se è davvero un'insegna: per una via qualsiasi è la
    // via stessa, e ripeterla sopra l'indirizzo non dice niente.
    name: model.value?.name || suggestedName(place),
    address: place.address,
    lat: place.lat,
    lon: place.lon,
  }
  open.value = false
  query.value = ''
  results.value = []
  loading.value = false
}

function suggestedName(place: PlaceResult) {
  return place.name && !place.address.startsWith(place.name) ? place.name : ''
}

/** L'indirizzo che la ricerca non ha trovato, scritto a mano. */
function useTypedAddress() {
  const typed = query.value.trim()
  if (!typed) {
    return
  }
  cancelPending()
  model.value = { name: model.value?.name ?? '', address: typed, lat: null, lon: null }
  open.value = false
  query.value = ''
  results.value = []
  loading.value = false
}

function updateName(name: string) {
  if (!model.value) {
    return
  }
  model.value = { ...model.value, name }
}

async function clearVenue() {
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
  // Invio non deve mai inviare il form da qui: si sta scegliendo il luogo.
  event.preventDefault()
  const active = results.value[activeIndex.value]
  if (open.value && active) {
    choose(active)
    return
  }
  useTypedAddress()
}

function onEscape() {
  cancelPending()
  loading.value = false
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

/**
 * Una riga della lista si legge in due tempi: prima cosa è quel posto —
 * l'insegna se ce l'ha, altrimenti la via — e sotto dove si trova.
 */
function resultTitle(place: PlaceResult) {
  return suggestedName(place) || place.address.split(',')[0].trim()
}

function resultMeta(place: PlaceResult) {
  return suggestedName(place) ? place.address : place.address.split(',').slice(1).join(',').trim()
}

watch(activeIndex, async () => {
  await nextTick()
  list.value?.querySelector('.is-active')?.scrollIntoView({ block: 'nearest' })
})

onBeforeUnmount(cancelPending)
</script>

<template>
  <div class="venue-select">
    <div v-if="model" class="venue-chosen">
      <span class="venue-pin" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none">
          <path
            d="M12 21s7-5.3 7-11a7 7 0 1 0-14 0c0 5.7 7 11 7 11Z"
            stroke="currentColor"
            stroke-width="1.7"
            stroke-linejoin="round"
          />
          <circle cx="12" cy="10" r="2.4" stroke="currentColor" stroke-width="1.7" />
        </svg>
      </span>
      <span class="venue-chosen-text">
        <span class="venue-address">{{ model.address }}</span>
        <span class="venue-meta">
          <template v-if="model.lat !== null">Posizione trovata sulla mappa</template>
          <template v-else>Indirizzo scritto a mano: nessuna mappa sull'evento</template>
        </span>
      </span>
      <button type="button" class="btn-secondary is-compact" @click="clearVenue">Cambia</button>
    </div>

    <div v-else class="venue-combobox" @focusout="onFocusOut">
      <label :for="`${baseId}-input`" class="field-label">Cerca l'indirizzo</label>
      <input
        :id="`${baseId}-input`"
        ref="input"
        v-model="query"
        type="text"
        role="combobox"
        autocomplete="off"
        placeholder="Via, numero civico, città"
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
        class="venue-results"
        role="listbox"
        aria-label="Indirizzi trovati su OpenStreetMap"
      >
        <li
          v-for="(p, i) in results"
          :id="optionId(i)"
          :key="`${p.lat},${p.lon},${p.address}`"
          class="venue-result"
          :class="{ 'is-active': i === activeIndex }"
          role="option"
          :aria-selected="i === activeIndex"
          @mousedown.prevent="choose(p)"
          @mousemove="activeIndex = i"
        >
          <span class="venue-result-text">
            <span class="venue-line">{{ resultTitle(p) }}</span>
            <span class="venue-meta">{{ resultMeta(p) }}</span>
          </span>
        </li>
      </ul>

      <p class="field-hint" role="status" aria-live="polite">
        <template v-if="error">
          <span class="error-text">{{ error }}</span>
        </template>
        <template v-else-if="loading">Ricerca su OpenStreetMap…</template>
        <template v-else-if="query.trim().length >= MIN_QUERY && results.length === 0">
          Nessun indirizzo trovato per “{{ query.trim() }}”.
        </template>
        <template v-else-if="results.length > 0">
          {{ results.length === 1 ? '1 indirizzo trovato' : `${results.length} indirizzi trovati` }}.
        </template>
        <template v-else>Scrivi almeno {{ MIN_QUERY }} caratteri: la ricerca parte da sé.</template>
      </p>

      <button
        v-if="query.trim().length >= MIN_QUERY && !loading"
        type="button"
        class="btn-secondary is-compact venue-manual"
        @click="useTypedAddress"
      >
        Usa “{{ query.trim() }}” così com'è
      </button>
    </div>

    <label v-if="model" class="venue-name-field">
      <span>Nome del luogo <span class="field-optional">(opzionale)</span></span>
      <input
        :value="model.name"
        type="text"
        placeholder="Circolo Arci, Oratorio, Ludoteca…"
        @input="updateName(($event.target as HTMLInputElement).value)"
      />
    </label>
  </div>
</template>
