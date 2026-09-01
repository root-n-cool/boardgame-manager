<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface EventSummary {
  id: number
  title: string
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
    <h1>Eventi</h1>
    <router-link to="/admin/events/new">Crea evento</router-link>
    <p v-if="error" class="error">{{ error }}</p>
    <ul class="event-grid">
      <li v-for="e in events" :key="e.id">
        <router-link :to="`/admin/events/${e.id}`">
          <h2>{{ e.title }}</h2>
          <p>{{ e.eventDate }} · {{ e.startTime }}</p>
        </router-link>
      </li>
    </ul>
  </div>
</template>
