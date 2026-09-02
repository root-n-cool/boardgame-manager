<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'

interface GameSummary {
  id: number
  name: string
}

interface SelectedGame {
  gameId: number
  quantity: number
}

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  quantity: number
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

interface BookingAdminInfo {
  id: number
  gameName: string
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
  bookingId: number
  participantName: string
  gameName: string
  players: MatchResultPlayer[]
}

const route = useRoute()
const eventId = route.params.id as string

const title = ref('')
const description = ref('')
const eventDate = ref('')
const startTime = ref('')
const error = ref('')
const saveMessage = ref('')

const availableGames = ref<GameSummary[]>([])
const selectedGames = ref<SelectedGame[]>([])
const bookings = ref<BookingAdminInfo[]>([])
const matchResults = ref<MatchResultAdminInfo[]>([])

function isSelected(gameId: number) {
  return selectedGames.value.some((g) => g.gameId === gameId)
}

function toggleGame(gameId: number, checked: boolean) {
  if (checked) {
    selectedGames.value.push({ gameId, quantity: 1 })
  } else {
    selectedGames.value = selectedGames.value.filter((g) => g.gameId !== gameId)
  }
}

function quantityFor(gameId: number) {
  return selectedGames.value.find((g) => g.gameId === gameId)?.quantity ?? 1
}

function setQuantity(gameId: number, quantity: number) {
  const entry = selectedGames.value.find((g) => g.gameId === gameId)
  if (entry) {
    entry.quantity = quantity
  }
}

async function load() {
  const [event, games] = await Promise.all([
    api.get<EventDetail>(`/events/${eventId}`),
    api.get<GameSummary[]>('/games'),
  ])
  title.value = event.title
  description.value = event.description || ''
  eventDate.value = event.eventDate
  startTime.value = event.startTime
  availableGames.value = games
  selectedGames.value = event.games.map((g) => ({ gameId: g.gameId, quantity: g.quantity }))
  bookings.value = await api.get<BookingAdminInfo[]>(`/events/${eventId}/bookings`)
  matchResults.value = await api.get<MatchResultAdminInfo[]>(`/events/${eventId}/match-results`)
}

async function saveEvent() {
  error.value = ''
  saveMessage.value = ''
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
    <h1>Modifica evento</h1>
    <form @submit.prevent="saveEvent">
      <label>
        Titolo
        <input v-model="title" required />
      </label>
      <label>
        Descrizione
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

      <fieldset>
        <legend>Giochi</legend>
        <div v-for="g in availableGames" :key="g.id" class="game-select-row">
          <label class="checkbox-label">
            <input
              type="checkbox"
              :checked="isSelected(g.id)"
              @change="toggleGame(g.id, ($event.target as HTMLInputElement).checked)"
            />
            {{ g.name }}
          </label>
          <input
            v-if="isSelected(g.id)"
            type="number"
            min="1"
            :value="quantityFor(g.id)"
            @input="setQuantity(g.id, Number(($event.target as HTMLInputElement).value))"
          />
        </div>
      </fieldset>

      <button type="submit">Salva</button>
      <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>

    <h2>Prenotazioni attive</h2>
    <p v-if="bookings.length === 0">Nessuna prenotazione ancora.</p>
    <ul>
      <li v-for="b in bookings" :key="b.id">
        {{ b.participantName }} — {{ b.gameName }} — {{ b.participantEmail }} — {{ b.participantPhone }}
      </li>
    </ul>

    <h2>Risultati inseriti</h2>
    <p v-if="matchResults.length === 0">Nessun punteggio inserito ancora.</p>
    <ul>
      <li v-for="m in matchResults" :key="m.bookingId" class="list-row-text">
        {{ m.participantName }} — {{ m.gameName }}:
        <span v-for="(p, index) in m.players" :key="index">
          {{ p.name }} {{ p.score }}{{ index < m.players.length - 1 ? ', ' : '' }}
        </span>
      </li>
    </ul>
  </div>
</template>
