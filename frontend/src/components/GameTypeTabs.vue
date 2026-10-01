<script lang="ts">
/** Vero se il gioco sta nella tab scelta (`null` = "Tutti"). */
export function matchesType(g: { gameTypeId: number }, typeId: number | null) {
  return typeId === null || g.gameTypeId === typeId
}
</script>

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useGameTypesStore } from '../stores/gameTypes'

/**
 * Le tab per tipologia sopra una lista di giochi. Compaiono solo se la
 * lista mescola almeno due tipologie, e mostrano solo quelle presenti:
 * una tab che porta a una lista vuota è un tocco sprecato. L'ordine è
 * quello deciso dall'admin.
 */
const props = defineProps<{
  games: readonly { gameTypeId: number }[]
  modelValue: number | null
}>()
const emit = defineEmits<{ 'update:modelValue': [value: number | null] }>()

const types = useGameTypesStore()
onMounted(() => types.load())

const tabs = computed(() =>
  types.list
    .map((t) => ({ type: t, count: props.games.filter((g) => g.gameTypeId === t.id).length }))
    .filter((t) => t.count > 0),
)

// Se la tab scelta sparisce (la lista cambia), o le tab spariscono tutte
// perché resta una tipologia sola, si torna a "Tutti": un filtro attivo
// senza tab visibili toglierebbe giochi senza dire perché.
watch(tabs, (list) => {
  if (props.modelValue !== null && (list.length < 2 || !list.some((t) => t.type.id === props.modelValue))) {
    emit('update:modelValue', null)
  }
})
</script>

<template>
  <!-- Bottoni a pressione, non role="tab": filtrano la lista sotto,
       non mostrano pannelli separati. -->
  <div v-if="tabs.length >= 2" class="type-tabs" role="group" aria-label="Tipologia">
    <button
      type="button"
      :aria-pressed="modelValue === null"
      :class="{ 'is-active': modelValue === null }"
      @click="emit('update:modelValue', null)"
    >
      Tutti <span class="type-tab-count">{{ games.length }}</span>
    </button>
    <button
      v-for="t in tabs"
      :key="t.type.id"
      type="button"
      :aria-pressed="modelValue === t.type.id"
      :class="{ 'is-active': modelValue === t.type.id }"
      :title="t.type.name"
      @click="emit('update:modelValue', t.type.id)"
    >
      {{ t.type.name }} <span class="type-tab-count">{{ t.count }}</span>
    </button>
  </div>
</template>
