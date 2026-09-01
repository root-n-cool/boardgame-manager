import { mkdirSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'

const outDir = '../backend/internal/webui/dist'

/**
 * `emptyOutDir: true` deletes everything in the out dir, including the
 * tracked `.gitkeep` placeholder that keeps `//go:embed dist/*` compilable on
 * a fresh clone with no frontend build. Recreate it after the bundle is
 * written so a build never leaves the placeholder deleted in git.
 */
function keepEmbedPlaceholder(): Plugin {
  return {
    name: 'keep-embed-placeholder',
    apply: 'build',
    closeBundle() {
      const dir = resolve(import.meta.dirname, outDir)
      mkdirSync(dir, { recursive: true })
      writeFileSync(resolve(dir, '.gitkeep'), '')
    },
  }
}

export default defineConfig({
  plugins: [vue(), keepEmbedPlaceholder()],
  build: {
    outDir,
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
