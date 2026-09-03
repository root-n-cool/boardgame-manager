<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import EventImagePicker from '../components/EventImagePicker.vue'
import EventGamesPicker, { type PickerGame, type SelectedGame } from '../components/EventGamesPicker.vue'
import { formatEventDateTime } from '../utils/dates'

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
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

interface BookingAdminInfo {
  id: number
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  seats: number
  participantName: string
  participantEmail: string
  participantPhone: string
  createdAt: string
}

interface MatchResultPlayer {
  name: string
  score: number
}

interface MatchResultAdminInfo {
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  players: MatchResultPlayer[]
}

const route = useRoute()
const router = useRouter()
const eventId = route.params.id as string

const title = ref('')
const description = ref('')
const eventDate = ref('')
const startTime = ref('')
const error = ref('')
const saveMessage = ref('')
const saving = ref(false)

const imagePath = ref<string | null>(null)
const imageUploading = ref(false)
const imageError = ref('')

const availableGames = ref<PickerGame[]>([])
const selectedGames = ref<SelectedGame[]>([])
/** Copie con almeno una prenotazione: sotto quel numero non si scende. */
const occupiedCopiesByGame = ref<Record<number, number>>({})
/** Quante copie di ogni gioco ha l'evento: serve a numerarle solo se >1. */
const copiesByGame = ref<Record<number, number>>({})
const bookings = ref<BookingAdminInfo[]>([])
const matchResults = ref<MatchResultAdminInfo[]>([])
const bookingError = ref('')

/** Iniziale del partecipante per la pedina della riga, come in /users. */
function initial(name: string) {
  return name.trim().charAt(0) || '?'
}

/** Il titolo salvato, non quello in corso di modifica nel campo. */
const eventTitle = ref('')
const eventWhen = ref('')

const chosenLabel = computed(() =>
  selectedGames.value.length === 1 ? '1 scelto' : `${selectedGames.value.length} scelti`,
)

/**
 * L'etichetta di una copia: il numero compare solo quando quel gioco ha
 * più di una copia nell'evento, così un evento normale non si riempie di
 * "#1" inutili.
 */
function copyLabel(gameId: number, gameName: string, copyIndex: number) {
  return (copiesByGame.value[gameId] ?? 1) > 1 ? `${gameName} #${copyIndex}` : gameName
}

/** Prenotazioni raggruppate per copia, nell'ordine in cui arrivano. */
const bookingsByCopy = computed(() => {
  const groups: { eventGameId: number; label: string; seats: number; rows: BookingAdminInfo[] }[] = []
  for (const b of bookings.value) {
    let group = groups.find((g) => g.eventGameId === b.eventGameId)
    if (!group) {
      group = {
        eventGameId: b.eventGameId,
        label: copyLabel(b.gameId, b.gameName, b.copyIndex),
        seats: b.seats,
        rows: [],
      }
      groups.push(group)
    }
    group.rows.push(b)
  }
  return groups
})

async function load() {
  const [event, games] = await Promise.all([
    api.get<EventDetail>(`/events/${eventId}`),
    api.get<PickerGame[]>('/games'),
  ])
  title.value = event.title
  description.value = event.description || ''
  eventDate.value = event.eventDate
  startTime.value = event.startTime
  imagePath.value = event.imagePath
  eventTitle.value = event.title
  eventWhen.value = formatEventDateTime(event.eventDate, event.startTime)
  availableGames.value = games
  const copies: Record<number, number> = {}
  const occupied: Record<number, number> = {}
  for (const g of event.games) {
    copies[g.gameId] = (copies[g.gameId] ?? 0) + 1
    if (g.seats - g.remaining > 0) {
      occupied[g.gameId] = (occupied[g.gameId] ?? 0) + 1
    }
  }
  copiesByGame.value = copies
  occupiedCopiesByGame.value = occupied
  selectedGames.value = Object.entries(copies).map(([gameId, count]) => ({
    gameId: Number(gameId),
    copies: count,
  }))
  bookings.value = await api.get<BookingAdminInfo[]>(`/events/${eventId}/bookings`)
  matchResults.value = await api.get<MatchResultAdminInfo[]>(`/events/${eventId}/match-results`)
}

// L'immagine ha un endpoint suo e si salva da sé: non passa dal form, così
// non c'è un campo file che aspetta un "Salva" per fare una cosa che può
// fare subito.
async function uploadImage(file: File) {
  imageError.value = ''
  imageUploading.value = true
  try {
    const body = new FormData()
    body.append('file', file)
    const updated = await api.post<{ imagePath: string | null }>(`/events/${eventId}/image`, body)
    imagePath.value = updated.imagePath
  } catch (e) {
    imageError.value = (e as Error).message
  } finally {
    imageUploading.value = false
  }
}

async function saveEvent() {
  error.value = ''
  saveMessage.value = ''
  saving.value = true
  try {
    await api.put(`/events/${eventId}`, {
      title: title.value,
      description: description.value || null,
      eventDate: eventDate.value,
      startTime: startTime.value,
      games: selectedGames.value,
    })
    saveMessage.value = 'Salvato'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

// Annullare una prenotazione libera un posto prenotabile sul tavolo; il
// punteggio, se c'è, resta perché appartiene al tavolo e non a chi l'ha
// inserito — sparisce solo se quella era l'ultima prenotazione rimasta su
// quella copia. Quello che cambia non è solo la lista, quindi si ricarica
// tutta la scheda.
async function cancelBooking(booking: BookingAdminInfo) {
  const confirmed = window.confirm(
    `Annullare la prenotazione di ${booking.participantName} per ${booking.gameName}? ` +
      'Il posto prenotabile torna libero. Il punteggio del tavolo resta, a meno che non fosse la sua ultima prenotazione.',
  )
  if (!confirmed) {
    return
  }
  bookingError.value = ''
  try {
    await api.delete(`/bookings/${booking.id}`)
    await load()
  } catch (e) {
    bookingError.value = (e as Error).message
  }
}

async function deleteEvent() {
  const confirmed = window.confirm(
    `Eliminare "${eventTitle.value}"? Spariscono anche le sue prenotazioni e i punteggi inseriti. ` +
      "L'operazione non è reversibile.",
  )
  if (!confirmed) {
    return
  }
  try {
    await api.delete(`/events/${eventId}`)
    router.push('/admin/events')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(async () => {
  // Un'immagine rifiutata durante la creazione dell'evento arriva come query
  // string: l'errore si mostra qui, dov'è possibile riprovare, e la query
  // sparisce subito perché un ricaricamento non la ripeta.
  const failedImage = route.query.imageError
  if (typeof failedImage === 'string' && failedImage !== '') {
    imageError.value = `Immagine non caricata: ${failedImage}`
    router.replace({ path: route.path, query: {} })
  }
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <router-link to="/admin/events" class="back-link">&larr; Eventi</router-link>

    <div class="page-head">
      <div class="page-head-text">
        <h1>{{ eventTitle || 'Evento' }}</h1>
        <p v-if="eventWhen" class="page-meta">{{ eventWhen }}</p>
      </div>
      <div class="page-head-actions">
        <a
          class="action-link is-compact"
          :href="`/events/${eventId}`"
          target="_blank"
          rel="noopener"
          aria-label="Vedi la pagina pubblica dell'evento (si apre in una nuova scheda)"
        >
          Vedi pagina pubblica
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M14 4h6v6M20 4l-8.5 8.5M18 14v4.5c0 .83-.67 1.5-1.5 1.5h-11c-.83 0-1.5-.67-1.5-1.5v-11C4 6.67 4.67 6 5.5 6H10"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </a>
        <button type="button" class="btn-danger is-compact" @click="deleteEvent">
          Elimina
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M5 7h14M10 7V5.5c0-.55.45-1 1-1h2c.55 0 1 .45 1 1V7M6.5 7l.7 11.1c.05.79.7 1.4 1.5 1.4h6.6c.8 0 1.45-.61 1.5-1.4L17.5 7"
              stroke="currentColor"
              stroke-width="1.7"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>
      </div>
    </div>

    <form class="panel-form" @submit.prevent="saveEvent">
      <div class="panel-card">
        <div class="section-head">
          <h2>Dettagli</h2>
        </div>

        <div class="field-block">
          <span class="field-label">Immagine <span class="field-optional">(opzionale)</span></span>
          <EventImagePicker
            :src="imagePath ? `/api/uploads/${imagePath}` : null"
            :alt="`Immagine di ${eventTitle}`"
            :uploading="imageUploading"
            @select="uploadImage"
          />
          <p class="field-hint">JPEG, PNG o WebP, fino a 5MB.</p>
          <p v-if="imageError" class="error">{{ imageError }}</p>
        </div>

        <label>
          Titolo
          <input v-model="title" required />
        </label>
        <label>
          <span>Descrizione <span class="field-optional">(opzionale)</span></span>
          <textarea v-model="description"></textarea>
        </label>
        <label>
          Data
          <input v-model="eventDate" type="date" required />
        </label>
        <label>
          Ora
          <input v-model="startTime" type="time" required />
        </label>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Giochi dell'evento</h2>
          <span class="section-count">{{ chosenLabel }}</span>
        </div>
        <EventGamesPicker
          v-model="selectedGames"
          :games="availableGames"
          :occupied-copies="occupiedCopiesByGame"
          :existing-copies="copiesByGame"
        />
      </div>

      <div class="form-actions">
        <button type="submit" :disabled="saving">{{ saving ? 'Salvataggio…' : 'Salva' }}</button>
      </div>
      <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>

    <div class="panel-card">
      <div class="section-head">
        <h2>Prenotazioni</h2>
        <span class="section-count">{{ bookings.length }}</span>
      </div>
      <p v-if="bookings.length === 0" class="empty-note">
        Nessuna prenotazione ancora: compaiono qui appena qualcuno prenota dalla pagina pubblica.
      </p>
      <template v-else>
        <div v-for="group in bookingsByCopy" :key="group.eventGameId" class="booking-copy-group">
          <h3 class="booking-copy-head">
            {{ group.label }}
            <span v-if="group.seats > 1" class="row-meta"
              >· {{ group.rows.length }} di {{ group.seats }} posti prenotabili</span
            >
          </h3>
          <ul role="list" class="admin-list">
            <li v-for="b in group.rows" :key="b.id">
              <div class="admin-row">
                <span class="admin-pawn" aria-hidden="true">{{ initial(b.participantName) }}</span>
                <span class="admin-email booking-who">
                  {{ b.participantName }}
                  <span class="row-meta">{{ b.participantEmail }} · {{ b.participantPhone }}</span>
                </span>
                <div class="admin-row-actions">
                  <button type="button" @click="cancelBooking(b)">Annulla</button>
                </div>
              </div>
            </li>
          </ul>
        </div>
      </template>
      <p v-if="bookingError" class="error">{{ bookingError }}</p>
    </div>

    <div class="panel-card">
      <div class="section-head">
        <h2>Risultati</h2>
        <span class="section-count">{{ matchResults.length }}</span>
      </div>
      <p v-if="matchResults.length === 0" class="empty-note">
        Nessun punteggio inserito ancora: li inseriscono i partecipanti col loro codice, a fine partita.
      </p>
      <ul v-else role="list" class="admin-list">
        <li v-for="m in matchResults" :key="m.eventGameId">
          <div class="admin-row">
            <span class="admin-email booking-who">
              {{ copyLabel(m.gameId, m.gameName, m.copyIndex) }}
              <span class="row-meta match-scores">
                <span v-for="(p, index) in m.players" :key="index">
                  {{ p.name }} {{ p.score }}{{ index < m.players.length - 1 ? ' · ' : '' }}
                </span>
              </span>
            </span>
            <router-link class="booking-game" :to="`/games/${m.gameId}`">Scheda</router-link>
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>
