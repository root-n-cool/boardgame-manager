<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import QrCard from '../components/QrCard.vue'
import { formatEventDateTime, type EventWhen } from '../utils/dates'
import { GAME_QR_INVITE, downloadQrJpg, printableUrl, qrDataUrl } from '../utils/qr'

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
    invite: GAME_QR_INVITE,
  },
  event: {
    segment: 'events',
    back: 'admin-event-detail',
    backLabel: 'Evento',
    invite: 'Inquadra per info e prenotazioni',
  },
} as const

const route = useRoute()
const id = route.params.id as string
const cfg = KINDS[props.kind]

const card = ref<QrCard | null>(null)
const subtitle = ref('')
const qrSrc = ref('')
const printedUrl = ref('')
const error = ref('')
/** A parte: un JPG non riuscito non deve togliere il cartellino dallo schermo. */
const jpgError = ref('')

onMounted(async () => {
  try {
    const c = await api.get<QrCard>(`/${cfg.segment}/${id}/qr`)
    card.value = c
    qrSrc.value = qrDataUrl(c.svg)
    printedUrl.value = printableUrl(c.url)
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

const downloadJpg = async () => {
  if (!card.value) return
  jpgError.value = ''
  try {
    await downloadQrJpg(card.value.svg, card.value.title)
  } catch (e) {
    jpgError.value = (e as Error).message
  }
}
</script>

<template>
  <div class="qr-page">
    <div class="qr-toolbar">
      <router-link :to="{ name: cfg.back, params: { id } }" class="back-link">
        &larr; {{ cfg.backLabel }}
      </router-link>
      <div class="qr-toolbar-actions">
        <button type="button" class="is-compact btn-secondary" :disabled="!card" @click="downloadJpg">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M12 4v11m0 0-4.5-4.5M12 15l4.5-4.5M5 19h14"
              stroke="currentColor"
              stroke-width="1.7"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
          Scarica JPG
        </button>
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
    </div>

    <!-- Senza indirizzo pubblico il QR punta all'host del browser dell'admin:
         da stampato, spesso `localhost`, che da un altro telefono non si apre. -->
    <p v-if="card && !card.publicAddressConfigured" class="qr-warning" role="status">
      Manca l'indirizzo pubblico: questo QR porta a
      <strong>{{ printedUrl }}</strong>, che da un altro telefono potrebbe non aprirsi.
      Impostalo in <router-link :to="{ name: 'admin-settings' }">Impostazioni</router-link>
      prima di stampare.
    </p>
    <p v-if="jpgError" class="error qr-error" role="alert">{{ jpgError }}</p>
    <p v-if="error" class="error qr-error">{{ error }}</p>

    <QrCard
      v-else
      :title="card?.title ?? '…'"
      :subtitle="subtitle"
      :src="qrSrc"
      :invite="cfg.invite"
      :url="printedUrl"
    />
  </div>
</template>
