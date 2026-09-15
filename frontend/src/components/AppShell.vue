<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import UserMenu from './UserMenu.vue'

// Sopra questa soglia la sidebar aperta sta nel flusso accanto al contenuto;
// sotto diventa un drawer che ci scivola sopra con l'overlay.
const DESKTOP_QUERY = '(min-width: 900px)'
const STORAGE_KEY = 'bgm.sidebar'
const BODY_LOCK_CLASS = 'has-open-drawer'

const auth = useAuthStore()
const site = useSiteStore()
const route = useRoute()

interface NavItem {
  label: string
  to: string
  icon: string
  /** La voce è accesa su tutte le pagine che discendono da lei, non solo sul
   *  suo indirizzo esatto: la scheda di un evento resta sotto "Eventi". */
  matches: (path: string) => boolean
}

const ICONS: Record<string, string> = {
  calendar:
    'M4.5 8.5h15M8 4v3M16 4v3M5.5 6.5h13a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-11a1 1 0 0 1 1-1Z',
  ticket:
    'M4.5 9.2V7.5a1 1 0 0 1 1-1h13a1 1 0 0 1 1 1v1.7a2.8 2.8 0 0 0 0 5.6v1.7a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-1.7a2.8 2.8 0 0 0 0-5.6M13.5 6.5v11',
  box: 'M12 3.5l7.5 4v9L12 20.5l-7.5-4v-9l7.5-4ZM12 12l7.5-4.5M12 12v8.5M12 12L4.5 7.5',
  users:
    'M9.5 11.5a3.4 3.4 0 1 0 0-6.8 3.4 3.4 0 0 0 0 6.8ZM3.5 19.5c0-2.8 2.7-4.6 6-4.6s6 1.8 6 4.6M17 19.5c0-1.9-.6-3.4-1.8-4.4M16.2 5.1a3.4 3.4 0 0 1 0 6.4',
  sliders: 'M4.5 7.5h9M17.5 7.5h2M4.5 16.5h2M10.5 16.5h9M15.5 5.3v4.4M8.5 14.3v4.4',
}

const publicItems: NavItem[] = [
  {
    label: 'Eventi',
    to: '/',
    icon: 'calendar',
    // Anche le schede pubbliche dei giochi (/games/:id) stanno sotto Eventi:
    // ci si arriva dal tavolo di una serata, non da uno scaffale a sé.
    matches: (p) => p === '/' || p.startsWith('/events') || p.startsWith('/games'),
  },
  {
    label: 'Gestisci prenotazione',
    to: '/manage-booking',
    icon: 'ticket',
    matches: (p) => p.startsWith('/manage-booking') || p.startsWith('/prenotazione'),
  },
]

const adminItems: NavItem[] = [
  { label: 'Eventi', to: '/admin/events', icon: 'calendar', matches: (p) => p.startsWith('/admin/events') },
  { label: 'Giochi', to: '/admin/games', icon: 'box', matches: (p) => p.startsWith('/admin/games') },
  { label: 'Utenti', to: '/admin/users', icon: 'users', matches: (p) => p.startsWith('/admin/users') },
  {
    label: 'Impostazioni',
    to: '/admin/settings',
    icon: 'sliders',
    matches: (p) => p.startsWith('/admin/settings'),
  },
]

// Il gruppo di gestione esiste solo a sessione aperta: chi partecipa non deve
// nemmeno intuire che sotto ci sia un'area riservata.
const groups = computed(() =>
  auth.user
    ? [
        { id: 'pubblico', label: '', items: publicItems },
        { id: 'gestione', label: 'Gestione', items: adminItems },
      ]
    : [{ id: 'pubblico', label: '', items: publicItems }],
)

function readStored(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'open'
  } catch {
    // Safari in navigazione privata solleva sull'accesso allo storage: si
    // perde la preferenza, non la sidebar.
    return false
  }
}

// Lo stato iniziale si decide qui e non in onMounted: deciderlo dopo il primo
// paint farebbe lampeggiare il layout a sidebar chiusa.
const isDesktop = ref(window.matchMedia(DESKTOP_QUERY).matches)
const open = ref(isDesktop.value && readStored())
const toggleButton = ref<HTMLButtonElement | null>(null)
const sidebar = ref<HTMLElement | null>(null)

// Il drawer è quello che copre la pagina: solo lui blocca lo scroll dietro,
// rende inerte il contenuto e chiude su Esc.
const isDrawer = computed(() => open.value && !isDesktop.value)

function isActive(item: NavItem) {
  return item.matches(route.path)
}

function store(value: boolean) {
  // La preferenza è quella del desktop: aprire il drawer sul telefono non
  // deve decidere come si presenta l'app sul portatile.
  if (!isDesktop.value) return
  try {
    localStorage.setItem(STORAGE_KEY, value ? 'open' : 'closed')
  } catch {
    /* niente da fare: resta lo stato in memoria */
  }
}

function focusFirstLink() {
  nextTick(() => sidebar.value?.querySelector<HTMLElement>('a')?.focus())
}

function toggle() {
  open.value = !open.value
  store(open.value)
  if (open.value && !isDesktop.value) focusFirstLink()
}

function close(refocus = false) {
  if (!open.value) return
  open.value = false
  store(false)
  if (refocus) toggleButton.value?.focus()
}

function onMediaChange(e: MediaQueryListEvent) {
  isDesktop.value = e.matches
  // Passando a schermo stretto il drawer si chiude (coprirebbe la pagina);
  // tornando largo riprende la preferenza salvata.
  open.value = e.matches ? readStored() : false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isDrawer.value) close(true)
}

let media: MediaQueryList | null = null

onMounted(() => {
  site.load()
  media = window.matchMedia(DESKTOP_QUERY)
  media.addEventListener('change', onMediaChange)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  media?.removeEventListener('change', onMediaChange)
  document.removeEventListener('keydown', onKeydown)
  document.body.classList.remove(BODY_LOCK_CLASS)
})

// Con il drawer aperto la pagina sotto l'overlay non deve scorrere: sul
// telefono il pollice finisce sull'overlay molto prima che sul link.
watch(isDrawer, (drawer) => {
  document.body.classList.toggle(BODY_LOCK_CLASS, drawer)
})

// Su telefono il drawer si chiude appena si segue un link: la pagina appena
// aperta è quello che si vuole vedere.
watch(
  () => route.fullPath,
  () => {
    if (!isDesktop.value) open.value = false
  },
)
</script>

<template>
  <div class="app-shell" :class="{ 'is-open': open, 'is-drawer': isDrawer }">
    <header class="app-topbar">
      <button
        ref="toggleButton"
        type="button"
        class="app-nav-toggle"
        :aria-expanded="open"
        aria-controls="app-sidebar"
        :aria-label="open ? 'Chiudi il menù' : 'Apri il menù'"
        @click="toggle"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path
            d="M4 7h16M4 12h16M4 17h16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
          />
        </svg>
      </button>

      <router-link
        :to="{ name: 'events' }"
        class="brand"
        :aria-label="site.logoFilename ? site.siteTitle : undefined"
      >
        <img
          v-if="site.logoFilename"
          :src="`/api/uploads/${site.logoFilename}`"
          class="brand-logo"
          alt=""
        />
        <template v-else>
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <circle cx="12" cy="5.6" r="2.6" fill="currentColor" />
            <path
              d="M9 9.2h6c1.6 0 2.6.1 3.3.5l.1 3c.05.7-.6 1.2-1.3 1l-1.7-.5-.4 2c.6 1.8.8 3.6.6 5.4h-2.3l-.6-4.6h-1.4l-.6 4.6H8.4c-.2-1.8 0-3.6.6-5.4l-.4-2-1.7.5c-.7.2-1.35-.3-1.3-1l.1-3c.7-.4 1.7-.5 3.3-.5Z"
              fill="currentColor"
            />
          </svg>
          <span class="brand-name">{{ site.siteTitle }}</span>
        </template>
      </router-link>

      <UserMenu v-if="auth.user" />
    </header>

    <div class="app-body">
      <!-- `inert` a sidebar chiusa: fuori schermo non deve restare in tab. -->
      <nav id="app-sidebar" ref="sidebar" class="app-sidebar" :inert="!open" aria-label="Navigazione">
        <template v-for="group in groups" :key="group.id">
          <p v-if="group.label" :id="`app-sidebar-${group.id}`" class="app-sidebar-group">
            {{ group.label }}
          </p>
          <ul :aria-labelledby="group.label ? `app-sidebar-${group.id}` : undefined">
            <li v-for="item in group.items" :key="item.to">
              <router-link
                :to="item.to"
                :class="{ 'is-active': isActive(item) }"
                :aria-current="isActive(item) ? 'page' : undefined"
              >
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path
                    :d="ICONS[item.icon]"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.7"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                </svg>
                {{ item.label }}
              </router-link>
            </li>
          </ul>
        </template>
      </nav>

      <div v-if="isDrawer" class="app-overlay" @click="close()"></div>

      <!-- Con il drawer aperto il contenuto sotto l'overlay è inerte: il Tab
           non deve uscire dal menù per finire in una pagina che non si vede. -->
      <main class="app-main" :inert="isDrawer">
        <div class="app-page">
          <slot />
        </div>
        <footer class="app-footer">
          <router-link to="/terms">Termini e condizioni</router-link>
          <span aria-hidden="true">·</span>
          <router-link to="/privacy">Privacy</router-link>
        </footer>
      </main>
    </div>
  </div>
</template>
