<script setup lang="ts">
/**
 * L'elenco dei componenti fisici del gioco (tessere, meeple, plance...) che
 * chi presta il gioco spunta uno a uno alla riconsegna. Vive nella scheda
 * admin del gioco, sorella di `SuggestedQuestionsPanel` e con lo stesso
 * scheletro: `loading`/`saving`/`suggesting`/`error`, il confronto con
 * l'ultimo stato salvato invece di un flag `saved`, e un `busy` che spegne
 * ogni bottone finché una richiesta è in volo.
 *
 * Due scelte che chi legge questo file metterebbe in dubbio:
 *
 * 1. Si salva tutta la lista in un colpo solo (PUT che sostituisce l'array
 *    intero) e non riga per riga. L'ordine qui è un dato — è l'ordine in cui
 *    l'elenco compare al banco prestiti — quindi riordinare è già una
 *    scrittura sull'intera lista; salvare ogni riga a parte aggiungerebbe
 *    N richieste concorrenti che possono tornare in un ordine diverso da
 *    quello in cui sono partite, lasciando l'elenco mescolato.
 * 2. "Genera dal manuale" sta nella testata e non accanto a "Salva materiali":
 *    non è un'alternativa al salvataggio, è l'azione che *riempie* il
 *    pannello leggendo il manuale indicizzato. Vicino a "Salva" si leggerebbe
 *    come "scegli uno dei due per finire", mentre una proposta va sempre
 *    controllata e poi salvata con lo stesso bottone di sempre.
 */
import { computed, nextTick, onBeforeUpdate, onMounted, ref } from 'vue'

import { api } from '../api/client'

const props = defineProps<{
  gameId: number
  /** Senza provider AI la generazione dal manuale non è possibile. */
  aiConfigured: boolean
}>()

interface MaterialRow {
  /**
   * Identità della riga lato client, non l'id del catalogo: serve solo come
   * `:key` del `v-for`. Con la chiave sull'indice, spostare una voce in su
   * lasciava il focus sull'indice — cioè sulla riga scesa — e il secondo
   * Invio la riportava giù: dalla tastiera una voce non poteva salire di due
   * posti. Un contatore basta: vive quanto il pannello.
   */
  uid: number
  name: string
  quantity: string
}

interface MaterialDTO {
  id?: number
  name: string
  quantity: number
}

let nextUid = 0
function newRow(name: string, quantity: string): MaterialRow {
  nextUid += 1
  return { uid: nextUid, name, quantity }
}

const rows = ref<MaterialRow[]>([])
const loading = ref(false)
const saving = ref(false)
const suggesting = ref(false)
const error = ref('')
// La lista attuale viene da "Genera dal manuale" e non è ancora stata
// salvata: lo dice questo booleano, non l'assenza di un id, perché anche una
// proposta può arrivare già coi campi giusti.
const proposal = ref(false)

// Lo stato all'ultimo salvataggio riuscito, oppure null. Confrontarlo con
// `rows` invece di tenere un flag `saved` evita che il messaggio "Materiali
// salvati" resti visibile mentre l'admin sta ancora modificando un campo.
const savedRows = ref<MaterialRow[] | null>(null)
const saved = computed(
  () =>
    savedRows.value !== null &&
    savedRows.value.length === rows.value.length &&
    savedRows.value.every((r, i) => r.name === rows.value[i].name && r.quantity === rows.value[i].quantity),
)

// Salva e Genera scrivono lo stesso stato (`rows`): se partono insieme, la
// risposta più lenta sovrascrive in silenzio il lavoro dell'altra. Ogni
// bottone si spegne quindi finché una qualunque delle due è in volo.
const busy = computed(() => saving.value || suggesting.value)

// Quel che l'esito racconta a chi non vede lo schermo: un salvataggio e una
// generazione dal manuale non hanno altro segnale che il testo del bottone
// che li ha avviati, e quel testo un lettore di schermo non lo rilegge da
// solo quando cambia.
const status = computed(() => {
  if (saving.value) return 'Salvataggio in corso.'
  if (suggesting.value) return 'Generazione dal manuale in corso.'
  if (error.value) return ''
  if (saved.value) return 'Materiali salvati.'
  return ''
})

const path = computed(() => `/games/${props.gameId}/materials`)

function rowLabel(row: MaterialRow, i: number) {
  return row.name.trim() || `voce ${i + 1}`
}

/**
 * La quantità accetta solo cifre, come il campo della modale di riconsegna
 * (`onReturnedInput` in `LoanDeskView`): `inputmode="numeric"` è un
 * suggerimento alla tastiera, non un vincolo, e "abc" diventava `NaN`, cioè
 * `null` nel JSON e 0 per il server, che rifiutava tutta la lista.
 */
function onQuantityInput(row: MaterialRow, event: Event) {
  const el = event.target as HTMLInputElement
  const digits = el.value.replace(/\D/g, '')
  row.quantity = digits
  // Rimette a posto il campo quando il filtro ha scartato qualcosa: il
  // valore legato non è cambiato, quindi Vue non ridisegnerebbe niente e nel
  // DOM resterebbe il testo rifiutato.
  if (el.value !== digits) {
    el.value = digits
  }
}

// Il server rifiuta la lista *intera* se una riga ha il nome vuoto o la
// quantità sotto 1, e il messaggio non dice quale riga: dopo "Aggiungi voce"
// bastava premere Salva per perdere tutto il pannello in un errore muto. Le
// righe si nominano una per una — con sessanta voci "manca qualcosa"
// costringerebbe a ricontrollarle tutte.
const incompleteLabels = computed(() =>
  rows.value
    .map((row, i) => ({
      label: rowLabel(row, i),
      bad: row.name.trim() === '' || Number(row.quantity) < 1,
    }))
    .filter((r) => r.bad)
    .map((r) => r.label),
)
const incomplete = computed(() => incompleteLabels.value.length > 0)

// Ref sull'ultimo input "nome" per portarci il focus dopo "Aggiungi voce".
// Pattern da elenco dinamico: l'array si azzera prima di ogni patch e lo
// ripopolano le callback `:ref` del v-for, altrimenti dopo una rimozione o
// un riordino resterebbero indici rivolti a input non più in quella riga.
let nameInputs: (HTMLInputElement | null)[] = []
// Le due frecce, per rimettere il focus dov'era finita la riga: il bottone
// premuto si sposta con lei, e quando arriva in cima o in fondo sparisce del
// tutto — senza questo il focus tornerebbe al `<body>` a metà di un
// riordino fatto da tastiera.
let upButtons: (HTMLButtonElement | null)[] = []
let downButtons: (HTMLButtonElement | null)[] = []
onBeforeUpdate(() => {
  nameInputs = []
  upButtons = []
  downButtons = []
})
// I `null` non si scrivono: quando una riga cambia posto Vue chiama la
// callback della vecchia riga con `null` e l'indice *vecchio*, che dopo lo
// spostamento appartiene già a un altro bottone. Gli array si azzerano a ogni
// patch, quindi ignorare i `null` non lascia comunque niente di stantio.
function setNameInputRef(el: Element | null, i: number) {
  if (el) nameInputs[i] = el as HTMLInputElement
}
function setUpButtonRef(el: Element | null, i: number) {
  if (el) upButtons[i] = el as HTMLButtonElement
}
function setDownButtonRef(el: Element | null, i: number) {
  if (el) downButtons[i] = el as HTMLButtonElement
}

async function load() {
  loading.value = true
  error.value = ''
  savedRows.value = null
  proposal.value = false
  try {
    const res = await api.get<{ materials: MaterialDTO[] }>(path.value)
    rows.value = res.materials.map((m) => newRow(m.name, String(m.quantity)))
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile leggere i materiali.'
  } finally {
    loading.value = false
  }
}

async function addRow() {
  if (busy.value) return
  rows.value.push(newRow('', ''))
  await nextTick()
  nameInputs[rows.value.length - 1]?.focus()
}

function removeRow(i: number) {
  if (busy.value) return
  rows.value.splice(i, 1)
}

async function move(from: number, to: number) {
  if (busy.value || to < 0 || to >= rows.value.length) return
  const [row] = rows.value.splice(from, 1)
  rows.value.splice(to, 0, row)
  await nextTick()
  // Il focus segue la voce, non la posizione: si riprende la stessa freccia
  // all'indice nuovo. Se la voce è arrivata al capo della lista quella
  // freccia non c'è più, e allora prende l'altra della stessa riga —
  // l'unico bottone da cui il riordino può continuare.
  const same = to < from ? upButtons : downButtons
  const other = to < from ? downButtons : upButtons
  ;(same[to] ?? other[to])?.focus()
}

function moveUp(i: number) {
  void move(i, i - 1)
}

function moveDown(i: number) {
  void move(i, i + 1)
}

async function save() {
  if (busy.value || incomplete.value) return
  saving.value = true
  error.value = ''
  try {
    const res = await api.put<{ materials: MaterialDTO[] }>(path.value, {
      materials: rows.value.map((r) => ({ name: r.name.trim(), quantity: Number(r.quantity) })),
    })
    rows.value = res.materials.map((m) => newRow(m.name, String(m.quantity)))
    savedRows.value = rows.value.map((r) => ({ ...r }))
    proposal.value = false
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Impossibile salvare i materiali.'
  } finally {
    saving.value = false
  }
}

async function suggest() {
  if (busy.value || !props.aiConfigured) return
  suggesting.value = true
  error.value = ''
  try {
    const res = await api.post<{ materials: MaterialDTO[] }>(`${path.value}/suggest`)
    rows.value = res.materials.map((m) => newRow(m.name, String(m.quantity)))
    proposal.value = true
    savedRows.value = null
  } catch (e) {
    // Un 422/502 qui è già il messaggio giusto da mostrare così com'è: dice
    // se manca un manuale indicizzato, un provider AI, o se il modello ha
    // risposto in un formato inutilizzabile.
    error.value = e instanceof Error ? e.message : 'Impossibile generare i materiali dal manuale.'
  } finally {
    suggesting.value = false
  }
}

onMounted(load)
</script>

<template>
  <form class="game-materials" @submit.prevent="save">
    <div class="game-materials-head">
      <div class="section-head">
        <h3>Materiali</h3>
        <button
          type="button"
          class="btn-secondary is-compact"
          :disabled="busy || !props.aiConfigured"
          :aria-describedby="props.aiConfigured ? undefined : 'gm-no-ai'"
          @click="suggest"
        >
          {{ suggesting ? 'Generazione…' : 'Genera dal manuale' }}
        </button>
      </div>

      <p v-if="!props.aiConfigured" id="gm-no-ai" class="empty-note">
        Nessun provider AI configurato: controlla le impostazioni.
      </p>
    </div>

    <p v-if="loading" class="empty-note">Caricamento…</p>

    <template v-else>
      <p v-if="proposal" class="empty-note">Proposta dal manuale: correggi quel che serve, poi salva.</p>

      <p v-if="rows.length === 0" class="empty-note">
        Serve alla riconsegna: ogni voce diventa una casella da spuntare quando il gioco torna.
      </p>

      <ol v-else class="game-materials-list" role="list" :aria-busy="suggesting">
        <li v-for="(row, i) in rows" :key="row.uid" class="game-materials-row">
          <label :for="`gm-name-${i}`" class="visually-hidden">Nome materiale {{ i + 1 }}</label>
          <input
            :id="`gm-name-${i}`"
            :ref="(el) => setNameInputRef(el as Element | null, i)"
            v-model="row.name"
            type="text"
            maxlength="60"
            placeholder="Nome (es. tessere)"
            :disabled="busy"
          />
          <label :for="`gm-qty-${i}`" class="visually-hidden">Quantità materiale {{ i + 1 }}</label>
          <input
            :id="`gm-qty-${i}`"
            :value="row.quantity"
            type="text"
            inputmode="numeric"
            maxlength="4"
            placeholder="Quantità"
            class="game-materials-qty"
            :disabled="busy"
            @input="onQuantityInput(row, $event)"
          />
          <div class="game-materials-row-actions">
            <button
              v-if="i > 0"
              :ref="(el) => setUpButtonRef(el as Element | null, i)"
              type="button"
              class="btn-secondary game-materials-move-up"
              :disabled="busy"
              :aria-label="`Sposta ${rowLabel(row, i)} in su`"
              @click="moveUp(i)"
            >
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="M6 14.5 12 8.5 18 14.5"
                  stroke="currentColor"
                  stroke-width="1.7"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </button>
            <button
              v-if="i < rows.length - 1"
              :ref="(el) => setDownButtonRef(el as Element | null, i)"
              type="button"
              class="btn-secondary game-materials-move-down"
              :disabled="busy"
              :aria-label="`Sposta ${rowLabel(row, i)} in giù`"
              @click="moveDown(i)"
            >
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="M6 9.5 12 15.5 18 9.5"
                  stroke="currentColor"
                  stroke-width="1.7"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
            </button>
            <button
              type="button"
              class="btn-danger game-materials-remove"
              :disabled="busy"
              :aria-label="`Rimuovi ${rowLabel(row, i)}`"
              @click="removeRow(i)"
            >
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="M5.4 6.9h13.2M9.9 6.9V4.7h4.2v2.2M7.3 6.9l.8 12.4h7.8l.8-12.4"
                  stroke="currentColor"
                  stroke-width="1.7"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
                <path d="M10.6 10.3v5.9M13.4 10.3v5.9" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
              </svg>
            </button>
          </div>
        </li>
      </ol>

      <button type="button" class="btn-secondary btn-with-icon game-materials-add" :disabled="busy" @click="addRow">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
        Aggiungi voce
      </button>

      <!-- L'esito, per chi non vede lo schermo: salvataggio e generazione
           durano da un attimo a qualche secondo e cambiano campi che uno
           screen reader non rilegge da solo. -->
      <p class="visually-hidden" role="status" aria-live="polite">{{ status }}</p>

      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-else-if="saved" class="empty-note">Materiali salvati.</p>

      <!-- Un solo messaggio per lo stato della lista, ed è anche il motivo
           per cui "Salva" è spento: il server rifiuta tutta la lista se una
           riga è incompleta, e prima la regola si scopriva solo dall'errore
           dopo il click, che per giunta non dice quale riga. -->
      <p v-if="incomplete" id="gm-save-note" class="empty-note">
        Da completare prima di salvare: {{ incompleteLabels.join(', ') }}. Ogni voce vuole un nome e
        una quantità di almeno 1.
      </p>

      <div class="form-actions">
        <button
          type="submit"
          :disabled="busy || incomplete"
          :aria-describedby="incomplete ? 'gm-save-note' : undefined"
        >
          {{ saving ? 'Salvataggio…' : 'Salva materiali' }}
        </button>
      </div>
    </template>
  </form>
</template>
