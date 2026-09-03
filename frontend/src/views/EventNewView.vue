<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import EventImagePicker from '../components/EventImagePicker.vue'
import EventGamesPicker, { type PickerGame, type SelectedGame } from '../components/EventGamesPicker.vue'
import VenueSearchSelect, { type Venue } from '../components/VenueSearchSelect.vue'

const router = useRouter()

const title = ref('')
const description = ref('')
const eventDate = ref('')
const startTime = ref('')
const venue = ref<Venue | null>(null)
const error = ref('')
const saving = ref(false)

const availableGames = ref<PickerGame[]>([])
const selectedGames = ref<SelectedGame[]>([])

const chosenLabel = computed(() =>
  selectedGames.value.length === 1 ? '1 scelto' : `${selectedGames.value.length} scelti`,
)

// L'immagine si sceglie qui ma si carica dopo: il file ha bisogno di un evento
// a cui appartenere, quindi resta in memoria fino al salvataggio e intanto si
// vede in anteprima da un object URL.
const imageFile = ref<File | null>(null)
const imagePreview = ref<string | null>(null)

async function loadGames() {
  availableGames.value = await api.get<PickerGame[]>('/games')
}

function onImageSelected(file: File) {
  releasePreview()
  imageFile.value = file
  imagePreview.value = URL.createObjectURL(file)
}

function releasePreview() {
  if (imagePreview.value) {
    URL.revokeObjectURL(imagePreview.value)
    imagePreview.value = null
  }
}

async function createEvent() {
  error.value = ''
  saving.value = true
  try {
    const event = await api.post<{ id: number }>('/events', {
      title: title.value,
      description: description.value || null,
      eventDate: eventDate.value,
      startTime: startTime.value,
      venue: venue.value,
      games: selectedGames.value,
    })
    // L'evento esiste già: se l'immagine non passa (formato, dimensione, rete)
    // non si torna indietro — si va sulla scheda e lì si dice cos'è andato
    // storto, perché è lì che si riprova.
    if (imageFile.value) {
      const body = new FormData()
      body.append('file', imageFile.value)
      try {
        await api.post(`/events/${event.id}/image`, body)
      } catch (e) {
        router.push({
          path: `/admin/events/${event.id}`,
          query: { imageError: (e as Error).message },
        })
        return
      }
    }
    router.push(`/admin/events/${event.id}`)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

onMounted(loadGames)
onBeforeUnmount(releasePreview)
</script>

<template>
  <div>
    <router-link to="/admin/events" class="back-link">&larr; Eventi</router-link>

    <div class="page-head">
      <div class="page-head-text">
        <h1>Crea evento</h1>
        <p class="page-meta">Una serata, i giochi che ci saranno e quante copie di ciascuno.</p>
      </div>
    </div>

    <form class="panel-form" @submit.prevent="createEvent">
      <div class="panel-card">
        <div class="section-head">
          <h2>Dettagli</h2>
        </div>

        <div class="field-block">
          <span class="field-label">Immagine <span class="field-optional">(opzionale)</span></span>
          <EventImagePicker
            :src="imagePreview"
            alt="Anteprima dell'immagine scelta per l'evento"
            @select="onImageSelected"
          />
          <p class="field-hint">JPEG, PNG o WebP, fino a 5MB. Si può aggiungere anche dopo.</p>
        </div>

        <label>
          Titolo
          <input v-model="title" required />
        </label>
        <label>
          <span>Descrizione <span class="field-optional">(opzionale)</span></span>
          <textarea v-model="description"></textarea>
        </label>
        <label>
          Data
          <input v-model="eventDate" type="date" required />
        </label>
        <label>
          Ora
          <input v-model="startTime" type="time" required />
        </label>

        <div class="field-block">
          <span class="field-label">Luogo <span class="field-optional">(opzionale)</span></span>
          <VenueSearchSelect v-model="venue" />
        </div>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Giochi dell'evento</h2>
          <span class="section-count">{{ chosenLabel }}</span>
        </div>
        <EventGamesPicker v-model="selectedGames" :games="availableGames" />
      </div>

      <div class="form-actions">
        <button type="submit" :disabled="saving">{{ saving ? 'Creazione…' : 'Crea evento' }}</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
