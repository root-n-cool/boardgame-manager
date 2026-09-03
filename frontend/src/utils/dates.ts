/**
 * Le date arrivano dall'API come le tiene SQLite — "2026-10-01" e "21:00" —
 * e così non si leggono: in lista servono in italiano, con il giorno della
 * settimana, che è l'informazione che conta per chi organizza una serata.
 */
const dateFormatter = new Intl.DateTimeFormat('it-IT', {
  weekday: 'short',
  day: 'numeric',
  month: 'short',
  year: 'numeric',
})

/** Interpreta "2026-10-01" + "21:00" come ora locale, non come UTC: senza
 *  l'ora, `new Date('2026-10-01')` è mezzanotte UTC e a occidente di
 *  Greenwich mostrerebbe il giorno prima. */
function parseEventDate(eventDate: string, startTime: string): Date | null {
  const parsed = new Date(`${eventDate}T${startTime || '00:00'}`)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

/** "2026-10-01", "21:00" → "gio 1 ott 2026 · 21:00" */
export function formatEventDateTime(eventDate: string, startTime: string): string {
  const parsed = parseEventDate(eventDate, startTime)
  if (!parsed) {
    return `${eventDate} · ${startTime}`
  }
  return `${dateFormatter.format(parsed)} · ${startTime}`
}
