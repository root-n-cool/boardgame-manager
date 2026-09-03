<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import BookingConfirmation from '../components/BookingConfirmation.vue'
import GameDifficulty from '../components/GameDifficulty.vue'
import ModalDialog from '../components/ModalDialog.vue'
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
  weight: number | null
}

interface EventDetail {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
  imagePath: string | null
  venue: EventVenue | null
  games: EventGameInfo[]
}

interface EventVenue {
  name: string
  address: string
  lat: number | null
  lon: number | null
}

interface BookingResult {
  id: number
  bookingCode: string
}

/** Una prenotazione andata a buon fine in questa visita alla pagina. */
interface ConfirmedBooking {
  code: string
  label: string
  multiSeat: boolean
}

// Leaflet pesa quanto tutto il resto dell'app: si scarica solo quando un
// evento ha davvero delle coordinate da mostrare, non a ogni pagina aperta.
const EventMap = defineAsyncComponent(() => import('../components/EventMap.vue'))

const route = useRoute()
const eventId = route.params.id as string

const event = ref<EventDetail | null>(null)
const error = ref('')

const selectedEventGameId = ref<number | null>(null)
const bookingOpen = ref(false)
const participantName = ref('')
const participantEmail = ref('')
const participantPhone = ref('')
const bookingError = ref('')
const bookingResult = ref<BookingResult | null>(null)

/**
 * I codici già confermati restano qui, e nessuno li cancella: al tavolo un
 * telefono solo prenota per due o tre persone, e ogni codice si vede una
 * volta sola — aprire la modale per il tavolo successivo non può far
 * sparire quello di prima.
 */
const confirmed = ref<ConfirmedBooking[]>([])

async function load() {
  event.value = await api.get<EventDetail>(`/events/${eventId}`)
}

/**
 * Cosa si legge sulla riga del luogo: l'insegna se c'è, e sotto l'indirizzo
 * per intero — è quello che si copia in un navigatore, o si legge a voce a
 * chi sta guidando.
 */
const venueLines = computed(() => {
  const venue = event.value?.venue
  if (!venue) {
    return null
  }
  return { title: venue.name || venue.address, detail: venue.name ? venue.address : '' }
})

const hasStarted = computed(() => {
  if (!event.value) {
    return false
  }
  const startsAt = new Date(`${event.value.eventDate}T${event.value.startTime}`)
  return startsAt <= new Date()
})

/**
 * Il form riparte vuoto a ogni tavolo: chi prenota una seconda copia è
 * un'altra persona — un solo booking attivo per telefono — e ritrovare i
 * dati del compagno precompilati porta solo a prenotare a nome suo.
 */
function startBooking(eventGameId: number) {
  selectedEventGameId.value = eventGameId
  participantName.value = ''
  participantEmail.value = ''
  participantPhone.value = ''
  bookingError.value = ''
  bookingResult.value = null
  bookingOpen.value = true
}

const selectedGame = computed(
  () => event.value?.games.find((g) => g.eventGameId === selectedEventGameId.value) ?? null,
)

const selectedLabel = computed(() => (selectedGame.value ? copyLabel(selectedGame.value) : ''))

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

function isFull(g: EventGameInfo) {
  return g.remaining <= 0
}

/**
 * Quanti posti restano, e solo quando la risposta è un numero che serve:
 * su un tavolo aperto sapere se ne resta uno o quattro cambia se ti
 * siedi. Su una copia singola il numero è sempre 1 o 0 — un booleano
 * travestito da cifra — e chi guarda ha già il bottone "Prenota" o la
 * pastiglia "Al completo" per capirlo.
 */
function seatsLabel(g: EventGameInfo) {
  if (g.seats <= 1 || isFull(g)) {
    return ''
  }
  return g.remaining === 1
    ? 'Un posto prenotabile libero'
    : `${g.remaining} posti prenotabili liberi`
}

async function submitBooking() {
  bookingError.value = ''
  if (selectedEventGameId.value === null) {
    return
  }
  try {
    const result = await api.post<BookingResult>(`/events/${eventId}/bookings`, {
      eventGameId: selectedEventGameId.value,
      participantName: participantName.value,
      participantEmail: participantEmail.value,
      participantPhone: participantPhone.value,
    })
    bookingResult.value = result
    confirmed.value.push({
      code: result.bookingCode,
      label: selectedLabel.value,
      multiSeat: !!selectedGame.value && selectedGame.value.seats > 1,
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
      <router-link :to="{ name: 'events' }" class="back-link">&larr; Eventi</router-link>
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

      <div v-if="event.venue && venueLines" class="event-venue">
        <p class="event-venue-line">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M12 21s7-5.3 7-11a7 7 0 1 0-14 0c0 5.7 7 11 7 11Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linejoin="round"
            />
            <circle cx="12" cy="10" r="2.4" stroke="currentColor" stroke-width="1.6" />
          </svg>
          <span>
            {{ venueLines.title }}
            <span v-if="venueLines.detail" class="event-venue-address">{{ venueLines.detail }}</span>
          </span>
        </p>
        <EventMap
          v-if="event.venue.lat !== null && event.venue.lon !== null"
          :lat="event.venue.lat"
          :lon="event.venue.lon"
          :label="venueLines.title"
        />
      </div>

      <!-- Il codice si vede una volta sola: chiusa la modale resta qui, in
           cima al tavolo, finché la pagina non viene lasciata. -->
      <section v-if="confirmed.length > 0" class="booking-recap">
        <h2>{{ confirmed.length > 1 ? 'Le tue prenotazioni' : 'La tua prenotazione' }}</h2>
        <BookingConfirmation
          v-for="b in confirmed"
          :key="b.code"
          :game-label="b.label"
          :code="b.code"
          :multi-seat="b.multiSeat"
          :hint="false"
        />
        <p class="recap-hint">
          {{ confirmed.length > 1 ? 'Conservali' : 'Conservalo' }} per gestire la prenotazione o
          inserire il punteggio finale da "Gestisci prenotazione".
        </p>
      </section>

      <h2 class="table-heading">Al tavolo</h2>
      <p v-if="hasStarted" class="table-note">
        Questo evento è già iniziato: non è più possibile prenotare.
      </p>

      <ul class="event-games">
        <li v-for="g in event.games" :key="g.eventGameId" :class="{ 'is-full': isFull(g) }">
          <img
            v-if="g.coverPath"
            :src="`/api/uploads/${g.coverPath}`"
            :alt="g.name"
            width="300"
            height="400"
            loading="lazy"
            decoding="async"
          />
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
          <div class="event-game-body">
            <h3>{{ copyLabel(g) }}</h3>
            <GameDifficulty :weight="g.weight" />
            <p v-if="isFull(g)" class="seat-state">Al completo</p>
            <p v-else-if="seatsLabel(g)">{{ seatsLabel(g) }}</p>
          </div>
          <div class="event-game-actions">
            <button
              v-if="!hasStarted && !isFull(g)"
              type="button"
              @click="startBooking(g.eventGameId)"
            >
              Prenota
            </button>
            <router-link class="detail-link" :to="`/games/${g.gameId}`">
              Dettagli
              <span aria-hidden="true">&rarr;</span>
              <span class="visually-hidden">di {{ copyLabel(g) }}</span>
            </router-link>
          </div>
        </li>
      </ul>
      <p v-if="event.games.length === 0" class="empty-note">
        Per questa serata non è ancora stato messo in tavola nessun gioco.
      </p>
    </div>

    <div class="public-page" v-else-if="error">
      <router-link :to="{ name: 'events' }" class="back-link">&larr; Eventi</router-link>
      <p class="error">{{ error }}</p>
    </div>

    <ModalDialog
      :open="bookingOpen"
      :title="bookingResult ? 'Prenotazione confermata' : `Prenota: ${selectedLabel}`"
      @close="bookingOpen = false"
    >
      <template v-if="bookingResult">
        <BookingConfirmation
          :game-label="selectedLabel"
          :code="bookingResult.bookingCode"
          :multi-seat="!!selectedGame && selectedGame.seats > 1"
        />
        <div class="form-actions">
          <button type="button" @click="bookingOpen = false">Ho segnato il codice</button>
        </div>
      </template>
      <form v-else @submit.prevent="submitBooking">
        <label>
          Nome
          <!-- Il <dialog> nativo rispetta autofocus: al tavolo, da telefono,
               si prenota con la tastiera già aperta sul primo campo. -->
          <input v-model="participantName" autofocus required />
        </label>
        <label>
          Email
          <input v-model="participantEmail" type="email" required />
        </label>
        <label>
          Telefono
          <input v-model="participantPhone" required />
        </label>
        <p v-if="bookingError" class="error">{{ bookingError }}</p>
        <div class="form-actions">
          <button type="button" class="btn-secondary" @click="bookingOpen = false">Annulla</button>
          <button type="submit">Conferma prenotazione</button>
        </div>
      </form>
    </ModalDialog>
  </div>
</template>
