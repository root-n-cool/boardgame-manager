import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import DashboardLayout from '../views/DashboardLayout.vue'
import UsersView from '../views/UsersView.vue'
import SettingsView from '../views/SettingsView.vue'

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
        { path: 'settings', name: 'settings', component: SettingsView },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.checked) {
    await auth.checkStatus()
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
