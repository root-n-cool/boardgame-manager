/**
 * La forma di un gioco come la restituisce l'API: la condividono la scheda
 * pubblica (`/games/:id`), quella di modifica (`/admin/games/:id`) e i
 * componenti che entrambe montano.
 */

export interface GameMediaInfo {
  id: number
  type: 'file' | 'link' | 'youtube'
  url: string
  title: string | null
  /** Chunk indicizzati per questo media: 0 quando non è (ancora) una fonte. */
  indexedChunks: number
}

export interface GameLanguageInfo {
  code: string
  isBaseLanguage: boolean
  name: string
  description: string | null
  media: GameMediaInfo[]
}

export interface GameDetail {
  id: number
  bggId: string | null
  name: string
  year: number | null
  minPlayers: number | null
  maxPlayers: number | null
  playtimeMinutes: number | null
  weight: number | null
  owner: string | null
  coverPath: string | null
  seats: number
  /** Vero quando esiste una descrizione BGG originale da cui ritradurre. */
  canTranslate: boolean
  /** Vero quando il gioco ha un manuale indicizzato e il provider AI è configurato. */
  canAsk: boolean
  /**
   * Le tre domande suggerite della chat, scritte dal modello a partire dal
   * manuale indicizzato. L'admin può correggerle a mano nella scheda di
   * modifica. Vuota quando il gioco non ne ha (indicizzato prima di questa
   * funzione, o generazione mai riuscita): in quel caso il pannello usa le
   * sue domande fisse.
   */
  suggestedQuestions: string[]
  languages: GameLanguageInfo[]
}

/**
 * Il nome esteso di una lingua, dal suo codice. Rende leggibile un bottone
 * ("Traduci in italiano" invece di "Traduci in it") e traduce le etichette
 * che BoardGameGeek manda in inglese sui suoi file. L'elenco copre le
 * lingue che BGG riconosce, le stesse su cui l'indice dei file sa filtrare;
 * un codice sconosciuto torna com'è.
 */
const languageNames: Record<string, string> = {
  ar: 'arabo',
  ca: 'catalano',
  cs: 'ceco',
  da: 'danese',
  de: 'tedesco',
  el: 'greco',
  en: 'inglese',
  es: 'spagnolo',
  et: 'estone',
  fa: 'persiano',
  fi: 'finlandese',
  fr: 'francese',
  gl: 'galiziano',
  he: 'ebraico',
  hr: 'croato',
  hu: 'ungherese',
  it: 'italiano',
  ja: 'giapponese',
  ko: 'coreano',
  lt: 'lituano',
  nl: 'olandese',
  no: 'norvegese',
  pl: 'polacco',
  pt: 'portoghese',
  ro: 'romeno',
  ru: 'russo',
  sr: 'serbo',
  sv: 'svedese',
  th: 'thailandese',
  tr: 'turco',
  ug: 'uiguro',
  uk: 'ucraino',
  vi: 'vietnamita',
  zh: 'cinese',
}

export function languageName(code: string): string {
  return languageNames[code] || code
}

/**
 * L'etichetta di formato di un file dalla sua estensione (PDF, TXT, MD,
 * DOCX, …): stessa regola in `GameMediaList.vue` (griglia media) e nella
 * card «Knowledge base» della scheda di modifica (`GameAdminDetailView.vue`),
 * perché è la stessa domanda — "che tipo di file è" — posta in due posti.
 * Un'estensione anomala (assente o più lunga di 5 caratteri) torna "File"
 * invece di un'etichetta illeggibile.
 */
export function fileExtensionLabel(url: string): string {
  const ext = url.split('.').pop()
  return ext && ext.length <= 5 ? ext.toUpperCase() : 'File'
}
