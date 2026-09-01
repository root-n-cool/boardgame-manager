import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import DashboardLayout from '../views/DashboardLayout.vue'
import UsersView from '../views/UsersView.vue'
import SettingsView from '../views/SettingsView.vue'
import GamesView from '../views/GamesView.vue'
import GameNewView from '../views/GameNewView.vue'
import GameDetailView from '../views/GameDetailView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/',
      component: DashboardLayout,
      children: [
        { path: '', redirect: '/users' },
        { path: 'users', name: 'users', component: UsersView },
        { path: 'games', name: 'games', component: GamesView },
        { path: 'games/new', name: 'game-new', component: GameNewView },
        { path: 'games/:id', name: 'game-detail', component: GameDetailView },
        { path: 'settings', name: 'settings', component: SettingsView },
      ],
    },
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
    return { name: 'users' }
  }
  if (!auth.needsSetup && !auth.user && to.name !== 'login') {
    return { name: 'login' }
  }
  if (auth.user && to.name === 'login') {
    return { name: 'users' }
  }
  return true
})

export default router
