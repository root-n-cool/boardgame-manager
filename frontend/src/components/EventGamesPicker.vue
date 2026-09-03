<script setup lang="ts">
import { computed, ref } from 'vue'

/**
 * Scelta dei giochi di un evento. Le due pagine che la usano — creazione e
 * scheda — avevano la stessa logica copiata due volte; qui sta una sola volta.
 *
 * Le righe già scelte si staccano in cima con la loro quantità di copie: con
 * un catalogo che cresce, quello che conta è cosa c'è sul tavolo, non dove
 * finisce un nome nell'ordine alfabetico. Il resto si filtra per nome.
 */
export interface PickerGame {
  id: number
  name: string
  /** Posti prenotabili per copia, dal catalogo. */
  seats: number
}

export interface SelectedGame {
  gameId: number
  copies: number
}

const props = defineProps<{
  games: PickerGame[]
  modelValue: SelectedGame[]
  /**
   * Copie con almeno una prenotazione attiva, per gioco (solo nella scheda
   * di un evento esistente): il backend rifiuta di scendere sotto quel
   * numero di copie, quindi qui il minimo si alza e il gioco non si può
   * togliere — meglio un campo che non scende che un 409 dopo il salvataggio.
   */
  occupiedCopies?: Record<number, number>
  /**
   * Copie che il gioco aveva già prima di aprire questo form, per gioco
   * (solo nella scheda di un evento esistente): quelle copie portano una
   * fotografia dei posti prenotabili presa quando sono nate, che può non
   * coincidere più con `game.seats` del catalogo — moltiplicarle per il
   * valore di adesso darebbe un totale falso. Un gioco assente da questa
   * mappa è appena stato scelto in questo form: tutte le sue copie nascono
   * ora, quindi il valore del catalogo è esatto e il totale si può mostrare.
   */
  existingCopies?: Record<number, number>
}>()

const emit = defineEmits<{ 'update:modelValue': [value: SelectedGame[]] }>()

const query = ref('')

const selectedRows = computed(() =>
  props.modelValue
    .map((s) => ({ game: props.games.find((g) => g.id === s.gameId), copies: s.copies }))
    .filter((row): row is { game: PickerGame; copies: number } => row.game !== undefined),
)

const available = computed(() =>
  props.games.filter((g) => !props.modelValue.some((s) => s.gameId === g.id)),
)

const filtered = computed(() => {
  const needle = query.value.trim().toLowerCase()
  if (!needle) {
    return available.value
  }
  return available.value.filter((g) => g.name.toLowerCase().includes(needle))
})

// Sotto una manciata di giochi il campo di ricerca è solo un ingombro: si
// vedono tutti insieme.
const showSearch = computed(() => props.games.length > 6)

function occupiedFor(gameId: number) {
  return props.occupiedCopies?.[gameId] ?? 0
}

function add(gameId: number) {
  emit('update:modelValue', [...props.modelValue, { gameId, copies: 1 }])
}

function remove(gameId: number) {
  emit('update:modelValue', props.modelValue.filter((s) => s.gameId !== gameId))
}

function setCopies(gameId: number, copies: number) {
  const min = Math.max(1, occupiedFor(gameId))
  emit(
    'update:modelValue',
    props.modelValue.map((s) =>
      s.gameId === gameId ? { ...s, copies: Number.isFinite(copies) ? Math.max(min, copies) : min } : s,
    ),
  )
}

/**
 * "× 5 posti prenotabili = 10 in tutto", solo quando c'è qualcosa da
 * spiegare e quando il conto è vero: se il gioco ha già copie sue da prima
 * di questo form, quelle portano una fotografia di posti prenotabili che
 * può differire dal valore corrente del catalogo, e moltiplicare tutte le
 * copie per quel valore darebbe un totale falso.
 */
function capacityLabel(game: PickerGame, copies: number) {
  if (game.seats <= 1 || (props.existingCopies?.[game.id] ?? 0) > 0) {
    return ''
  }
  return `× ${game.seats} posti prenotabili = ${copies * game.seats} in tutto`
}
</script>

<template>
  <div class="games-picker">
    <ul v-if="selectedRows.length > 0" role="list" class="games-picker-chosen">
      <li v-for="row in selectedRows" :key="row.game.id" class="game-select-row">
        <label class="checkbox-label">
          <input
            type="checkbox"
            checked
            :disabled="occupiedFor(row.game.id) > 0"
            @change="remove(row.game.id)"
          />
          {{ row.game.name }}
        </label>
        <span class="game-select-quantity">
          <span v-if="occupiedFor(row.game.id) > 0" class="game-select-booked">
            {{
              occupiedFor(row.game.id) === 1
                ? '1 copia occupata'
                : `${occupiedFor(row.game.id)} copie occupate`
            }}
          </span>
          <label class="game-select-copies">
            copie
            <input
              type="number"
              :min="Math.max(1, occupiedFor(row.game.id))"
              :value="row.copies"
              @input="setCopies(row.game.id, Number(($event.target as HTMLInputElement).value))"
            />
          </label>
          <span v-if="capacityLabel(row.game, row.copies)" class="game-select-seats">
            {{ capacityLabel(row.game, row.copies) }}
          </span>
        </span>
      </li>
    </ul>

    <p v-if="games.length === 0" class="empty-note">
      Il catalogo è vuoto:
      <router-link :to="{ name: 'admin-game-new' }">aggiungi un gioco</router-link>
      e poi torna qui.
    </p>

    <template v-else>
      <label v-if="showSearch" class="games-picker-search">
        Cerca nel catalogo
        <input v-model="query" type="search" placeholder="Nome del gioco" />
      </label>

      <ul v-if="filtered.length > 0" role="list" class="games-picker-list">
        <li v-for="g in filtered" :key="g.id" class="game-select-row">
          <label class="checkbox-label">
            <input type="checkbox" :checked="false" @change="add(g.id)" />
            {{ g.name }}
          </label>
        </li>
      </ul>
      <p v-else class="empty-note">
        {{
          query.trim()
            ? `Nessun gioco in catalogo corrisponde a “${query.trim()}”.`
            : 'Tutti i giochi del catalogo sono già sul tavolo.'
        }}
      </p>
    </template>
  </div>
</template>
