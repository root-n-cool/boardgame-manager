<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { GameMaterialInfo } from '../utils/game'

/**
 * Il contenuto della scatola come checklist per chi gioca: prima di
 * riconsegnare si spunta voce per voce. Le spunte non vanno al server —
 * il controllo ufficiale resta quello dell'admin al banco prestiti — e
 * restano solo su questo telefono, per gioco, così un'occhiata alla chat o
 * un refresh a metà controllo non fanno ripartire da capo.
 *
 * La chiave è il nome della voce, non un id: la scheda pubblica non espone
 * gli id, e il nome è già unico per gioco (UNIQUE(game_id, name)).
 */
const props = defineProps<{ gameId: number; materials: GameMaterialInfo[] }>()

const storageKey = computed(() => `bgm:materials-check:${props.gameId}`)

function readChecked(): Set<string> {
  try {
    const raw = localStorage.getItem(storageKey.value)
    const parsed = raw ? JSON.parse(raw) : []
    return new Set(Array.isArray(parsed) ? parsed : [])
  } catch {
    // Navigazione privata o storage disabilitato: la checklist funziona
    // lo stesso, semplicemente non sopravvive al refresh.
    return new Set()
  }
}

const checked = ref<Set<string>>(readChecked())

watch(checked, (value) => {
  try {
    if (value.size === 0) {
      localStorage.removeItem(storageKey.value)
    } else {
      localStorage.setItem(storageKey.value, JSON.stringify([...value]))
    }
  } catch {
    // Stesso discorso di readChecked.
  }
})

// Una voce tolta dal catalogo dopo che l'avevi spuntata non deve contare
// nel "7 di 7": il conteggio guarda solo le voci che ci sono adesso.
const doneCount = computed(() => props.materials.filter((m) => checked.value.has(m.name)).length)
const allDone = computed(() => doneCount.value === props.materials.length)

function toggle(name: string, on: boolean) {
  const next = new Set(checked.value)
  if (on) {
    next.add(name)
  } else {
    next.delete(name)
  }
  checked.value = next
}

function reset() {
  checked.value = new Set()
}
</script>

<template>
  <section class="panel-card materials-checklist" aria-labelledby="materials-checklist-title">
    <div class="section-head">
      <h2 id="materials-checklist-title">Controllo componenti prima della riconsegna</h2>
      <span class="lang-chip materials-checklist-progress" :class="{ 'is-done': allDone }" aria-live="polite">
        {{ doneCount }} di {{ materials.length }}
        {{ materials.length === 1 ? 'controllata' : 'controllate' }}
      </span>
    </div>
    <p class="materials-checklist-intro">Spunta ogni voce mentre rimetti il gioco nella scatola.</p>
    <ul class="materials-checklist-list" role="list">
      <li v-for="m in materials" :key="m.name">
        <label class="checkbox-label" :class="{ 'is-checked': checked.has(m.name) }">
          <input
            type="checkbox"
            :checked="checked.has(m.name)"
            @change="toggle(m.name, ($event.target as HTMLInputElement).checked)"
          />
          <span class="materials-checklist-qty">{{ m.quantity }}&thinsp;×</span>
          <span class="material-check-name">{{ m.name }}</span>
        </label>
      </li>
    </ul>
    <p v-if="allDone" class="materials-checklist-done">
      Tutto a posto: puoi riconsegnare il gioco.
    </p>
    <button v-if="doneCount > 0" type="button" class="btn-secondary is-compact" @click="reset">
      Azzera
    </button>
  </section>
</template>
