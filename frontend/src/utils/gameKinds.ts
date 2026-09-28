/**
 * Le tipologie di gioco. È l'unico elenco del frontend: selettori admin,
 * filtri e pastiglie leggono da qui, e deve restare allineato a
 * `games.Kinds` nel backend. Una tipologia nuova si aggiunge in fondo a
 * entrambi, senza migrazioni.
 */
export const GAME_KINDS = [
  { value: 'board', label: 'Gioco da tavolo', plural: 'Giochi da tavolo', short: 'GDT', onBgg: true },
  // Un gioco di ruolo su BoardGameGeek non c'è: si inserisce a mano.
  { value: 'rpg', label: 'Gioco di ruolo', plural: 'Giochi di ruolo', short: 'GDR', onBgg: false },
] as const

export type GameKind = (typeof GAME_KINDS)[number]['value']

/** Il default, anche per un valore che il frontend non conosce ancora. */
export const DEFAULT_GAME_KIND: GameKind = 'board'

export function gameKindInfo(kind: string | null | undefined) {
  return GAME_KINDS.find((k) => k.value === kind) ?? GAME_KINDS[0]
}

/** Le voci della tendina "Tipo", pronte per `SelectFilter`. */
export const KIND_FILTER_OPTIONS = GAME_KINDS.map((k) => ({
  value: k.value as GameKind,
  label: `${k.plural} (${k.short})`,
}))

/**
 * Vero se nella lista ci sono almeno due tipologie: con un tipo solo il
 * filtro avrebbe una voce che non toglie niente.
 */
export function hasKindMix(list: readonly { kind: string }[]) {
  return new Set(list.map((g) => gameKindInfo(g.kind).value)).size > 1
}
