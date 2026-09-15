# Branding del sito e pagine legali — design

Data: 2026-09-15
Stato: approvato

## Scopo

Colmare un buco lasciato indietro nell'amministrazione: oggi il nome
"BoardGames Manager", l'icona omino e l'assenza di favicon
personalizzabile sono tutti fissi nel codice, e non esistono pagine
termini/privacy. Un'associazione che installa l'app deve poterla
rimarchizzare (titolo, favicon, logo) e pubblicare i propri testi
legali a un indirizzo fisso, tutto dal pannello impostazioni, senza
toccare codice o rebuildare.

## Invariante: tutto opzionale

Come `PublicBaseURL`, il provider AI e l'SMTP: **nessun campo qui è
obbligatorio**, e l'app senza configurazione funziona esattamente come
oggi.

| campo | vuoto/non impostato |
|---|---|
| titolo del sito | `<title>` e testo header restano "BoardGames Manager" |
| logo | header mostra icona omino + testo, come oggi |
| favicon | resta `favicon.svg` di default, come oggi |
| termini/privacy | `/terms` e `/privacy` restano raggiungibili ma mostrano "contenuto non ancora disponibile", mai un errore |

## Architettura

### Migrazione `0019_site_branding.sql`

Cinque colonne nullable su `app_settings`, stesso schema delle altre
sezioni:

```sql
ALTER TABLE app_settings ADD COLUMN site_title TEXT;
ALTER TABLE app_settings ADD COLUMN logo_filename TEXT;
ALTER TABLE app_settings ADD COLUMN favicon_filename TEXT;
ALTER TABLE app_settings ADD COLUMN terms_markdown TEXT;
ALTER TABLE app_settings ADD COLUMN privacy_markdown TEXT;
```

`settings.Settings`/`Store` guadagnano i campi corrispondenti con lo
stesso trattamento `nullIfEmpty` già in uso per `PublicBaseURL`.
Nessuno di questi è un segreto: escono in chiaro da `GET /api/settings`
come `PublicBaseURL`, non mascherati come i token.

### Upload di logo e favicon

Riusano `storage.CoverCategory` (jpg/png/webp, 5MB) così com'è: stessi
formati già accettati per copertine di giochi ed eventi, stesso
meccanismo di validazione estensione/tipo sniffato. Niente nuova
categoria — un favicon PNG è pienamente supportato da tutti i browser
correnti.

Due nuovi handler admin, ricalcati su `uploadCoverHandler`:

```
POST /api/settings/logo      multipart "file" → salva logo_filename
POST /api/settings/favicon   multipart "file" → salva favicon_filename
```

Entrambi dentro il gruppo `protected`. Il file finisce nello stesso
`Storage` (`data/uploads`) e si legge da `GET /api/uploads/{filename}`,
già pubblico — nessun nuovo endpoint di lettura.

### `GET /api/site` (nuovo, pubblico)

```json
{ "siteTitle": "Ludoteca Vicolo Corto", "logoUrl": "/api/uploads/…png" }
```

`logoUrl` è `null` quando non configurato. `siteTitle` è sempre
valorizzato: il fallback `"BoardGames Manager"` si applica qui, non nel
frontend, così ogni consumatore (header, eventualmente altro in
futuro) legge un solo valore già pronto.

### `GET /api/legal/terms` e `GET /api/legal/privacy` (nuovi, pubblici)

```json
{ "markdown": "# Termini e condizioni\n\n…" }
```

Stringa vuota se non configurato — mai 404. Sono letture dirette della
riga `app_settings`, nessuna cache.

### `PUT /api/settings` (esteso)

`updateSettingsRequest`/`settingsResponse` guadagnano `siteTitle`,
`termsMarkdown`, `privacyMarkdown`. Nessuna validazione oltre al
trim: qualunque markdown, anche vuoto, è un valore legittimo — stessa
filosofia di `AIModel`.

Qui `siteTitle` esce e rientra **grezzo** (stringa vuota se non
impostato): è il campo che l'admin edita, e mostrargli già il fallback
"BoardGames Manager" nel form lo farebbe sembrare un valore salvato
quando non lo è. Il fallback si applica solo in lettura pubblica
(`GET /api/site`, `webui` handler, template email), mai qui.

### Titolo e favicon nell'HTML servito

`webui.Handler()` cambia firma per ricevere `*settings.Store`:

```go
func Handler(store *settings.Store) (http.Handler, error)
```

L'handler richiesto per `index.html` (sia diretto sia via fallback SPA)
legge le impostazioni correnti e sostituisce due placeholder fissi nel
file embeddato prima di scriverlo in risposta:

```html
<title>__SITE_TITLE__</title>
<link rel="icon" type="image/svg+xml" href="__FAVICON_URL__" />
```

`__SITE_TITLE__` → `cfg.SiteTitle` o `"BoardGames Manager"`.
`__FAVICON_URL__` → `/api/uploads/{favicon_filename}` se impostato,
altrimenti `/favicon.svg` (il default statico attuale, invariato).
Sostituzione con `strings.Replace`, non un motore di template: sono due
placeholder fissi in un file che il progetto controlla, non input
esterno.

Tutte le altre risorse (`/assets/*`, ecc.) continuano a essere servite
da `http.FileServer` senza passare da questo percorso: nessun impatto
sulle performance o sulla cache degli asset con hash nel nome.

Un errore nel leggere le impostazioni (DB non raggiungibile) non deve
mai rompere il caricamento della pagina: in quel caso l'handler serve
`index.html` invariato (fallback ai due default), loggando l'errore.

### Branding nelle email

`mail_templates.go`: le occorrenze fisse di "BoardGames Manager"
(intestazione invito, oggetto e corpo del test SMTP) diventano un
parametro `siteName string` passato dalle tre funzioni template, letto
da chi le chiama (`createUserHandler`, `testSMTPHandler`) dalle
`Settings` già caricate in quell'handler. Fallback identico:
`siteName` vuoto → `"BoardGames Manager"`, applicato una sola volta in
un helper `siteNameOrDefault(cfg.SiteTitle)`.

## Frontend

### Header (`AppShell.vue`)

Nuovo store Pinia `site` (`stores/site.ts`), speculare a `auth`: carica
`GET /api/site` una volta in `main.ts` prima del mount (non protetto,
funziona anche in `/login`, `/setup`, pagine pubbliche).

Il blocco `.brand` diventa condizionale:

- `site.logoUrl` presente → `<img :src="site.logoUrl" class="brand-logo" alt="">`,
  altezza fissa coerente con la topbar (stessa area occupata oggi da
  icona+testo), nessun testo accanto — il logo sostituisce interamente
  icona e nome, come richiesto.
- Altrimenti → icona omino invariata + `{{ site.siteTitle }}` al posto
  del testo fisso "BoardGames Manager".

### Footer legale (nuovo, in `AppShell.vue`)

Striscia sottile in fondo alla shell, sempre visibile (dentro e fuori
sessione), due link: "Termini e condizioni" → `/terms`, "Privacy" →
`/privacy`. Stile minimo, testo piccolo, coerente con `DESIGN.md`
(feltro/cartoncino), non compete visivamente con la topbar.

### `TermsView.vue` / `PrivacyView.vue` (nuove)

Route pubbliche, `meta: { public: true }`:

```
/terms     → TermsView
/privacy   → PrivacyView
```

Entrambe: `onMounted` chiama il rispettivo `GET /api/legal/*`, e
renderizzano il markdown con `MarkdownText.vue` già esistente (stesso
componente usato per descrizioni di giochi/eventi). Se la stringa è
vuota, mostrano un messaggio neutro ("Contenuto non ancora
disponibile.") invece di un errore o una pagina bianca.

Vanno aggiunte anche nel gruppo `publicItems`? No — non sono voci di
navigazione primaria, restano raggiungibili solo dal footer e da link
diretto, per non affollare il menu.

### `SettingsView.vue` — nuova sezione "Sito"

Nuovo `panel-card` "Sito", prima o dopo quello lingua/base-URL:

- Campo testo "Titolo del sito" (con hint: usato nel titolo della
  pagina e nell'header quando non c'è un logo).
- Upload logo: anteprima dell'immagine corrente (se presente),
  `<input type="file">`, stesso pattern degli upload copertina già
  presenti altrove (invio diretto a `POST /api/settings/logo` all'atto
  della scelta file, non in coda col resto del form — coerente con
  come funzionano già le copertine di gioco/evento).
- Upload favicon: stesso pattern, anteprima piccola.
- Due `MarkdownEditor.vue` per "Termini e condizioni" e "Privacy",
  salvati col resto del form (`PUT /api/settings`), non con un
  bottone a parte.

## Verifica

- Backend: test su migrazione e `settings.Store` esteso; nuovi handler
  (`GET /api/site`, `POST /api/settings/{logo,favicon}`,
  `GET /api/legal/{terms,privacy}`); `webui` handler con `Settings`
  finte per titolo/favicon presenti/assenti e per il caso "DB non
  raggiungibile" (fallback ai default); `mail_templates_test.go` esteso
  col nuovo parametro `siteName`.
- Suite Go completa in Docker; `npm run build` per il type-check
  (nuove view, nuovo store).
- Verifica visiva in Claude in Chrome: header con e senza logo,
  `/terms` e `/privacy` con e senza contenuto configurato, footer su
  desktop e mobile, sezione "Sito" in `SettingsView`.
- Ultimo passo della todo: `/impeccable` su header, footer, pagine
  legali e sezione "Sito".

## Fuori scope

- Consenso esplicito ai termini (checkbox in fase di prenotazione) o
  versionamento dei testi legali: qui sono pagine informative, non un
  flusso di accettazione.
- Titoli `<title>` diversi per pagina (es. "Evento X — {sito}"): un
  solo titolo globale, come richiesto.
- SVG/ICO per logo o favicon: solo jpg/png/webp, vedi sezione upload.
- Multi-tenant o più set di branding: resta una singola riga di
  configurazione, come tutto `app_settings`.
