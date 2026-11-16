<script setup lang="ts">
import logoUrl from '../assets/logo-rootncool.png'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import UserMenu from './UserMenu.vue'

// Sopra questa soglia la sidebar è una colonna a tutta altezza accanto al
// contenuto — aperta con le voci, chiusa ridotta alle sole icone — e si
// apre e chiude dall'hamburger in cima alla colonna; sotto diventa un drawer che
// scivola sopra la pagina con l'overlay. Lo apre sempre l'hamburger: da
// desktop sta in cima alla sidebar stessa, da telefono nella topbar.
const DESKTOP_QUERY = '(min-width: 900px)'
const STORAGE_KEY = 'bgm.sidebar'
const BODY_LOCK_CLASS = 'has-open-drawer'

const auth = useAuthStore()
const site = useSiteStore()
const route = useRoute()

interface NavItem {
  label: string
  /** Etichetta corta per la riga di menù sul telefono, dove le tre voci
   *  pubbliche devono stare in 390px. */
  short?: string
  to: string
  icon: string
  /** La pedina del logo che segna la sezione (come le rubriche del blog):
   *  colora il pallino nel menù e la barra accanto al titolo di pagina. */
  pedina: 'arancio' | 'blu' | 'verde' | 'grigio'

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
  tag: 'M4.5 12.3V5.5a1 1 0 0 1 1-1h6.8l7.2 7.2-7.7 7.7-7.3-7.1ZM8.6 8.6h.01',
  sliders: 'M4.5 7.5h9M17.5 7.5h2M4.5 16.5h2M10.5 16.5h9M15.5 5.3v4.4M8.5 14.3v4.4',
}

const publicItems: NavItem[] = [
  {
    label: 'Eventi',
    to: '/',
    icon: 'calendar',
    pedina: 'arancio',
    matches: (p) => p === '/' || p.startsWith('/events'),
  },
  {
    label: 'Giochi',
    to: '/giochi',
    icon: 'box',
    pedina: 'blu',
    // Le schede pubbliche (/games/:id) stanno sotto lo scaffale da cui
    // vengono, anche quando ci si arriva dal tavolo di una serata.
    matches: (p) => p.startsWith('/giochi') || p.startsWith('/games'),
  },
  {
    label: 'Gestisci prenotazione',
    short: 'Prenotazione',
    to: '/manage-booking',
    icon: 'ticket',
    pedina: 'verde',
    matches: (p) => p.startsWith('/manage-booking') || p.startsWith('/prenotazione'),
  },
]

const adminItems: NavItem[] = [
  {
    label: 'Eventi',
    to: '/admin/events',
    icon: 'calendar',
    pedina: 'arancio',
    matches: (p) => p.startsWith('/admin/events'),
  },
  { label: 'Giochi', to: '/admin/games', icon: 'box', pedina: 'blu', matches: (p) => p.startsWith('/admin/games') },
  {
    label: 'Tipologie',
    to: '/admin/game-types',
    icon: 'tag',
    pedina: 'grigio',
    matches: (p) => p.startsWith('/admin/game-types'),
  },
  { label: 'Utenti', to: '/admin/users', icon: 'users', pedina: 'verde', matches: (p) => p.startsWith('/admin/users') },
  {
    label: 'Impostazioni',
    to: '/admin/settings',
    icon: 'sliders',
    pedina: 'grigio',
    matches: (p) => p.startsWith('/admin/settings'),
  },
]

// Il gruppo di gestione esiste solo a sessione aperta: chi partecipa non deve
// nemmeno intuire che sotto ci sia un'area riservata.
// Sul telefono le voci pubbliche stanno già nella riga sotto il logo: il
// drawer porta solo la gestione.
const groups = computed(() => {
  const pubblico = { id: 'pubblico', label: '', items: publicItems }
  const gestione = { id: 'gestione', label: 'Gestione', items: adminItems }
  if (!auth.user) return [pubblico]
  return isDesktop.value ? [pubblico, gestione] : [gestione]
})

function readStored(): boolean {
  try {
    // Aperta finché non la si chiude: da chiusa resta la colonna di icone,
    // ma chi arriva per la prima volta deve leggere le voci per intero.
    return localStorage.getItem(STORAGE_KEY) !== 'closed'
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

// La sezione corrente dà il colore alla barra accanto al titolo di pagina;
// fuori dalle sezioni (termini, privacy) resta il gesso.
const segno = computed(() => {
  const item = [...publicItems, ...adminItems].find(isActive)
  return item ? `var(--${item.pedina})` : 'var(--ink)'
})

// Sul telefono la testata si toglie di mezzo scorrendo in giù e torna appena
// si risale, come quella del blog: al tavolo lo schermo serve alle scatole.
// In cima alla pagina resta sempre, e non sparisce mai col menù aperto o col
// focus dentro (chi naviga da tastiera non deve perderla).
const HIDE_THRESHOLD = 120
const header = ref<HTMLElement | null>(null)
const headerHidden = ref(false)
let lastScrollY = 0

function onScroll() {
  const y = window.scrollY
  if (Math.abs(y - lastScrollY) < 6) return
  const focusInside = header.value?.contains(document.activeElement) ?? false
  headerHidden.value = !isDesktop.value && !open.value && !focusInside && y > lastScrollY && y > HIDE_THRESHOLD
  lastScrollY = y
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
  media = window.matchMedia(DESKTOP_QUERY)
  media.addEventListener('change', onMediaChange)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('scroll', onScroll, { passive: true })
})

onBeforeUnmount(() => {
  media?.removeEventListener('change', onMediaChange)
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('scroll', onScroll)
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
    headerHidden.value = false
  },
)
</script>

<template>
  <div
    class="app-shell"
    :class="{ 'is-open': open, 'is-drawer': isDrawer, 'is-header-hidden': headerHidden }"
    :style="{ '--segno': segno }"
  >
    <header ref="header" class="app-topbar" @focusin="headerHidden = false">
      <!-- Sul telefono l'hamburger serve solo all'area di gestione: le voci
           pubbliche stanno in chiaro nella riga sotto il logo. -->
      <button
        v-if="auth.user || isDesktop"
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

      <!-- Il logo di Root'n'Cool, preso dal blog: fisso, non si carica più
           dalle impostazioni. Il nome del link è il titolo del sito. -->
      <router-link :to="{ name: 'events' }" class="brand" :aria-label="site.siteTitle">
        <img :src="logoUrl" class="brand-logo" alt="" />
      </router-link>

      <UserMenu v-if="auth.user" />

      <!-- Telefono: le tre voci pubbliche in chiaro, ognuna con la sua pedina,
           come le rubriche nella testata del blog. Da desktop ci pensa la
           sidebar. -->
      <nav v-if="!isDesktop" class="app-mobile-nav" aria-label="Sezioni">
        <router-link
          v-for="item in publicItems"
          :key="item.to"
          :to="item.to"
          :aria-current="isActive(item) ? 'page' : undefined"
          :style="{ '--pedina': `var(--${item.pedina})` }"
        >
          <i class="app-pedina" aria-hidden="true"></i>{{ item.short ?? item.label }}
        </router-link>
      </nav>
    </header>

    <div class="app-body">
      <!-- `inert` solo per il drawer chiuso, che sta fuori schermo e non deve
           restare in tab. Da desktop chiusa è la colonna di icone: si usa. -->
      <nav
        id="app-sidebar"
        ref="sidebar"
        class="app-sidebar"
        :inert="!open && !isDesktop"
        aria-label="Navigazione"
      >
        <!--
          Da desktop l'hamburger sta qui, nella prima riga della sidebar,
          alta quanto la topbar: la sidebar è una colonna a sé e non sta più
          sotto la topbar, quindi il bottone che la apre e chiude le
          appartiene. Da aperta dice anche "Menù"; da chiusa resta l'icona,
          allineata alle altre della colonna.
        -->
        <div v-if="isDesktop" class="app-sidebar-head">
          <button
            type="button"
            class="app-sidebar-toggle"
            :aria-expanded="open"
            aria-controls="app-sidebar"
            :aria-label="open ? 'Riduci il menù' : 'Espandi il menù'"
            :title="open ? undefined : 'Espandi il menù'"
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
            <span v-if="open" aria-hidden="true">Menù</span>
          </button>
        </div>
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
                :title="isDesktop && !open ? item.label : undefined"
                :style="{ '--pedina': `var(--${item.pedina})` }"
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
                <!-- Nella colonna di icone il testo è nascosto a vista ma
                     resta il nome del link per chi usa uno screen reader. -->
                <span class="app-sidebar-label">{{ item.label }}</span>
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
        <!-- Come il piè di pagina del blog: il manager non è un vicolo cieco,
             da qui si torna al blog e a Instagram. -->
        <footer class="app-footer">
          <a href="https://rootncool.org">Il blog di Root'n'Cool</a>
          <a href="https://www.instagram.com/rootncool/">Instagram</a>
          <router-link to="/terms">Termini e condizioni</router-link>
          <router-link to="/privacy">Privacy</router-link>
        </footer>
      </main>
    </div>
  </div>
</template>
