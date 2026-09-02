<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'

/**
 * Modale basata su <dialog> nativo: focus trap, Esc e backdrop li gestisce il
 * browser, quindi zero dipendenze e zero gestione manuale del focus.
 */
const props = defineProps<{ open: boolean; title: string }>()
const emit = defineEmits<{ close: [] }>()

const dialog = ref<HTMLDialogElement | null>(null)

function sync(open: boolean) {
  const el = dialog.value
  if (!el) {
    return
  }
  if (open && !el.open) {
    el.showModal()
  } else if (!open && el.open) {
    el.close()
  }
}

watch(() => props.open, sync)
onMounted(() => sync(props.open))

// Il backdrop è il <dialog> stesso: un click sul contenuto colpisce il figlio.
function onClick(event: MouseEvent) {
  if (event.target === dialog.value) {
    emit('close')
  }
}
</script>

<template>
  <!--
    `cancel` (Esc) va ascoltato insieme a `close`: Chrome chiude il dialog
    sull'Esc emettendo solo `cancel`, e senza questo lo stato del parent
    resterebbe "aperto" e la modale non si riaprirebbe più. Un doppio emit,
    dove il browser manda entrambi, è innocuo.
  -->
  <dialog ref="dialog" class="modal" @close="emit('close')" @cancel="emit('close')" @click="onClick">
    <div class="modal-sheet">
      <div class="modal-head">
        <h2>{{ title }}</h2>
        <button type="button" class="modal-close" aria-label="Chiudi" @click="emit('close')">
          <svg viewBox="0 0 24 24" fill="none">
            <path d="M6 6l12 12M18 6L6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </div>
      <slot />
    </div>
  </dialog>
</template>
