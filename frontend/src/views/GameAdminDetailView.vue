<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import ModalDialog from '../components/ModalDialog.vue'
import GameFacts from '../components/GameFacts.vue'
import GameMediaList from '../components/GameMediaList.vue'
import type { GameDetail, GameLanguageInfo } from '../utils/game'

const route = useRoute()
const router = useRouter()
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

const coverInput = ref<HTMLInputElement | null>(null)
const coverUploading = ref(false)
const coverError = ref('')

const editSeats = ref(1)
const seatsSaving = ref(false)
const seatsError = ref('')

const aiConfigured = ref(false)
const translating = ref(false)
const translateError = ref('')

// Il nome esteso della lingua rende il bottone leggibile: "Traduci in
// italiano" invece di "Traduci in it".
const languageNames: Record<string, string> = {
  it: 'italiano',
  en: 'inglese',
  fr: 'francese',
  de: 'tedesco',
  es: 'spagnolo',
}

function languageName(code: string): string {
  return languageNames[code] || code
}

async function translateDescription() {
  if (!window.confirm(`Ritradurre la descrizione in ${languageName(activeLangCode.value)}? Il testo attuale viene sostituito.`)) {
    return
  }
  translateError.value = ''
  translating.value = true
  try {
    await api.post(`/games/${gameId}/languages/${activeLangCode.value}/translate`, {})
    await load()
    selectLanguage(activeLangCode.value)
    saveMessage.value = 'Descrizione tradotta'
  } catch (e) {
    translateError.value = (e as Error).message
  } finally {
    translating.value = false
  }
}

function activeLanguage(): GameLanguageInfo | undefined {
  return game.value?.languages.find((l) => l.code === activeLangCode.value)
}

async function load() {
  game.value = await api.get<GameDetail>(`/games/${gameId}`)
  editSeats.value = game.value.seats
  try {
    const s = await api.get<{ aiConfigured: boolean }>('/settings')
    aiConfigured.value = s.aiConfigured
  } catch {
    aiConfigured.value = false
  }
}

function selectLanguage(code: string) {
  activeLangCode.value = code
  saveMessage.value = ''
  translateError.value = ''
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

// I posti prenotabili sono l'unico dato del gioco modificabile da qui: si
// salvano da sé, senza un "Salva" generale che non esiste in questa pagina.
async function saveSeats() {
  seatsError.value = ''
  seatsSaving.value = true
  try {
    await api.patch(`/games/${gameId}`, { seats: editSeats.value })
    await load()
  } catch (e) {
    seatsError.value = (e as Error).message
  } finally {
    seatsSaving.value = false
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
    router.push({ name: 'admin-games' })
  } catch (e) {
    error.value = (e as Error).message
  }
}

function onFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  uploadFile.value = target.files?.[0] || null
}

function pickCover() {
  coverError.value = ''
  coverInput.value?.click()
}

/** La copertina non ha un bottone "salva": scelto il file, parte il caricamento. */
async function onCoverFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  // Reset subito: senza, riscegliere lo stesso file non emette un altro change.
  target.value = ''
  if (!file) {
    return
  }
  coverError.value = ''
  coverUploading.value = true
  const formData = new FormData()
  formData.append('file', file)
  try {
    await api.post(`/games/${gameId}/cover`, formData)
    await load()
  } catch (e) {
    coverError.value = (e as Error).message
  } finally {
    coverUploading.value = false
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
    <router-link :to="{ name: 'admin-games' }" class="back-link">&larr; Catalogo</router-link>

    <template v-if="game">
      <div class="page-head">
        <div class="page-head-text">
          <h1>{{ game.name }}</h1>
          <p class="page-meta">
            <template v-if="game.owner">Proprietario: {{ game.owner }} · </template>
            <router-link :to="`/games/${game.id}/leaderboard`">Classifica</router-link>
          </p>
        </div>
        <div class="page-head-actions">
          <a
            class="action-link is-compact"
            :href="`/games/${game.id}`"
            target="_blank"
            rel="noopener"
            aria-label="Vedi la scheda pubblica del gioco (si apre in una nuova scheda)"
          >
            Vedi scheda pubblica
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M14 4h6v6M20 4l-8.5 8.5M18 14v4.5c0 .83-.67 1.5-1.5 1.5h-11c-.83 0-1.5-.67-1.5-1.5v-11C4 6.67 4.67 6 5.5 6H10"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </a>
          <button type="button" class="btn-danger is-compact" @click="deleteGame">
            Elimina
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M4 7h16M9.5 7V5.2c0-.66.54-1.2 1.2-1.2h2.6c.66 0 1.2.54 1.2 1.2V7M6.5 7l.8 12.06c.05.72.65 1.28 1.37 1.28h6.66c.72 0 1.32-.56 1.37-1.28L17.5 7M10.4 11v5.6M13.6 11v5.6"
                stroke="currentColor"
                stroke-width="1.7"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </button>
        </div>
      </div>

      <div class="game-cover-card">
        <!-- La copertina È il controllo di caricamento: si clicca l'immagine,
             si sceglie il file e parte da sé. -->
        <button
          type="button"
          class="cover-uploader"
          :class="{ 'is-uploading': coverUploading }"
          :aria-label="game.coverPath ? 'Cambia la copertina' : 'Carica una copertina'"
          :disabled="coverUploading"
          @click="pickCover"
        >
          <img
            v-if="game.coverPath"
            :src="`/api/uploads/${game.coverPath}`"
            :alt="`Copertina di ${game.name}`"
            class="cover"
            width="170"
            height="227"
            decoding="async"
          />
          <span v-else class="cover cover-empty" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none">
              <rect x="4" y="4" width="16" height="16" rx="4" stroke="currentColor" stroke-width="1.7" />
              <circle cx="8.3" cy="8.3" r="1.3" fill="currentColor" />
              <circle cx="15.7" cy="8.3" r="1.3" fill="currentColor" />
              <circle cx="12" cy="12" r="1.3" fill="currentColor" />
              <circle cx="8.3" cy="15.7" r="1.3" fill="currentColor" />
              <circle cx="15.7" cy="15.7" r="1.3" fill="currentColor" />
            </svg>
          </span>
          <span class="cover-overlay">
            <svg v-if="!coverUploading" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M12 16V4.8M12 4.8 7.6 9.2M12 4.8l4.4 4.4M4.5 15v3.2c0 .72.58 1.3 1.3 1.3h12.4c.72 0 1.3-.58 1.3-1.3V15"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <span class="cover-overlay-label">
              {{
                coverUploading
                  ? 'Caricamento…'
                  : game.coverPath
                    ? 'Cambia copertina'
                    : 'Carica copertina'
              }}
            </span>
          </span>
        </button>
        <input
          ref="coverInput"
          class="visually-hidden"
          type="file"
          accept="image/jpeg,image/png,image/webp"
          tabindex="-1"
          aria-hidden="true"
          @change="onCoverFileSelected"
        />

        <div class="game-cover-info">
          <GameFacts :game="game" />
          <div class="game-seats-edit">
            <label>
              Posti prenotabili per copia
              <input v-model.number="editSeats" type="number" min="1" />
            </label>
            <button
              type="button"
              :disabled="seatsSaving || editSeats === game.seats || editSeats < 1"
              @click="saveSeats"
            >
              {{ seatsSaving ? 'Salvo…' : 'Salva' }}
            </button>
            <p class="field-hint">
              Più di 1 apre il tavolo: a un evento, ogni posto prenotabile ha
              un proprio codice.
            </p>
            <p v-if="seatsError" class="error">{{ seatsError }}</p>
          </div>
          <p v-if="coverError" class="error">{{ coverError }}</p>
        </div>
      </div>

      <nav class="tab-bar language-tabs">
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
        <button type="button" class="language-tab-add" @click="openLanguageModal">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
          </svg>
          Lingua
        </button>
      </nav>

      <section class="panel-card">
        <div class="section-head">
          <h2>Scheda</h2>
          <span class="lang-chip">{{ activeLangCode }}</span>
        </div>

        <form @submit.prevent="saveLanguage">
          <label>
            Nome
            <input v-model="editName" required />
          </label>
          <label>
            Descrizione
            <textarea v-model="editDescription" rows="4"></textarea>
          </label>
          <p v-if="game.canTranslate && aiConfigured" class="field-hint">
            <button
              type="button"
              class="link-button"
              :disabled="translating"
              @click="translateDescription"
            >
              {{ translating ? 'Traduzione in corso…' : `Traduci in ${languageName(activeLangCode)} da BoardGameGeek` }}
            </button>
            — sostituisce il testo qui sopra con una nuova traduzione della
            descrizione originale.
          </p>
          <p v-if="translateError" class="error">{{ translateError }}</p>
          <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
          <div class="form-actions">
            <button type="submit">
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="m5 12.4 4.6 4.6L19 7.6"
                  stroke="currentColor"
                  stroke-width="2.2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
              Salva
            </button>
          </div>
        </form>
      </section>

      <section class="panel-card">
        <div class="section-head">
          <h2>Media</h2>
          <span class="lang-chip">{{ activeLangCode }}</span>
          <button type="button" class="btn-secondary" @click="openMediaModal">
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
            </svg>
            Aggiungi
          </button>
        </div>

        <GameMediaList
          :media="activeLanguage()?.media || []"
          editable
          @add="openMediaModal"
          @remove="removeMedia"
        />
        <p v-if="mediaError && !mediaModalOpen" class="error">{{ mediaError }}</p>
      </section>
    </template>

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

    <ModalDialog :open="mediaModalOpen" title="Aggiungi media" @close="mediaModalOpen = false">
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
</template>
