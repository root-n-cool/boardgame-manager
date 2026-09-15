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
  } finally {
    loaded.value = true
  }
})
</script>

<template>
  <div>
    <h1>{{ title }}</h1>
    <MarkdownText v-if="markdown" :text="markdown" />
    <p v-else-if="loaded" class="field-hint">Contenuto non ancora disponibile.</p>
  </div>
</template>
