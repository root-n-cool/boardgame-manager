<script setup lang="ts">
import { useSiteStore } from '../stores/site'

/** Il cartellino da stampare, uguale da solo e nel foglio di tutto il
 *  catalogo: nome del sito, titolo, codice, invito e indirizzo in chiaro.
 *  Senza `src` mostra il riquadro vuoto mentre il codice arriva. */
withDefaults(defineProps<{
  title: string
  subtitle?: string
  src?: string
  invite: string
  url: string
  /** Nel foglio i cartellini sono tanti: il titolo della pagina è altrove. */
  heading?: 'h1' | 'h2'
}>(), { heading: 'h1' })

const site = useSiteStore()
</script>

<template>
  <article class="qr-card" :aria-label="`Cartellino da stampare: ${title}`">
    <p class="qr-card-site">{{ site.siteTitle }}</p>
    <component :is="heading" class="qr-card-title">{{ title }}</component>
    <p v-if="subtitle" class="qr-card-subtitle">{{ subtitle }}</p>
    <img v-if="src" :src="src" alt="" class="qr-card-code" width="200" height="200" />
    <div v-else class="qr-card-code" aria-hidden="true"></div>
    <p class="qr-card-invite">{{ invite }}</p>
    <p class="qr-card-url">{{ url }}</p>
  </article>
</template>
