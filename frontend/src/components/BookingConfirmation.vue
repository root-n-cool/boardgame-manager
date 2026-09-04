<script setup lang="ts">
/**
 * L'esito di una prenotazione. Il codice compare due volte — dentro la
 * modale appena confermata e nel riepilogo in cima alla pagina — e questo
 * componente tiene le due copie identiche.
 *
 * `hint` porta la riga "conservalo per...": vera nella modale, falsa nel
 * riepilogo, dove la nota si dice una volta sotto tutti i codici invece di
 * ripetersi identica sotto ognuno.
 *
 * `mailed` dice se una mail col codice e i link è partita davvero: su
 * un'istanza senza SMTP configurato non parte niente, e il codice a
 * schermo resta l'unica cosa che il partecipante si porta via — prometterne
 * una che non arriva sarebbe il modo più rapido di fargli perdere il
 * codice.
 */
withDefaults(
  defineProps<{
    gameLabel: string
    code: string
    multiSeat: boolean
    hint?: boolean
    mailed?: boolean
  }>(),
  { hint: true, mailed: false },
)
</script>

<template>
  <div class="booking-confirmation">
    <p class="success">Prenotazione confermata per {{ gameLabel }}!</p>
    <div class="booking-code-card">
      <span class="label">Il tuo codice</span>
      <span class="booking-code">{{ code }}</span>
    </div>
    <p v-if="hint">
      Conservalo per gestire la prenotazione o inserire il punteggio finale da "Gestisci
      prenotazione".
    </p>
    <p v-if="mailed">
      Ti abbiamo mandato una mail con il codice e i link per annullare o segnare
      i punti.
    </p>
    <p v-if="multiSeat">
      Questo tavolo ha più posti prenotabili, uno a testa: il punteggio finale è uno per tavolo e
      chiunque sieda qui può inserirlo o correggerlo con il proprio codice.
    </p>
  </div>
</template>
