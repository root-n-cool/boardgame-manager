<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface SettingsResponse {
  defaultLanguage: string
  publicBaseUrl: string
  bggApiTokenSet: boolean
  bggApiTokenMasked?: string
}

const defaultLanguage = ref('it')
const publicBaseUrl = ref('')
const bggApiToken = ref('')
const bggApiTokenMasked = ref('')
const message = ref('')
const error = ref('')

async function load() {
  const s = await api.get<SettingsResponse>('/settings')
  defaultLanguage.value = s.defaultLanguage
  publicBaseUrl.value = s.publicBaseUrl || ''
  bggApiTokenMasked.value = s.bggApiTokenMasked || ''
}

async function save() {
  error.value = ''
  message.value = ''
  try {
    await api.put('/settings', {
      defaultLanguage: defaultLanguage.value,
      publicBaseUrl: publicBaseUrl.value,
      bggApiToken: bggApiToken.value,
    })
    // Il token è un segreto e si riscrive solo per sostituirlo: il campo torna
    // vuoto. L'indirizzo pubblico invece è un dato da rileggere, quindi resta.
    bggApiToken.value = ''
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
        Indirizzo pubblico
        <input v-model="publicBaseUrl" type="url" inputmode="url" placeholder="https://giochi.example.org" />
      </label>
      <p class="field-hint">
        Serve a comporre i link che mandi fuori dall'app, come l'invito di un
        amministratore. Se lo lasci vuoto si usa l'indirizzo da cui stai navigando.
      </p>

      <button type="submit">Salva</button>
      <p v-if="message" class="success">{{ message }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
