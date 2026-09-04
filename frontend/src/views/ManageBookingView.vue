<script setup lang="ts">
import { computed, onMounted, ref, nextTick } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

/**
 * `code` arriva dai link mandati per mail: quando c'è, la pagina si
 * risolve da sé e il form del codice non compare — chiederlo a chi ha
 * appena cliccato un link che lo contiene sarebbe un passaggio in più
 * davanti a un tavolo di gioco.
 *
 * `mode` dice su cosa aprire: 'score' è il link "segna i punti a fine
 * partita", che porta direttamente al form del punteggio.
 */
const props = withDefaults(
  defineProps<{ code?: string; mode?: 'manage' | 'score' }>(),
  { code: '', mode: 'manage' },
)

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

/** Vero quando il codice arriva dall'indirizzo invece che dal form. */
const deepLinked = computed(() => props.code !== '')
const scoreSection = ref<HTMLFormElement | null>(null)

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
    // Chi arriva da un link non ha sbagliato a digitare: il codice è
    // quello che gli abbiamo mandato noi. Se non risolve, la
    // prenotazione è stata annullata — LookupBooking cerca solo fra le
    // attive — e dirlo così evita di far sembrare un guasto nostro.
    error.value = deepLinked.value
      ? 'Questa prenotazione non è più attiva, o il link non è più valido.'
      : (e as Error).message
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

onMounted(async () => {
  if (!deepLinked.value) {
    return
  }
  bookingCode.value = props.code
  await lookup()
  if (props.mode === 'score' && booking.value?.status === 'active') {
    await nextTick()
    const form = scoreSection.value
    // Lo scroll da solo non basta: su una pagina corta (il caso comune, una
    // prenotazione sola) non c'è nulla da scorrere e il link "punteggio"
    // finisce identico al link "gestisci". Il focus sul primo nome è il
    // segnale — sposta il cursore e apre la tastiera sul telefono.
    form?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    form?.querySelector<HTMLInputElement>('input')?.focus()
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Gestisci prenotazione</h1>

      <form v-if="!deepLinked" @submit.prevent="lookup">
        <label>
          Codice prenotazione
          <input v-model="bookingCode" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="error && deepLinked">
        <router-link :to="{ name: 'manage-booking' }" @click="error = ''"
          >Cerca un'altra prenotazione</router-link
        >
      </p>

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

        <form
          v-if="booking.status === 'active'"
          ref="scoreSection"
          @submit.prevent="submitScore"
        >
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
