<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import QrCard from '../components/QrCard.vue'
import GameTypeTabs, { matchesType } from '../components/GameTypeTabs.vue'
import { GAME_QR_INVITE, printableUrl, qrDataUrl } from '../utils/qr'

/**
 * I cartellini di tutto il catalogo su fogli A4, da stampare in un colpo (o
 * salvare in PDF dalla stampa del browser) e ritagliare lungo il
 * tratteggio. Il backend li manda già in ordine di nome e senza i giochi
 * nascosti dal catalogo pubblico.
 */
interface SheetCard {
  id: number
  title: string
  url: string
  svg: string
  gameTypeId: number
}

const cards = ref<Array<SheetCard & { src: string; printedUrl: string }>>([])
// Si stampa la tab scelta: un foglio per tipologia, se servono separati.
const typeId = ref<number | null>(null)
const visibleCards = computed(() => cards.value.filter((c) => matchesType(c, typeId.value)))
const configured = ref(true)
const loaded = ref(false)
const error = ref('')

onMounted(async () => {
  try {
    const res = await api.get<{ publicAddressConfigured: boolean; cards: SheetCard[] }>('/games/qr')
    configured.value = res.publicAddressConfigured
    cards.value = res.cards.map((c) => ({ ...c, src: qrDataUrl(c.svg), printedUrl: printableUrl(c.url) }))
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loaded.value = true
  }
})

const print = () => window.print()
</script>

<template>
  <div class="qr-page is-sheet">
    <div class="qr-toolbar is-wide">
      <router-link :to="{ name: 'admin-games' }" class="back-link">&larr; Catalogo giochi</router-link>
      <button type="button" class="is-compact" :disabled="visibleCards.length === 0" @click="print">
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

    <div class="qr-sheet-intro is-wide">
      <h1>QR di tutti i giochi</h1>
      <p v-if="loaded && !error" class="page-meta">
        {{ visibleCards.length === 1 ? '1 cartellino' : `${visibleCards.length} cartellini` }}, nove per foglio A4.
        Per un PDF scegli «Salva come PDF» nella stampa. I giochi nascosti dal catalogo restano fuori.
      </p>
    </div>

    <!-- Vedi QrPrintView: senza indirizzo pubblico i codici portano all'host
         del browser dell'admin. -->
    <p v-if="!configured && cards.length" class="qr-warning is-wide" role="status">
      Manca l'indirizzo pubblico: questi QR portano a
      <strong>{{ cards[0]!.printedUrl.split('/')[0] }}</strong>, che da un altro telefono potrebbe non aprirsi.
      Impostalo in <router-link :to="{ name: 'admin-settings' }">Impostazioni</router-link>
      prima di stampare.
    </p>
    <p v-if="error" class="error qr-error">{{ error }}</p>
    <p v-else-if="loaded && cards.length === 0" class="page-meta">
      Nessun gioco visibile nel catalogo pubblico: non c'è niente da stampare.
    </p>

    <GameTypeTabs v-model="typeId" :games="cards" />
    <div v-if="cards.length" class="qr-sheet">
      <QrCard
        v-for="c in visibleCards"
        :key="c.id"
        heading="h2"
        :title="c.title"
        :src="c.src"
        :invite="GAME_QR_INVITE"
        :url="c.printedUrl"
      />
    </div>
  </div>
</template>
