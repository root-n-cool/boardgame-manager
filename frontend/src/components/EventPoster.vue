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
