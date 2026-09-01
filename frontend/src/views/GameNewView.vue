<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface BGGSearchResult {
  bggId: string
  name: string
  year: number
}

const router = useRouter()
const mode = ref<'search' | 'bgg-import' | 'manual'>('search')

const query = ref('')
const results = ref<BGGSearchResult[]>([])
const searchError = ref('')

const selectedBggId = ref('')
const selectedName = ref('')
const languageCode = ref('it')
const owner = ref('')

const manualName = ref('')
const manualYear = ref<number | null>(null)
const manualMinPlayers = ref<number | null>(null)
const manualMaxPlayers = ref<number | null>(null)
const manualPlaytime = ref<number | null>(null)

const createError = ref('')

async function search() {
  searchError.value = ''
  try {
    results.value = await api.get<BGGSearchResult[]>(`/games/search?q=${encodeURIComponent(query.value)}`)
  } catch (e) {
    searchError.value = (e as Error).message
  }
}

function selectResult(r: BGGSearchResult) {
  selectedBggId.value = r.bggId
  selectedName.value = r.name
  mode.value = 'bgg-import'
}

function startManual() {
  mode.value = 'manual'
}

async function createFromBGG() {
  createError.value = ''
  try {
    const game = await api.post<{ id: number }>('/games', {
      bggId: selectedBggId.value,
      languageCode: languageCode.value,
      owner: owner.value,
    })
    router.push(`/games/${game.id}`)
  } catch (e) {
    createError.value = (e as Error).message
  }
}

async function createManual() {
  createError.value = ''
  try {
    const game = await api.post<{ id: number }>('/games', {
      name: manualName.value,
      year: manualYear.value,
      minPlayers: manualMinPlayers.value,
      maxPlayers: manualMaxPlayers.value,
      playtimeMinutes: manualPlaytime.value,
      owner: owner.value,
      languageCode: languageCode.value,
      nameTranslated: manualName.value,
    })
    router.push(`/games/${game.id}`)
  } catch (e) {
    createError.value = (e as Error).message
  }
}
</script>

<template>
  <div>
    <h1>Aggiungi gioco</h1>

    <div v-if="mode === 'search'">
      <form @submit.prevent="search">
        <label>
          Cerca su BoardGameGeek
          <input v-model="query" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="searchError" class="error">{{ searchError }}</p>
      <ul>
        <li v-for="r in results" :key="r.bggId">
          <button type="button" @click="selectResult(r)">{{ r.name }} ({{ r.year }})</button>
        </li>
      </ul>
      <button type="button" @click="startManual">Crea manualmente</button>
    </div>

    <div v-if="mode === 'bgg-import'">
      <h2>{{ selectedName }}</h2>
      <form @submit.prevent="createFromBGG">
        <label>
          Lingua base
          <select v-model="languageCode">
            <option value="it">Italiano</option>
            <option value="en">Inglese</option>
          </select>
        </label>
        <label>
          Proprietario
          <input v-model="owner" />
        </label>
        <button type="submit">Importa</button>
      </form>
      <p v-if="createError" class="error">{{ createError }}</p>
    </div>

    <div v-if="mode === 'manual'">
      <form @submit.prevent="createManual">
        <label>
          Nome
          <input v-model="manualName" required />
        </label>
        <label>
          Anno
          <input v-model.number="manualYear" type="number" />
        </label>
        <label>
          Min giocatori
          <input v-model.number="manualMinPlayers" type="number" />
        </label>
        <label>
          Max giocatori
          <input v-model.number="manualMaxPlayers" type="number" />
        </label>
        <label>
          Durata (minuti)
          <input v-model.number="manualPlaytime" type="number" />
        </label>
        <label>
          Lingua base
          <select v-model="languageCode">
            <option value="it">Italiano</option>
            <option value="en">Inglese</option>
          </select>
        </label>
        <label>
          Proprietario
          <input v-model="owner" />
        </label>
        <button type="submit">Crea</button>
      </form>
      <p v-if="createError" class="error">{{ createError }}</p>
    </div>
  </div>
</template>
