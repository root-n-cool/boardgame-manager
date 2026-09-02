<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// Il backend distingue apposta un token morto (404 "invite not found") da un
// guasto infrastrutturale (500): solo il primo caso è un invito da
// rifiutare, il secondo è un problema temporaneo da far riprovare.
const MESSAGES: Record<string, string> = {
  'invite not found': 'Questo invito non è più valido: la password è già stata impostata.',
  'password must be at least 8 characters': 'La password deve essere di almeno 8 caratteri.',
  'password is required': 'Inserisci una password.',
}

function toItalian(message: string) {
  return MESSAGES[message] ?? 'Si è verificato un errore, riprova.'
}

const token = String(route.params.token)
const state = ref<'loading' | 'ready' | 'invalid' | 'unavailable'>('loading')
const email = ref('')
const password = ref('')
const confirmation = ref('')
const error = ref('')
const saving = ref(false)

async function loadInvite() {
  state.value = 'loading'
  try {
    // skipAuthRedirect non serve (un token invalido risponde 404, non 401) ma
    // vale come dichiarazione: questa pagina non deve mai finire su /login
    // per colpa del client.
    const invite = await api.get<{ email: string }>(`/invites/${token}`, { skipAuthRedirect: true })
    email.value = invite.email
    state.value = 'ready'
  } catch (e) {
    // Solo un token morto o già usato è "invalid": qualsiasi altro errore
    // (es. un 500 momentaneo) non deve dire all'invitato che il link non
    // funziona più, quando probabilmente basta riprovare.
    state.value = (e as Error).message === 'invite not found' ? 'invalid' : 'unavailable'
  }
}

onMounted(loadInvite)

async function submit() {
  error.value = ''
  if (password.value !== confirmation.value) {
    error.value = 'Le due password non coincidono.'
    return
  }
  saving.value = true
  try {
    await auth.acceptInvite(token, password.value)
    router.push({ name: 'users' })
  } catch (e) {
    error.value = toItalian((e as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <template v-if="state === 'loading'">
      <h1>Invito</h1>
      <p>Verifica del link…</p>
    </template>

    <template v-else-if="state === 'invalid'">
      <h1>Invito non valido</h1>
      <p>
        Questo link non funziona più: la password è già stata impostata, oppure
        l'invito è stato annullato. Chiedi a chi ti ha invitato di generarne uno nuovo.
      </p>
      <p><router-link to="/login">Vai all'accesso</router-link></p>
    </template>

    <template v-else-if="state === 'unavailable'">
      <h1>Non riesco a verificare l'invito</h1>
      <p>
        Sembra un problema temporaneo di connessione: il link è probabilmente
        ancora valido. Riprova fra qualche istante.
      </p>
      <button type="button" @click="loadInvite">Riprova</button>
    </template>

    <template v-else>
      <h1>Imposta la tua password</h1>
      <form @submit.prevent="submit">
        <label>
          Email
          <input :value="email" type="email" disabled />
        </label>
        <label>
          Password
          <input v-model="password" type="password" required minlength="8" autocomplete="new-password" />
        </label>
        <label>
          Conferma password
          <input v-model="confirmation" type="password" required minlength="8" autocomplete="new-password" />
        </label>
        <button type="submit" :disabled="saving">
          {{ saving ? 'Salvataggio…' : 'Imposta password e accedi' }}
        </button>
        <p v-if="error" class="error">{{ error }}</p>
      </form>
    </template>
  </div>
</template>
