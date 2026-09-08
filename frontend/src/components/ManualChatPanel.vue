<script setup lang="ts">
import { computed, ref } from 'vue'

/**
 * Il contenuto della chat, indipendente da dove vive: la sidebar su
 * desktop e il dialog a tutto schermo su mobile montano questo.
 *
 * Nello stato di riposo il markup è nostro: titolo, tre domande suggerite e
 * un finto campo di input. deep-chat si monta al primo gesto — clic sul
 * campo o su una domanda — perché pesa 105 KB gzip, più dell'intera app, e
 * chi apre una scheda gioco per leggerla non deve pagarli. È questo che
 * permette alla sidebar di stare aperta per default.
 */
const props = defineProps<{
  gameId: number
  gameName: string
  /** I titoli di sezione del manuale, per le domande suggerite. */
  headings: string[]
}>()

const started = ref(false)
const pendingQuestion = ref('')

// Le domande suggerite: dai titoli del manuale quando ce ne sono almeno
// tre, altrimenti tre domande fisse. Una chat vuota su un telefono non
// suggerisce cosa farne.
const fallbackQuestions = [
  'Come finisce la partita?',
  'In quanti si gioca?',
  'Come si contano i punti?',
]

/** Rende un titolo di sezione come domanda. Tabella fissa, niente magia. */
const headingToQuestion: Record<string, string> = {
  'fine partita': 'Come finisce la partita?',
  'fine della partita': 'Come finisce la partita?',
  preparazione: 'Come si prepara il gioco?',
  punteggio: 'Come si contano i punti?',
  conteggio: 'Come si contano i punti?',
  turno: 'Cosa posso fare nel mio turno?',
  'turno del giocatore': 'Cosa posso fare nel mio turno?',
}

const suggestions = computed<string[]>(() => {
  const fromHeadings = props.headings
    .map((h) => headingToQuestion[h.trim().toLowerCase()] ?? `Cosa dice il manuale su "${h}"?`)
    .filter((q, i, all) => all.indexOf(q) === i)
  return fromHeadings.length >= 3 ? fromHeadings.slice(0, 3) : fallbackQuestions
})

function start(question = '') {
  pendingQuestion.value = question
  started.value = true
}
</script>

<template>
  <div class="manual-chat-panel">
    <p class="manual-chat-note">
      Risposte generate dal manuale: controlla sempre la pagina citata.
    </p>

    <div v-if="!started" class="manual-chat-rest">
      <p class="manual-chat-intro">
        Chiedi una regola di <strong>{{ gameName }}</strong> a parole tue.
      </p>
      <ul class="manual-chat-suggestions">
        <li v-for="q in suggestions" :key="q">
          <button type="button" @click="start(q)">{{ q }}</button>
        </li>
      </ul>
      <button type="button" class="manual-chat-fakeinput" @click="start()">
        Chiedi una regola…
      </button>
    </div>

    <!-- Il componente vero arriva nel task successivo. -->
    <p v-else class="manual-chat-placeholder">
      Chat in arrivo (domanda: {{ pendingQuestion || '—' }})
    </p>
  </div>
</template>
