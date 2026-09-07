<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import ModalDialog from '../components/ModalDialog.vue'
import { formatEventDateTime } from '../utils/dates'

/** Una prenotazione attiva su una copia, come la manda il banco. */
interface CopyBooking {
  id: number
  name: string
  phone: string
}

interface OpenLoan {
  id: number
  borrowerName: string
  borrowerPhone: string
  lentAt: string
  notes: string | null
}

interface DeskCopy {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  copyIndex: number
  /** Quante copie di questo gioco ha la serata: sotto 2, "#1" è rumore. */
  copies: number
  bookable: boolean
  seats: number
  activeBookings: CopyBooking[]
  openLoan: OpenLoan | null
}

interface ReturnedLoan {
  id: number
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  borrowerName: string
  borrowerPhone: string
  lentAt: string
  returnedAt: string
  notes: string | null
}

interface LoanDesk {
  copies: DeskCopy[]
  returned: ReturnedLoan[]
}

interface EventHeader {
  title: string
  eventDate: string
  startTime: string
}

const route = useRoute()
const eventId = route.params.id as string

const desk = ref<LoanDesk>({ copies: [], returned: [] })
const eventTitle = ref('')
const eventWhen = ref('')
const error = ref('')
const loading = ref(true)

/** La copia che si sta consegnando, o null se la modale è chiusa. */
const lending = ref<DeskCopy | null>(null)
const borrowerName = ref('')
const borrowerPhone = ref('')
const lendNotes = ref('')
const lendError = ref('')
const lendSaving = ref(false)

/** Il prestito che si sta chiudendo, con la sua copia per l'etichetta. */
const returning = ref<{ copy: DeskCopy; loan: OpenLoan } | null>(null)
const returnNotes = ref('')
const returnError = ref('')
const returnSaving = ref(false)

const logOpen = ref(false)

const out = computed(() =>
  desk.value.copies.filter((c): c is DeskCopy & { openLoan: OpenLoan } => c.openLoan !== null),
)
const available = computed(() => desk.value.copies.filter((c) => c.openLoan === null))

/**
 * L'etichetta di una copia: il numero compare solo quando quel gioco ha
 * più di una copia nella serata.
 */
function copyLabel(copy: { name: string; copies: number; copyIndex: number }) {
  return copy.copies > 1 ? `${copy.name} #${copy.copyIndex}` : copy.name
}

function returnedLabel(row: ReturnedLoan) {
  // Il log non porta il conteggio delle copie, quindi lo cerca fra le
  // copie della serata; se la copia non c'è più, il numero si mostra
  // comunque perché senza di esso la riga sarebbe ambigua.
  const copy = desk.value.copies.find((c) => c.eventGameId === row.eventGameId)
  const copies = copy?.copies ?? 2
  return copies > 1 ? `${row.gameName} #${row.copyIndex}` : row.gameName
}

/** "da 25 minuti", che al tavolo è più utile di un orario. */
function since(iso: string) {
  const minutes = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000))
  if (minutes < 1) {
    return 'da poco'
  }
  if (minutes < 60) {
    return `da ${minutes} ${minutes === 1 ? 'minuto' : 'minuti'}`
  }
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest === 0
    ? `da ${hours} ${hours === 1 ? 'ora' : 'ore'}`
    : `da ${hours}h ${rest}′`
}

function clockTime(iso: string) {
  return new Date(iso).toLocaleTimeString('it-IT', { hour: '2-digit', minute: '2-digit' })
}

/**
 * Un nome che non compare fra i prenotati di una copia prenotata: si
 * consegna comunque — alle 21:30 chi non si è presentato non deve tenere
 * in ostaggio la scatola — ma l'avviso lo dice.
 */
const lendWarning = computed(() => {
  const copy = lending.value
  if (!copy || copy.activeBookings.length === 0) {
    return ''
  }
  const typed = borrowerName.value.trim().toLowerCase()
  if (typed === '') {
    return ''
  }
  if (copy.activeBookings.some((b) => b.name.trim().toLowerCase() === typed)) {
    return ''
  }
  return copy.activeBookings.length === 1
    ? `Questa copia è prenotata da ${copy.activeBookings[0].name}: la consegna a un altro nome resta registrata così.`
    : `Questa copia ha ${copy.activeBookings.length} prenotazioni: la consegna a un nome che non c'è resta registrata così.`
})

async function load() {
  const [event, loans] = await Promise.all([
    api.get<EventHeader>(`/events/${eventId}`),
    api.get<LoanDesk>(`/events/${eventId}/loans`),
  ])
  eventTitle.value = event.title
  eventWhen.value = formatEventDateTime(event.eventDate, event.startTime)
  desk.value = loans
}

function startLending(copy: DeskCopy) {
  lending.value = copy
  lendError.value = ''
  lendNotes.value = ''
  // Con una prenotazione sola non c'è niente da scegliere: si precompila.
  if (copy.activeBookings.length === 1) {
    borrowerName.value = copy.activeBookings[0].name
    borrowerPhone.value = copy.activeBookings[0].phone
  } else {
    borrowerName.value = ''
    borrowerPhone.value = ''
  }
}

function pickBooking(booking: CopyBooking) {
  borrowerName.value = booking.name
  borrowerPhone.value = booking.phone
}

/** La prenotazione da agganciare al prestito: quella col nome scelto. */
function matchedBooking(copy: DeskCopy) {
  const typed = borrowerName.value.trim().toLowerCase()
  return copy.activeBookings.find((b) => b.name.trim().toLowerCase() === typed) ?? null
}

async function submitLend() {
  const copy = lending.value
  if (!copy) {
    return
  }
  lendError.value = ''
  lendSaving.value = true
  try {
    const booking = matchedBooking(copy)
    await api.post(`/events/${eventId}/loans`, {
      eventGameId: copy.eventGameId,
      bookingId: booking ? booking.id : null,
      borrowerName: borrowerName.value,
      borrowerPhone: borrowerPhone.value,
      notes: lendNotes.value.trim() || null,
    })
    lending.value = null
    await load()
  } catch (e) {
    lendError.value = (e as Error).message
  } finally {
    lendSaving.value = false
  }
}

function startReturning(copy: DeskCopy, loan: OpenLoan) {
  returning.value = { copy, loan }
  returnNotes.value = loan.notes ?? ''
  returnError.value = ''
}

async function submitReturn() {
  const current = returning.value
  if (!current) {
    return
  }
  returnError.value = ''
  returnSaving.value = true
  try {
    await api.post(`/loans/${current.loan.id}/return`, {
      notes: returnNotes.value.trim() || null,
    })
    returning.value = null
    await load()
  } catch (e) {
    returnError.value = (e as Error).message
  } finally {
    returnSaving.value = false
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <router-link :to="`/admin/events/${eventId}`" class="back-link">&larr; Evento</router-link>

    <div class="page-head">
      <div class="page-head-text">
        <h1>Banco prestiti</h1>
        <p v-if="eventTitle" class="page-meta">{{ eventTitle }} · {{ eventWhen }}</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <template v-if="!loading && !error">
      <div class="panel-card">
        <div class="section-head">
          <h2>Fuori</h2>
          <span class="section-count">{{ out.length }}</span>
        </div>
        <p v-if="out.length === 0" class="empty-note">
          Nessun gioco è fuori: tutte le scatole sono al banco.
        </p>
        <ul v-else role="list" class="loan-list">
          <li v-for="copy in out" :key="copy.eventGameId">
            <button type="button" class="loan-row" @click="startReturning(copy, copy.openLoan)">
              <span class="loan-row-text">
                <span class="loan-row-title">{{ copyLabel(copy) }}</span>
                <span v-if="!copy.bookable" class="loan-tag">Senza prenotazione</span>
                <span class="row-meta">
                  {{ copy.openLoan.borrowerName }} · {{ copy.openLoan.borrowerPhone }}
                </span>
                <span class="row-meta">
                  {{ since(copy.openLoan.lentAt) }}, dalle {{ clockTime(copy.openLoan.lentAt) }}
                </span>
                <span v-if="copy.openLoan.notes" class="loan-row-notes">
                  {{ copy.openLoan.notes }}
                </span>
              </span>
              <span class="loan-row-action">Restituito</span>
            </button>
          </li>
        </ul>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Disponibili</h2>
          <span class="section-count">{{ available.length }}</span>
        </div>
        <p v-if="available.length === 0 && desk.copies.length === 0" class="empty-note">
          Questa serata non ha ancora giochi:
          <router-link :to="`/admin/events/${eventId}`">aggiungili dalla scheda</router-link>.
        </p>
        <p v-else-if="available.length === 0" class="empty-note">
          Tutte le copie sono fuori.
        </p>
        <ul v-else role="list" class="loan-list">
          <li v-for="copy in available" :key="copy.eventGameId">
            <button type="button" class="loan-row" @click="startLending(copy)">
              <span class="loan-row-text">
                <span class="loan-row-title">{{ copyLabel(copy) }}</span>
                <span v-if="!copy.bookable" class="loan-tag">Senza prenotazione</span>
                <span v-if="copy.activeBookings.length > 0" class="row-meta">
                  prenotata da
                  {{ copy.activeBookings.map((b) => b.name).join(', ') }}
                </span>
              </span>
              <span class="loan-row-action">Consegna</span>
            </button>
          </li>
        </ul>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Restituiti</h2>
          <span class="section-count">{{ desk.returned.length }}</span>
          <button
            v-if="desk.returned.length > 0"
            type="button"
            class="btn-secondary"
            :aria-expanded="logOpen"
            @click="logOpen = !logOpen"
          >
            {{ logOpen ? 'Nascondi' : 'Mostra' }}
            <span class="visually-hidden">il registro delle restituzioni</span>
          </button>
        </div>
        <p v-if="desk.returned.length === 0" class="empty-note">
          Ancora nessuna restituzione in questa serata.
        </p>
        <ul v-else-if="logOpen" role="list" class="admin-list">
          <li v-for="row in desk.returned" :key="row.id">
            <div class="admin-row">
              <span class="admin-email booking-who">
                {{ returnedLabel(row) }}
                <span class="row-meta">
                  {{ row.borrowerName }} · {{ clockTime(row.lentAt) }}–{{ clockTime(row.returnedAt) }}
                </span>
                <!--
                  La nota è testo libero, non un dato: prende la stessa
                  resa che ha nella riga di "Fuori" (`.loan-row-notes`) e
                  non il mono di `.row-meta`, riservato a telefono e orari.
                -->
                <span v-if="row.notes" class="loan-row-notes">{{ row.notes }}</span>
              </span>
            </div>
          </li>
        </ul>
      </div>
    </template>

    <ModalDialog
      :open="lending !== null"
      :title="lending ? `Consegna: ${copyLabel(lending)}` : 'Consegna'"
      @close="lending = null"
    >
      <form v-if="lending" class="panel-form" @submit.prevent="submitLend">
        <div v-if="lending.activeBookings.length > 1" class="field-block">
          <span class="field-label">Prenotazioni su questa copia</span>
          <ul role="list" class="loan-booking-picks">
            <li v-for="b in lending.activeBookings" :key="b.id">
              <button type="button" @click="pickBooking(b)">
                {{ b.name }}
                <span class="row-meta">{{ b.phone }}</span>
              </button>
            </li>
          </ul>
        </div>

        <label>
          Nome
          <input v-model="borrowerName" required />
        </label>
        <label>
          Telefono
          <input v-model="borrowerPhone" required />
        </label>
        <label>
          <span>Note <span class="field-optional">(opzionale)</span></span>
          <textarea v-model="lendNotes"></textarea>
        </label>

        <!--
          Avviso ed errore si annunciano da due regioni live montate a
          permanenza — nate col testo dentro (`v-if`) sono il caso che gli
          screen reader in pratica non annunciano, vedi le impostazioni
          SMTP — e restano fuori dal flusso, così il `gap` del form non
          lascia uno spazio morto. L'avviso è `polite`: arriva mentre si
          scrive il nome e non deve interrompere la digitazione, né rubare
          il focus. I riquadri visibili sotto sono sola presentazione.
        -->
        <p class="visually-hidden" role="status" aria-live="polite">{{ lendWarning }}</p>
        <p class="visually-hidden" role="alert" aria-live="assertive">{{ lendError }}</p>

        <p v-if="lendWarning" class="loan-warning">{{ lendWarning }}</p>
        <p v-if="lendError" class="error">{{ lendError }}</p>

        <div class="form-actions">
          <button type="submit" :disabled="lendSaving">
            {{ lendSaving ? 'Consegna…' : 'Consegna' }}
          </button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog
      :open="returning !== null"
      :title="returning ? `Restituzione: ${copyLabel(returning.copy)}` : 'Restituzione'"
      @close="returning = null"
    >
      <form v-if="returning" class="panel-form" @submit.prevent="submitReturn">
        <p class="loan-modal-meta">
          {{ returning.loan.borrowerName }} · {{ returning.loan.borrowerPhone }},
          {{ since(returning.loan.lentAt) }}
        </p>
        <label>
          <span>Note <span class="field-optional">(opzionale)</span></span>
          <textarea v-model="returnNotes"></textarea>
        </label>

        <p class="visually-hidden" role="alert" aria-live="assertive">{{ returnError }}</p>
        <p v-if="returnError" class="error">{{ returnError }}</p>

        <div class="form-actions">
          <button type="submit" :disabled="returnSaving">
            {{ returnSaving ? 'Registrazione…' : 'Restituito' }}
          </button>
        </div>
      </form>
    </ModalDialog>
  </div>
</template>
