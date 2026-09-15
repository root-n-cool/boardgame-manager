<!-- eslint-disable vue/multi-word-component-names -->
<script lang="ts">
// A differenza di <script setup> sotto, questo blocco esegue una sola
// volta al primo caricamento del chunk (a livello di modulo), non a ogni
// istanza del componente: è il posto giusto per registrare la lingua
// italiana, una volta sola, prima che un MdEditor la usi.
import { config } from 'md-editor-v3'
import 'md-editor-v3/lib/style.css'

// L'editor ha di serie solo zh-CN ed en-US: registriamo le etichette
// italiane per l'unica toolbar che usiamo (vedi sotto). Il resto
// (immagini, tabelle, mermaid, katex, github, footer) è escluso dalla
// toolbar e non serve tradurlo.
config({
  editorConfig: {
    languageUserDefined: {
      'it-IT': {
        toolbarTips: {
          bold: 'Grassetto',
          italic: 'Corsivo',
          title: 'Titolo',
          quote: 'Citazione',
          unorderedList: 'Elenco puntato',
          orderedList: 'Elenco numerato',
          code: 'Codice',
          link: 'Collegamento',
          revoke: 'Annulla',
          next: 'Ripeti',
          preview: 'Anteprima',
        },
        titleItem: {
          h1: 'Titolo 1',
          h2: 'Titolo 2',
          h3: 'Titolo 3',
          h4: 'Titolo 4',
          h5: 'Titolo 5',
          h6: 'Titolo 6',
        },
        linkModalTips: {
          linkTitle: 'Aggiungi collegamento',
          descLabel: 'Testo',
          descLabelPlaceHolder: 'Testo del collegamento',
          urlLabel: 'Indirizzo',
          urlLabelPlaceHolder: 'https://',
          buttonOK: 'OK',
        },
      },
    },
  },
})
</script>

<script setup lang="ts">
import { onMounted, ref, useId } from 'vue'
import { MdEditor, type ToolbarNames } from 'md-editor-v3'

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    ariaLabel?: string
  }>(),
  { ariaLabel: 'Descrizione' },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const editorId = useId()

// La superficie editabile di CodeMirror è un <div role="textbox"
// contenteditable>, non una <textarea>: il <label> che lo avvolge nel
// form non gli dà un nome accessibile (l'associazione implicita di
// <label> non copre in modo affidabile un contenteditable), quindi
// senza questo screen reader lo leggono come "casella di testo" senza
// altro. La libreria non espone un prop aria-*, quindi lo impostiamo a
// mano sul nodo dopo il mount.
const editorRef = ref<InstanceType<typeof MdEditor> | null>(null)

onMounted(() => {
  const root = editorRef.value?.$el as HTMLElement | undefined
  root?.querySelector('.cm-content')?.setAttribute('aria-label', props.ariaLabel)
})

// Toolbar ridotta al minimo utile per una descrizione di gioco/evento:
// niente immagini (nessun endpoint di upload dedicato), niente
// tabelle/mermaid/katex/github/catalogo — strumenti da editor tecnico,
// fuori posto in una scheda.
const toolbars: ToolbarNames[] = [
  'bold',
  'italic',
  'title',
  'unorderedList',
  'orderedList',
  'quote',
  'link',
  'code',
  '-',
  'revoke',
  'next',
  '-',
  'preview',
]
</script>

<template>
  <MdEditor
    ref="editorRef"
    :id="editorId"
    class="markdown-editor"
    :model-value="modelValue"
    language="it-IT"
    theme="light"
    :toolbars="toolbars"
    :footers="[]"
    :no-upload-img="true"
    :no-prettier="true"
    :placeholder="placeholder"
    @update:model-value="emit('update:modelValue', $event)"
  />
</template>
