<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface SettingsResponse {
  defaultLanguage: string
  youtubeApiKeySet: boolean
  youtubeApiKeyMasked?: string
  searchApiKeySet: boolean
  searchApiKeyMasked?: string
  searchApiProvider: string
  bggApiTokenSet: boolean
  bggApiTokenMasked?: string
}

const defaultLanguage = ref('it')
const youtubeApiKey = ref('')
const searchApiKey = ref('')
const searchApiProvider = ref('google')
const bggApiToken = ref('')
const youtubeApiKeyMasked = ref('')
const searchApiKeyMasked = ref('')
const bggApiTokenMasked = ref('')
const message = ref('')
const error = ref('')

async function load() {
  const s = await api.get<SettingsResponse>('/settings')
  defaultLanguage.value = s.defaultLanguage
  searchApiProvider.value = s.searchApiProvider || 'google'
  youtubeApiKeyMasked.value = s.youtubeApiKeyMasked || ''
  searchApiKeyMasked.value = s.searchApiKeyMasked || ''
  bggApiTokenMasked.value = s.bggApiTokenMasked || ''
}

async function save() {
  error.value = ''
  message.value = ''
  try {
    await api.put('/settings', {
      defaultLanguage: defaultLanguage.value,
      youtubeApiKey: youtubeApiKey.value,
      searchApiKey: searchApiKey.value,
      searchApiProvider: searchApiProvider.value,
      bggApiToken: bggApiToken.value,
    })
    youtubeApiKey.value = ''
    searchApiKey.value = ''
    bggApiToken.value = ''
    message.value = 'Impostazioni salvate'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <h1>Impostazioni</h1>
    <form @submit.prevent="save">
      <label>
        Lingua di default
        <select v-model="defaultLanguage">
          <option value="it">Italiano</option>
          <option value="en">Inglese</option>
        </select>
      </label>

      <label>
        BoardGameGeek API token
        <input v-model="bggApiToken" type="password" :placeholder="bggApiTokenMasked || 'non configurato'" />
      </label>

      <label>
        YouTube Data API key
        <input v-model="youtubeApiKey" type="password" :placeholder="youtubeApiKeyMasked || 'non configurata'" />
      </label>

      <label>
        Provider ricerca web
        <select v-model="searchApiProvider">
          <option value="google">Google Custom Search</option>
          <option value="bing">Bing Search</option>
        </select>
      </label>

      <label>
        Search API key
        <input v-model="searchApiKey" type="password" :placeholder="searchApiKeyMasked || 'non configurata'" />
      </label>

      <button type="submit">Salva</button>
      <p v-if="message" class="success">{{ message }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
