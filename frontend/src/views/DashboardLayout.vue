<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  await auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="layout">
    <nav>
      <router-link :to="{ name: 'users' }">Utenti</router-link>
      <router-link :to="{ name: 'settings' }">Impostazioni</router-link>
      <button @click="logout">Esci</button>
    </nav>
    <main>
      <router-view />
    </main>
  </div>
</template>
