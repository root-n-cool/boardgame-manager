<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  remaining: number
}

interface EventDetail {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
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
      <h1>{{ event.title }}</h1>
      <p v-if="event.description">{{ event.description }}</p>
      <p>{{ event.eventDate }} · {{ event.startTime }}</p>

      <ul class="event-games">
        <li v-for="g in event.games" :key="g.eventGameId">
          <img v-if="g.coverPath" :src="`/api/uploads/${g.coverPath}`" :alt="g.name" />
          <router-link :to="`/games/${g.gameId}`">{{ g.name }}</router-link>
          <p>Disponibilità: {{ g.remaining }}</p>
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

      <div v-if="bookingResult" class="success">
        <p>Prenotazione confermata! Il tuo codice è:</p>
        <p class="booking-code">{{ bookingResult.bookingCode }}</p>
        <p>Conservalo insieme alla tua email per gestire la prenotazione da "Gestisci prenotazione".</p>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>
