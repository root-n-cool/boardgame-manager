<script setup lang="ts">
import { computed } from 'vue'

// L'immagine di un evento è spesso una locandina verticale piena di testo:
// ritagliarla la rende illeggibile. Si mostra intera, e lo spazio che avanza
// lo riempie la stessa immagine sfocata.
const props = defineProps<{
  path: string
  variant: 'card' | 'banner'
}>()

const src = computed(() => `/api/uploads/${props.path}`)
const backdrop = computed(() => ({ '--poster': `url("${src.value}")` }))
</script>

<template>
  <div
    class="event-poster"
    :class="`event-poster--${variant}`"
    :style="backdrop"
  >
    <!-- La locandina in cima alla pagina si apre intera: sul telefono è
         contenuta a metà schermo e il testo piccolo va letto da vicino. -->
    <a
      v-if="variant === 'banner'"
      :href="src"
      target="_blank"
      rel="noopener"
      class="event-poster-open"
      aria-label="Apri la locandina intera (nuova scheda)"
    ></a>
    <img
      class="event-poster-img"
      :src="src"
      alt=""
      :width="variant === 'card' ? 480 : 880"
      :height="variant === 'card' ? 270 : 495"
      :loading="variant === 'card' ? 'lazy' : undefined"
      decoding="async"
    />
  </div>
</template>
