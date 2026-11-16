<script setup lang="ts">
import type { ListView } from '../utils/listView'

/**
 * Tre bottoni-icona per scegliere come vedere una lista di giochi. Un
 * gruppo di toggle (`aria-pressed`), non radio: da tastiera ogni vista è un
 * tab stop, e il nome di ciascuna è nel testo nascosto.
 */
defineProps<{ modelValue: ListView }>()
const emit = defineEmits<{ 'update:modelValue': [value: ListView] }>()

const OPTIONS: { value: ListView; label: string; path: string }[] = [
  {
    value: 'cards',
    label: 'Card',
    path: 'M4.5 4.5h6v8h-6zM13.5 4.5h6v8h-6zM4.5 15.5h6v4h-6zM13.5 15.5h6v4h-6z',
  },
  {
    value: 'list',
    label: 'Lista',
    path: 'M4.5 5.5h3v3h-3zM10 7h9.5M4.5 10.5h3v3h-3zM10 12h9.5M4.5 15.5h3v3h-3zM10 17h9.5',
  },
  {
    value: 'thumbs',
    label: 'Miniature',
    path: 'M4.5 4.5h4v5h-4zM10 4.5h4v5h-4zM15.5 4.5h4v5h-4zM4.5 14.5h4v5h-4zM10 14.5h4v5h-4zM15.5 14.5h4v5h-4z',
  },
]
</script>

<template>
  <div class="view-switcher" role="group" aria-label="Vista">
    <button
      v-for="o in OPTIONS"
      :key="o.value"
      type="button"
      :aria-pressed="modelValue === o.value"
      :title="o.label"
      @click="emit('update:modelValue', o.value)"
    >
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path
          :d="o.path"
          fill="none"
          stroke="currentColor"
          stroke-width="1.6"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
      <span class="visually-hidden">{{ o.label }}</span>
    </button>
  </div>
</template>
