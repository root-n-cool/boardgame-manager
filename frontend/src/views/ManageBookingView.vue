<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface BookingResult {
  id: number
  eventId: number
  eventGameId: number
  participantName: string
  bookingCode: string
  status: 'active' | 'cancelled'
  eventTitle: string
  eventDate: string
  startTime: string
  gameName: string
}

const email = ref('')
const bookingCode = ref('')
const booking = ref<BookingResult | null>(null)
const error = ref('')
const cancelMessage = ref('')

async function lookup() {
  error.value = ''
  cancelMessage.value = ''
  try {
    booking.value = await api.post<BookingResult>('/bookings/lookup', {
      email: email.value,
      bookingCode: bookingCode.value,
    })
  } catch (e) {
    booking.value = null
    error.value = (e as Error).message
  }
}

async function cancel() {
  if (!booking.value) {
    return
  }
  error.value = ''
  try {
    booking.value = await api.post<BookingResult>(`/bookings/${booking.value.id}/cancel`, {
      email: email.value,
      bookingCode: bookingCode.value,
    })
    cancelMessage.value = 'Prenotazione annullata.'
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Gestisci prenotazione</h1>

      <form @submit.prevent="lookup">
        <label>
          Email
          <input v-model="email" type="email" required />
        </label>
        <label>
          Codice prenotazione
          <input v-model="bookingCode" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>

      <div v-if="booking">
        <p>
          Prenotazione per {{ booking.participantName }} — {{ booking.gameName }} —
          {{ booking.eventTitle }} ({{ booking.eventDate }} · {{ booking.startTime }}) —
          stato: {{ booking.status }}
        </p>
        <button v-if="booking.status === 'active'" type="button" @click="cancel">
          Annulla prenotazione
        </button>
        <p v-if="cancelMessage" class="success">{{ cancelMessage }}</p>
      </div>
    </div>
  </div>
</template>
