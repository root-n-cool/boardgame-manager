<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface SettingsResponse {
  defaultLanguage: string
  publicBaseUrl: string
  bggApiTokenSet: boolean
  bggApiTokenMasked?: string
  aiBaseUrl: string
  aiModel: string
  aiApiKeySet: boolean
  aiApiKeyMasked?: string
  aiConfigured: boolean
}

const defaultLanguage = ref('it')
const publicBaseUrl = ref('')
const bggApiToken = ref('')
const bggApiTokenMasked = ref('')
const aiBaseUrl = ref('')
const aiModel = ref('')
const aiApiKey = ref('')
const aiApiKeyMasked = ref('')
const message = ref('')
const error = ref('')

async function load() {
  const s = await api.get<SettingsResponse>('/settings')
  defaultLanguage.value = s.defaultLanguage
  publicBaseUrl.value = s.publicBaseUrl || ''
  bggApiTokenMasked.value = s.bggApiTokenMasked || ''
  aiBaseUrl.value = s.aiBaseUrl || ''
  aiModel.value = s.aiModel || ''
  aiApiKeyMasked.value = s.aiApiKeyMasked || ''
}

async function save() {
  error.value = ''
  message.value = ''
  try {
    await api.put('/settings', {
      defaultLanguage: defaultLanguage.value,
      publicBaseUrl: publicBaseUrl.value,
      bggApiToken: bggApiToken.value,
      aiBaseUrl: aiBaseUrl.value,
      aiApiKey: aiApiKey.value,
      aiModel: aiModel.value,
    })
    // Il token è un segreto e si riscrive solo per sostituirlo: il campo torna
    // vuoto. L'indirizzo pubblico invece è un dato da rileggere, quindi resta.
    bggApiToken.value = ''
    aiApiKey.value = ''
    message.value = 'Impostazioni salvate'
    await load()
  } catch (e) {
    const raw = (e as Error).message
    error.value =
      raw === 'publicBaseUrl must be an absolute http or https address'
        ? "L'indirizzo pubblico deve essere completo, per esempio https://giochi.example.org"
        : raw
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
    <form class="panel-form" @submit.prevent="save">
      <div class="panel-card">
        <div class="section-head">
          <h2>Generale</h2>
        </div>
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
          Indirizzo pubblico
          <input v-model="publicBaseUrl" type="url" inputmode="url" placeholder="https://giochi.example.org" />
        </label>
        <p class="field-hint">
          Serve a comporre i link che mandi fuori dall'app, come l'invito di un
          amministratore. Se lo lasci vuoto si usa l'indirizzo da cui stai navigando.
        </p>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Provider AI</h2>
        </div>
        <p class="field-hint">
          Se lo configuri, le descrizioni scaricate da BoardGameGeek arrivano già
          tradotte nella lingua della scheda. Vale un qualsiasi servizio
          compatibile con le API OpenAI: Google Gemini, OpenAI, OpenRouter, o un
          Ollama in casa. Lasciandolo vuoto l'app funziona come prima, con le
          descrizioni in inglese.
        </p>

        <label>
          Indirizzo del provider
          <input
            v-model="aiBaseUrl"
            type="url"
            inputmode="url"
            placeholder="https://generativelanguage.googleapis.com/v1beta/openai"
          />
        </label>
        <p class="field-hint">
          L'indirizzo base, senza <code>/chat/completions</code> in fondo. Gemini:
          <code>https://generativelanguage.googleapis.com/v1beta/openai</code> ·
          OpenAI: <code>https://api.openai.com/v1</code> · Ollama:
          <code>http://localhost:11434/v1</code>
        </p>

        <label>
          Chiave API
          <input v-model="aiApiKey" type="password" :placeholder="aiApiKeyMasked || 'non configurata'" />
        </label>

        <label>
          Modello
          <input v-model="aiModel" placeholder="gemini-flash-lite-latest" />
        </label>
        <p class="field-hint">
          Per tradurre basta un modello economico e veloce. Esempi:
          <code>gemini-flash-lite-latest</code>, <code>gpt-4.1-mini</code>,
          <code>llama3.1</code>.
        </p>
      </div>

      <div class="form-actions">
        <button type="submit">Salva</button>
      </div>
      <p v-if="message" class="success">{{ message }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
