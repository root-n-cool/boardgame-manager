import { defineStore } from 'pinia'
import { api } from '../api/client'

/** Una tipologia di gioco (GDT, GDR, MTG...), gestita dall'admin. */
export interface GameType {
  id: number
  name: string
  slug: string
  /** Se la creazione di un gioco di questa tipologia passa da BoardGameGeek. */
  bggSearch: boolean
  position: number
  /** Solo con sessione admin. */
  gameCount?: number
}

/**
 * Quanti colori ha la palette delle pastiglie (`.type-color-N` in app.css).
 * Il colore segue l'ordine: la prima tipologia prende il neutro, la seconda
 * l'accento, e così via ricominciando.
 */
const PALETTE_SIZE = 6

export const useGameTypesStore = defineStore('gameTypes', {
  state: () => ({
    list: [] as GameType[],
    loaded: false,
  }),
  getters: {
    defaultType: (state) => state.list[0],
  },
  actions: {
    /** Una richiesta per sessione; `force` dopo una modifica admin. */
    async load(force = false) {
      if (this.loaded && !force) {
        return
      }
      try {
        this.list = await api.get<GameType[]>('/game-types')
      } catch (e) {
        // Senza tipologie le liste restano senza tab e le pastiglie neutre:
        // la pagina si usa lo stesso.
        console.error('could not load game types', e)
      } finally {
        this.loaded = true
      }
    },
    byId(id: number | null | undefined) {
      return this.list.find((t) => t.id === id)
    },
    colorClass(id: number | null | undefined) {
      const i = this.list.findIndex((t) => t.id === id)
      return `type-color-${i < 0 ? 0 : i % PALETTE_SIZE}`
    },
  },
})
