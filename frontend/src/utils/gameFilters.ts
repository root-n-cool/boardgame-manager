/**
 * I filtri che catalogo e pagina evento condividono. Tutti lato client:
 * il catalogo di un'associazione sta in una pagina, e un filtro che
 * risponde al tocco senza giro di rete è quello che serve in piedi al
 * tavolo.
 */

interface PlayerCounts {
  minPlayers: number | null
  maxPlayers: number | null
}

/**
 * Il range di giocatori di un gioco, o null se non ne ha. Con un solo
 * estremo noto vale per entrambi: è la regola unica che leggono la scheda
 * (GameFacts), la card del catalogo e il filtro, così un gioco etichettato
 * "2–4" non può restare fuori dal filtro "4".
 */
export function playerRange(g: PlayerCounts): { min: number; max: number } | null {
  const min = g.minPlayers ?? g.maxPlayers
  const max = g.maxPlayers ?? g.minPlayers
  return min == null || max == null ? null : { min, max }
}

/** "2–4", "3", o '' se il gioco non ha il dato. */
export function formatPlayers(g: PlayerCounts) {
  const r = playerRange(g)
  if (!r) {
    return ''
  }
  return r.min === r.max ? `${r.min}` : `${r.min}–${r.max}`
}

const PLAYER_OPTIONS = [2, 3, 4, 5, 6] as const
/** L'ultima opzione vale "sei o più". */
const PLAYERS_MAX_OPTION = PLAYER_OPTIONS[PLAYER_OPTIONS.length - 1]

/** Le voci della tendina "Giocatori", pronte per `SelectFilter`. */
export const PLAYER_FILTER_OPTIONS = PLAYER_OPTIONS.map((n) => ({
  value: n as number,
  label: n === PLAYERS_MAX_OPTION ? `${n}+ giocatori` : `${n} giocatori`,
}))

/**
 * Vero se il gioco si gioca in `n`. Con "6+" basta che arrivi almeno a sei.
 * Un gioco senza numero di giocatori non si può dire compatibile: resta
 * fuori quando il filtro è attivo, e c'è sempre quando non lo è.
 */
export function fitsPlayers(g: PlayerCounts, n: number) {
  const r = playerRange(g)
  if (!r) {
    return false
  }
  return n === PLAYERS_MAX_OPTION ? r.max >= n : r.min <= n && n <= r.max
}

/** Confronto di nomi che ignora maiuscole e accenti ("Città" = "citta"). */
export function normalizeName(s: string) {
  return s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase().trim()
}
