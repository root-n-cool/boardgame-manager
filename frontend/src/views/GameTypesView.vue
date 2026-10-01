<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import { useGameTypesStore, type GameType } from '../stores/gameTypes'

/**
 * Le tipologie di gioco: ognuna diventa una tab sopra le liste di giochi,
 * nell'ordine di questa pagina. Una tipologia con giochi non si elimina:
 * prima si spostano i giochi dalla loro scheda.
 */
const store = useGameTypesStore()
const error = ref('')
const busy = ref(false)
const editingId = ref<number | null>(null)
const draft = ref({ name: '', slug: '', bggSearch: true })
const newType = ref({ name: '', slug: '', bggSearch: true })

async function run(action: () => Promise<unknown>) {
  error.value = ''
  busy.value = true
  try {
    await action()
    await store.load(true)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function startEdit(t: GameType) {
  editingId.value = t.id
  draft.value = { name: t.name, slug: t.slug, bggSearch: t.bggSearch }
}

const saveEdit = (id: number) =>
  run(async () => {
    await api.patch(`/game-types/${id}`, draft.value)
    editingId.value = null
  })

const move = (id: number, direction: 'up' | 'down') => run(() => api.post(`/game-types/${id}/move`, { direction }))

const remove = (id: number) => run(() => api.delete(`/game-types/${id}`))

const create = () =>
  run(async () => {
    await api.post('/game-types', newType.value)
    newType.value = { name: '', slug: '', bggSearch: true }
  })

function countLabel(t: GameType) {
  const n = t.gameCount ?? 0
  return n === 1 ? '1 gioco' : `${n} giochi`
}

// Il gameCount lo manda solo una sessione admin: si ricarica sempre, anche
// se lo store era già stato riempito da una pagina pubblica senza conteggi.
onMounted(() => store.load(true))
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Tipologie di gioco</h1>
        <p class="page-meta">Ogni tipologia è una tab sopra le liste di giochi, in quest'ordine.</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <div class="panel-card">
      <ul role="list" class="type-admin-list">
        <li v-for="(t, i) in store.list" :key="t.id">
          <form v-if="editingId === t.id" class="type-admin-edit" @submit.prevent="saveEdit(t.id)">
            <label>Nome <input v-model="draft.name" required /></label>
            <label>Sigla <input v-model="draft.slug" required maxlength="6" /></label>
            <label class="checkbox-label">
              <input v-model="draft.bggSearch" type="checkbox" /> Cerca su BoardGameGeek
            </label>
            <div class="type-admin-actions">
              <button type="button" class="btn-secondary" @click="editingId = null">Annulla</button>
              <button type="submit" class="type-admin-save" :disabled="busy">Salva</button>
            </div>
          </form>
          <template v-else>
            <span class="type-chip is-inline" :class="store.colorClass(t.id)">{{ t.slug }}</span>
            <div class="type-admin-text">
              <strong>{{ t.name }}</strong>
              <span class="page-meta">
                {{ t.bggSearch ? 'Da BoardGameGeek' : 'Inserimento a mano' }} · {{ countLabel(t) }}
              </span>
            </div>
            <div class="type-admin-actions">
              <button
                type="button"
                class="btn-secondary"
                :disabled="busy || i === 0"
                aria-label="Sposta su"
                @click="move(t.id, 'up')"
              >
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 19V5M6 11l6-6 6 6" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" /></svg>
              </button>
              <button
                type="button"
                class="btn-secondary"
                :disabled="busy || i === store.list.length - 1"
                aria-label="Sposta giù"
                @click="move(t.id, 'down')"
              >
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 5v14M6 13l6 6 6-6" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" /></svg>
              </button>
              <button type="button" class="btn-secondary" :disabled="busy" @click="startEdit(t)">Modifica</button>
              <!-- Elimina resta visibile anche disattivato: dice che l'azione
                   esiste, e la nota sotto la lista dice perché non si può. -->
              <button
                type="button"
                :disabled="busy || (t.gameCount ?? 0) > 0 || store.list.length === 1"
                @click="remove(t.id)"
              >
                Elimina
              </button>
            </div>
          </template>
        </li>
      </ul>
      <p class="field-hint type-admin-note">
        Una tipologia che ha giochi non si elimina: cambia prima la tipologia di quei giochi dalla loro scheda.
        Ne serve sempre almeno una.
      </p>
    </div>

    <form class="panel-card type-admin-edit" @submit.prevent="create">
      <div class="section-head"><h2>Nuova tipologia</h2></div>
      <label>Nome <input v-model="newType.name" required placeholder="Magic: The Gathering" /></label>
      <label>Sigla <input v-model="newType.slug" required maxlength="6" placeholder="MTG" /></label>
      <label class="checkbox-label">
        <input v-model="newType.bggSearch" type="checkbox" /> Cerca su BoardGameGeek
      </label>
      <div class="type-admin-actions">
        <button type="submit" :disabled="busy">Aggiungi</button>
      </div>
    </form>
  </div>
</template>
