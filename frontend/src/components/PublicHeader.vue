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
    <router-link :to="{ name: 'events' }" class="brand">
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="12" cy="5.6" r="2.6" fill="currentColor" />
        <path
          d="M9 9.2h6c1.6 0 2.6.1 3.3.5l.1 3c.05.7-.6 1.2-1.3 1l-1.7-.5-.4 2c.6 1.8.8 3.6.6 5.4h-2.3l-.6-4.6h-1.4l-.6 4.6H8.4c-.2-1.8 0-3.6.6-5.4l-.4-2-1.7.5c-.7.2-1.35-.3-1.3-1l.1-3c.7-.4 1.7-.5 3.3-.5Z"
          fill="currentColor"
        />
      </svg>
      <span class="brand-name">BoardGames Manager</span>
    </router-link>
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
