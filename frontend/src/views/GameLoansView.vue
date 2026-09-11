<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import { formatEventDate } from '../utils/dates'
import { clockTime, issuesLabel, type MaterialIssue } from '../utils/loans'

interface GameLoan {
  id: number
  eventId: number
  eventTitle: string
  eventDate: string
  copyIndex: number
  /** Quante copie di questo gioco aveva quella serata: sotto 2, "#1" è rumore. */
  copies: number
  borrowerName: string
  borrowerPhone: string
  lentAt: string
  returnedAt: string | null
  notes: string | null
  materialIssues: MaterialIssue[]
}

interface LoansResponse {
  loans: GameLoan[]
}

interface GameSummary {
  name: string
}

const route = useRoute()
const gameId = route.params.id as string

const loans = ref<GameLoan[]>([])
const game = ref<GameSummary | null>(null)
const error = ref('')
const loading = ref(true)

/**
 * Una riga si tinge solo se qualcuno ha contato e mancava qualcosa
 * (`returned` valorizzato). Non su `length`: una riconsegna in cui
 * nessuno ha aperto la scatola lascia una riga "non verificata" per ogni
 * voce del catalogo, e sul numero di righe un rientro ordinario
 * risulterebbe rosso — proprio sulla pagina che deve spiegare il marchio
 * "Incompleto", che invece quelle voci non le conta.
 */
function hasShortage(loan: GameLoan) {
  return loan.materialIssues.some((i) => i.returned !== null)
}

onMounted(async () => {
  try {
    const [gameResult, loansResult] = await Promise.all([
      api.get<GameSummary>(`/games/${gameId}`),
      api.get<LoansResponse>(`/games/${gameId}/loans`),
    ])
    game.value = gameResult
    loans.value = loansResult.loans
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <router-link :to="{ name: 'admin-game-detail', params: { id: gameId } }" class="back-link">
      &larr; Gioco
    </router-link>

    <h1>Prestiti{{ game ? `: ${game.name}` : '' }}</h1>

    <p v-if="error" class="error" role="alert">{{ error }}</p>

    <template v-if="!loading && !error">
      <p v-if="loans.length === 0" class="empty-note">
        Questo gioco non è mai stato dato in prestito.
      </p>
      <ul v-else role="list" class="game-loans-list">
        <li v-for="loan in loans" :key="loan.id" :class="{ 'has-issues': hasShortage(loan) }">
          <span class="game-loans-title">
            {{ loan.eventTitle }} · {{ formatEventDate(loan.eventDate) }}
            <template v-if="loan.copies > 1"> #{{ loan.copyIndex }}</template>
          </span>
          <span class="row-meta">
            {{ loan.borrowerName }} ·
            <template v-if="loan.returnedAt">{{ clockTime(loan.lentAt) }} &rarr; {{ clockTime(loan.returnedAt) }}</template>
            <template v-else>fuori dalle {{ clockTime(loan.lentAt) }}</template>
          </span>
          <span v-if="loan.notes" class="loan-row-notes">{{ loan.notes }}</span>
          <span v-if="loan.materialIssues.length" class="material-issues">
            {{ issuesLabel(loan.materialIssues) }}
          </span>
        </li>
      </ul>
    </template>
  </div>
</template>
