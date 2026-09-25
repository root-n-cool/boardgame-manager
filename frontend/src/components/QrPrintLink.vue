<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'

/** "Stampa QR" nella testa delle schede admin di gioco ed evento: azione
 *  secondaria, accanto a una primaria rossa non ne serve una seconda. */
defineProps<{ to: RouteLocationRaw; label: string }>()
</script>

<template>
  <router-link class="action-link is-compact is-secondary" :to="to" :aria-label="label">
    Stampa QR
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2.5v2.5H14zM17.5 17.5H20V20h-2.5zM14 18.5V20M18.5 14H20"
        stroke="currentColor"
        stroke-width="1.7"
        stroke-linejoin="round"
      />
    </svg>
  </router-link>
</template>
