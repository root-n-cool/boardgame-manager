<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import BggSearchSelect, { type BggResult } from '../components/BggSearchSelect.vue'

const router = useRouter()

// Un gioco entra in catalogo da BoardGameGeek: l'inserimento a mano è la via
// di riserva per quel che su BGG non c'è, non un percorso alla pari.
const manual = ref(false)

const selected = ref<BggResult | null>(null)
const languageCode = ref('it')
const owner = ref('')

const manualName = ref('')
const manualYear = ref<number | null>(null)
const manualMinPlayers = ref<number | null>(null)
const manualMaxPlayers = ref<number | null>(null)
const manualPlaytime = ref<number | null>(null)
const manualWeight = ref<number | null>(null)

// Posti prenotabili per copia: 1 = chi prenota si prende la copia. Vale per
// entrambe le vie di creazione (BGG e manuale), quindi vive nei "Dettagli"
// condivisi invece che duplicato nei due blocchi sopra.
const seats = ref(1)

const error = ref('')
const saving = ref(false)

const ready = computed(() => (manual.value ? manualName.value.trim() !== '' : selected.value !== null))

function startManual() {
  manual.value = true
  selected.value = null
  error.value = ''
}

function backToSearch() {
  manual.value = false
  error.value = ''
}

async function createGame() {
  if (!ready.value) {
    return
  }
  error.value = ''
  saving.value = true
  try {
    const payload = manual.value
      ? {
          name: manualName.value,
          year: manualYear.value,
          minPlayers: manualMinPlayers.value,
          maxPlayers: manualMaxPlayers.value,
          playtimeMinutes: manualPlaytime.value,
          weight: manualWeight.value,
          nameTranslated: manualName.value,
          owner: owner.value,
          languageCode: languageCode.value,
          seats: seats.value,
        }
      : {
          bggId: selected.value!.bggId,
          owner: owner.value,
          languageCode: languageCode.value,
          seats: seats.value,
        }
    const game = await api.post<{ id: number }>('/games', payload)
    router.push(`/games/${game.id}`)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <router-link to="/games" class="back-link">&larr; Catalogo</router-link>

    <div class="page-head">
      <div class="page-head-text">
        <h1>Aggiungi gioco</h1>
        <p class="page-meta">Cercalo su BoardGameGeek, o inseriscilo a mano se non c'è.</p>
      </div>
    </div>

    <form class="panel-form" @submit.prevent="createGame">
      <div class="panel-card">
        <div class="section-head">
          <h2>Gioco</h2>
        </div>

        <template v-if="!manual">
          <BggSearchSelect v-model="selected" />
          <p class="field-hint">
            Non lo trovi?
            <button type="button" class="link-button" @click="startManual">Inseriscilo a mano</button>
          </p>
        </template>

        <template v-else>
          <label>
            Nome
            <input v-model="manualName" required />
          </label>
          <label>
            <span>Anno <span class="field-optional">(opzionale)</span></span>
            <input v-model.number="manualYear" type="number" />
          </label>
          <div class="field-row">
            <label>
              <span>Min giocatori <span class="field-optional">(opz.)</span></span>
              <input v-model.number="manualMinPlayers" type="number" min="1" />
            </label>
            <label>
              <span>Max giocatori <span class="field-optional">(opz.)</span></span>
              <input v-model.number="manualMaxPlayers" type="number" min="1" />
            </label>
          </div>
          <label>
            <span>Durata in minuti <span class="field-optional">(opzionale)</span></span>
            <input v-model.number="manualPlaytime" type="number" min="1" />
          </label>
          <label>
            <span>Complessità <span class="field-optional">(opzionale)</span></span>
            <input v-model.number="manualWeight" type="number" min="1" max="5" step="0.1" />
          </label>
          <p class="field-hint">Da 1 (leggero) a 5 (pesante), come il peso di BoardGameGeek.</p>
          <p class="field-hint">
            <button type="button" class="link-button" @click="backToSearch">
              Torna alla ricerca su BoardGameGeek
            </button>
          </p>
        </template>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Dettagli</h2>
        </div>
        <label>
          Lingua base
          <select v-model="languageCode">
            <option value="it">Italiano</option>
            <option value="en">Inglese</option>
          </select>
        </label>
        <label>
          <span>Proprietario <span class="field-optional">(opzionale)</span></span>
          <input v-model="owner" placeholder="Chi porta la scatola" />
        </label>
        <label>
          Posti prenotabili per copia
          <input v-model.number="seats" type="number" min="1" />
          <span class="field-hint">
            1 = chi prenota si prende la copia e si porta i suoi. Più di 1 per
            un tavolo aperto, dove ci si iscrive uno alla volta (D&amp;D,
            giochi di ruolo, tornei).
          </span>
        </label>
      </div>

      <div class="form-actions">
        <button type="submit" :disabled="!ready || saving">
          {{ saving ? 'Aggiunta…' : 'Aggiungi gioco' }}
        </button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
