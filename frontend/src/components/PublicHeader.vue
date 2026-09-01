<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  try {
    await auth.logout()
  } catch (e) {
    console.error('logout request failed', e)
  }
  router.push({ name: 'events' })
}
</script>

<template>
  <header class="public-header">
    <router-link :to="{ name: 'events' }" class="brand">BoardGames Manager</router-link>
    <nav>
      <router-link :to="{ name: 'manage-booking' }">Gestisci prenotazione</router-link>
      <template v-if="auth.user">
        <router-link to="/games">Area admin</router-link>
        <button type="button" @click="logout">Esci</button>
      </template>
      <template v-else>
        <router-link :to="{ name: 'login' }">Accedi</router-link>
      </template>
    </nav>
  </header>
</template>
