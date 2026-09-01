<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'

interface GameLanguageInfo {
  code: string
  isBaseLanguage: boolean
  name: string
  description: string | null
}

interface GameDetail {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
  languages: GameLanguageInfo[]
}

const route = useRoute()
const router = useRouter()
const gameId = route.params.id as string

const game = ref<GameDetail | null>(null)
const error = ref('')
const activeLangCode = ref('')

const editName = ref('')
const editDescription = ref('')
const saveMessage = ref('')

const newLangCode = ref('')

async function load() {
  game.value = await api.get<GameDetail>(`/games/${gameId}`)
}

function selectLanguage(code: string) {
  activeLangCode.value = code
  const lang = game.value?.languages.find((l) => l.code === code)
  if (lang) {
    editName.value = lang.name
    editDescription.value = lang.description || ''
  }
}

async function saveLanguage() {
  error.value = ''
  saveMessage.value = ''
  try {
    await api.patch(`/games/${gameId}/languages/${activeLangCode.value}`, {
      name: editName.value,
      description: editDescription.value || null,
    })
    saveMessage.value = 'Salvato'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function addLanguage() {
  error.value = ''
  try {
    await api.post(`/games/${gameId}/languages`, { languageCode: newLangCode.value })
    newLangCode.value = ''
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function deleteGame() {
  try {
    await api.delete(`/games/${gameId}`)
    router.push('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
    if (game.value && game.value.languages.length > 0) {
      selectLanguage(game.value.languages[0].code)
    }
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div v-if="game">
    <h1>{{ game.name }}</h1>
    <img v-if="game.coverPath" :src="`/api/uploads/${game.coverPath}`" :alt="game.name" class="cover" />
    <p v-if="game.owner">Proprietario: {{ game.owner }}</p>
    <button type="button" @click="deleteGame">Elimina gioco</button>

    <nav class="language-tabs">
      <button
        v-for="l in game.languages"
        :key="l.code"
        type="button"
        :class="{ active: l.code === activeLangCode }"
        @click="selectLanguage(l.code)"
      >
        {{ l.code }}
      </button>
    </nav>

    <form @submit.prevent="saveLanguage">
      <label>
        Nome
        <input v-model="editName" required />
      </label>
      <label>
        Descrizione
        <textarea v-model="editDescription"></textarea>
      </label>
      <button type="submit">Salva</button>
      <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
    </form>

    <form @submit.prevent="addLanguage">
      <label>
        Aggiungi lingua (es. en)
        <input v-model="newLangCode" required />
      </label>
      <button type="submit">Aggiungi</button>
    </form>

    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>
