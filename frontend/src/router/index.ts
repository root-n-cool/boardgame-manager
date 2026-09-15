import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import UsersView from '../views/UsersView.vue'
import SettingsView from '../views/SettingsView.vue'
import GamesView from '../views/GamesView.vue'
import GameNewView from '../views/GameNewView.vue'
import GameDetailView from '../views/GameDetailView.vue'
import GameAdminDetailView from '../views/GameAdminDetailView.vue'
import GameLeaderboardView from '../views/GameLeaderboardView.vue'
import GameLoansView from '../views/GameLoansView.vue'
import EventsView from '../views/EventsView.vue'
import EventDetailView from '../views/EventDetailView.vue'
import ManageBookingView from '../views/ManageBookingView.vue'
import InviteAcceptView from '../views/InviteAcceptView.vue'
import EventsAdminView from '../views/EventsAdminView.vue'
import EventNewView from '../views/EventNewView.vue'
import EventAdminDetailView from '../views/EventAdminDetailView.vue'
import LoanDeskView from '../views/LoanDeskView.vue'
import LegalPageView from '../views/LegalPageView.vue'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    /** Pagina senza shell: nessuna topbar, nessuna sidebar. */
    bare?: boolean
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView, meta: { bare: true } },
    { path: '/login', name: 'login', component: LoginView, meta: { bare: true } },
    { path: '/', name: 'events', component: EventsView, meta: { public: true } },
    { path: '/events/:id', name: 'event-detail', component: EventDetailView, meta: { public: true } },
    { path: '/manage-booking', name: 'manage-booking', component: ManageBookingView, meta: { public: true } },
    {
      path: '/terms',
      name: 'terms',
      component: LegalPageView,
      props: { title: 'Termini e condizioni', apiPath: '/legal/terms' },
      meta: { public: true },
    },
    {
      path: '/privacy',
      name: 'privacy',
      component: LegalPageView,
      props: { title: 'Privacy', apiPath: '/legal/privacy' },
      meta: { public: true },
    },
    // I due link che partono nella mail di conferma. Stesso componente di
    // /manage-booking: la pagina sa già fare entrambe le cose, e il path
    // decide solo se il codice arriva dall'indirizzo o dal form.
    // `props` è una funzione perché il codice viene dai params mentre
    // `mode` è fisso per rotta.
    {
      path: '/prenotazione/:code',
      name: 'booking-manage',
      component: ManageBookingView,
      props: (route) => ({ code: String(route.params.code), mode: 'manage' }),
      meta: { public: true },
    },
    {
      path: '/prenotazione/:code/punteggio',
      name: 'booking-score',
      component: ManageBookingView,
      props: (route) => ({ code: String(route.params.code), mode: 'score' }),
      meta: { public: true },
    },
    {
      path: '/invito/:token',
      name: 'invite-accept',
      component: InviteAcceptView,
      meta: { public: true, bare: true },
    },
    { path: '/games/:id', name: 'game-detail', component: GameDetailView, meta: { public: true } },
    { path: '/games/:id/leaderboard', name: 'game-leaderboard', component: GameLeaderboardView, meta: { public: true } },
    // L'area di gestione non ha più un layout proprio (la shell è unica per
    // tutta l'app), quindi le rotte stanno piatte invece che annidate.
    { path: '/admin', redirect: { name: 'admin-events' } },
    { path: '/admin/events', name: 'admin-events', component: EventsAdminView },
    { path: '/admin/events/new', name: 'admin-event-new', component: EventNewView },
    { path: '/admin/events/:id', name: 'admin-event-detail', component: EventAdminDetailView },
    { path: '/admin/events/:id/prestiti', name: 'admin-event-loans', component: LoanDeskView },
    { path: '/admin/games', name: 'admin-games', component: GamesView },
    { path: '/admin/games/new', name: 'admin-game-new', component: GameNewView },
    { path: '/admin/games/:id', name: 'admin-game-detail', component: GameAdminDetailView },
    { path: '/admin/games/:id/prestiti', name: 'admin-game-loans', component: GameLoansView },
    { path: '/admin/users', name: 'admin-users', component: UsersView },
    { path: '/admin/settings', name: 'admin-settings', component: SettingsView },
    // Le pagine di gestione stavano sulla root prima di finire sotto /admin:
    // i vecchi indirizzi restano validi per i segnalibri di chi organizza.
    // Attenzione all'ordine: /games/new e /games/:id sono path distinti, il
    // redirect di /games non intercetta la scheda pubblica.
    { path: '/games', redirect: { name: 'admin-games' } },
    { path: '/games/new', redirect: { name: 'admin-game-new' } },
    { path: '/users', redirect: { name: 'admin-users' } },
    { path: '/settings', redirect: { name: 'admin-settings' } },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.checked) {
    // checkStatus already swallows a backend outage, but a rejected guard
    // promise means a blank page, so never let one escape from here.
    try {
      await auth.checkStatus()
    } catch (e) {
      console.error('auth status check failed', e)
    }
  }

  if (auth.needsSetup && to.name !== 'setup') {
    return { name: 'setup' }
  }
  if (!auth.needsSetup && to.name === 'setup') {
    return { name: 'admin-events' }
  }
  if (to.meta.public) {
    return true
  }
  if (!auth.needsSetup && !auth.user && to.name !== 'login') {
    return { name: 'login' }
  }
  if (auth.user && to.name === 'login') {
    return { name: 'admin-events' }
  }
  return true
})

export default router
