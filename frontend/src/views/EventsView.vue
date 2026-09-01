<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface EventSummary {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
}

const events = ref<EventSummary[]>([])
const error = ref('')

async function loadEvents() {
  try {
    events.value = await api.get<EventSummary[]>('/events')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadEvents)
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Prossimi eventi</h1>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="!error && events.length === 0">Nessun evento in programma.</p>
      <ul class="event-list">
        <li v-for="e in events" :key="e.id">
          <router-link :to="`/events/${e.id}`">
            <h2>{{ e.title }}</h2>
            <p>{{ e.eventDate }} · {{ e.startTime }}</p>
          </router-link>
        </li>
      </ul>
    </div>
  </div>
</template>
