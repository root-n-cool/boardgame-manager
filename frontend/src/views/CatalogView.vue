<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import SelectFilter from '../components/SelectFilter.vue'
import GameDifficulty from '../components/GameDifficulty.vue'
import { DIFFICULTY_LEVELS, difficultyLevel, type DifficultyLevel } from '../utils/difficulty'
import { PLAYER_FILTER_OPTIONS, fitsPlayers, formatPlayers, normalizeName } from '../utils/gameFilters'

/**
 * Lo scaffale dell'associazione, aperto a tutti: cosa c'è da giocare, a
 * prescindere dalla serata. Legge la stessa rotta pubblica della vista
 * admin, che senza sessione non porta i campi di gestione. Il
 * proprietario non compare: è il nome di una persona, e a chi sfoglia il
 * catalogo non serve.
 */
interface CatalogGame {
  id: number
  name: string
  year: number | null
  minPlayers: number | null
  maxPlayers: number | null
  playtimeMinutes: number | null
  weight: number | null
  coverPath: string | null
}

const games = ref<CatalogGame[]>([])
const loaded = ref(false)
const error = ref('')

const query = ref('')
const players = ref<number | null>(null)
const difficulty = ref<DifficultyLevel | null>(null)

const difficultyOptions = DIFFICULTY_LEVELS.map((d) => ({ value: d, label: d }))

const filtering = computed(() => query.value.trim() !== '' || players.value !== null || difficulty.value !== null)

const visible = computed(() => {
  const q = normalizeName(query.value)
  return games.value.filter(
    (g) =>
      (q === '' || normalizeName(g.name).includes(q)) &&
      (players.value === null || fitsPlayers(g, players.value)) &&
      (difficulty.value === null || difficultyLevel(g.weight) === difficulty.value),
  )
})

const countLabel = computed(() => {
  const total = games.value.length
  if (filtering.value) {
    return `${visible.value.length} di ${total} giochi`
  }
  return total === 1 ? '1 gioco' : `${total} giochi`
})

/** La riga sotto il nome: "2–4 giocatori", "45 min", saltando ciò che manca. */
function factsOf(g: CatalogGame) {
  const players = formatPlayers(g)
  return [players && `${players} giocatori`, g.playtimeMinutes && `${g.playtimeMinutes} min`].filter(
    (f): f is string => !!f,
  )
}

function resetFilters() {
  query.value = ''
  players.value = null
  difficulty.value = null
}

onMounted(async () => {
  try {
    const list = await api.get<CatalogGame[]>('/games')
    games.value = [...list].sort((a, b) => a.name.localeCompare(b.name, 'it'))
  } catch (e) {
    console.error('caricamento catalogo', e)
    error.value = 'Il catalogo non è disponibile in questo momento. Riprova tra poco.'
  } finally {
    loaded.value = true
  }
})
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Catalogo giochi</h1>
        <p v-if="loaded && !error" class="page-meta">{{ countLabel }}</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <template v-else-if="loaded && games.length > 0">
      <div class="catalog-filters">
        <label class="catalog-search">
          <span class="visually-hidden">Cerca per nome</span>
          <input v-model="query" type="search" placeholder="Cerca per nome" autocomplete="off" />
        </label>
        <SelectFilter v-model="players" label="Giocatori" :options="PLAYER_FILTER_OPTIONS" />
        <SelectFilter v-model="difficulty" label="Difficoltà" :options="difficultyOptions" anyLabel="Tutte" />
      </div>

      <ul v-if="visible.length > 0" role="list" class="game-grid catalog-grid">
        <li v-for="g in visible" :key="g.id">
          <router-link :to="{ name: 'game-detail', params: { id: g.id } }">
            <img
              v-if="g.coverPath"
              :src="`/api/uploads/${g.coverPath}`"
              :alt="g.name"
              width="300"
              height="400"
              loading="lazy"
              decoding="async"
            />
            <div v-else class="cover-placeholder" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none">
                <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
                <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
                <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
                <circle cx="12" cy="12" r="1.3" fill="currentColor" />
                <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
                <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
              </svg>
            </div>
            <h2>{{ g.name }}</h2>
            <p v-if="factsOf(g).length" class="catalog-facts">
              <span v-for="f in factsOf(g)" :key="f">{{ f }}</span>
            </p>
            <GameDifficulty :weight="g.weight" />
          </router-link>
        </li>
      </ul>
      <div v-else class="filter-empty">
        <p class="empty-note">Nessun gioco corrisponde a questi filtri.</p>
        <button type="button" class="btn-secondary is-compact" @click="resetFilters">Azzera filtri</button>
      </div>
    </template>

    <p v-else-if="loaded" class="empty-note">Il catalogo è ancora vuoto.</p>
  </div>
</template>
