<script setup lang="ts" generic="T extends string | number">
/**
 * Un filtro a tendina: la select nativa, che sul telefono apre il
 * selettore di sistema. La prima voce è sempre "nessun filtro" (`null`),
 * così il valore scelto si legge nella select stessa senza una fila di
 * pastiglie accanto. Per un toggle a due voci (Tutti / Prenotabili) la
 * pagina evento usa due bottoni: lì un tocco basta.
 */
const props = defineProps<{
  label: string
  options: readonly { value: T; label: string }[]
  modelValue: T | null
  /** L'etichetta della voce "nessun filtro". */
  anyLabel?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: T | null] }>()

// Per posizione, non per valore: la select restituisce sempre una stringa,
// e confrontarla col valore numerico vorrebbe una conversione. La voce 0 è
// "nessun filtro".
function onChange(event: Event) {
  const i = (event.target as HTMLSelectElement).selectedIndex
  emit('update:modelValue', i === 0 ? null : props.options[i - 1].value)
}
</script>

<template>
  <label class="select-filter">
    <span class="filter-label">{{ label }}</span>
    <select
      :value="modelValue ?? ''"
      :class="{ 'is-active': modelValue !== null }"
      @change="onChange"
    >
      <option value="">{{ anyLabel ?? 'Tutti' }}</option>
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
    </select>
  </label>
</template>
