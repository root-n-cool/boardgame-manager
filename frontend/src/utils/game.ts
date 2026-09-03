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
  languages: GameLanguageInfo[]
}
