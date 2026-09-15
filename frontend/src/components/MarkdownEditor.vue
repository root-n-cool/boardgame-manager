<script setup lang="ts">
import { defineAsyncComponent } from 'vue'

defineProps<{
  modelValue: string
  placeholder?: string
  // Nome accessibile per il campo di editing: la superficie di CodeMirror
  // è un contenteditable, non una <textarea>, e il <label> che lo avvolge
  // nel form non gliene dà uno per conto proprio (vedi
  // MarkdownEditorImpl.vue). Default 'Descrizione' perché è l'unica
  // etichetta usata finora sui campi Markdown del progetto.
  ariaLabel?: string
}>()

defineEmits<{
  'update:modelValue': [value: string]
}>()

// Import dinamico: md-editor-v3 (più CodeMirror) è un chunk pesante che
// serve solo qui, nei form admin. Le pagine pubbliche (prenotazione,
// punteggi) — quelle usate da smartphone — non devono scaricarlo mai.
const MarkdownEditorImpl = defineAsyncComponent(() => import('./MarkdownEditorImpl.vue'))
</script>

<template>
  <MarkdownEditorImpl
    :model-value="modelValue"
    :placeholder="placeholder"
    :aria-label="ariaLabel"
    @update:model-value="$emit('update:modelValue', $event)"
  />
</template>
