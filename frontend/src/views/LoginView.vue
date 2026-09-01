<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const email = ref('')
const password = ref('')
const error = ref('')
const auth = useAuthStore()
const router = useRouter()

async function submit() {
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    router.push({ name: 'users' })
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>

<template>
  <div class="auth-page">
    <h1>Accedi</h1>
    <form @submit.prevent="submit">
      <label>
        Email
        <input v-model="email" type="email" required />
      </label>
      <label>
        Password
        <input v-model="password" type="password" required />
      </label>
      <button type="submit">Accedi</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
