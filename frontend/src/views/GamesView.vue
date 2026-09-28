<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import QrPrintLink from '../components/QrPrintLink.vue'

interface GameSummary {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
  /**
   * Presente solo quando la richiesta ha una sessione admin (assente per il
   * catalogo pubblico): questa vista legge la rotta pubblica `/games`, da
   * cui il campo opzionale.
   */
  incomplete?: boolean
  /** Anche questo solo con la sessione admin: il gioco non è nel catalogo pubblico. */
  hiddenFromCatalog?: boolean
}

const games = ref<GameSummary[]>([])
const error = ref('')

async function loadGames() {
  try {
    games.value = await api.get<GameSummary[]>('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadGames)
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Catalogo giochi</h1>
        <p class="page-meta">
          {{ games.length === 0 ? 'Nessun gioco in catalogo' : games.length === 1 ? '1 gioco' : `${games.length} giochi` }}
        </p>
      </div>
      <div class="page-head-actions">
        <QrPrintLink
          v-if="games.length"
          :to="{ name: 'admin-games-qr' }"
          label="Stampa i QR di tutti i giochi, nove per foglio"
        />
        <router-link :to="{ name: 'admin-game-new' }" class="action-link is-compact">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
          Aggiungi gioco
        </router-link>
      </div>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="game-grid">
      <ul role="list" class="game-grid-items">
        <li v-for="g in games" :key="g.id">
          <router-link :to="{ name: 'admin-game-detail', params: { id: g.id } }">
            <span v-if="g.incomplete || g.hiddenFromCatalog" class="state-chips">
              <span v-if="g.incomplete" class="state-chip is-inline is-danger">Incompleto</span>
              <span v-if="g.hiddenFromCatalog" class="state-chip is-inline is-muted">Nascosto</span>
            </span>
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
            <h2>{{ g.name }}</h2>
            <p v-if="g.year">{{ g.year }}</p>
            <p v-if="g.owner">Proprietario: {{ g.owner }}</p>
          </router-link>
        </li>
      </ul>
      <router-link :to="{ name: 'admin-game-new' }" class="add-card">
        <span class="add-slot" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none">
            <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          </svg>
        </span>
        <span class="add-label">Aggiungi gioco</span>
      </router-link>
    </div>
  </div>
</template>
