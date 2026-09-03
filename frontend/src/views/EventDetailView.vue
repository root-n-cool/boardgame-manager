<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'
import { formatEventDateTime } from '../utils/dates'

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  copyIndex: number
  seats: number
  remaining: number
}

interface EventDetail {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
  imagePath: string | null
  games: EventGameInfo[]
}

interface BookingResult {
  id: number
  bookingCode: string
}

const route = useRoute()
const eventId = route.params.id as string

const event = ref<EventDetail | null>(null)
const error = ref('')

const selectedEventGameId = ref<number | null>(null)
const participantName = ref('')
const participantEmail = ref('')
const participantPhone = ref('')
const bookingError = ref('')
const bookingResult = ref<BookingResult | null>(null)

async function load() {
  event.value = await api.get<EventDetail>(`/events/${eventId}`)
}

const hasStarted = computed(() => {
  if (!event.value) {
    return false
  }
  const startsAt = new Date(`${event.value.eventDate}T${event.value.startTime}`)
  return startsAt <= new Date()
})

function startBooking(eventGameId: number) {
  selectedEventGameId.value = eventGameId
  bookingError.value = ''
  bookingResult.value = null
}

const selectedGame = computed(
  () => event.value?.games.find((g) => g.eventGameId === selectedEventGameId.value) ?? null,
)

/** Quante copie ha ogni gioco in questo evento. */
const copiesByGame = computed(() => {
  const counts: Record<number, number> = {}
  for (const g of event.value?.games ?? []) {
    counts[g.gameId] = (counts[g.gameId] ?? 0) + 1
  }
  return counts
})

/**
 * L'etichetta di una copia: il numero compare solo quando quel gioco ha
 * più di una copia nell'evento, così un evento normale non si riempie di
 * "#1" inutili.
 */
function copyLabel(g: EventGameInfo) {
  return (copiesByGame.value[g.gameId] ?? 1) > 1 ? `${g.name} #${g.copyIndex}` : g.name
}

/**
 * Un tavolo aperto dice quanti posti prenotabili restano; una copia
 * singola resta con la dicitura di sempre.
 */
function availabilityLabel(g: EventGameInfo) {
  if (g.seats > 1) {
    return `Posti prenotabili liberi: ${g.remaining} di ${g.seats}`
  }
  return `Disponibilità: ${g.remaining}`
}

async function submitBooking() {
  bookingError.value = ''
  if (selectedEventGameId.value === null) {
    return
  }
  try {
    bookingResult.value = await api.post<BookingResult>(`/events/${eventId}/bookings`, {
      eventGameId: selectedEventGameId.value,
      participantName: participantName.value,
      participantEmail: participantEmail.value,
      participantPhone: participantPhone.value,
    })
    await load()
  } catch (e) {
    bookingError.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page" v-if="event">
      <img
        v-if="event.imagePath"
        :src="`/api/uploads/${event.imagePath}`"
        alt=""
        class="event-banner"
        width="880"
        height="495"
        decoding="async"
      />
      <h1>{{ event.title }}</h1>
      <p v-if="event.description">{{ event.description }}</p>
      <p class="event-card-date">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <rect x="3" y="5" width="18" height="16" rx="2" stroke="currentColor" stroke-width="1.6" />
          <path d="M3 9.5h18" stroke="currentColor" stroke-width="1.6" />
          <path d="M8 3v4M16 3v4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
        </svg>
        {{ formatEventDateTime(event.eventDate, event.startTime) }}
      </p>

      <ul class="event-games">
        <li v-for="g in event.games" :key="g.eventGameId">
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
          <router-link :to="`/games/${g.gameId}`">{{ copyLabel(g) }}</router-link>
          <p>{{ availabilityLabel(g) }}</p>
          <button
            v-if="!hasStarted"
            type="button"
            :disabled="g.remaining <= 0"
            @click="startBooking(g.eventGameId)"
          >
            Prenota
          </button>
        </li>
      </ul>

      <p v-if="hasStarted">Questo evento è già iniziato: non è più possibile prenotare.</p>

      <form
        v-if="selectedEventGameId !== null && !bookingResult && !hasStarted"
        @submit.prevent="submitBooking"
      >
        <h2>Prenota: {{ selectedGame ? copyLabel(selectedGame) : '' }}</h2>
        <label>
          Nome
          <input v-model="participantName" required />
        </label>
        <label>
          Email
          <input v-model="participantEmail" type="email" required />
        </label>
        <label>
          Telefono
          <input v-model="participantPhone" required />
        </label>
        <button type="submit">Conferma prenotazione</button>
        <p v-if="bookingError" class="error">{{ bookingError }}</p>
      </form>

      <div v-if="bookingResult">
        <p class="success">
          Prenotazione confermata per {{ selectedGame ? copyLabel(selectedGame) : '' }}!
        </p>
        <div class="booking-code-card">
          <span class="label">Il tuo codice</span>
          <span class="booking-code">{{ bookingResult.bookingCode }}</span>
        </div>
        <p>Conservalo per gestire la prenotazione o inserire il punteggio finale da "Gestisci prenotazione".</p>
        <p v-if="selectedGame && selectedGame.seats > 1">
          Questo tavolo ha più posti prenotabili, uno a testa: il punteggio finale è uno per
          tavolo e chiunque sieda qui può inserirlo o correggerlo con il proprio codice.
        </p>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>
