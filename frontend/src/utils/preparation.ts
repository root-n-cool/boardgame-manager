/**
 * "Da preparare" nella scheda evento admin: i giochi della serata divisi
 * per proprietario, con chi li ha prenotati. Serve a chi riempie le buste
 * prima della serata: di chi è ogni scatola e quali sono già prese.
 */

export interface PrepGameInput {
  eventGameId: number
  gameId: number
  name: string
  copyIndex: number
}

export interface PrepBookingInput {
  eventGameId: number
  participantName: string
  seatsReserved: number
}

export interface PrepItem {
  eventGameId: number
  label: string
  /** "Mario", "Giulia +2" (posti in più presi per il gruppo). */
  bookedBy: string[]
}

export interface PrepGroup {
  key: string
  /** La grafia del primo gioco del gruppo; null = senza proprietario. */
  owner: string | null
  items: PrepItem[]
}

// `numeric`: "Catan #2" viene prima di "Catan #10".
const byLabel = (a: PrepItem, b: PrepItem) => a.label.localeCompare(b.label, 'it', { numeric: true })

export function groupForPreparation(
  games: PrepGameInput[],
  ownerOf: (gameId: number) => string | null | undefined,
  bookings: PrepBookingInput[],
  copies: Record<number, number>,
): PrepGroup[] {
  const bookedBy = new Map<number, string[]>()
  for (const b of bookings) {
    const extra = b.seatsReserved > 1 ? ` +${b.seatsReserved - 1}` : ''
    const list = bookedBy.get(b.eventGameId) ?? []
    list.push(`${b.participantName.trim()}${extra}`)
    bookedBy.set(b.eventGameId, list)
  }

  // Il proprietario è testo libero: "Furt", "furt " e "FURT" sono la
  // stessa persona.
  const groups = new Map<string, PrepGroup>()
  for (const g of games) {
    const raw = (ownerOf(g.gameId) ?? '').trim()
    const key = raw.toLowerCase()
    let group = groups.get(key)
    if (!group) {
      group = { key, owner: raw || null, items: [] }
      groups.set(key, group)
    }
    group.items.push({
      eventGameId: g.eventGameId,
      label: (copies[g.gameId] ?? 1) > 1 ? `${g.name} #${g.copyIndex}` : g.name,
      bookedBy: bookedBy.get(g.eventGameId) ?? [],
    })
  }

  for (const group of groups.values()) {
    const booked = group.items.filter((i) => i.bookedBy.length > 0).sort(byLabel)
    const free = group.items.filter((i) => i.bookedBy.length === 0).sort(byLabel)
    group.items = [...booked, ...free]
  }

  // Alfabetico per proprietario, chi non ce l'ha in fondo.
  return [...groups.values()].sort((a, b) => {
    if (a.owner === null) return 1
    if (b.owner === null) return -1
    return a.owner.localeCompare(b.owner, 'it')
  })
}
