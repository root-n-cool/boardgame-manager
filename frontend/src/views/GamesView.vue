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
    <router-link to="/games/new">Aggiungi gioco</router-link>
    <p v-if="error" class="error">{{ error }}</p>
    <ul class="game-grid">
      <li v-for="g in games" :key="g.id">
        <router-link :to="`/games/${g.id}`">
          <img v-if="g.coverPath" :src="`/api/uploads/${g.coverPath}`" :alt="g.name" />
          <h2>{{ g.name }}</h2>
          <p v-if="g.year">{{ g.year }}</p>
          <p v-if="g.owner">Proprietario: {{ g.owner }}</p>
        </router-link>
      </li>
    </ul>
  </div>
</template>
