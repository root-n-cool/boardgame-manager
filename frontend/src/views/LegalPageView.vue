<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import MarkdownText from '../components/MarkdownText.vue'

const props = defineProps<{
  title: string
  apiPath: '/legal/terms' | '/legal/privacy'
}>()

const markdown = ref('')
const loaded = ref(false)

onMounted(async () => {
  try {
    const res = await api.get<{ markdown: string }>(props.apiPath, { skipAuthRedirect: true })
    markdown.value = res.markdown
  } catch (e) {
    // La pagina si degrada già correttamente (resta "Contenuto non ancora
    // disponibile."): questo catch serve solo a evitare una promise
    // rifiutata senza gestore in console.
    console.error('could not load the legal page content', e)
  } finally {
    loaded.value = true
  }
})
</script>

<template>
  <div class="legal-page">
    <h1>{{ title }}</h1>
    <MarkdownText v-if="markdown" :text="markdown" />
    <p v-else-if="loaded" class="field-hint">Contenuto non ancora disponibile.</p>
  </div>
</template>
