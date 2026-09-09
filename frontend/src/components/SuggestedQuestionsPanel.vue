<script setup lang="ts">
/**
 * Le tre domande suggerite che la chat mostra nello stato di riposo.
 *
 * Sono generate dal modello a ogni indicizzazione, ma una domanda riscritta
 * a mano non viene più toccata: è la ragione per cui il pannello mostra
 * quali sono a mano — è l'informazione che spiega perché una domanda non è
 * cambiata dopo un reindex.
 */
import { computed, onMounted, ref } from 'vue'

import { api } from '../api/client'

const props = defineProps<{
  gameId: number
  /** Senza provider AI la rigenerazione non è possibile: il pulsante si spegne. */
  aiConfigured: boolean
}>()

type Question = { text: string; edited: boolean }

const questions = ref<Question[]>([
  { text: '', edited: false },
  { text: '', edited: false },
  { text: '', edited: false },
])
const loading = ref(false)
const saving = ref(false)
const regenerating = ref(false)
const error = ref('')
const saved = ref(false)

// Salva e Rigenera scrivono lo stesso stato: se partono insieme, le due
// risposte arrivano in ordine imprevedibile e l'ultima sovrascrive
// `questions.value` dell'altra in silenzio. Ogni bottone si spegne quindi
// finché una qualunque delle due richieste è in volo, non solo la propria.
const busy = computed(() => saving.value || regenerating.value)

const path = computed(() => `/games/${props.gameId}/suggested-questions`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ questions: Question[] }>(path.value)
    questions.value = res.questions
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile leggere le domande suggerite.'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    const res = await api.put<{ questions: Question[] }>(path.value, {
      questions: questions.value.map((q) => q.text),
    })
    questions.value = res.questions
    saved.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile salvare le domande.'
  } finally {
    saving.value = false
  }
}

async function regenerate() {
  regenerating.value = true
  error.value = ''
  saved.value = false
  try {
    const res = await api.post<{ questions: Question[] }>(`${path.value}/regenerate`)
    questions.value = res.questions
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile rigenerare le domande.'
  } finally {
    regenerating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="suggested-questions">
    <h3>Domande suggerite</h3>
    <p class="field-hint">
      Le tre domande che la chat propone prima che qualcuno scriva. Si rigenerano da sé a ogni
      indicizzazione, tranne quelle che riscrivi qui: quelle restano come le hai messe.
    </p>

    <p v-if="loading" class="empty-note">Caricamento…</p>

    <template v-else>
      <ol class="suggested-questions-list">
        <li v-for="(q, i) in questions" :key="i">
          <label :for="`suggested-question-${i}`" class="visually-hidden">Domanda {{ i + 1 }}</label>
          <input
            :id="`suggested-question-${i}`"
            v-model="q.text"
            type="text"
            :maxlength="120"
            placeholder="Nessuna domanda: indicizza un manuale o scrivila a mano"
          />
          <span v-if="q.edited" class="suggested-questions-badge">scritta a mano</span>
        </li>
      </ol>

      <p class="field-hint">
        Rigenera è l'eccezione alla regola qui sopra: riscrive tutte e tre le domande, comprese
        quelle scritte a mano.
      </p>

      <div class="suggested-questions-actions">
        <button type="button" :disabled="busy" @click="save">
          {{ saving ? 'Salvataggio…' : 'Salva' }}
        </button>
        <button
          type="button"
          class="btn-secondary"
          :disabled="busy || !props.aiConfigured"
          @click="regenerate"
        >
          {{ regenerating ? 'Rigenerazione…' : 'Rigenera' }}
        </button>
      </div>

      <p v-if="saved" class="field-hint">Domande salvate.</p>
      <p v-if="error" class="error">{{ error }}</p>
    </template>
  </div>
</template>
