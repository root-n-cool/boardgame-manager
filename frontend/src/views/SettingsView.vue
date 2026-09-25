<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import { useSiteStore } from '../stores/site'
import MarkdownEditor from '../components/MarkdownEditor.vue'

interface SettingsResponse {
  defaultLanguage: string
  publicBaseUrl: string
  siteTitle: string
  logoFilename: string
  faviconFilename: string
  termsMarkdown: string
  privacyMarkdown: string
  hideFromSearchEngines: boolean
  bggApiTokenSet: boolean
  bggApiTokenMasked?: string
  aiBaseUrl: string
  aiModel: string
  aiVisionModel: string
  aiApiKeySet: boolean
  aiApiKeyMasked?: string
  tavilyApiKeySet: boolean
  tavilyApiKeyMasked?: string
  aiConfigured: boolean
  smtpHost: string
  smtpPort: number
  smtpUsername: string
  smtpFromAddress: string
  smtpFromName: string
  smtpTlsMode: string
  smtpPasswordSet: boolean
  smtpPasswordMasked?: string
  smtpConfigured: boolean
}

const site = useSiteStore()
const defaultLanguage = ref('it')
const publicBaseUrl = ref('')
const siteTitle = ref('')
const logoFilename = ref('')
const faviconFilename = ref('')
const termsMarkdown = ref('')
const privacyMarkdown = ref('')
const hideFromSearchEngines = ref(true)
const logoInput = ref<HTMLInputElement | null>(null)
const faviconInput = ref<HTMLInputElement | null>(null)
const logoUploading = ref(false)
const faviconUploading = ref(false)
const brandingError = ref('')
const bggApiToken = ref('')
const bggApiTokenMasked = ref('')
const aiBaseUrl = ref('')
const aiModel = ref('')
const aiVisionModel = ref('')
const aiApiKey = ref('')
const aiApiKeyMasked = ref('')
const tavilyApiKey = ref('')
const tavilyApiKeyMasked = ref('')
const message = ref('')
const error = ref('')
const smtpHost = ref('')
// 587 come valore di partenza: è la porta di Gmail, Mailjet e Brevo, cioè
// di quasi ogni provider che un'associazione userebbe.
const smtpPort = ref<number>(587)
const smtpUsername = ref('')
const smtpPassword = ref('')
const smtpPasswordMasked = ref('')
const smtpFromAddress = ref('')
const smtpFromName = ref('')
const smtpTlsMode = ref('starttls')
const smtpConfigured = ref(false)
const smtpTesting = ref(false)
const smtpTestMessage = ref('')
const smtpTestError = ref('')

async function load() {
  const s = await api.get<SettingsResponse>('/settings')
  defaultLanguage.value = s.defaultLanguage
  publicBaseUrl.value = s.publicBaseUrl || ''
  siteTitle.value = s.siteTitle || ''
  logoFilename.value = s.logoFilename || ''
  faviconFilename.value = s.faviconFilename || ''
  termsMarkdown.value = s.termsMarkdown || ''
  privacyMarkdown.value = s.privacyMarkdown || ''
  hideFromSearchEngines.value = s.hideFromSearchEngines
  bggApiTokenMasked.value = s.bggApiTokenMasked || ''
  aiBaseUrl.value = s.aiBaseUrl || ''
  aiModel.value = s.aiModel || ''
  aiVisionModel.value = s.aiVisionModel || ''
  aiApiKeyMasked.value = s.aiApiKeyMasked || ''
  tavilyApiKeyMasked.value = s.tavilyApiKeyMasked || ''
  smtpHost.value = s.smtpHost || ''
  smtpPort.value = s.smtpPort || 587
  smtpUsername.value = s.smtpUsername || ''
  smtpPasswordMasked.value = s.smtpPasswordMasked || ''
  smtpFromAddress.value = s.smtpFromAddress || ''
  smtpFromName.value = s.smtpFromName || ''
  smtpTlsMode.value = s.smtpTlsMode || 'starttls'
  smtpConfigured.value = s.smtpConfigured
}

async function save() {
  error.value = ''
  message.value = ''
  try {
    await api.put('/settings', {
      defaultLanguage: defaultLanguage.value,
      publicBaseUrl: publicBaseUrl.value,
      siteTitle: siteTitle.value,
      termsMarkdown: termsMarkdown.value,
      privacyMarkdown: privacyMarkdown.value,
      hideFromSearchEngines: hideFromSearchEngines.value,
      bggApiToken: bggApiToken.value,
      aiBaseUrl: aiBaseUrl.value,
      aiApiKey: aiApiKey.value,
      aiModel: aiModel.value,
      aiVisionModel: aiVisionModel.value,
      tavilyApiKey: tavilyApiKey.value,
      smtpHost: smtpHost.value,
      // v-model.number su un <input type="number"> svuotato torna la stringa
      // vuota, non NaN (looseToNumber la lascia com'è quando parseFloat
      // fallisce): senza questa coercizione il backend riceverebbe
      // "smtpPort": "" al posto di un numero. 0 è già il valore che il
      // backend tratta come "porta non impostata".
      smtpPort: Number(smtpPort.value) || 0,
      smtpUsername: smtpUsername.value,
      smtpPassword: smtpPassword.value,
      smtpFromAddress: smtpFromAddress.value,
      smtpFromName: smtpFromName.value,
      smtpTlsMode: smtpTlsMode.value,
    })
    // Il token è un segreto e si riscrive solo per sostituirlo: il campo torna
    // vuoto. L'indirizzo pubblico invece è un dato da rileggere, quindi resta.
    bggApiToken.value = ''
    aiApiKey.value = ''
    tavilyApiKey.value = ''
    smtpPassword.value = ''
    // Un salvataggio cambia la configurazione che la prova userebbe:
    // l'esito precedente non vale più e resta a schermo mentendo.
    smtpTestMessage.value = ''
    smtpTestError.value = ''
    message.value = 'Impostazioni salvate'
    await load()
  } catch (e) {
    const raw = (e as Error).message
    error.value =
      raw === 'publicBaseUrl must be an absolute http or https address'
        ? "L'indirizzo pubblico deve essere completo, per esempio https://giochi.example.org"
        : raw
  }
}

/**
 * Prova la configurazione salvata, non quella nel form: è quella che
 * verrà usata davvero quando parte una prenotazione. La mail arriva
 * all'indirizzo dell'admin in sessione.
 */
async function sendTestEmail() {
  smtpTesting.value = true
  smtpTestMessage.value = ''
  smtpTestError.value = ''
  try {
    const res = await api.post<{ to: string }>('/settings/smtp/test')
    smtpTestMessage.value = `Email di prova inviata a ${res.to}. Se non arriva, controlla la casella spam.`
  } catch (e) {
    smtpTestError.value = (e as Error).message
  } finally {
    smtpTesting.value = false
  }
}

function pickLogo() {
  brandingError.value = ''
  logoInput.value?.click()
}

function pickFavicon() {
  brandingError.value = ''
  faviconInput.value?.click()
}

async function onLogoSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  target.value = ''
  if (!file) return
  brandingError.value = ''
  logoUploading.value = true
  const formData = new FormData()
  formData.append('file', file)
  try {
    await api.post('/settings/logo', formData)
    await load()
    await site.load()
  } catch (e) {
    brandingError.value = (e as Error).message
  } finally {
    logoUploading.value = false
  }
}

async function onFaviconSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  target.value = ''
  if (!file) return
  brandingError.value = ''
  faviconUploading.value = true
  const formData = new FormData()
  formData.append('file', file)
  try {
    await api.post('/settings/favicon', formData)
    await load()
    await site.load()
  } catch (e) {
    brandingError.value = (e as Error).message
  } finally {
    faviconUploading.value = false
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <h1>Impostazioni</h1>
    <form class="panel-form" @submit.prevent="save">
      <div class="panel-card">
        <div class="section-head">
          <h2>Generale</h2>
        </div>
        <label>
          Lingua di default
          <select v-model="defaultLanguage">
            <option value="it">Italiano</option>
            <option value="en">Inglese</option>
          </select>
        </label>

        <label>
          BoardGameGeek API token
          <input v-model="bggApiToken" type="password" :placeholder="bggApiTokenMasked || 'non configurato'" />
        </label>

        <label>
          Indirizzo pubblico
          <input v-model="publicBaseUrl" type="url" inputmode="url" placeholder="https://giochi.example.org" />
        </label>
        <p class="field-hint">
          Serve a comporre i link che mandi fuori dall'app, come l'invito di un
          amministratore. Se lo lasci vuoto si usa l'indirizzo da cui stai navigando.
        </p>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Sito</h2>
        </div>
        <p class="field-hint">
          Rimarchizza l'installazione: il titolo compare nel titolo della pagina
          e nell'header quando non c'è un logo, logo e favicon sono immagini
          JPEG, PNG o WebP fino a 5MB.
        </p>

        <label>
          Titolo del sito
          <input v-model="siteTitle" placeholder="BoardGames Manager" />
        </label>

        <div class="field-row">
          <div>
            <span class="field-label">Logo</span>
            <div class="branding-upload">
              <img
                v-if="logoFilename"
                :src="`/api/uploads/${logoFilename}`"
                class="branding-preview"
                alt="Logo attuale"
              />
              <button type="button" class="btn-secondary" :disabled="logoUploading" @click="pickLogo">
                {{ logoUploading ? 'Caricamento…' : logoFilename ? 'Cambia logo' : 'Carica logo' }}
              </button>
              <input
                ref="logoInput"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                class="visually-hidden"
                tabindex="-1"
                aria-hidden="true"
                @change="onLogoSelected"
              />
            </div>
          </div>

          <div>
            <span class="field-label">Favicon</span>
            <div class="branding-upload">
              <img
                v-if="faviconFilename"
                :src="`/api/uploads/${faviconFilename}`"
                class="branding-preview branding-preview-small"
                alt="Favicon attuale"
              />
              <button type="button" class="btn-secondary" :disabled="faviconUploading" @click="pickFavicon">
                {{ faviconUploading ? 'Caricamento…' : faviconFilename ? 'Cambia favicon' : 'Carica favicon' }}
              </button>
              <input
                ref="faviconInput"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                class="visually-hidden"
                tabindex="-1"
                aria-hidden="true"
                @change="onFaviconSelected"
              />
            </div>
          </div>
        </div>
        <p v-if="brandingError" class="error">{{ brandingError }}</p>

        <label class="checkbox-label">
          <input v-model="hideFromSearchEngines" type="checkbox" />
          Nascondi dai motori di ricerca
        </label>
        <p class="field-hint">
          Il sito resta raggiungibile da chi ha il link o il QR code, ma Google
          e gli altri motori di ricerca non lo mostrano nei risultati.
        </p>

        <div class="markdown-field">
          Termini e condizioni
          <MarkdownEditor v-model="termsMarkdown" aria-label="Termini e condizioni" />
        </div>
        <p class="field-hint">Pubblicati alla pagina <code>/terms</code>, raggiungibile dal footer.</p>

        <div class="markdown-field">
          Privacy
          <MarkdownEditor v-model="privacyMarkdown" aria-label="Privacy" />
        </div>
        <p class="field-hint">Pubblicata alla pagina <code>/privacy</code>, raggiungibile dal footer.</p>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Provider AI</h2>
        </div>
        <p class="field-hint">
          Se lo configuri, le descrizioni scaricate da BoardGameGeek arrivano già
          tradotte nella lingua della scheda. Vale un qualsiasi servizio
          compatibile con le API OpenAI: Google Gemini, OpenAI, OpenRouter, o un
          Ollama in casa. Lasciandolo vuoto l'app funziona come prima, con le
          descrizioni in inglese.
        </p>

        <label>
          Indirizzo del provider
          <input
            v-model="aiBaseUrl"
            type="url"
            inputmode="url"
            placeholder="https://generativelanguage.googleapis.com/v1beta/openai"
          />
        </label>
        <p class="field-hint">
          L'indirizzo base, senza <code>/chat/completions</code> in fondo. Gemini:
          <code>https://generativelanguage.googleapis.com/v1beta/openai</code> ·
          OpenAI: <code>https://api.openai.com/v1</code> · Ollama:
          <code>http://localhost:11434/v1</code>
        </p>

        <label>
          Chiave API
          <input v-model="aiApiKey" type="password" :placeholder="aiApiKeyMasked || 'non configurata'" />
        </label>

        <label>
          Modello
          <input v-model="aiModel" placeholder="gemini-flash-lite-latest" />
        </label>
        <p class="field-hint">
          Per tradurre basta un modello economico e veloce. Esempi:
          <code>gemini-flash-lite-latest</code>, <code>gpt-4.1-mini</code>,
          <code>llama3.1</code>.
        </p>

        <label>
          Modello per i manuali scansionati (opzionale)
          <input v-model="aiVisionModel" placeholder="deepseek-v4-flash-vision-exp" />
        </label>
        <p class="field-hint">
          Serve un modello che sappia leggere le immagini. Il modello di chat qui
          sopra spesso non ne è capace: per esempio <code>deepseek-v4-flash</code> è
          solo testo, la sua variante <code>deepseek-v4-flash-vision-exp</code> legge
          anche le pagine. Lasciandolo vuoto i manuali scansionati si trascrivono a
          mano.
        </p>

        <label>
          Chiave Tavily per le FAQ di BoardGameGeek (opzionale)
          <input
            v-model="tavilyApiKey"
            type="password"
            autocomplete="off"
            :placeholder="tavilyApiKeyMasked || 'non configurata'"
          />
        </label>
        <p class="field-hint">
          Con questa chiave l'assistente regole, quando il manuale non basta, cerca
          la risposta nel forum Rules del gioco su BoardGameGeek e cita il commento.
          Serve un gioco collegato a BGG. Il piano gratuito di
          <a href="https://tavily.com" target="_blank" rel="noopener">Tavily</a>
          dà 1.000 ricerche al mese, senza carta di credito.
        </p>
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Configurazione Email (SMTP)</h2>
        </div>
        <p class="field-hint">
          Se lo configuri, l'app manda da sé l'invito di un amministratore e
          la conferma di una prenotazione — con il codice e i link per
          disdire o segnare i punti — quando chi prenota lascia un'email
          (è facoltativa). Lasciandolo vuoto funziona come prima: il codice
          resta solo a schermo e il link di invito si copia a mano.
        </p>
        <p v-if="smtpConfigured && !publicBaseUrl" class="field-hint">
          Manca l'indirizzo pubblico, qui sopra in "Generale": senza,
          l'invito di un amministratore porta un link composto
          dall'indirizzo con cui stai navigando adesso, che chi lo riceve
          potrebbe non riuscire a raggiungere.
        </p>

        <label>
          Server SMTP
          <input v-model="smtpHost" placeholder="smtp.gmail.com" autocomplete="off" />
        </label>

        <div class="field-row">
          <label>
            Porta
            <input v-model.number="smtpPort" type="number" min="1" max="65535" inputmode="numeric" />
          </label>
          <label>
            Sicurezza
            <select v-model="smtpTlsMode">
              <option value="starttls">STARTTLS (porta 587)</option>
              <option value="tls">TLS (porta 465)</option>
              <option value="none">Nessuna</option>
            </select>
          </label>
        </div>

        <label>
          Nome utente
          <input v-model="smtpUsername" autocomplete="off" placeholder="serate@example.org" />
        </label>

        <label>
          Password
          <input
            v-model="smtpPassword"
            type="password"
            autocomplete="new-password"
            :placeholder="smtpPasswordMasked || 'non configurata'"
          />
        </label>
        <p class="field-hint">
          Con Gmail serve una <strong>app password</strong> generata dal tuo
          account Google (richiede la verifica in due passaggi), non la password
          con cui accedi. Con Mailjet: <code>in-v3.mailjet.com</code>, nome
          utente = API key e password = secret key.
        </p>

        <label>
          Indirizzo mittente
          <input v-model="smtpFromAddress" type="email" inputmode="email" placeholder="serate@example.org" />
        </label>

        <label>
          Nome mittente
          <input v-model="smtpFromName" placeholder="Serate Ludiche" />
        </label>
        <p class="field-hint">
          È il nome che chi riceve legge nella casella, accanto all'indirizzo.
        </p>

        <div class="smtp-test">
          <button
            type="button"
            class="btn-secondary"
            :disabled="!smtpConfigured || smtpTesting"
            @click="sendTestEmail"
          >
            {{ smtpTesting ? 'Invio…' : 'Invia email di prova' }}
          </button>
          <p class="field-hint">
            <template v-if="smtpConfigured">
              Manda una mail al tuo indirizzo usando la configurazione salvata.
            </template>
            <template v-else>
              Compila server, porta e indirizzo mittente, poi salva: la prova usa
              la configurazione salvata.
            </template>
          </p>
        </div>
        <!--
          Annuncio e riquadro sono due elementi separati: uno screen reader
          ha bisogno di una regione live montata a permanenza (v-if la
          farebbe entrare nel DOM già col testo dentro, il caso classico che
          non si annuncia), ma un riquadro sempre montato in un
          `.panel-card` flex con gap si porta dietro lo spazio vuoto anche a
          vuoto — il gap non collassa come i margini. La regione live resta
          fuori dal flusso visivo (`.visually-hidden`), il riquadro sotto
          torna puramente presentazionale.
        -->
        <p class="visually-hidden" role="status" aria-live="polite">{{ smtpTestMessage }}</p>
        <p class="visually-hidden" role="alert" aria-live="assertive">{{ smtpTestError }}</p>
        <p v-if="smtpTestMessage" class="success">{{ smtpTestMessage }}</p>
        <p v-if="smtpTestError" class="error">{{ smtpTestError }}</p>
      </div>

      <div class="form-actions">
        <button type="submit">Salva</button>
      </div>
      <p v-if="message" class="success">{{ message }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
