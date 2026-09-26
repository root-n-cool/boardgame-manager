<script setup lang="ts">
import { computed } from 'vue'
import { difficultyLevel } from '../utils/difficulty'

/**
 * La difficoltà di un gioco, letta dal peso BGG (1 leggero .. 5 pesante).
 *
 * Su una card si guarda, non si legge: cinque pip come le facce di un dado,
 * accesi fino al peso arrotondato, più la parola che dice cosa significa. Il
 * decimale esatto resta nel `title` per chi conosce la scala BGG e lo cerca.
 */
const props = defineProps<{ weight: number | null }>()

const PIPS = [1, 2, 3, 4, 5]

const filled = computed(() => Math.min(5, Math.max(1, Math.round(props.weight ?? 0))))

// Le soglie stanno in utils/difficulty: le legge anche il filtro del catalogo.
const label = computed(() => difficultyLevel(props.weight))

const exact = computed(() => `${(props.weight ?? 0).toFixed(1).replace('.', ',')}/5`)
</script>

<template>
  <p v-if="weight" class="game-difficulty" :title="`Complessità BGG ${exact}`">
    <span class="difficulty-pips" aria-hidden="true">
      <span v-for="p in PIPS" :key="p" :class="{ 'is-on': p <= filled }"></span>
    </span>
    <span class="visually-hidden">Difficoltà: </span>{{ label }}
  </p>
</template>
