/**
 * Un esito di riconsegna incompleta: una voce mancante (`returned` un
 * numero sotto l'atteso) o mai verificata (`returned` null). La condividono
 * il banco prestiti di una serata (`LoanDeskView`) e il registro dei
 * prestiti di un gioco (`GameLoansView`): vive qui, non duplicata in due
 * viste — altrimenti la stessa frase finisce per divergere fra le due.
 */
export interface MaterialIssue {
  name: string
  expected: number
  /** null = voce non verificata al momento della riconsegna. */
  returned: number | null
}

/**
 * "mancano: carte 35/40 · non verificate: dadi" — il problema si legge dalla
 * lista, senza aprire niente. Un prestito pulito non ha esiti e non stampa
 * nessuna riga.
 */
export function issuesLabel(issues: MaterialIssue[]): string {
  const missing = issues
    .filter((i) => i.returned !== null)
    .map((i) => `${i.name} ${i.returned}/${i.expected}`)
  const unchecked = issues.filter((i) => i.returned === null).map((i) => i.name)
  const parts: string[] = []
  if (missing.length) {
    parts.push(`mancano: ${missing.join(', ')}`)
  }
  if (unchecked.length) {
    parts.push(`non verificate: ${unchecked.join(', ')}`)
  }
  return parts.join(' · ')
}
