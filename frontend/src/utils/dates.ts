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

/** Il "quando" di una serata come lo manda l'API. */
export interface EventWhen {
  eventDate: string
  startTime: string
  endTime: string | null
}

/** "2026-10-01", "21:00", "01:00" → "gio 1 ott 2026 · 21:00–01:00".
 *  Senza fine resta il solo orario d'inizio. Una fine che precede l'inizio
 *  è del giorno dopo, ma "21:00–01:00" si legge già così: niente data in più. */
export function formatEventDateTime({ eventDate, startTime, endTime }: EventWhen): string {
  const hours = endTime ? `${startTime}–${endTime}` : startTime
  const parsed = parseEventDate(eventDate, startTime)
  if (!parsed) {
    return `${eventDate} · ${hours}`
  }
  return `${dateFormatter.format(parsed)} · ${hours}`
}

/** "2026-10-01" → "gio 1 ott 2026", senza orario: il registro dei prestiti
 *  di un gioco elenca serate diverse e non ha un orario per riga, solo la
 *  data di ciascuna. */
export function formatEventDate(eventDate: string): string {
  const parsed = parseEventDate(eventDate, '00:00')
  return parsed ? dateFormatter.format(parsed) : eventDate
}
