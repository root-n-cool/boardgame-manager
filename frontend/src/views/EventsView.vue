<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'
import { formatEventDateTime } from '../utils/dates'

interface EventSummary {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
  imagePath: string | null
  venue: { name: string; address: string; lat: number | null; lon: number | null } | null
  gamesCount: number
}

/** Sulla card sta l'insegna del posto, o la sua prima riga d'indirizzo. */
function venueLabel(venue: EventSummary['venue']) {
  if (!venue) {
    return ''
  }
  return venue.name || venue.address.split(',').slice(0, 2).join(',').trim()
}

interface EventListResponse {
  items: EventSummary[]
}

const events = ref<EventSummary[]>([])
const error = ref('')

async function loadEvents() {
  try {
    const res = await api.get<EventListResponse>('/events')
    events.value = res.items
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
      <p v-if="!error" class="page-meta">
        {{ events.length === 0 ? 'Nessun evento in programma' : events.length === 1 ? '1 evento in programma' : `${events.length} eventi in programma` }}
      </p>
      <p v-if="error" class="error">{{ error }}</p>
      <ul role="list" class="game-grid event-card-grid">
        <li v-for="e in events" :key="e.id">
          <router-link :to="`/events/${e.id}`">
            <img
              v-if="e.imagePath"
              :src="`/api/uploads/${e.imagePath}`"
              alt=""
              width="480"
              height="270"
              loading="lazy"
              decoding="async"
            />
            <div v-else class="event-image-placeholder" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none">
                <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
                <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
                <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
                <circle cx="12" cy="12" r="1.3" fill="currentColor" />
                <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
                <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
              </svg>
            </div>
            <h2>{{ e.title }}</h2>
            <p class="event-card-date">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <rect x="3" y="5" width="18" height="16" rx="2" stroke="currentColor" stroke-width="1.6" />
                <path d="M3 9.5h18" stroke="currentColor" stroke-width="1.6" />
                <path d="M8 3v4M16 3v4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
              </svg>
              {{ formatEventDateTime(e.eventDate, e.startTime) }}
            </p>
            <p v-if="e.venue" class="event-card-venue">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="M12 21s7-5.3 7-11a7 7 0 1 0-14 0c0 5.7 7 11 7 11Z"
                  stroke="currentColor"
                  stroke-width="1.6"
                  stroke-linejoin="round"
                />
                <circle cx="12" cy="10" r="2.4" stroke="currentColor" stroke-width="1.6" />
              </svg>
              <span class="event-card-venue-name">{{ venueLabel(e.venue) }}</span>
            </p>
            <p class="event-card-games">
              {{ e.gamesCount === 1 ? '1 gioco al tavolo' : `${e.gamesCount} giochi al tavolo` }}
            </p>
          </router-link>
        </li>
      </ul>
    </div>
  </div>
</template>
