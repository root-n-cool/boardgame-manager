<script setup lang="ts">
import { ref } from 'vue'
import type { GameMediaInfo } from '../utils/game'

/**
 * La griglia dei materiali di una lingua: manuali PDF, link e tutorial
 * YouTube. In sola lettura sulla scheda pubblica, con rimozione e tessera
 * "aggiungi" su quella di modifica.
 */
const props = defineProps<{ media: GameMediaInfo[]; editable?: boolean }>()
const emit = defineEmits<{ add: []; remove: [id: number, title: string] }>()

const mediaKindLabels: Record<string, string> = {
  file: 'PDF',
  link: 'Link',
  youtube: 'YouTube',
}

function mediaHref(m: GameMediaInfo): string {
  return m.type === 'file' ? `/api/uploads/${m.url}` : m.url
}

function mediaTitle(m: GameMediaInfo): string {
  if (m.title) {
    return m.title
  }
  return m.type === 'file' ? 'Manuale' : m.url
}

/**
 * L'id video dai tre formati che YouTube produce (watch?v=, youtu.be/,
 * /embed/). Serve solo per la miniatura: se non lo troviamo, la card cade
 * sull'icona come per gli altri media.
 */
function youtubeThumb(url: string): string | null {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return null
  }
  const host = parsed.hostname.replace(/^www\./, '')
  let id = ''
  if (host === 'youtu.be') {
    id = parsed.pathname.slice(1)
  } else if (host.endsWith('youtube.com')) {
    id = parsed.searchParams.get('v') || parsed.pathname.replace(/^\/(embed|shorts)\//, '')
  }
  id = id.split('/')[0]
  return /^[\w-]{6,}$/.test(id) ? `https://img.youtube.com/vi/${id}/hqdefault.jpg` : null
}

// Miniature che non hanno caricato (video privato, rimosso, rete assente):
// la card torna alla tessera con l'icona invece di lasciare un buco.
const brokenThumbs = ref<number[]>([])

function previewThumb(m: GameMediaInfo): string | null {
  if (m.type !== 'youtube' || brokenThumbs.value.includes(m.id)) {
    return null
  }
  return youtubeThumb(m.url)
}

function onThumbError(m: GameMediaInfo) {
  if (!brokenThumbs.value.includes(m.id)) {
    brokenThumbs.value.push(m.id)
  }
}
</script>

<template>
  <p v-if="!editable && props.media.length === 0" class="empty-note">
    Nessun media per questa lingua.
  </p>
  <div v-else class="game-grid media-grid">
    <ul role="list" class="game-grid-items">
      <li v-for="m in props.media" :key="m.id">
        <a :href="mediaHref(m)" target="_blank" rel="noopener">
          <img
            v-if="previewThumb(m)"
            :src="previewThumb(m) as string"
            alt=""
            width="480"
            height="360"
            loading="lazy"
            decoding="async"
            @error="onThumbError(m)"
          />
          <span v-else class="cover-placeholder media-thumb" aria-hidden="true">
            <svg v-if="m.type === 'file'" viewBox="0 0 24 24" fill="none">
              <path
                d="M6.5 3.5h7.2L18.5 8.3v12.2H6.5V3.5Z"
                stroke="currentColor"
                stroke-width="1.6"
                stroke-linejoin="round"
              />
              <path d="M13.4 3.7v4.8h4.9M9.3 13h5.4M9.3 16.4h5.4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
            </svg>
            <svg v-else-if="m.type === 'youtube'" viewBox="0 0 24 24" fill="none">
              <rect x="2.8" y="5.4" width="18.4" height="13.2" rx="3.4" stroke="currentColor" stroke-width="1.6" />
              <path d="m10.4 9.4 4.6 2.6-4.6 2.6V9.4Z" fill="currentColor" />
            </svg>
            <svg v-else viewBox="0 0 24 24" fill="none">
              <path
                d="M10.2 13.8a3.6 3.6 0 0 0 5.1 0l2.9-2.9a3.6 3.6 0 1 0-5.1-5.1l-1 1M13.8 10.2a3.6 3.6 0 0 0-5.1 0l-2.9 2.9a3.6 3.6 0 1 0 5.1 5.1l1-1"
                stroke="currentColor"
                stroke-width="1.6"
                stroke-linecap="round"
              />
            </svg>
          </span>
          <h3>{{ mediaTitle(m) }}</h3>
          <p class="media-kind">{{ mediaKindLabels[m.type] ?? m.type }}</p>
        </a>
        <button
          v-if="editable"
          type="button"
          class="media-remove"
          :aria-label="`Rimuovi ${mediaTitle(m)}`"
          @click="emit('remove', m.id, mediaTitle(m))"
        >
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M7 7l10 10M17 7 7 17" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
        </button>
      </li>
    </ul>
    <button v-if="editable" type="button" class="add-card" @click="emit('add')">
      <span class="add-slot" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none">
          <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
      </span>
      <span class="add-label">Aggiungi media</span>
    </button>
  </div>
</template>
