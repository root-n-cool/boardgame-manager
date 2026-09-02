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
    <p class="page-meta">
      {{ events.length === 0 ? 'Nessun evento creato' : events.length === 1 ? '1 evento' : `${events.length} eventi` }}
    </p>
    <router-link to="/admin/events/new" class="action-link">Crea evento</router-link>
    <p v-if="error" class="error">{{ error }}</p>
    <ul class="event-grid">
      <li v-for="e in events" :key="e.id">
        <router-link :to="`/admin/events/${e.id}`">
          <h2>{{ e.title }}</h2>
          <p>
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <rect x="3" y="5" width="18" height="16" rx="2" stroke="currentColor" stroke-width="1.6" />
              <path d="M3 9.5h18" stroke="currentColor" stroke-width="1.6" />
              <path d="M8 3v4M16 3v4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
            </svg>
            {{ e.eventDate }} · {{ e.startTime }}
          </p>
        </router-link>
      </li>
    </ul>
  </div>
</template>
