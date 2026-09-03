<script setup lang="ts">
import { computed } from 'vue'
import type { GameDetail } from '../utils/game'

const props = defineProps<{ game: GameDetail }>()

/** I dati che arrivano da BGG, saltando quelli che il gioco non ha. */
const facts = computed(() => {
  const g = props.game
  const list: { label: string; value: string; href?: string }[] = []
  if (g.minPlayers || g.maxPlayers) {
    const min = g.minPlayers ?? g.maxPlayers
    const max = g.maxPlayers ?? g.minPlayers
    list.push({ label: 'Giocatori', value: min === max ? `${min}` : `${min}–${max}` })
  }
  if (g.playtimeMinutes) {
    list.push({ label: 'Durata', value: `${g.playtimeMinutes} min` })
  }
  if (g.weight) {
    // Il peso di BGG è una media con quattro decimali: uno basta per
    // distinguere un filler da un gestionale, il resto è rumore.
    list.push({ label: 'Complessità', value: `${g.weight.toFixed(1).replace('.', ',')}/5` })
  }
  if (g.year) {
    list.push({ label: 'Anno', value: `${g.year}` })
  }
  if (g.bggId) {
    list.push({
      label: 'BGG',
      value: `#${g.bggId}`,
      href: `https://boardgamegeek.com/boardgame/${g.bggId}`,
    })
  }
  return list
})
</script>

<template>
  <dl v-if="facts.length > 0" class="game-facts">
    <div v-for="f in facts" :key="f.label">
      <dt>{{ f.label }}</dt>
      <dd>
        <a v-if="f.href" :href="f.href" target="_blank" rel="noopener">{{ f.value }}</a>
        <template v-else>{{ f.value }}</template>
      </dd>
    </div>
  </dl>
  <p v-else class="empty-note">Nessun dato da BoardGameGeek per questo gioco.</p>
</template>
