<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface GameSummary {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
}

const games = ref<GameSummary[]>([])
const error = ref('')

async function loadGames() {
  try {
    games.value = await api.get<GameSummary[]>('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadGames)
</script>

<template>
  <div>
    <h1>Catalogo giochi</h1>
    <p class="page-meta">
      {{ games.length === 0 ? 'Nessun gioco in catalogo' : games.length === 1 ? '1 gioco' : `${games.length} giochi` }}
    </p>
    <router-link to="/games/new" class="action-link">Aggiungi gioco</router-link>
    <p v-if="error" class="error">{{ error }}</p>
    <ul class="game-grid">
      <li v-for="g in games" :key="g.id">
        <router-link :to="`/games/${g.id}`">
          <img v-if="g.coverPath" :src="`/api/uploads/${g.coverPath}`" :alt="g.name" />
          <div v-else class="cover-placeholder" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none">
              <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
              <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
              <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
              <circle cx="12" cy="12" r="1.3" fill="currentColor" />
              <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
              <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
            </svg>
          </div>
          <h2>{{ g.name }}</h2>
          <p v-if="g.year">{{ g.year }}</p>
          <p v-if="g.owner">Proprietario: {{ g.owner }}</p>
        </router-link>
      </li>
    </ul>
  </div>
</template>
