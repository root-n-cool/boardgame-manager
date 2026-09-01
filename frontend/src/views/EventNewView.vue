<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface GameSummary {
  id: number
  name: string
}

interface SelectedGame {
  gameId: number
  quantity: number
}

const router = useRouter()

const title = ref('')
const description = ref('')
const eventDate = ref('')
const startTime = ref('')
const error = ref('')

const availableGames = ref<GameSummary[]>([])
const selectedGames = ref<SelectedGame[]>([])

async function loadGames() {
  availableGames.value = await api.get<GameSummary[]>('/games')
}

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

async function createEvent() {
  error.value = ''
  try {
    const event = await api.post<{ id: number }>('/events', {
      title: title.value,
      description: description.value || null,
      eventDate: eventDate.value,
      startTime: startTime.value,
      games: selectedGames.value,
    })
    router.push(`/admin/events/${event.id}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadGames)
</script>

<template>
  <div>
    <h1>Crea evento</h1>
    <form @submit.prevent="createEvent">
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
          <label>
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

      <button type="submit">Crea</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
