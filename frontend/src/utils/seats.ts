/**
 * I posti prenotabili che una prenotazione tiene: "1 posto riservato",
 * "3 posti riservati". Mai "posti" da solo (vedi DESIGN.md, Posti
 * prenotabili): si confonderebbe con le sedie o coi giocatori del gioco.
 */
export function seatsLabel(n: number) {
  return n === 1 ? '1 posto riservato' : `${n} posti riservati`
}
