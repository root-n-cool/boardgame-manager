<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  // The store clears the local session either way; a failing POST /logout
  // must not turn this button into a no-op.
  try {
    await auth.logout()
  } catch (e) {
    console.error('logout request failed', e)
  }
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="layout">
    <nav>
      <span class="brand">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <circle cx="12" cy="5.6" r="2.6" fill="currentColor" />
          <path
            d="M9 9.2h6c1.6 0 2.6.1 3.3.5l.1 3c.05.7-.6 1.2-1.3 1l-1.7-.5-.4 2c.6 1.8.8 3.6.6 5.4h-2.3l-.6-4.6h-1.4l-.6 4.6H8.4c-.2-1.8 0-3.6.6-5.4l-.4-2-1.7.5c-.7.2-1.35-.3-1.3-1l.1-3c.7-.4 1.7-.5 3.3-.5Z"
            fill="currentColor"
          />
        </svg>
        Admin
      </span>
      <router-link :to="{ name: 'admin-events' }">Eventi</router-link>
      <router-link :to="{ name: 'games' }">Giochi</router-link>
      <router-link :to="{ name: 'users' }">Utenti</router-link>
      <router-link :to="{ name: 'settings' }">Impostazioni</router-link>
      <button @click="logout">Esci</button>
    </nav>
    <main>
      <router-view />
    </main>
  </div>
</template>
