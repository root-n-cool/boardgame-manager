<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const open = ref(false)
const trigger = ref<HTMLButtonElement | null>(null)
const panel = ref<HTMLDivElement | null>(null)

// L'unico dato che il backend espone su chi è connesso è l'email: le
// iniziali vengono dalla parte locale, spezzata sui separatori che la gente
// usa davvero (nome.cognome, nome_cognome, nome+tag).
const initials = computed(() => {
  const local = (auth.user?.email ?? '').split('@')[0]
  const parts = local.split(/[.\-_+]+/).filter(Boolean)
  const letters = parts.slice(0, 2).map((p) => p[0])
  return (letters.join('') || '?').toUpperCase()
})

function focusFirstItem() {
  nextTick(() => panel.value?.querySelector<HTMLElement>('[role="menuitem"]')?.focus())
}

function close(refocus = false) {
  if (!open.value) return
  open.value = false
  if (refocus) trigger.value?.focus()
}

function toggle() {
  open.value = !open.value
  if (open.value) focusFirstItem()
}

function onPointerDown(e: PointerEvent) {
  const target = e.target as Node
  if (trigger.value?.contains(target) || panel.value?.contains(target)) return
  close()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    close(true)
    return
  }
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  const items = Array.from(panel.value?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])
  if (items.length === 0) return
  e.preventDefault()
  const current = items.indexOf(document.activeElement as HTMLElement)
  const step = e.key === 'ArrowDown' ? 1 : -1
  const next = (current + step + items.length) % items.length
  items[next].focus()
}

// I listener globali vivono solo mentre il menù è aperto: un dropdown chiuso
// non deve intercettare i tasti del resto della pagina.
watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeydown)
  } else {
    document.removeEventListener('pointerdown', onPointerDown)
    document.removeEventListener('keydown', onKeydown)
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown)
  document.removeEventListener('keydown', onKeydown)
})

watch(() => route.fullPath, () => close())

async function logout() {
  close()
  // Lo store chiude comunque la sessione locale: un POST /logout fallito non
  // deve trasformare questa voce in un pulsante inerte.
  try {
    await auth.logout()
  } catch (e) {
    console.error('logout request failed', e)
  }
  // Si torna alla bacheca pubblica, non a /login: l'indirizzo di accesso ora
  // non è più esposto da nessun link.
  router.push({ name: 'events' })
}
</script>

<template>
  <div class="user-menu">
    <button
      ref="trigger"
      type="button"
      class="user-menu-trigger"
      :aria-expanded="open"
      aria-haspopup="menu"
      aria-controls="user-menu-panel"
      @click="toggle"
    >
      <span class="user-avatar" aria-hidden="true">{{ initials }}</span>
      <span class="visually-hidden">Menù utente ({{ auth.user?.email }})</span>
      <svg class="user-menu-caret" viewBox="0 0 24 24" aria-hidden="true">
        <path
          d="M7 10l5 5 5-5"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </button>

    <div v-if="open" id="user-menu-panel" ref="panel" class="user-menu-panel" role="menu">
      <p class="user-menu-email">{{ auth.user?.email }}</p>
      <button type="button" role="menuitem" class="user-menu-item" @click="logout">
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path
            d="M15.5 8.2V6.5a1 1 0 0 0-1-1h-8a1 1 0 0 0-1 1v11a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1v-1.7M11 12h8.5M17 9.3l2.5 2.7-2.5 2.7"
            fill="none"
            stroke="currentColor"
            stroke-width="1.7"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
        Esci
      </button>
    </div>
  </div>
</template>
