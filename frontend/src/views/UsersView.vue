<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api/client'
import ModalDialog from '../components/ModalDialog.vue'

interface AdminUser {
  id: number
  email: string
  createdAt: string
  pending: boolean
  inviteToken: string | null
}

// Gli errori dell'API sono in inglese perché sono codici, non copy: qui
// diventano le frasi che l'admin legge davvero.
const MESSAGES: Record<string, string> = {
  'email is required': "Inserisci l'email del nuovo amministratore.",
  'email already in use': 'Questa email è già registrata.',
  'cannot delete the last remaining user': "Non puoi eliminare l'unico amministratore attivo.",
  'user not found': 'Questo amministratore non esiste più.',
}

function toItalian(message: string) {
  return MESSAGES[message] ?? message
}

const users = ref<AdminUser[]>([])
const error = ref('')
const adding = ref(false)
const newEmail = ref('')
const emailInput = ref<HTMLInputElement | null>(null)
const saving = ref(false)
const copiedId = ref<number | null>(null)
const pendingDelete = ref<AdminUser | null>(null)

const countLabel = computed(() =>
  users.value.length === 1 ? '1 amministratore' : `${users.value.length} amministratori`,
)

function initial(email: string) {
  return email.trim().charAt(0) || '?'
}

// Il link lo compone il browser: il backend non conosce il proprio URL
// pubblico e non vogliamo una variabile d'ambiente in più per il selfhost.
function inviteUrl(user: AdminUser) {
  return `${window.location.origin}/invito/${user.inviteToken}`
}

async function loadUsers() {
  users.value = await api.get<AdminUser[]>('/users')
}

async function startAdding() {
  adding.value = true
  newEmail.value = ''
  error.value = ''
  await nextTick()
  emailInput.value?.focus()
}

function cancelAdding() {
  adding.value = false
  newEmail.value = ''
}

async function invite() {
  error.value = ''
  saving.value = true
  try {
    await api.post<AdminUser>('/users', { email: newEmail.value })
    cancelAdding()
    await loadUsers()
  } catch (e) {
    error.value = toItalian((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function copyInvite(user: AdminUser) {
  error.value = ''
  try {
    await navigator.clipboard.writeText(inviteUrl(user))
    copiedId.value = user.id
    window.setTimeout(() => {
      if (copiedId.value === user.id) {
        copiedId.value = null
      }
    }, 2000)
  } catch {
    // Clipboard non disponibile (origine non sicura, permesso negato): il
    // link resta a schermo e selezionabile, quindi basta dirlo.
    error.value = 'Copia non riuscita: seleziona il link e copialo a mano.'
  }
}

async function confirmDelete() {
  const target = pendingDelete.value
  if (!target) {
    return
  }
  pendingDelete.value = null
  error.value = ''
  try {
    await api.delete(`/users/${target.id}`)
    await loadUsers()
  } catch (e) {
    error.value = toItalian((e as Error).message)
  }
}

onMounted(async () => {
  // Senza il try questa diventa una unhandled rejection e una pagina vuota
  // ogni volta che la richiesta fallisce per un motivo diverso dal 401.
  try {
    await loadUsers()
  } catch (e) {
    error.value = toItalian((e as Error).message)
  }
})
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Amministratori</h1>
        <p class="page-meta">{{ countLabel }}</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <div class="panel-card">
      <ul role="list" class="admin-list">
        <li v-for="u in users" :key="u.id">
          <div class="admin-row" :class="{ 'is-pending': u.pending }">
            <span class="admin-pawn" aria-hidden="true">{{ initial(u.email) }}</span>
            <span class="admin-email">{{ u.email }}</span>
            <span class="status-badge" :class="u.pending ? 'status-pending' : 'status-active'">
              {{ u.pending ? 'In attesa' : 'Attivo' }}
            </span>
            <div class="admin-row-actions">
              <button
                v-if="u.pending && u.inviteToken"
                type="button"
                class="btn-invite"
                @click="copyInvite(u)"
              >
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M10 13a5 5 0 007.07 0l2.12-2.12a5 5 0 00-7.07-7.07L10.7 5.24M14 11a5 5 0 00-7.07 0L4.81 13.12a5 5 0 007.07 7.07l1.42-1.41"
                    stroke="currentColor"
                    stroke-width="1.7"
                    stroke-linecap="round"
                  />
                </svg>
                Copia link invito
              </button>
              <button type="button" @click="pendingDelete = u">
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M4 7h16M9 7V5a1 1 0 011-1h4a1 1 0 011 1v2M6 7l1 12a1 1 0 001 1h8a1 1 0 001-1l1-12M10 11v6M14 11v6"
                    stroke="currentColor"
                    stroke-width="1.7"
                    stroke-linecap="round"
                  />
                </svg>
                Elimina
              </button>
            </div>
            <div v-if="u.pending && u.inviteToken" class="admin-invite">
              <code>{{ inviteUrl(u) }}</code>
              <span v-if="copiedId === u.id" class="admin-invite-copied" role="status">Copiato</span>
            </div>
          </div>
        </li>
      </ul>

      <form v-if="adding" class="admin-add-form" @submit.prevent="invite">
        <input
          ref="emailInput"
          v-model="newEmail"
          type="email"
          required
          placeholder="email@esempio.it"
          aria-label="Email del nuovo amministratore"
          @keydown.esc="cancelAdding"
        />
        <button type="submit" :disabled="saving">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M5 13l4 4L19 7" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          {{ saving ? 'Invito…' : 'Invita' }}
        </button>
        <button type="button" class="btn-secondary" @click="cancelAdding">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M6 6l12 12M18 6L6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
          Annulla
        </button>
      </form>
      <button v-else type="button" class="admin-add" @click="startAdding">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
        Aggiungi admin
      </button>
    </div>

    <ModalDialog
      :open="pendingDelete !== null"
      title="Eliminare l'amministratore?"
      @close="pendingDelete = null"
    >
      <p>
        {{ pendingDelete?.email }} non potrà più accedere.
        <template v-if="pendingDelete?.pending">Il link di invito smetterà di funzionare.</template>
      </p>
      <div class="form-actions">
        <button type="button" class="btn-secondary" @click="pendingDelete = null">Annulla</button>
        <button type="button" class="btn-danger" @click="confirmDelete">Elimina</button>
      </div>
    </ModalDialog>
  </div>
</template>
