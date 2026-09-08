<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/client'

/**
 * Prepara un manuale PDF per le domande: estrae o trascrive, mostra il
 * risultato pagina per pagina, e salva solo dopo la conferma.
 *
 * L'anteprima editabile non è una comodità: la trascrizione di uno scan
 * può uscire disordinata, e senza la possibilità di correggerla l'admin
 * resterebbe bloccato. Il testo salvato è la fonte di verità, non una
 * cache di quel che ha detto il modello.
 */
const props = defineProps<{
  gameId: number
  lang: string
  mediaId: number
  mediaTitle: string
}>()

interface ManualPage {
  pageNumber: number
  text: string
  heading: string
  source: string
}

const saved = ref<ManualPage[]>([])
const draft = ref<ManualPage[] | null>(null)
const source = ref('')
// Un discriminatore invece di tre booleani: senza, un unico `busy` condiviso
// fa leggere a chi usa uno screen reader "Salvataggio…" sul bottone
// "Prepara di nuovo" che non ha mai toccato — l'azione in corso non è
// quella del controllo che la mostra. Ogni bottone legge solo il proprio
// stato; `busy` sotto resta il segnale unico per il disabled di tutti.
const busyAction = ref<'prepare' | 'save' | 'remove' | null>(null)
const busy = computed(() => busyAction.value !== null)
const error = ref('')
const notice = ref('')
const loadError = ref('')

const base = `/games/${props.gameId}/languages/${props.lang}/media/${props.mediaId}`

async function loadSaved() {
  loadError.value = ''
  try {
    const res = await api.get<{ pages: ManualPage[] }>(`${base}/pages`)
    saved.value = res.pages || []
  } catch (e) {
    // Un errore qui non deve leggersi come "non preparato": sono due stati
    // diversi, e confondere il secondo col primo dice all'admin una cosa
    // falsa sui propri dati.
    loadError.value =
      e instanceof Error
        ? `Non riesco a leggere lo stato di questo manuale: ${e.message}`
        : 'Non riesco a leggere lo stato di questo manuale.'
  }
}

// Chiudere la scheda con una bozza aperta perde le correzioni tanto quanto
// "Prepara di nuovo" le perde — stessa causa, un'altra via. Il listener si
// registra solo mentre esiste, e la guardia (draft non nullo) sta dentro
// alla funzione, non nell'attach/detach, così basta un solo listener per
// tutta la vita del componente.
function handleBeforeUnload(e: BeforeUnloadEvent) {
  if (draft.value) {
    e.preventDefault()
    e.returnValue = ''
  }
}

onMounted(() => {
  loadSaved()
  window.addEventListener('beforeunload', handleBeforeUnload)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', handleBeforeUnload)
})

async function prepare() {
  // La bozza è già una correzione umana, non solo la proposta del modello:
  // ripartire senza chiedere la butterebbe via in silenzio. Stessa logica
  // di conferma di remove(), che dice cosa si perde invece di "sei sicuro?".
  if (
    draft.value &&
    !window.confirm(
      `Rifare la preparazione di "${props.mediaTitle}"? Le correzioni scritte finora in questa bozza vengono perse e sostituite da un nuovo tentativo.`,
    )
  ) {
    return
  }
  busyAction.value = 'prepare'
  error.value = ''
  notice.value = ''
  draft.value = null
  try {
    const res = await api.post<{ source: string; pages: ManualPage[] }>(`${base}/extract`)
    source.value = res.source
    draft.value = res.pages
    const empty = res.pages.filter((p) => !p.text.trim()).length
    if (empty > 0) {
      notice.value =
        `${empty} pagine su ${res.pages.length} sono uscite vuote: ` +
        'scrivile a mano qui sotto, oppure controlla il modello per i manuali ' +
        'scansionati nelle impostazioni prima di riprovare.'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Preparazione non riuscita.'
  } finally {
    busyAction.value = null
  }
}

async function save() {
  if (!draft.value) {
    return
  }
  busyAction.value = 'save'
  error.value = ''
  notice.value = ''
  try {
    const res = await api.put<{ pages: ManualPage[] }>(`${base}/pages`, {
      pages: draft.value.map((p) => ({
        pageNumber: p.pageNumber,
        text: p.text,
        // Una pagina toccata a mano resta marcata come veniva: il campo
        // dice da dove arriva il testo, non chi l'ha rivisto.
        source: p.source,
      })),
    })
    saved.value = res.pages || []
    draft.value = null
    notice.value = 'Manuale indicizzato: le domande sulla scheda pubblica ora funzionano.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Salvataggio non riuscito.'
  } finally {
    busyAction.value = null
  }
}

async function remove() {
  // Stessa causa del confirm in prepare(), altra via: una bozza aperta porta
  // correzioni non salvate, e "Rimuovi indice" resta visibile e attivo anche
  // mentre draft non è nullo. Il testo deve dirlo, non solo che l'indice sparisce.
  const draftWarning = draft.value
    ? ' Le correzioni scritte finora nella bozza aperta vengono perse insieme all\'indice.'
    : ''
  if (!window.confirm(`Rimuovere l'indice di "${props.mediaTitle}"? Le domande su questo gioco smettono di funzionare finché non lo prepari di nuovo.${draftWarning}`)) {
    return
  }
  busyAction.value = 'remove'
  error.value = ''
  notice.value = ''
  try {
    await api.delete(`${base}/pages`)
    saved.value = []
    draft.value = null
    notice.value = 'Indice rimosso: la chat non compare più per questo gioco.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Rimozione non riuscita.'
  } finally {
    busyAction.value = null
  }
}

function discardDraft() {
  draft.value = null
  notice.value = ''
}
</script>

<template>
  <div class="manual-prep">
    <!--
      Un solo gioco può avere più di un PDF (regolamento + espansione): il
      titolo distingue quale pannello prepara quale manuale, dove
      GameMediaList sopra mostra già le tessere ma non questa sezione.
    -->
    <h3 class="manual-prep-title">{{ mediaTitle }}</h3>
    <p class="manual-prep-state">
      <template v-if="loadError">{{ loadError }}</template>
      <template v-else-if="saved.length">{{ saved.length }} pagine indicizzate.</template>
      <template v-else>Non preparato per le domande.</template>
    </p>

    <div class="manual-prep-actions">
      <button type="button" :disabled="busy" @click="prepare">
        {{
          busyAction === 'prepare'
            ? 'Lettura del manuale in corso, pagina per pagina…'
            : saved.length
              ? 'Prepara di nuovo'
              : 'Prepara per le domande'
        }}
      </button>
      <button v-if="saved.length" type="button" class="btn-danger" :disabled="busy" @click="remove">
        {{ busyAction === 'remove' ? 'Rimozione…' : 'Rimuovi indice' }}
      </button>
    </div>

    <!--
      Stesso schema di SettingsView per smtpTestMessage/smtpTestError: una
      regione live montata a permanenza (uno screen reader non annuncia un
      v-if che entra nel DOM già col testo dentro) e sotto il riquadro
      visibile, puramente presentazionale.
    -->
    <p class="visually-hidden" role="alert" aria-live="assertive">{{ error }}</p>
    <p class="visually-hidden" role="status" aria-live="polite">{{ notice }}</p>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="empty-note">{{ notice }}</p>

    <div v-if="draft" class="manual-prep-draft">
      <p class="row-meta">
        {{ source === 'vision' ? 'Trascritto dalle immagini delle pagine' : 'Estratto dal testo del PDF' }}
        — correggi quel che serve, poi salva. Il testo che salvi qui sostituisce
        quello che vedi, non solo quello che il modello ha proposto.
      </p>
      <div class="manual-prep-pages">
        <div v-for="(page, i) in draft" :key="page.pageNumber" class="manual-prep-page">
          <label :for="`manual-page-${mediaId}-${i}`">
            Pagina {{ page.pageNumber }}
            <span v-if="!page.text.trim()" class="manual-prep-page-empty"> — vuota, da scrivere a mano</span>
          </label>
          <textarea
            :id="`manual-page-${mediaId}-${i}`"
            v-model="draft[i].text"
            rows="8"
            spellcheck="false"
          ></textarea>
        </div>
      </div>
      <div class="manual-prep-actions">
        <button type="button" :disabled="busy" @click="save">
          {{ busyAction === 'save' ? 'Salvataggio…' : 'Salva e indicizza' }}
        </button>
        <button type="button" class="btn-secondary" :disabled="busy" @click="discardDraft">Annulla</button>
      </div>
    </div>
  </div>
</template>
