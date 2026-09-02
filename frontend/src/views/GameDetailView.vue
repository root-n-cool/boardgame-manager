<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'
import PublicHeader from '../components/PublicHeader.vue'
import ModalDialog from '../components/ModalDialog.vue'

interface GameMediaInfo {
  id: number
  type: 'file' | 'link' | 'youtube'
  url: string
  title: string | null
}

interface GameLanguageInfo {
  code: string
  isBaseLanguage: boolean
  name: string
  description: string | null
  media: GameMediaInfo[]
}

interface GameDetail {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
  languages: GameLanguageInfo[]
}

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const gameId = route.params.id as string

const game = ref<GameDetail | null>(null)
const error = ref('')
const activeLangCode = ref('')

const editName = ref('')
const editDescription = ref('')
const saveMessage = ref('')

const newLangCode = ref('')
const languageModalOpen = ref(false)
const languageError = ref('')

const linkUrl = ref('')
const linkTitle = ref('')
const uploadFile = ref<File | null>(null)
const mediaError = ref('')
const mediaModalOpen = ref(false)
const mediaKind = ref<'file' | 'link' | 'youtube'>('file')

const coverFile = ref<File | null>(null)
const coverError = ref('')

function activeLanguage(): GameLanguageInfo | undefined {
  return game.value?.languages.find((l) => l.code === activeLangCode.value)
}

async function load() {
  game.value = await api.get<GameDetail>(`/games/${gameId}`)
}

function selectLanguage(code: string) {
  activeLangCode.value = code
  const lang = activeLanguage()
  if (lang) {
    editName.value = lang.name
    editDescription.value = lang.description || ''
  }
}

async function saveLanguage() {
  error.value = ''
  saveMessage.value = ''
  try {
    await api.patch(`/games/${gameId}/languages/${activeLangCode.value}`, {
      name: editName.value,
      description: editDescription.value || null,
    })
    saveMessage.value = 'Salvato'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

function openLanguageModal() {
  newLangCode.value = ''
  languageError.value = ''
  languageModalOpen.value = true
}

async function addLanguage() {
  languageError.value = ''
  const code = newLangCode.value.trim().toLowerCase()
  try {
    await api.post(`/games/${gameId}/languages`, { languageCode: code })
    languageModalOpen.value = false
    newLangCode.value = ''
    await load()
    selectLanguage(code)
  } catch (e) {
    languageError.value = (e as Error).message
  }
}

async function deleteGame() {
  if (!window.confirm(`Eliminare "${game.value?.name}" dal catalogo? L'operazione non è reversibile.`)) {
    return
  }
  try {
    await api.delete(`/games/${gameId}`)
    router.push('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

function onFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  uploadFile.value = target.files?.[0] || null
}

function onCoverFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  coverFile.value = target.files?.[0] || null
}

async function uploadCover() {
  coverError.value = ''
  if (!coverFile.value) {
    coverError.value = 'Seleziona un immagine'
    return
  }
  const formData = new FormData()
  formData.append('file', coverFile.value)
  try {
    await api.post(`/games/${gameId}/cover`, formData)
    coverFile.value = null
    await load()
  } catch (e) {
    coverError.value = (e as Error).message
  }
}

function openMediaModal() {
  mediaKind.value = 'file'
  uploadFile.value = null
  linkUrl.value = ''
  linkTitle.value = ''
  mediaError.value = ''
  mediaModalOpen.value = true
}

/** Un solo submit per i tre tipi: file caricato, link esterno, video YouTube. */
async function submitMedia() {
  mediaError.value = ''
  const base = `/games/${gameId}/languages/${activeLangCode.value}/media`
  try {
    if (mediaKind.value === 'file') {
      if (!uploadFile.value) {
        mediaError.value = 'Seleziona un file PDF'
        return
      }
      const formData = new FormData()
      formData.append('file', uploadFile.value)
      await api.post(base, formData)
    } else {
      await api.post(base, {
        type: mediaKind.value,
        url: linkUrl.value,
        title: linkTitle.value,
      })
    }
    mediaModalOpen.value = false
    uploadFile.value = null
    linkUrl.value = ''
    linkTitle.value = ''
    await load()
  } catch (e) {
    mediaError.value = (e as Error).message
  }
}

const mediaKindLabels: Record<string, string> = {
  file: 'PDF',
  link: 'Link',
  youtube: 'YouTube',
}

async function removeMedia(mediaId: number, title: string) {
  if (!window.confirm(`Rimuovere "${title}"?`)) {
    return
  }
  mediaError.value = ''
  try {
    await api.delete(`/games/${gameId}/languages/${activeLangCode.value}/media/${mediaId}`)
    await load()
  } catch (e) {
    mediaError.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
    if (game.value && game.value.languages.length > 0) {
      selectLanguage(game.value.languages[0].code)
    }
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page" v-if="game">
      <h1>{{ game.name }}</h1>

      <div class="game-cover-card">
        <img
          v-if="game.coverPath"
          :src="`/api/uploads/${game.coverPath}`"
          :alt="`Copertina di ${game.name}`"
          class="cover"
        />
        <div v-else class="cover cover-empty" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none">
            <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
            <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
            <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
            <circle cx="12" cy="12" r="1.3" fill="currentColor" />
            <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
            <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
          </svg>
        </div>

        <div class="game-cover-info">
          <div class="meta-row">
            <span v-if="game.owner">Proprietario: {{ game.owner }}</span>
            <span v-if="game.owner" class="divider">·</span>
            <router-link :to="`/games/${game.id}/leaderboard`">Classifica</router-link>
            <template v-if="auth.user">
              <span class="divider">·</span>
              <button type="button" class="btn-danger" @click="deleteGame">Elimina gioco</button>
            </template>
          </div>

          <form v-if="auth.user" class="inline-form" @submit.prevent="uploadCover">
            <label>
              {{ game.coverPath ? 'Cambia copertina' : 'Carica una copertina' }}
              <input type="file" accept="image/jpeg,image/png,image/webp" @change="onCoverFileSelected" />
            </label>
            <button type="submit" class="btn-secondary">Carica</button>
          </form>
          <p v-if="coverError" class="error">{{ coverError }}</p>
        </div>
      </div>

      <nav class="language-tabs">
        <button
          v-for="l in game.languages"
          :key="l.code"
          type="button"
          :class="{ active: l.code === activeLangCode }"
          @click="selectLanguage(l.code)"
        >
          {{ l.code }}
          <svg
            v-if="l.isBaseLanguage"
            class="base-language-badge"
            viewBox="0 0 24 24"
            fill="none"
            aria-label="Lingua base"
          >
            <path
              d="M12 3.5l2.47 5.77 6.24.56-4.73 4.16 1.42 6.1L12 16.9l-5.4 3.2 1.42-6.1-4.73-4.16 6.24-.56L12 3.5Z"
              fill="currentColor"
            />
          </svg>
        </button>
        <button v-if="auth.user" type="button" class="language-tab-add" @click="openLanguageModal">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
          </svg>
          Lingua
        </button>
      </nav>

      <div class="language-panel">
        <section class="panel-section">
          <div class="section-head">
            <h2>Scheda</h2>
            <span class="lang-chip">{{ activeLangCode }}</span>
          </div>

          <form v-if="auth.user" @submit.prevent="saveLanguage">
            <label>
              Nome
              <input v-model="editName" required />
            </label>
            <label>
              Descrizione
              <textarea v-model="editDescription" rows="3"></textarea>
            </label>
            <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
            <div class="form-actions">
              <button type="submit">Salva</button>
            </div>
          </form>
          <template v-else>
            <h3 class="language-name">{{ activeLanguage()?.name }}</h3>
            <p v-if="activeLanguage()?.description">{{ activeLanguage()?.description }}</p>
            <p v-else class="empty-note">Nessuna descrizione per questa lingua.</p>
          </template>
        </section>

        <section class="panel-section">
          <div class="section-head">
            <h2>Manuale e tutorial</h2>
            <button v-if="auth.user" type="button" class="btn-secondary" @click="openMediaModal">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
              </svg>
              Aggiungi
            </button>
          </div>

          <ul v-if="(activeLanguage()?.media || []).length > 0" class="media-list">
            <li v-for="m in activeLanguage()?.media || []" :key="m.id">
              <a v-if="m.type === 'file'" :href="`/api/uploads/${m.url}`" target="_blank">{{ m.title || 'Manuale' }}</a>
              <a v-else :href="m.url" target="_blank">{{ m.title || m.url }}</a>
              <span class="media-kind">{{ mediaKindLabels[m.type] ?? m.type }}</span>
              <button
                v-if="auth.user"
                type="button"
                class="btn-danger"
                @click="removeMedia(m.id, m.title || m.url)"
              >
                Rimuovi
              </button>
            </li>
          </ul>
          <p v-else class="empty-note">Nessun manuale o tutorial per questa lingua.</p>
          <p v-if="mediaError && !mediaModalOpen" class="error">{{ mediaError }}</p>
        </section>
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <ModalDialog
        :open="languageModalOpen"
        title="Aggiungi lingua"
        @close="languageModalOpen = false"
      >
        <form @submit.prevent="addLanguage">
          <label>
            Codice lingua
            <input v-model="newLangCode" placeholder="es. en" required autofocus />
          </label>
          <p class="field-hint">Due lettere, come <code>it</code>, <code>en</code>, <code>de</code>.</p>
          <p v-if="languageError" class="error">{{ languageError }}</p>
          <div class="form-actions">
            <button type="button" class="btn-secondary" @click="languageModalOpen = false">Annulla</button>
            <button type="submit">Aggiungi lingua</button>
          </div>
        </form>
      </ModalDialog>

      <ModalDialog
        :open="mediaModalOpen"
        title="Aggiungi manuale o tutorial"
        @close="mediaModalOpen = false"
      >
        <form @submit.prevent="submitMedia">
          <div class="segmented" role="radiogroup" aria-label="Tipo di materiale">
            <label :class="{ active: mediaKind === 'file' }">
              <input v-model="mediaKind" type="radio" value="file" />
              File PDF
            </label>
            <label :class="{ active: mediaKind === 'link' }">
              <input v-model="mediaKind" type="radio" value="link" />
              Link
            </label>
            <label :class="{ active: mediaKind === 'youtube' }">
              <input v-model="mediaKind" type="radio" value="youtube" />
              YouTube
            </label>
          </div>

          <template v-if="mediaKind === 'file'">
            <label>
              File del manuale
              <input type="file" accept="application/pdf" @change="onFileSelected" />
            </label>
            <p class="field-hint">Solo PDF, massimo 20MB.</p>
          </template>
          <template v-else>
            <label>
              URL
              <input v-model="linkUrl" :placeholder="mediaKind === 'youtube' ? 'https://youtube.com/watch?v=...' : 'https://...'" required />
            </label>
            <label>
              Titolo
              <input v-model="linkTitle" placeholder="Come si gioca" />
            </label>
          </template>

          <p v-if="mediaError" class="error">{{ mediaError }}</p>
          <div class="form-actions">
            <button type="button" class="btn-secondary" @click="mediaModalOpen = false">Annulla</button>
            <button type="submit">Aggiungi</button>
          </div>
        </form>
      </ModalDialog>
    </div>
  </div>
</template>
