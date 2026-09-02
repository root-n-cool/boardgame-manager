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

interface GameSummary {
  name: string
}

const route = useRoute()
const gameId = route.params.id as string

const leaderboard = ref<LeaderboardResponse | null>(null)
const game = ref<GameSummary | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    const [leaderboardResult, gameResult] = await Promise.all([
      api.get<LeaderboardResponse>(`/games/${gameId}/leaderboard`),
      api.get<GameSummary>(`/games/${gameId}`),
    ])
    leaderboard.value = leaderboardResult
    game.value = gameResult
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <router-link :to="`/games/${gameId}`" class="back-link">&larr; {{ game?.name ?? 'Torna al gioco' }}</router-link>
      <h1>Classifica{{ game ? `: ${game.name}` : '' }}</h1>
      <p v-if="error" class="error">{{ error }}</p>

      <div v-if="leaderboard && leaderboard.players.length > 0" class="table-scroll">
        <table>
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
            <tr v-for="(p, index) in leaderboard.players" :key="p.name">
              <td><span class="rank-medal">{{ index + 1 }}</span>{{ p.name }}</td>
              <td>{{ p.gamesPlayed }}</td>
              <td>{{ p.wins }}</td>
              <td>{{ p.averageScore.toFixed(1) }}</td>
              <td>{{ p.totalScore }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else-if="leaderboard">Nessun punteggio ancora registrato per questo gioco.</p>

      <h2 v-if="leaderboard && leaderboard.matches.length > 0">Storico partite</h2>
      <ul>
        <li v-for="(m, index) in leaderboard?.matches" :key="index" class="list-row-text">
          {{ m.eventTitle }} ({{ m.eventDate }} · {{ m.startTime }}):
          <span v-for="(p, pIndex) in m.players" :key="pIndex">
            <span :class="{ 'win-badge': p.isWinner }">
              {{ p.name }} {{ p.score }}
              <svg v-if="p.isWinner" viewBox="0 0 24 24" fill="none" aria-label="Vincitore">
                <path
                  d="M7 4h10v3.2c0 3.3-2.2 6-5 6.6-2.8-.6-5-3.3-5-6.6V4Z"
                  stroke="currentColor"
                  stroke-width="1.6"
                  stroke-linejoin="round"
                />
                <path d="M7 5.5H4.5A2 2 0 0 0 5 9.4L7 10.5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
                <path d="M17 5.5h2.5A2 2 0 0 1 19 9.4L17 10.5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
                <path d="M12 13.8v3.4M9 20h6" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
              </svg>
            </span>{{ pIndex < m.players.length - 1 ? ', ' : '' }}
          </span>
        </li>
      </ul>
    </div>
  </div>
</template>
