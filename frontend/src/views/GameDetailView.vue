<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'
import PublicHeader from '../components/PublicHeader.vue'
import GameFacts from '../components/GameFacts.vue'
import GameMediaList from '../components/GameMediaList.vue'
import type { GameDetail, GameLanguageInfo } from '../utils/game'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const gameId = route.params.id as string

const game = ref<GameDetail | null>(null)
const error = ref('')
const activeLangCode = ref('')

function activeLanguage(): GameLanguageInfo | undefined {
  return game.value?.languages.find((l) => l.code === activeLangCode.value)
}

// Qui si arriva da una scheda evento o da un link condiviso al tavolo: il
// rimando indietro segue la storia del browser, e solo se non c'è nulla a
// cui tornare porta al calendario.
function goBack() {
  if (window.history.state?.back) {
    router.back()
  } else {
    router.push({ name: 'events' })
  }
}

onMounted(async () => {
  try {
    game.value = await api.get<GameDetail>(`/games/${gameId}`)
    if (game.value.languages.length > 0) {
      activeLangCode.value = game.value.languages[0].code
    }
  } catch (e) {
    // Il messaggio dell'API è inglese e tecnico ("game not found"): su una
    // pagina che si apre da un QR o da un link vecchio serve una frase che
    // dica cosa fare, non cosa è andato storto nel backend.
    console.error('caricamento scheda gioco', e)
    error.value = 'Questa scheda non è disponibile: il gioco potrebbe non essere più in catalogo.'
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page" v-if="game">
      <button type="button" class="back-link" @click="goBack">&larr; Indietro</button>

      <h1>{{ game.name }}</h1>
      <p class="page-meta">
        <template v-if="game.owner">Proprietario: {{ game.owner }} · </template>
        <router-link :to="`/games/${game.id}/leaderboard`">Classifica</router-link>
        <!-- Questa pagina non ha un'azione principale: chi la guarda legge.
             La scorciatoia alla scheda di modifica vale solo per l'admin già
             loggato, quindi sta tra i metadati come gli altri rimandi, non in
             testa dove sarebbe la chiamata all'azione di tutti. -->
        <template v-if="auth.user">
          ·
          <router-link :to="{ name: 'admin-game-detail', params: { id: game.id } }">
            Modifica
          </router-link>
        </template>
      </p>

      <div class="game-cover-card">
        <img
          v-if="game.coverPath"
          :src="`/api/uploads/${game.coverPath}`"
          :alt="`Copertina di ${game.name}`"
          class="cover"
          width="170"
          height="227"
          decoding="async"
        />
        <div v-else class="cover cover-empty" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none">
            <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
            <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
            <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
            <circle cx="12" cy="12" r="1.3" fill="currentColor" />
            <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
            <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
          </svg>
        </div>

        <div class="game-cover-info">
          <GameFacts :game="game" />
          <p v-if="game.seats > 1" class="row-meta">
            Tavolo da {{ game.seats }} posti prenotabili.
          </p>
        </div>
      </div>

      <nav class="tab-bar language-tabs">
        <button
          v-for="l in game.languages"
          :key="l.code"
          type="button"
          :class="{ active: l.code === activeLangCode }"
          @click="activeLangCode = l.code"
        >
          {{ l.code }}
          <svg
            v-if="l.isBaseLanguage"
            class="base-language-badge"
            viewBox="0 0 24 24"
            fill="none"
            aria-label="Lingua base"
          >
            <path
              d="M12 3.5l2.47 5.77 6.24.56-4.73 4.16 1.42 6.1L12 16.9l-5.4 3.2 1.42-6.1-4.73-4.16 6.24-.56L12 3.5Z"
              fill="currentColor"
            />
          </svg>
        </button>
      </nav>

      <section class="panel-card">
        <div class="section-head">
          <h2>Scheda</h2>
          <span class="lang-chip">{{ activeLangCode }}</span>
        </div>
        <h3 class="language-name">{{ activeLanguage()?.name }}</h3>
        <p v-if="activeLanguage()?.description">{{ activeLanguage()?.description }}</p>
        <p v-else class="empty-note">Nessuna descrizione per questa lingua.</p>
      </section>

      <section class="panel-card">
        <div class="section-head">
          <h2>Media</h2>
          <span class="lang-chip">{{ activeLangCode }}</span>
        </div>
        <GameMediaList :media="activeLanguage()?.media || []" />
      </section>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
    <div class="public-page" v-else-if="error">
      <button type="button" class="back-link" @click="goBack">&larr; Indietro</button>
      <p class="error">{{ error }}</p>
      <p><router-link :to="{ name: 'events' }">Vedi i prossimi eventi</router-link></p>
    </div>
  </div>
</template>
