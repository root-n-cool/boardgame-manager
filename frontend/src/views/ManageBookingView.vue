<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface PlayerScore {
  name: string
  score: number
}

interface BookingResult {
  id: number
  eventId: number
  eventGameId: number
  participantName: string
  bookingCode: string
  status: 'active' | 'cancelled'
  eventTitle: string
  eventDate: string
  startTime: string
  gameName: string
  copyIndex: number
  seats: number
  gameCopies: number
  /** Quante prenotazioni attive ci sono su questo tavolo, compresa la mia. */
  tableBookings: number
  matchResult: { players: PlayerScore[] } | null
}

const bookingCode = ref('')
const booking = ref<BookingResult | null>(null)
const error = ref('')
const cancelMessage = ref('')
const scoreError = ref('')
const scoreMessage = ref('')
const players = ref<PlayerScore[]>([{ name: '', score: 0 }])

/** Un tavolo condiviso: più di un posto prenotabile e più di un prenotato. */
const isSharedTable = computed(
  () => (booking.value?.seats ?? 1) > 1 && (booking.value?.tableBookings ?? 1) > 1,
)

/**
 * Il numero della copia serve solo quando l'evento porta più copie di questo
 * gioco: con una copia sola, "#1" è rumore.
 */
const gameLabel = computed(() => {
  const b = booking.value
  if (!b) {
    return ''
  }
  return b.gameCopies > 1 ? `${b.gameName} #${b.copyIndex}` : b.gameName
})

async function lookup() {
  error.value = ''
  cancelMessage.value = ''
  scoreMessage.value = ''
  scoreError.value = ''
  try {
    booking.value = await api.post<BookingResult>('/bookings/lookup', {
      bookingCode: bookingCode.value,
    })
    players.value = booking.value.matchResult
      ? booking.value.matchResult.players.map((p) => ({ ...p }))
      : [{ name: '', score: 0 }]
  } catch (e) {
    booking.value = null
    error.value = (e as Error).message
  }
}

async function cancel() {
  if (!booking.value) {
    return
  }
  if (!window.confirm(`Annullare la prenotazione per ${gameLabel.value}?`)) {
    return
  }
  error.value = ''
  try {
    booking.value = await api.post<BookingResult>(`/bookings/${booking.value.id}/cancel`, {
      bookingCode: booking.value.bookingCode,
    })
    cancelMessage.value = 'Prenotazione annullata.'
  } catch (e) {
    error.value = (e as Error).message
  }
}

function addPlayerRow() {
  players.value.push({ name: '', score: 0 })
}

function removePlayerRow(index: number) {
  if (players.value.length > 1) {
    players.value.splice(index, 1)
  }
}

async function submitScore() {
  if (!booking.value) {
    return
  }
  scoreError.value = ''
  scoreMessage.value = ''
  try {
    const result = await api.post<{ players: PlayerScore[] }>(
      `/bookings/${booking.value.id}/match-result`,
      { bookingCode: booking.value.bookingCode, players: players.value },
    )
    booking.value.matchResult = result
    scoreMessage.value = 'Punteggio salvato.'
  } catch (e) {
    scoreError.value = (e as Error).message
  }
}
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Gestisci prenotazione</h1>

      <form @submit.prevent="lookup">
        <label>
          Codice prenotazione
          <input v-model="bookingCode" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>

      <div v-if="booking">
        <div class="booking-summary">
          <h2>{{ gameLabel }}</h2>
          <p class="booking-summary-meta">
            {{ booking.eventTitle }} · {{ booking.eventDate }} · {{ booking.startTime }}
          </p>
          <p class="booking-summary-participant">Prenotato da <strong>{{ booking.participantName }}</strong></p>
          <p v-if="booking.seats > 1" class="row-meta">
            Tavolo da {{ booking.seats }} posti prenotabili · {{ booking.tableBookings }} prenotati
          </p>
          <span
            class="status-badge"
            :class="booking.status === 'active' ? 'status-active' : 'status-cancelled'"
          >
            {{ booking.status === 'active' ? 'Attiva' : 'Annullata' }}
          </span>
        </div>
        <button v-if="booking.status === 'active'" type="button" class="btn-danger" @click="cancel">
          Annulla prenotazione
        </button>
        <p v-if="cancelMessage" class="success">{{ cancelMessage }}</p>

        <form v-if="booking.status === 'active'" @submit.prevent="submitScore">
          <h2>Punteggio finale</h2>
          <p v-if="isSharedTable" class="row-meta">
            Il punteggio è del tavolo: lo vedono e lo possono correggere tutti
            quelli che hanno prenotato qui. Se qualcuno l'ha già inserito, qui
            sotto c'è il suo, e salvando lo sostituisci.
          </p>
          <div v-for="(p, index) in players" :key="index" class="player-score-row">
            <input v-model="p.name" placeholder="Nome giocatore" required />
            <input v-model.number="p.score" type="number" placeholder="Punteggio" required />
            <button type="button" class="btn-danger" @click="removePlayerRow(index)">Rimuovi</button>
          </div>
          <button type="button" class="btn-secondary" @click="addPlayerRow">Aggiungi giocatore</button>
          <button type="submit">
            {{ booking.matchResult ? 'Aggiorna punteggio' : 'Invia punteggio' }}
          </button>
          <p v-if="scoreMessage" class="success">{{ scoreMessage }}</p>
          <p v-if="scoreError" class="error">{{ scoreError }}</p>
        </form>
      </div>
    </div>
  </div>
</template>
