<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import { formatEventDateTime } from '../utils/dates'

interface EventListItem {
  id: number
  title: string
  eventDate: string
  startTime: string
  imagePath: string | null
  gamesCount: number
}

interface EventListResponse {
  items: EventListItem[]
  total: number
  page: number
  /** 0 = tutto su una pagina: la lista "in programma" non è paginata. */
  pageSize: number
}

type Scope = 'upcoming' | 'past'

const scope = ref<Scope>('upcoming')
const events = ref<EventListItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(0)
const loading = ref(false)
const error = ref('')

const showingPast = computed(() => scope.value === 'past')

const pageCount = computed(() =>
  pageSize.value > 0 ? Math.max(1, Math.ceil(total.value / pageSize.value)) : 1,
)

const countLabel = computed(() => {
  if (showingPast.value) {
    if (total.value === 0) return 'Nessun evento passato'
    return total.value === 1 ? '1 evento passato' : `${total.value} eventi passati`
  }
  if (total.value === 0) return 'Nessun evento in programma'
  return total.value === 1 ? '1 evento in programma' : `${total.value} eventi in programma`
})

async function loadEvents() {
  loading.value = true
  error.value = ''
  try {
    const query = showingPast.value ? `?past=true&page=${page.value}` : ''
    const res = await api.get<EventListResponse>(`/events${query}`)
    events.value = res.items
    total.value = res.total
    page.value = res.page
    pageSize.value = res.pageSize
  } catch (e) {
    events.value = []
    total.value = 0
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

function selectScope(next: Scope) {
  if (scope.value === next) return
  scope.value = next
  page.value = 1
  loadEvents()
}

function goToPage(next: number) {
  if (next < 1 || next > pageCount.value) return
  page.value = next
  loadEvents()
}

onMounted(loadEvents)
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Eventi</h1>
        <p class="page-meta" aria-live="polite">{{ countLabel }}</p>
      </div>
      <router-link to="/admin/events/new" class="action-link is-compact">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
        </svg>
        Crea evento
      </router-link>
    </div>

    <!-- Due insiemi mutuamente esclusivi, non due filtri componibili: le
         linguette dicono quale dei due si sta guardando. -->
    <nav class="tab-bar is-inline" aria-label="Periodo">
      <button
        type="button"
        :class="{ active: !showingPast }"
        :aria-pressed="!showingPast"
        @click="selectScope('upcoming')"
      >
        In programma
      </button>
      <button
        type="button"
        :class="{ active: showingPast }"
        :aria-pressed="showingPast"
        @click="selectScope('past')"
      >
        Passati
      </button>
    </nav>

    <p v-if="error" class="error">{{ error }}</p>

    <div class="game-grid event-card-grid" :aria-busy="loading">
      <ul role="list" class="game-grid-items">
        <li v-for="e in events" :key="e.id">
          <router-link :to="`/admin/events/${e.id}`">
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
            <p class="event-card-games">
              {{ e.gamesCount === 1 ? '1 gioco' : `${e.gamesCount} giochi` }}
            </p>
          </router-link>
        </li>
      </ul>
      <!-- Lo slot libero chiude la griglia solo dove si può aggiungere: negli
           eventi passati non si crea niente. -->
      <router-link v-if="!showingPast" to="/admin/events/new" class="add-card">
        <span class="add-slot" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none">
            <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          </svg>
        </span>
        <span class="add-label">Crea evento</span>
      </router-link>
    </div>

    <p v-if="showingPast && !loading && !error && total === 0" class="empty-note">
      Nessun evento è ancora passato: quelli conclusi finiscono qui.
    </p>

    <nav v-if="pageCount > 1" class="pager" aria-label="Pagine degli eventi passati">
      <button type="button" :disabled="page <= 1" @click="goToPage(page - 1)">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M15 5l-7 7 7 7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        Precedente
      </button>
      <span class="pager-position">Pagina {{ page }} di {{ pageCount }}</span>
      <button type="button" :disabled="page >= pageCount" @click="goToPage(page + 1)">
        Successiva
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M9 5l7 7-7 7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    </nav>
  </div>
</template>
