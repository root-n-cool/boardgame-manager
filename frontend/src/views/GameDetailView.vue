<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
  bggId: string | null
  name: string
  year: number | null
  minPlayers: number | null
  maxPlayers: number | null
  playtimeMinutes: number | null
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

const coverInput = ref<HTMLInputElement | null>(null)
const coverUploading = ref(false)
const coverError = ref('')

function activeLanguage(): GameLanguageInfo | undefined {
  return game.value?.languages.find((l) => l.code === activeLangCode.value)
}

/** I dati che arrivano da BGG, saltando quelli che il gioco non ha. */
const gameFacts = computed(() => {
  const g = game.value
  if (!g) {
    return [] as { label: string; value: string; href?: string }[]
  }
  const facts: { label: string; value: string; href?: string }[] = []
  if (g.minPlayers || g.maxPlayers) {
    const min = g.minPlayers ?? g.maxPlayers
    const max = g.maxPlayers ?? g.minPlayers
    facts.push({ label: 'Giocatori', value: min === max ? `${min}` : `${min}–${max}` })
  }
  if (g.playtimeMinutes) {
    facts.push({ label: 'Durata', value: `${g.playtimeMinutes} min` })
  }
  if (g.year) {
    facts.push({ label: 'Anno', value: `${g.year}` })
  }
  if (g.bggId) {
    facts.push({
      label: 'BGG',
      value: `#${g.bggId}`,
      href: `https://boardgamegeek.com/boardgame/${g.bggId}`,
    })
  }
  return facts
})

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

const mediaKindLabels: Record<string, string> = {
  file: 'PDF',
  link: 'Link',
  youtube: 'YouTube',
}

function mediaHref(m: GameMediaInfo): string {
  return m.type === 'file' ? `/api/uploads/${m.url}` : m.url
}

function mediaTitle(m: GameMediaInfo): string {
  if (m.title) {
    return m.title
  }
  return m.type === 'file' ? 'Manuale' : m.url
}

/**
 * L'id video dai tre formati che YouTube produce (watch?v=, youtu.be/,
 * /embed/). Serve solo per la miniatura: se non lo troviamo, la card cade
 * sull'icona come per gli altri media.
 */
function youtubeThumb(url: string): string | null {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return null
  }
  const host = parsed.hostname.replace(/^www\./, '')
  let id = ''
  if (host === 'youtu.be') {
    id = parsed.pathname.slice(1)
  } else if (host.endsWith('youtube.com')) {
    id = parsed.searchParams.get('v') || parsed.pathname.replace(/^\/(embed|shorts)\//, '')
  }
  id = id.split('/')[0]
  return /^[\w-]{6,}$/.test(id) ? `https://img.youtube.com/vi/${id}/hqdefault.jpg` : null
}

// Miniature che non hanno caricato (video privato, rimosso, rete assente):
// la card torna alla tessera con l'icona invece di lasciare un buco.
const brokenThumbs = ref<number[]>([])

function previewThumb(m: GameMediaInfo): string | null {
  if (m.type !== 'youtube' || brokenThumbs.value.includes(m.id)) {
    return null
  }
  return youtubeThumb(m.url)
}

function onThumbError(m: GameMediaInfo) {
  if (!brokenThumbs.value.includes(m.id)) {
    brokenThumbs.value.push(m.id)
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
    <PublicHeader />
    <div class="public-page" v-if="game">
      <div class="page-head">
        <div class="page-head-text">
          <h1>{{ game.name }}</h1>
          <p class="page-meta">
            <template v-if="game.owner">Proprietario: {{ game.owner }} · </template>
            <router-link :to="`/games/${game.id}/leaderboard`">Classifica</router-link>
          </p>
        </div>
        <button v-if="auth.user" type="button" class="btn-danger is-compact" @click="deleteGame">
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

      <div class="game-cover-card">
        <!-- Per l'admin la copertina È il controllo di caricamento: si clicca
             l'immagine, si sceglie il file e parte da sé. Per tutti gli altri
             resta un'immagine e basta. -->
        <button
          v-if="auth.user"
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
        <template v-else>
          <img
            v-if="game.coverPath"
            :src="`/api/uploads/${game.coverPath}`"
            :alt="`Copertina di ${game.name}`"
            class="cover"
            width="170"
            height="227"
            decoding="async"
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
        </template>
        <input
          v-if="auth.user"
          ref="coverInput"
          class="visually-hidden"
          type="file"
          accept="image/jpeg,image/png,image/webp"
          tabindex="-1"
          aria-hidden="true"
          @change="onCoverFileSelected"
        />

        <div class="game-cover-info">
          <dl v-if="gameFacts.length > 0" class="game-facts">
            <div v-for="f in gameFacts" :key="f.label">
              <dt>{{ f.label }}</dt>
              <dd>
                <a v-if="f.href" :href="f.href" target="_blank" rel="noopener">{{ f.value }}</a>
                <template v-else>{{ f.value }}</template>
              </dd>
            </div>
          </dl>
          <p v-else class="empty-note">Nessun dato da BoardGameGeek per questo gioco.</p>
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

      <section class="panel-card">
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
            <textarea v-model="editDescription" rows="4"></textarea>
          </label>
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
        <template v-else>
          <h3 class="language-name">{{ activeLanguage()?.name }}</h3>
          <p v-if="activeLanguage()?.description">{{ activeLanguage()?.description }}</p>
          <p v-else class="empty-note">Nessuna descrizione per questa lingua.</p>
        </template>
      </section>

      <section class="panel-card">
        <div class="section-head">
          <h2>Media</h2>
          <span class="lang-chip">{{ activeLangCode }}</span>
          <button v-if="auth.user" type="button" class="btn-secondary" @click="openMediaModal">
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M12 5.5v13M5.5 12h13" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
            </svg>
            Aggiungi
          </button>
        </div>

        <p v-if="!auth.user && (activeLanguage()?.media || []).length === 0" class="empty-note">
          Nessun media per questa lingua.
        </p>
        <div v-else class="game-grid media-grid">
          <ul role="list" class="game-grid-items">
            <li v-for="m in activeLanguage()?.media || []" :key="m.id">
              <a :href="mediaHref(m)" target="_blank" rel="noopener">
                <img
                  v-if="previewThumb(m)"
                  :src="previewThumb(m) as string"
                  alt=""
                  width="480"
                  height="360"
                  loading="lazy"
                  decoding="async"
                  @error="onThumbError(m)"
                />
                <span v-else class="cover-placeholder media-thumb" aria-hidden="true">
                  <svg v-if="m.type === 'file'" viewBox="0 0 24 24" fill="none">
                    <path
                      d="M6.5 3.5h7.2L18.5 8.3v12.2H6.5V3.5Z"
                      stroke="currentColor"
                      stroke-width="1.6"
                      stroke-linejoin="round"
                    />
                    <path d="M13.4 3.7v4.8h4.9M9.3 13h5.4M9.3 16.4h5.4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
                  </svg>
                  <svg v-else-if="m.type === 'youtube'" viewBox="0 0 24 24" fill="none">
                    <rect x="2.8" y="5.4" width="18.4" height="13.2" rx="3.4" stroke="currentColor" stroke-width="1.6" />
                    <path d="m10.4 9.4 4.6 2.6-4.6 2.6V9.4Z" fill="currentColor" />
                  </svg>
                  <svg v-else viewBox="0 0 24 24" fill="none">
                    <path
                      d="M10.2 13.8a3.6 3.6 0 0 0 5.1 0l2.9-2.9a3.6 3.6 0 1 0-5.1-5.1l-1 1M13.8 10.2a3.6 3.6 0 0 0-5.1 0l-2.9 2.9a3.6 3.6 0 1 0 5.1 5.1l1-1"
                      stroke="currentColor"
                      stroke-width="1.6"
                      stroke-linecap="round"
                    />
                  </svg>
                </span>
                <h3>{{ mediaTitle(m) }}</h3>
                <p class="media-kind">{{ mediaKindLabels[m.type] ?? m.type }}</p>
              </a>
              <button
                v-if="auth.user"
                type="button"
                class="media-remove"
                :aria-label="`Rimuovi ${mediaTitle(m)}`"
                @click="removeMedia(m.id, mediaTitle(m))"
              >
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M7 7l10 10M17 7 7 17" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
                </svg>
              </button>
            </li>
          </ul>
          <button v-if="auth.user" type="button" class="add-card" @click="openMediaModal">
            <span class="add-slot" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none">
                <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
              </svg>
            </span>
            <span class="add-label">Aggiungi media</span>
          </button>
        </div>
        <p v-if="mediaError && !mediaModalOpen" class="error">{{ mediaError }}</p>
      </section>

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
  </div>
</template>
