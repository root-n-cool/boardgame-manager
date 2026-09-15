/**
 * Le prenotazioni fatte da questo browser, per mostrare "Prenotato" senza
 * doverle rimatchare per telefono o email: nessuno dei due si salva più
 * lato server (vedi il design in docs/superpowers/specs), quindi il
 * promemoria vive solo qui — sparisce cambiando dispositivo o browser, e
 * chi lo perde ricade comunque su "Gestisci prenotazione" col codice.
 */
export interface MyBooking {
  id: number
  bookingCode: string
  eventId: number
  eventGameId: number
  gameLabel: string
}

const STORAGE_KEY = 'bgm:my-bookings'

function readAll(): MyBooking[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return []
    }
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    // Navigazione privata, storage pieno o disabilitato: nessuna
    // prenotazione ricordata, mai un errore in pagina.
    return []
  }
}

function writeAll(bookings: MyBooking[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(bookings))
  } catch {
    // Stesso discorso di readAll: il promemoria semplicemente non
    // sopravvive.
  }
}

export function listMyBookings(): MyBooking[] {
  return readAll()
}

export function saveMyBooking(booking: MyBooking) {
  const bookings = readAll().filter((b) => b.id !== booking.id)
  bookings.push(booking)
  writeAll(bookings)
}

export function removeMyBooking(id: number) {
  writeAll(readAll().filter((b) => b.id !== id))
}
