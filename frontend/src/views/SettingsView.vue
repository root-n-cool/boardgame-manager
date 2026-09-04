<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface SettingsResponse {
  defaultLanguage: string
  publicBaseUrl: string
  bggApiTokenSet: boolean
  bggApiTokenMasked?: string
  aiBaseUrl: string
  aiModel: string
  aiApiKeySet: boolean
  aiApiKeyMasked?: string
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

const defaultLanguage = ref('it')
const publicBaseUrl = ref('')
const bggApiToken = ref('')
const bggApiTokenMasked = ref('')
const aiBaseUrl = ref('')
const aiModel = ref('')
const aiApiKey = ref('')
const aiApiKeyMasked = ref('')
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
  bggApiTokenMasked.value = s.bggApiTokenMasked || ''
  aiBaseUrl.value = s.aiBaseUrl || ''
  aiModel.value = s.aiModel || ''
  aiApiKeyMasked.value = s.aiApiKeyMasked || ''
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
      bggApiToken: bggApiToken.value,
      aiBaseUrl: aiBaseUrl.value,
      aiApiKey: aiApiKey.value,
      aiModel: aiModel.value,
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
      </div>

      <div class="panel-card">
        <div class="section-head">
          <h2>Configurazione Email (SMTP)</h2>
        </div>
        <p class="field-hint">
          Se lo configuri, l'app manda da sé l'invito di un amministratore, la
          conferma di una prenotazione — con il codice e i link per disdire o
          segnare i punti — e l'avviso di annullamento. Lasciandolo vuoto
          funziona come prima: il codice resta solo a schermo e il link di
          invito si copia a mano.
        </p>
        <p v-if="smtpConfigured && !publicBaseUrl" class="field-hint">
          Manca l'indirizzo pubblico, qui sopra in "Generale": senza,
          l'invito di un amministratore e l'avviso di annullamento portano
          un link composto dall'indirizzo con cui stai navigando adesso, che
          chi lo riceve potrebbe non riuscire a raggiungere.
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
