<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface PlayerStats {
  name: string
  gamesPlayed: number
  wins: number
  averageScore: number
  totalScore: number
}

interface MatchPlayer {
  name: string
  score: number
  isWinner: boolean
}

interface MatchEntry {
  eventTitle: string
  eventDate: string
  startTime: string
  players: MatchPlayer[]
}

interface LeaderboardResponse {
  players: PlayerStats[]
  matches: MatchEntry[]
}

const route = useRoute()
const gameId = route.params.id as string

const leaderboard = ref<LeaderboardResponse | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    leaderboard.value = await api.get<LeaderboardResponse>(`/games/${gameId}/leaderboard`)
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Classifica</h1>
      <p v-if="error" class="error">{{ error }}</p>

      <table v-if="leaderboard && leaderboard.players.length > 0">
        <thead>
          <tr>
            <th>Giocatore</th>
            <th>Partite</th>
            <th>Vittorie</th>
            <th>Punteggio medio</th>
            <th>Punteggio totale</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in leaderboard.players" :key="p.name">
            <td>{{ p.name }}</td>
            <td>{{ p.gamesPlayed }}</td>
            <td>{{ p.wins }}</td>
            <td>{{ p.averageScore.toFixed(1) }}</td>
            <td>{{ p.totalScore }}</td>
          </tr>
        </tbody>
      </table>
      <p v-else-if="leaderboard">Nessun punteggio ancora registrato per questo gioco.</p>

      <h2 v-if="leaderboard && leaderboard.matches.length > 0">Storico partite</h2>
      <ul>
        <li v-for="(m, index) in leaderboard?.matches" :key="index">
          {{ m.eventTitle }} ({{ m.eventDate }} · {{ m.startTime }}):
          <span v-for="(p, pIndex) in m.players" :key="pIndex">
            {{ p.name }} {{ p.score }}{{ p.isWinner ? ' 🏆' : '' }}{{ pIndex < m.players.length - 1 ? ', ' : '' }}
          </span>
        </li>
      </ul>
    </div>
  </div>
</template>
