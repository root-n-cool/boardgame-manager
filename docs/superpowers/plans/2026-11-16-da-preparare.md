# "Da preparare" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Sezione "Da preparare" (giochi per proprietario, prenotati in evidenza) nella scheda evento admin.
**Architecture:** Funzione pura di raggruppamento in `frontend/src/utils/preparation.ts`, usata da `EventAdminDetailView.vue`; stili in `app.css`.
**Spec:** `docs/superpowers/specs/2026-11-16-da-preparare-design.md`

## Global Constraints
- Nessun cambio backend, nessuna dipendenza. Verifica: `npm run build` + Chrome (desktop e 390px). Commit solo su richiesta.

## Review Focus
- Proprietario vuoto/null/solo spazi → "Senza proprietario".
- "Furt" e " furt " nello stesso gruppo.
- Più copie dello stesso gioco, una prenotata e l'altra no.
- Più prenotazioni sulla stessa copia; prenotazione di gruppo (+N).
- Evento senza giochi: sezione con nota vuota, non un errore.

### Task 1: `groupForPreparation`
`frontend/src/utils/preparation.ts`:
`groupForPreparation(games: {eventGameId, gameId, name, copyIndex}[], ownerOf: (gameId) => string | null | undefined, bookings: {eventGameId, participantName, seatsReserved}[], copies: Record<number, number>): PrepGroup[]`
con `PrepGroup = { key: string; owner: string | null; items: PrepItem[] }` e
`PrepItem = { eventGameId: number; label: string; bookedBy: string[] }` (es. `"Mario +2"`).
Ordinamenti come da spec (`localeCompare(..., 'it')`).

### Task 2: sezione nella pagina
`EventAdminDetailView.vue`: tenere `event.games` in un ref, `owner?: string | null` in `PickerGame` (o tipo locale), computed `preparation`, pannello "Da preparare" prima di "Prenotazioni". CSS `.prep-*` in `app.css`. `npm run build`, Chrome.

### Task 3: chiusura
`DESIGN.md` (sezione breve), revisione, `/impeccable audit` sulla sezione.
