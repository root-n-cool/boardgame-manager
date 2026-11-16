/**
 * Come si vede una lista di giochi: card grandi, righe compatte o
 * miniature. La scelta è di chi guarda e resta nel suo browser, una per
 * pagina (catalogo, tavolo dell'evento). Senza scelta salvata il telefono
 * parte dalla lista — si scorre meglio in piedi al tavolo — e lo schermo
 * largo dalle card.
 */
export type ListView = 'cards' | 'list' | 'thumbs'

const VIEWS: readonly ListView[] = ['cards', 'list', 'thumbs']
const PHONE_QUERY = '(max-width: 560px)'

function storageKey(key: string) {
  return `bgm.view.${key}`
}

export function defaultListView(): ListView {
  return window.matchMedia(PHONE_QUERY).matches ? 'list' : 'cards'
}

export function readListView(key: string): ListView {
  try {
    const stored = localStorage.getItem(storageKey(key))
    if (stored && (VIEWS as readonly string[]).includes(stored)) return stored as ListView
  } catch {
    // Safari in navigazione privata solleva sull'accesso allo storage: vale
    // il default, la pagina funziona lo stesso.
  }
  return defaultListView()
}

export function saveListView(key: string, view: ListView) {
  try {
    localStorage.setItem(storageKey(key), view)
  } catch {
    /* niente da fare: la scelta resta per questa visita */
  }
}
