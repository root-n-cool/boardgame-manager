<script setup lang="ts">
import { ref } from 'vue'

/**
 * L'immagine di un evento è opzionale, e come la copertina di un gioco È il
 * controllo di caricamento: si clicca l'immagine (o la cornice vuota), si
 * sceglie il file e parte da sé. Il componente non sa se il file finirà su
 * disco subito (scheda evento) o solo al salvataggio (creazione): emette il
 * File e chi lo usa decide.
 */
const props = defineProps<{
  /** URL da mostrare: un `/api/uploads/...` o un object URL di anteprima. */
  src: string | null
  /** Testo alternativo dell'immagine quando c'è. */
  alt: string
  uploading?: boolean
}>()

const emit = defineEmits<{ select: [file: File] }>()

const input = ref<HTMLInputElement | null>(null)

function pick() {
  input.value?.click()
}

function onFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  // Azzerare il campo permette di riscegliere lo stesso file dopo un errore:
  // senza questo il browser non emette un secondo `change`.
  target.value = ''
  if (file) {
    emit('select', file)
  }
}
</script>

<template>
  <div class="event-image-field">
    <button
      type="button"
      class="cover-uploader event-image-uploader"
      :class="{ 'is-uploading': props.uploading }"
      :aria-label="props.src ? 'Cambia l\'immagine dell\'evento' : 'Aggiungi un\'immagine all\'evento'"
      :disabled="props.uploading"
      @click="pick"
    >
      <img
        v-if="props.src"
        :src="props.src"
        :alt="props.alt"
        class="event-image"
        width="480"
        height="270"
        decoding="async"
      />
      <span v-else class="event-image event-image-empty" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none">
          <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
          <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
          <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
          <circle cx="12" cy="12" r="1.3" fill="currentColor" />
          <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
          <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
        </svg>
      </span>
      <span class="cover-overlay">
        <svg v-if="!props.uploading" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            d="M12 16V4.8M12 4.8 7.6 9.2M12 4.8l4.4 4.4M4.5 15v3.2c0 .72.58 1.3 1.3 1.3h12.4c.72 0 1.3-.58 1.3-1.3V15"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
        <span class="cover-overlay-label">
          {{ props.uploading ? 'Caricamento…' : props.src ? 'Cambia immagine' : 'Aggiungi immagine' }}
        </span>
      </span>
    </button>
    <input
      ref="input"
      class="visually-hidden"
      type="file"
      accept="image/jpeg,image/png,image/webp"
      tabindex="-1"
      aria-hidden="true"
      @change="onFileSelected"
    />
  </div>
</template>
