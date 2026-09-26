<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import BookingConfirmation from '../components/BookingConfirmation.vue'
import GameDifficulty from '../components/GameDifficulty.vue'
import MarkdownText from '../components/MarkdownText.vue'
import ModalDialog from '../components/ModalDialog.vue'
import { formatEventDateTime } from '../utils/dates'
import type { ChatAvailability } from '../utils/game'
import { listMyBookings, removeMyBooking, saveMyBooking, type MyBooking } from '../utils/myBookings'

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  copyIndex: number
  seats: number
  remaining: number
  weight: number | null
  bookable: boolean
  /**
   * Quali agenti stanno dietro il link "Chiedi al Mentore" (manuale
   * preparato / chiave Tavily + provider AI configurati). Nessuno dei due =
   * niente link: senza, il link portava alla scheda del gioco e non
   * succedeva niente — al tavolo si legge come un'app rotta.
   */
  chat: ChatAvailability
}

interface EventDetail {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
  endTime: string | null
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
  mailQueued: boolean
}

/** Una prenotazione andata a buon fine in questa visita alla pagina. */
interface ConfirmedBooking {
  code: string
  label: string
  multiSeat: boolean
  /** Se per questa prenotazione è partita davvero una mail. */
  mailed: boolean
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
const termsAccepted = ref(false)
const bookingError = ref('')
const bookingResult = ref<BookingResult | null>(null)
const chipActionError = ref('')

/**
 * I codici già confermati restano qui, e nessuno li cancella: al tavolo un
 * telefono solo prenota per due o tre persone, e ogni codice si vede una
 * volta sola — aprire la modale per il tavolo successivo non può far
 * sparire quello di prima.
 */
const confirmed = ref<ConfirmedBooking[]>([])

/**
 * Le prenotazioni di questo evento che il browser ricorda di aver già
 * fatto: sostituiscono, lato client, il vecchio vincolo server "un
 * booking attivo per telefono" — qui non è un'enforcement, solo un
 * promemoria per non riprenotare lo stesso tavolo per errore.
 */
const myBookingsForEvent = ref<MyBooking[]>(
  listMyBookings().filter((b) => b.eventId === Number(eventId)),
)

function refreshMyBookings() {
  myBookingsForEvent.value = listMyBookings().filter((b) => b.eventId === Number(eventId))
}

function myBookingFor(g: EventGameInfo): MyBooking | null {
  return myBookingsForEvent.value.find((b) => b.eventGameId === g.eventGameId) ?? null
}

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
 * un'altra persona, e ritrovare i dati del compagno precompilati (nome,
 * email, consenso già spuntato) porta solo a prenotare a nome suo.
 */
function startBooking(eventGameId: number) {
  selectedEventGameId.value = eventGameId
  participantName.value = ''
  participantEmail.value = ''
  termsAccepted.value = false
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

/**
 * Un gioco che la serata porta ma non mette a prenotazione: si vede —
 * "stasera c'è anche Love Letter" è informazione utile — ma non ha un
 * bottone, perché non c'è niente da prenotare.
 */
function tableOnly(g: EventGameInfo) {
  return !g.bookable
}

async function submitBooking() {
  bookingError.value = ''
  const eventGameId = selectedEventGameId.value
  if (eventGameId === null) {
    return
  }
  try {
    const result = await api.post<BookingResult>(`/events/${eventId}/bookings`, {
      eventGameId,
      participantName: participantName.value,
      participantEmail: participantEmail.value,
      termsAccepted: termsAccepted.value,
    })
    bookingResult.value = result
    const multiSeat = !!selectedGame.value && selectedGame.value.seats > 1
    confirmed.value.push({
      code: result.bookingCode,
      label: selectedLabel.value,
      multiSeat,
      mailed: result.mailQueued,
    })
    saveMyBooking({
      id: result.id,
      bookingCode: result.bookingCode,
      eventId: Number(eventId),
      eventGameId,
      gameLabel: selectedLabel.value,
    })
    refreshMyBookings()
    await load()
  } catch (e) {
    bookingError.value = (e as Error).message
  }
}

/** Annulla dalla pastiglia "Prenotato" sulla scheda evento, non dalla modale. */
async function cancelMyBooking(entry: MyBooking) {
  if (!window.confirm(`Annullare la prenotazione per ${entry.gameLabel}?`)) {
    return
  }
  chipActionError.value = ''
  try {
    await api.post(`/bookings/${entry.id}/cancel`, { bookingCode: entry.bookingCode })
    removeMyBooking(entry.id)
    refreshMyBookings()
    await load()
  } catch (e) {
    // Se non è più attiva (già annullata altrove — dall'admin, o da
    // "Gestisci prenotazione" su un altro dispositivo), il promemoria
    // locale è ormai falso: va tolto comunque, non lasciato a mostrare
    // "Prenotato" per un tavolo che si può riprenotare.
    removeMyBooking(entry.id)
    refreshMyBookings()
    chipActionError.value = (e as Error).message
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
  <div v-if="event">
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
    <MarkdownText v-if="event.description" :text="event.description" />
    <p class="event-card-date">
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <rect x="3" y="5" width="18" height="16" rx="2" stroke="currentColor" stroke-width="1.6" />
        <path d="M3 9.5h18" stroke="currentColor" stroke-width="1.6" />
        <path d="M8 3v4M16 3v4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
      </svg>
      <span>{{ formatEventDateTime(event) }}</span>
      <!-- Un link semplice, niente download: sul telefono il file .ics apre
           direttamente il calendario, che chiede se aggiungere la serata. -->
      <a
        v-if="!hasStarted"
        :href="`/api/events/${event.id}/calendar.ics`"
        class="event-calendar-link"
      >
        Aggiungi al calendario
      </a>
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
        :mailed="b.mailed"
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
    <p v-if="chipActionError" class="error">{{ chipActionError }}</p>

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
          <p v-if="tableOnly(g)" class="seat-state">Senza prenotazione</p>
          <p v-else-if="isFull(g)" class="seat-state">Al completo</p>
          <p v-else-if="seatsLabel(g)">{{ seatsLabel(g) }}</p>
          <!-- Il link al manuale sta QUI e non fra le azioni in fondo alla
               card: le azioni sono ancorate al fondo (`margin-top: auto`) per
               tenere allineata la fila dei "Prenota", e una voce che c'è solo
               su alcune schede spingerebbe quel bottone più in alto proprio
               sulle schede col manuale. Qui è anche il posto giusto per
               senso: è un'informazione sul gioco, non un passo della
               prenotazione. -->
          <p v-if="g.chat.rules || g.chat.strategy" class="event-game-ask">
            <router-link :to="{ path: `/games/${g.gameId}`, query: { chat: '1' } }">
              Dubbi o consigli? Chiedi al Mentore
              <span class="visually-hidden">di {{ copyLabel(g) }}</span>
            </router-link>
          </p>
        </div>
        <div class="event-game-actions">
          <template v-if="myBookingFor(g)">
            <span class="status-badge status-active">Prenotato</span>
            <button type="button" class="btn-danger" @click="cancelMyBooking(myBookingFor(g)!)">
              Annulla prenotazione
            </button>
            <router-link
              class="detail-link"
              :to="{ name: 'booking-score', params: { code: myBookingFor(g)!.bookingCode } }"
            >
              Aggiungi risultato
            </router-link>
          </template>
          <button
            v-if="!hasStarted && !isFull(g) && !tableOnly(g)"
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
    <p v-if="event.games.some(tableOnly)" class="table-note">
      I giochi segnati "Senza prenotazione" sono a disposizione al tavolo:
      chiedili all'organizzatore quando arrivi.
    </p>
    <p v-if="event.games.length === 0" class="empty-note">
      Per questa serata non è ancora stato messo in tavola nessun gioco.
    </p>
  </div>

  <div v-else-if="error">
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
        :mailed="bookingResult.mailQueued"
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
        <input v-model="participantEmail" type="email" />
      </label>
      <p class="field-hint">
        Facoltativa: verrà usata solo per inviarti la conferma della prenotazione.
      </p>
      <label class="checkbox-label booking-consent">
        <input v-model="termsAccepted" type="checkbox" required />
        Accetto i
        <router-link :to="{ name: 'terms' }" target="_blank">termini e condizioni</router-link>
        e ho letto l'<router-link :to="{ name: 'privacy' }" target="_blank">informativa privacy</router-link>
      </label>
      <p v-if="bookingError" class="error">{{ bookingError }}</p>
      <div class="form-actions">
        <button type="button" class="btn-secondary" @click="bookingOpen = false">Annulla</button>
        <button type="submit">Conferma prenotazione</button>
      </div>
    </form>
  </ModalDialog>
</template>
