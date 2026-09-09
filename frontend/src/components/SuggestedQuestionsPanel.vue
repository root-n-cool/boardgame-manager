<script setup lang="ts">
/**
 * Le tre domande suggerite che la chat mostra nello stato di riposo.
 *
 * Sono generate dal modello a ogni indicizzazione, ma una domanda riscritta
 * a mano non viene più toccata: è la ragione per cui il pannello mostra
 * quali sono a mano — è l'informazione che spiega perché una domanda non è
 * cambiata dopo un reindex.
 *
 * È un `<form>` e non un `<div>`: tre campi etichettati più un "Salva" sono
 * un form, e dichiararlo restituisce l'invio col tasto Invio, che qui
 * mancava. La lista resta una `<ol>` perché la posizione è un dato — le tre
 * domande compaiono nella chat in quest'ordine.
 *
 * "Rigenera" sta nella testata del pannello e non accanto a "Salva": non è
 * l'alternativa al salvataggio, è l'azione che *riempie* il pannello, e due
 * bottoni affiancati si leggono come "scegline uno per finire". Il progetto
 * ha escluso una conferma per Rigenera, quindi il lavoro a mano lo protegge
 * la distanza dal bottone che lo salva, non un modale.
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

// I tre testi come stavano all'ultimo salvataggio riuscito, oppure null.
// "Domande salvate." è un confronto e non un flag: un flag messo a vero nel
// `save()` resterebbe vero mentre l'admin ritocca un campo, e il messaggio
// parlerebbe di un testo che non è più quello salvato.
const savedTexts = ref<string[] | null>(null)
const saved = computed(
  () =>
    savedTexts.value !== null &&
    savedTexts.value.length === questions.value.length &&
    savedTexts.value.every((t, i) => t === questions.value[i].text),
)

// Salva e Rigenera scrivono lo stesso stato: se partono insieme, le due
// risposte arrivano in ordine imprevedibile e l'ultima sovrascrive
// `questions.value` dell'altra in silenzio. Ogni bottone si spegne quindi
// finché una qualunque delle due richieste è in volo, non solo la propria.
const busy = computed(() => saving.value || regenerating.value)

// Il server rifiuta una domanda vuota (400): senza questo controllo il
// pannello invitava a salvare tre campi vuoti e l'errore arrivava dopo il
// click, come unico modo di scoprire la regola.
const incomplete = computed(() => questions.value.some((q) => q.text.trim() === ''))
const allEmpty = computed(() => questions.value.every((q) => q.text.trim() === ''))

// Quel che l'esito racconta a chi non vede lo schermo: le tre domande
// cambiano da sole dopo una rigenerazione, ed è un cambio che va annunciato.
const status = computed(() => {
  if (saving.value) return 'Salvataggio in corso.'
  if (regenerating.value) return 'Rigenerazione in corso.'
  if (error.value) return ''
  if (saved.value) return 'Domande salvate.'
  return ''
})

const path = computed(() => `/games/${props.gameId}/suggested-questions`)

async function load() {
  loading.value = true
  error.value = ''
  savedTexts.value = null
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
  if (busy.value || incomplete.value) return
  saving.value = true
  error.value = ''
  try {
    const res = await api.put<{ questions: Question[] }>(path.value, {
      questions: questions.value.map((q) => q.text),
    })
    questions.value = res.questions
    savedTexts.value = questions.value.map((q) => q.text)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile salvare le domande.'
  } finally {
    saving.value = false
  }
}

async function regenerate() {
  if (busy.value) return
  regenerating.value = true
  error.value = ''
  savedTexts.value = null
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
  <form class="suggested-questions" @submit.prevent="save">
    <div class="suggested-questions-head">
      <div class="section-head suggested-questions-head-row">
        <h3>Domande suggerite</h3>
        <button
          type="button"
          class="btn-secondary is-compact"
          :disabled="busy || !props.aiConfigured"
          :aria-describedby="props.aiConfigured ? 'sq-note' : 'sq-note sq-no-ai'"
          @click="regenerate"
        >
          {{ regenerating ? 'Rigenerazione…' : 'Rigenera' }}
        </button>
      </div>

      <p class="field-hint">
        Le tre domande che la chat propone prima che qualcuno scriva. A ogni indicizzazione si
        rigenerano da sé, tranne quelle che riscrivi qui.
      </p>
      <p id="sq-note" class="field-hint">
        Rigenera invece le riscrive tutte e tre, comprese quelle scritte a mano.
      </p>
      <!-- Il bottone spento diceva da solo di essere spento e niente più: il
           motivo va detto qui, perché è l'unico posto in cui compare a
           prescindere da quanti documenti ha il gioco. -->
      <p v-if="!props.aiConfigured" id="sq-no-ai" class="empty-note">
        Rigenera è spento: serve un provider AI configurato nelle
        <router-link :to="{ name: 'admin-settings' }">impostazioni</router-link>. Senza provider la
        chat non compare sulla scheda pubblica, quindi per ora queste domande non le legge nessuno.
      </p>
    </div>

    <p v-if="loading" class="empty-note">Caricamento…</p>

    <template v-else>
      <ol class="suggested-questions-list" role="list" :aria-busy="regenerating">
        <li v-for="(q, i) in questions" :key="i">
          <label :for="`suggested-question-${i}`" class="visually-hidden">Domanda {{ i + 1 }}</label>
          <input
            :id="`suggested-question-${i}`"
            v-model="q.text"
            type="text"
            :maxlength="120"
            :disabled="busy"
            :aria-describedby="q.edited ? `suggested-question-badge-${i}` : undefined"
            placeholder="Scrivi una domanda"
          />
          <span
            v-if="q.edited"
            :id="`suggested-question-badge-${i}`"
            class="suggested-questions-badge"
            >scritta a mano</span
          >
        </li>
      </ol>

      <!-- Sta qui e non in fondo al pannello: la rigenerazione dura secondi
           e quel che cambia sono i tre campi appena sopra. -->
      <p v-if="regenerating" class="empty-note">
        Il modello sta rileggendo i titoli del manuale: ci vuole qualche secondo.
      </p>

      <!--
        Un solo messaggio per lo stato dei tre campi, ed è anche il motivo
        per cui "Salva" è spento: il server rifiuta una domanda vuota (400) e
        prima quella regola si scopriva solo dall'errore dopo il click.
        Tutte vuote non è un errore — è il gioco appena creato, e la chat
        intanto mostra le sue tre domande fisse. Lo diceva il placeholder, che
        sparisce alla prima lettera ed è il testo meno leggibile del campo.
      -->
      <p v-if="incomplete" id="sq-save-note" class="empty-note">
        <template v-if="allEmpty">
          Nessuna domanda ancora: rigenerale da un manuale indicizzato, oppure scrivile a mano qui e
          salvale. Finché sono vuote la chat propone tre domande generiche.
        </template>
        <template v-else>Servono tutte e tre le domande: una casella vuota non si salva.</template>
      </p>

      <div class="suggested-questions-actions">
        <button
          type="submit"
          :disabled="busy || incomplete"
          :aria-describedby="incomplete ? 'sq-save-note' : undefined"
        >
          {{ saving ? 'Salvataggio…' : 'Salva' }}
        </button>
      </div>

      <!-- L'esito, per chi non vede lo schermo: la rigenerazione dura
           secondi e cambia i tre campi da sé. -->
      <p class="visually-hidden" role="status" aria-live="polite">{{ status }}</p>

      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-else-if="saved" class="empty-note">Domande salvate.</p>
    </template>
  </form>
</template>
