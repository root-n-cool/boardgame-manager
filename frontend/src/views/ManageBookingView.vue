<script setup lang="ts">
import { computed, onMounted, ref, nextTick } from 'vue'
import { api } from '../api/client'

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
  gameId: number
  gameName: string
  copyIndex: number
  seats: number
  gameCopies: number
  /** Quante prenotazioni attive ci sono su questo tavolo, compresa la mia. */
  tableBookings: number
  /**
   * Vero quando il gioco ha un manuale preparato e il provider AI è
   * configurato: è la stessa condizione che fa comparire la chat sulla
   * scheda del gioco. Senza, il link "Chiedi al manuale" prometteva una
   * pagina dove non succedeva niente.
   */
  canAsk: boolean
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
    <h1>Gestisci prenotazione</h1>

    <form v-if="!deepLinked" class="booking-lookup" @submit.prevent="lookup">
      <label>
        Codice prenotazione
        <input v-model="bookingCode" required />
      </label>
      <button type="submit" class="btn-with-icon">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <circle cx="10.8" cy="10.8" r="6.3" stroke="currentColor" stroke-width="1.7" />
          <path d="m15.4 15.4 4.1 4.1" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
        Cerca
      </button>
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
        <p v-if="booking.gameId && booking.canAsk" class="row-meta">
          <router-link :to="{ path: `/games/${booking.gameId}`, query: { chat: '1' } }">
            Dubbi sulle regole? Chiedi al manuale
          </router-link>
        </p>
        <p class="booking-summary-meta">
          {{ booking.eventTitle }} · {{ booking.eventDate }} · {{ booking.startTime }}
        </p>
        <p class="booking-summary-participant">Prenotato da <strong>{{ booking.participantName }}</strong></p>
        <p v-if="booking.seats > 1" class="row-meta">
          Tavolo da {{ booking.seats }} posti prenotabili · {{ booking.tableBookings }} prenotati
        </p>
        <!-- Stato e disdetta sulla stessa riga, in fondo alla scheda: la
             conseguenza sta sull'oggetto a cui si applica, non spaiata
             sotto la card. -->
        <div class="booking-summary-foot">
          <span
            class="status-badge"
            :class="booking.status === 'active' ? 'status-active' : 'status-cancelled'"
          >
            {{ booking.status === 'active' ? 'Attiva' : 'Annullata' }}
          </span>
          <button
            v-if="booking.status === 'active'"
            type="button"
            class="btn-danger btn-with-icon"
            @click="cancel"
          >
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <circle cx="12" cy="12" r="8.4" stroke="currentColor" stroke-width="1.7" />
              <path
                d="m9.2 9.2 5.6 5.6M14.8 9.2l-5.6 5.6"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linecap="round"
              />
            </svg>
            Annulla prenotazione
          </button>
        </div>
      </div>
      <p v-if="cancelMessage" class="success">{{ cancelMessage }}</p>

      <form
        v-if="booking.status === 'active'"
        ref="scoreSection"
        class="score-form"
        @submit.prevent="submitScore"
      >
        <h2>Punteggio finale</h2>
        <p v-if="isSharedTable" class="row-meta">
          Il punteggio è del tavolo: lo vedono e lo possono correggere tutti
          quelli che hanno prenotato qui. Se qualcuno l'ha già inserito, qui
          sotto c'è il suo, e salvando lo sostituisci.
        </p>
        <div v-for="(p, index) in players" :key="index" class="player-score-row">
          <!-- Il segnaposto sparisce appena si scrive: senza `aria-label` la
               riga arriva a uno screen reader come due campi senza nome. -->
          <input
            v-model="p.name"
            :aria-label="`Nome giocatore ${index + 1}`"
            placeholder="Nome giocatore"
            required
          />
          <input
            v-model.number="p.score"
            :aria-label="`Punteggio giocatore ${index + 1}`"
            type="number"
            placeholder="Punteggio"
            required
          />
          <button
            type="button"
            class="btn-danger btn-with-icon player-score-remove"
            :aria-label="`Rimuovi giocatore ${index + 1}`"
            @click="removePlayerRow(index)"
          >
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M5.4 6.9h13.2M9.9 6.9V4.7h4.2v2.2M7.3 6.9l.8 12.4h7.8l.8-12.4"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
              <path
                d="M10.6 10.3v5.9M13.4 10.3v5.9"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linecap="round"
              />
            </svg>
            Rimuovi
          </button>
        </div>
        <button type="button" class="btn-secondary btn-with-icon player-score-add" @click="addPlayerRow">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          </svg>
          Aggiungi giocatore
        </button>
        <p v-if="scoreMessage" class="success">{{ scoreMessage }}</p>
        <p v-if="scoreError" class="error">{{ scoreError }}</p>
        <div class="form-actions">
          <button type="submit" class="btn-with-icon">
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M5.2 4.4h9.7l4.3 4.3v10.9H5.2V4.4Z"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linejoin="round"
              />
              <path
                d="M8.6 4.4v4.5h6.1V4.4M8 19.6v-5.5h8v5.5"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linejoin="round"
              />
            </svg>
            {{ booking.matchResult ? 'Aggiorna punteggio' : 'Invia punteggio' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
