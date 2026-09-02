<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const token = String(route.params.token)
const state = ref<'loading' | 'ready' | 'invalid'>('loading')
const email = ref('')
const password = ref('')
const confirmation = ref('')
const error = ref('')
const saving = ref(false)

onMounted(async () => {
  try {
    // skipAuthRedirect non serve (un token invalido risponde 404, non 401) ma
    // vale come dichiarazione: questa pagina non deve mai finire su /login
    // per colpa del client.
    const invite = await api.get<{ email: string }>(`/invites/${token}`, { skipAuthRedirect: true })
    email.value = invite.email
    state.value = 'ready'
  } catch {
    state.value = 'invalid'
  }
})

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
    const message = (e as Error).message
    error.value =
      message === 'invite not found'
        ? 'Questo invito non è più valido: la password è già stata impostata.'
        : message === 'password must be at least 8 characters'
          ? 'La password deve essere di almeno 8 caratteri.'
          : message
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
