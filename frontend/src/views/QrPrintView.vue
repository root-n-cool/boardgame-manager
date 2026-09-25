<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import { useSiteStore } from '../stores/site'
import { formatEventDateTime, type EventWhen } from '../utils/dates'

/**
 * Il cartellino da stampare: per un gioco va nella scatola, per una serata
 * sulla porta o sul tavolo. Una sola vista per i due casi, perché il
 * cartellino è lo stesso oggetto — cambiano titolo, riga sotto e invito.
 */
const props = defineProps<{ kind: 'game' | 'event' }>()

/** Quel che il backend manda: l'indirizzo è quello già codificato nel QR. */
interface QrCard extends Partial<EventWhen> {
  title: string
  url: string
  publicAddressConfigured: boolean
  svg: string
}

const KINDS = {
  game: {
    segment: 'games',
    back: 'admin-game-detail',
    backLabel: 'Scheda gioco',
    invite: 'Inquadra per regole, tutorial e classifica',
  },
  event: {
    segment: 'events',
    back: 'admin-event-detail',
    backLabel: 'Evento',
    invite: 'Inquadra per prenotare il tuo tavolo',
  },
} as const

const route = useRoute()
const site = useSiteStore()
const id = route.params.id as string
const cfg = KINDS[props.kind]

const card = ref<QrCard | null>(null)
const subtitle = ref('')
const qrSrc = ref('')
/** L'indirizzo in chiaro sotto il codice, senza schema: è il ripiego per
 *  chi il QR non lo legge, e "https://" non si scrive a mano. */
const printedUrl = ref('')
const error = ref('')

onMounted(async () => {
  try {
    const c = await api.get<QrCard>(`/${cfg.segment}/${id}/qr`)
    card.value = c
    qrSrc.value = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(c.svg)}`
    printedUrl.value = c.url.replace(/^https?:\/\//, '')
    if (c.eventDate && c.startTime) {
      subtitle.value = formatEventDateTime({
        eventDate: c.eventDate,
        startTime: c.startTime,
        endTime: c.endTime ?? null,
      })
    }
  } catch (e) {
    error.value = (e as Error).message
  }
})

const print = () => window.print()
</script>

<template>
  <div class="qr-page">
    <div class="qr-toolbar">
      <router-link :to="{ name: cfg.back, params: { id } }" class="back-link">
        &larr; {{ cfg.backLabel }}
      </router-link>
      <button type="button" class="is-compact" :disabled="!card" @click="print">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            d="M7 9V4h10v5M7 17H5.5A1.5 1.5 0 0 1 4 15.5v-5A1.5 1.5 0 0 1 5.5 9h13a1.5 1.5 0 0 1 1.5 1.5v5a1.5 1.5 0 0 1-1.5 1.5H17M7 14h10v6H7z"
            stroke="currentColor"
            stroke-width="1.7"
            stroke-linejoin="round"
          />
        </svg>
        Stampa
      </button>
    </div>

    <!-- Senza indirizzo pubblico il QR punta all'host del browser dell'admin:
         da stampato, spesso `localhost`, che da un altro telefono non si apre. -->
    <p v-if="card && !card.publicAddressConfigured" class="qr-warning" role="status">
      Manca l'indirizzo pubblico: questo QR porta a
      <strong>{{ printedUrl }}</strong>, che da un altro telefono potrebbe non aprirsi.
      Impostalo in <router-link :to="{ name: 'admin-settings' }">Impostazioni</router-link>
      prima di stampare.
    </p>
    <p v-if="error" class="error qr-error">{{ error }}</p>

    <article v-else class="qr-card" aria-label="Cartellino da stampare">
      <p class="qr-card-site">{{ site.siteTitle }}</p>
      <h1 class="qr-card-title">{{ card?.title ?? '…' }}</h1>
      <p v-if="subtitle" class="qr-card-subtitle">{{ subtitle }}</p>
      <img v-if="qrSrc" :src="qrSrc" alt="" class="qr-card-code" width="200" height="200" />
      <div v-else class="qr-card-code" aria-hidden="true"></div>
      <p class="qr-card-invite">{{ cfg.invite }}</p>
      <p class="qr-card-url">{{ printedUrl }}</p>
    </article>
  </div>
</template>
