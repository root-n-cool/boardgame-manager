<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import ModalDialog from '../components/ModalDialog.vue'
import GameFacts from '../components/GameFacts.vue'
import GameMediaList from '../components/GameMediaList.vue'
import BggFilesPicker, { type BggFile } from '../components/BggFilesPicker.vue'
import ManualPrepPanel from '../components/ManualPrepPanel.vue'
import GameMaterialsPanel from '../components/GameMaterialsPanel.vue'
import SuggestedQuestionsPanel from '../components/SuggestedQuestionsPanel.vue'
import { languageName, type GameDetail, type GameLanguageInfo } from '../utils/game'

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
const newLangSource = ref('bgg')
const addingLanguage = ref(false)
const languageModalOpen = ref(false)
const languageError = ref('')

const linkUrl = ref('')
const linkTitle = ref('')
const uploadFile = ref<File | null>(null)
const fileTitle = ref('')
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

// Gli stessi formati che il server accetta per l'indicizzazione (vedi
// indexMediaHandler): un file di un altro tipo non ha un pannello di
// preparazione, perché indicizzarlo fallirebbe comunque. Le foto ci sono
// perché una pagina fotografata la legge il modello vision, come una
// pagina di PDF scansionato.
const indexableExtensions = ['.pdf', '.txt', '.md', '.docx', '.jpg', '.png']

// Le fonti indicizzabili di TUTTE le lingue, non della sola lingua attiva.
// La ricerca della chat filtra su `game_id` e nient'altro (vedi `searchOne`
// in internal/manuals/store.go): una domanda pesca da ogni fonte del gioco,
// in qualunque lingua sia. Mostrare qui solo i file della lingua attiva
// faceva credere il contrario, e cambiando tab la sezione cambiava come se
// la chat fosse a scope di lingua.
//
// La lingua resta accanto a ogni voce per due ragioni concrete: la rotta di
// indicizzazione è per lingua (`/languages/{lang}/media/{id}/index`), e due
// manuali possono chiamarsi allo stesso modo in due lingue diverse.
const indexableMedia = computed(() =>
  (game.value?.languages || []).flatMap((l) =>
    l.media
      .filter(
        (m) =>
          m.type === 'file' && indexableExtensions.some((ext) => m.url.toLowerCase().endsWith(ext)),
      )
      .map((m) => ({ media: m, lang: l.code })),
  ),
)

// `load()` non azzera mai `game`, quindi il blocco `v-if="game"` resta
// montato tra una chiamata e l'altra: `SuggestedQuestionsPanel` non viene
// mai ricreato e il suo `onMounted(load)` non rileggerebbe le domande
// appena generate da un'indicizzazione. Questo contatore, passato come
// `:key`, forza Vue a ricreare il pannello così da farlo rileggere —
// senza un watcher che scatterebbe a ogni mutazione di `game`.
// Il bump vive solo nel percorso di indicizzazione (vedi
// `onIndexChanged`): gli altri `load()` sparsi in questa pagina (salvare
// lingue/seat, aggiungere media, tradurre la descrizione, ...) non
// possono aver cambiato le domande sul server, e ricreare il pannello lì
// butterebbe via il testo non salvato che l'admin sta scrivendo nei tre
// campi.
const suggestedQuestionsKey = ref(0)

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

// Indicizzare un manuale (o rimuoverne l'indice) è l'unico evento che può
// aver cambiato le domande suggerite sul server: solo qui ha senso
// ricreare il pannello per farlo rileggere.
async function onIndexChanged() {
  await load()
  suggestedQuestionsKey.value++
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
  translateError.value = ''
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

// Le descrizioni da cui si può partire: l'originale BoardGameGeek in cima
// quando c'è, poi una voce per ogni lingua già presente. Con una sola voce il
// select non si mostra — sarebbe una scelta senza alternative.
const languageSources = computed(() => {
  const sources: { value: string; label: string }[] = []
  if (game.value?.canTranslate) {
    sources.push({ value: 'bgg', label: 'BoardGameGeek (originale inglese)' })
  }
  for (const l of game.value?.languages || []) {
    sources.push({
      value: l.code,
      label: l.isBaseLanguage ? `${languageName(l.code)} — lingua base` : languageName(l.code),
    })
  }
  return sources
})

function openLanguageModal() {
  newLangCode.value = ''
  newLangSource.value = languageSources.value[0]?.value || 'bgg'
  languageError.value = ''
  languageModalOpen.value = true
}

async function addLanguage() {
  languageError.value = ''
  const code = newLangCode.value.trim().toLowerCase()
  addingLanguage.value = true
  try {
    await api.post(`/games/${gameId}/languages`, {
      languageCode: code,
      source: newLangSource.value,
    })
    languageModalOpen.value = false
    newLangCode.value = ''
    await load()
    selectLanguage(code)
  } catch (e) {
    languageError.value = (e as Error).message
  } finally {
    addingLanguage.value = false
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

// Una foto ha bisogno di un titolo scritto a mano, un documento no. Senza
// titolo il server ricade sul nome del file, ed è un ripiego che regge per
// "regolamento.pdf" e non per "IMG_4821.JPG": quel nome diventa il titolo
// della tessera nei media E la citazione che la chat pubblica mostra sotto
// la risposta. Da qui il campo obbligatorio solo in questo caso.
const photoSelected = computed(() => /\.(jpe?g|png)$/i.test(uploadFile.value?.name ?? ''))

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
  fileTitle.value = ''
  mediaError.value = ''
  mediaModalOpen.value = true
}

/** Il file scelto sull'indice BGG si scarica dal browser dell'admin, non da
 *  qui: al ritorno nel modale il titolo è già scritto e resta solo il file da
 *  allegare. */
function onBggFilePicked(file: BggFile) {
  fileTitle.value = file.title
}

/** Un solo submit per i tre tipi: file caricato, link esterno, video YouTube. */
async function submitMedia() {
  mediaError.value = ''
  const base = `/games/${gameId}/languages/${activeLangCode.value}/media`
  try {
    if (mediaKind.value === 'file') {
      if (!uploadFile.value) {
        mediaError.value = 'Seleziona un file'
        return
      }
      const formData = new FormData()
      formData.append('file', uploadFile.value)
      if (fileTitle.value.trim()) {
        formData.append('title', fileTitle.value.trim())
      }
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
    fileTitle.value = ''
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

      <!--
        Le due card della lingua vivono dentro il gruppo che la barra
        governa: in fila con le altre sembravano governate dalla barra tanto
        quanto il gruppo «Chatbot», che con la lingua non c'entra nulla.
      -->
      <div class="section-group">
        <div class="section-group-head">
          <h2>Lingue</h2>
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
        </div>

        <section class="panel-card">
          <div class="section-head">
            <h3>Scheda</h3>
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
            <h3>Media</h3>
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
            title-level="h4"
            @add="openMediaModal"
            @remove="removeMedia"
          />
          <p v-if="mediaError && !mediaModalOpen" class="error">{{ mediaError }}</p>
        </section>
      </div>

      <!--
        Il gruppo «Chatbot» non porta `lang-chip` da nessuna parte, e
        l'assenza è l'informazione: quel che sta qui dentro è per GIOCO, non
        per lingua — la chat cerca in tutte le fonti insieme (vedi
        `indexableMedia`) e le tre domande suggerite sono uniche. È anche il
        motivo per cui il gruppo esiste: staccare queste due card dalla barra
        delle lingue, che prima sembrava governare anche loro.
      -->
      <div class="section-group">
        <div class="section-group-head">
          <h2>Chatbot</h2>
        </div>

        <!--
          La preparazione di una fonte è un'azione da admin: GameMediaList è
          condiviso con la scheda pubblica, quindi la sezione vive qui e non
          lì. Una riga per ogni file indicizzabile del gioco (PDF, txt, md,
          docx, foto JPG o PNG — gli unici formati che l'indicizzazione
          accetta), non più un
          pannello alto per file: l'avviso sui tempi lunghi e il motivo per
          cui manca il bottone senza provider AI stanno una volta sola qui,
          non ripetuti a ogni riga.
        -->
        <section class="panel-card">
          <div class="section-head">
            <h3>Knowledge base</h3>
          </div>

          <template v-if="indexableMedia.length > 0">
            <!-- Una scansione passa per un modello di visione, una pagina per
                 chiamata: la richiesta può durare minuti, va detto prima del
                 click — una volta sola, non per ogni riga. -->
            <p v-if="aiConfigured" class="field-hint">
              Per un file di testo dura pochi secondi; per una scansione lunga può
              richiedere alcuni minuti, poche pagine alla volta — resta su questa
              pagina finché non finisce.
            </p>
            <!-- Senza provider il motivo è pratico, non tecnico: qui non c'è
                 ancora una chat con cui indicizzare avrebbe a che fare. -->
            <p v-else class="empty-note">
              Senza un provider AI configurato nelle impostazioni la chat con le domande non esiste:
              indicizzare un documento ora non servirebbe a niente.
            </p>

            <ul role="list" class="admin-list">
              <!-- La chiave porta anche la lingua: lo stesso id di media non si
                   ripete fra lingue diverse, ma la coppia e' l'identita' vera
                   di una riga qui, e la rotta che il pannello chiama la usa
                   tutta. -->
              <li v-for="row in indexableMedia" :key="`${row.lang}-${row.media.id}`">
                <ManualPrepPanel
                  :game-id="game.id"
                  :lang="row.lang"
                  :media-id="row.media.id"
                  :media-title="row.media.title || 'Manuale'"
                  :media-url="row.media.url"
                  :indexed-chunks="row.media.indexedChunks"
                  :ai-configured="aiConfigured"
                  @changed="onIndexChanged"
                />
              </li>
            </ul>
          </template>
          <p v-else class="empty-note">
            Nessun documento da preparare: carica un PDF, un file di testo (txt o md), un docx o la
            foto di una pagina nella sezione Media di una delle lingue del gioco.
          </p>
        </section>

        <!--
          Card sorella e non un blocco dentro "Knowledge base": le tre domande
          restano fuori da `indexableMedia` perché si devono poter scrivere a
          mano anche su un gioco senza documenti — ed e' il motivo per cui il
          PUT e' un upsert.
        -->
        <section class="panel-card">
          <SuggestedQuestionsPanel
            :key="suggestedQuestionsKey"
            :game-id="game.id"
            :ai-configured="aiConfigured"
          />
        </section>
      </div>

      <!--
        Fuori dal gruppo «Chatbot» e non una terza card sorella lì dentro:
        quel gruppo esiste per le card che dipendono dalla chat (nessun
        `lang-chip`, vedi sopra), e l'elenco dei materiali non ne dipende —
        serve a chi presta il gioco, non a chi prepara il manuale, e può
        esistere anche su un gioco senza documenti indicizzati (si scrive a
        mano). Sotto quel titolo si leggerebbe come una funzione della chat.
      -->
      <section class="panel-card">
        <GameMaterialsPanel :game-id="game.id" :ai-configured="aiConfigured" />
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

        <label v-if="languageSources.length > 1">
          Parti da
          <select v-model="newLangSource">
            <option v-for="s in languageSources" :key="s.value" :value="s.value">{{ s.label }}</option>
          </select>
        </label>
        <p v-if="languageSources.length > 1" class="field-hint">
          <template v-if="aiConfigured">
            La descrizione scelta viene tradotta nella lingua nuova. Una descrizione
            già corretta a mano è spesso un punto di partenza migliore
            dell'originale di BoardGameGeek.
          </template>
          <template v-else>
            Senza un provider AI configurato la descrizione scelta viene copiata
            così com'è, pronta da tradurre a mano.
          </template>
        </p>

        <p v-if="languageError" class="error">{{ languageError }}</p>
        <div class="form-actions">
          <button type="button" class="btn-secondary" :disabled="addingLanguage" @click="languageModalOpen = false">
            Annulla
          </button>
          <button type="submit" :disabled="addingLanguage">
            {{ addingLanguage ? (aiConfigured ? 'Traduzione in corso…' : 'Aggiunta…') : 'Aggiungi lingua' }}
          </button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog :open="mediaModalOpen" title="Aggiungi media" @close="mediaModalOpen = false">
      <form @submit.prevent="submitMedia">
        <div class="segmented" role="radiogroup" aria-label="Tipo di materiale">
          <label :class="{ active: mediaKind === 'file' }">
            <input v-model="mediaKind" type="radio" value="file" />
            File
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
            <input
              type="file"
              accept=".pdf,.txt,.md,.docx,.jpg,.jpeg,.png,application/pdf,text/plain,text/markdown,application/vnd.openxmlformats-officedocument.wordprocessingml.document,image/jpeg,image/png"
              @change="onFileSelected"
            />
          </label>
          <p class="field-hint">
            PDF, txt, md, docx o la foto di una pagina (JPG, PNG), massimo 20MB.
          </p>
          <label>
            Titolo
            <input
              v-model="fileTitle"
              :placeholder="photoSelected ? 'Regolamento, pagina 3' : 'Regolamento'"
              :required="photoSelected"
              :aria-describedby="photoSelected ? 'file-title-hint' : undefined"
            />
          </label>
          <p v-if="photoSelected" id="file-title-hint" class="field-hint">
            Il titolo di una foto va scritto: è quello che la chat cita sotto la risposta, e
            «IMG_4821.JPG» non dice a nessuno da dove viene la regola.
          </p>
          <!--
            Il contenuto della modale sta nel DOM anche a modale chiusa (è
            `<dialog>`, non un `v-if`): senza `mediaModalOpen` il picker si
            montava al caricamento della pagina, cioè prima che `load()`
            avesse scelto la lingua attiva — chiedeva a BGG i file di
            nessuna lingua, e li chiedeva a ogni apertura della scheda.
          -->
          <BggFilesPicker
            v-if="mediaModalOpen && game?.bggId"
            :game-id="gameId"
            :bgg-id="game.bggId"
            :lang="activeLangCode"
            @pick="onBggFilePicked"
          />
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
