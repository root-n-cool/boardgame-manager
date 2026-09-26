/**
 * Le fasce di difficoltà, dal peso BGG (1 leggero .. 5 pesante). Le
 * soglie sono quelle con cui su BGG si parla dei giochi: sotto 2 è un
 * filler che spieghi in cinque minuti, sopra 4 è una serata sola.
 *
 * Vivono qui e non nel badge perché le leggono in due — `GameDifficulty`
 * per l'etichetta e il filtro del catalogo per selezionare — e un gioco
 * che il badge chiama "Medio" deve uscire col filtro "Medio".
 */
export const DIFFICULTY_LEVELS = ['Facile', 'Medio', 'Impegnativo', 'Esperto'] as const

export type DifficultyLevel = (typeof DIFFICULTY_LEVELS)[number]

/** La fascia di un peso; null se il gioco non ha un peso. */
export function difficultyLevel(weight: number | null | undefined): DifficultyLevel | null {
  if (!weight) {
    return null
  }
  if (weight < 2) {
    return 'Facile'
  }
  if (weight < 3) {
    return 'Medio'
  }
  if (weight < 4) {
    return 'Impegnativo'
  }
  return 'Esperto'
}
