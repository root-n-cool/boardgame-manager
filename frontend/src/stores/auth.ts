import { defineStore } from 'pinia'
import { api } from '../api/client'

interface CurrentUser {
  id: number
  email: string
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as CurrentUser | null,
    needsSetup: false,
    checked: false,
  }),
  actions: {
    async checkStatus() {
      try {
        const status = await api.get<{ needsSetup: boolean }>('/bootstrap/status')
        this.needsSetup = status.needsSetup
      } catch (e) {
        // The backend being unreachable must not leave the router guard's
        // promise rejected — that renders a blank page with only a console
        // error. Fall back to "set up, nobody signed in" so the guard sends
        // the visitor to the login page, where the failure is at least
        // visible as a form error when they try to sign in.
        console.error('could not read bootstrap status', e)
        this.needsSetup = false
        this.user = null
        this.checked = true
        return
      }

      if (!this.needsSetup) {
        // A 401 here is the ordinary "not signed in" answer, not a failure.
        try {
          this.user = await api.get<CurrentUser>('/me')
        } catch {
          this.user = null
        }
      }
      this.checked = true
    },
    async bootstrap(email: string, password: string) {
      this.user = await api.post<CurrentUser>('/bootstrap', { email, password })
      this.needsSetup = false
    },
    async login(email: string, password: string) {
      this.user = await api.post<CurrentUser>('/login', { email, password })
    },
    async logout() {
      try {
        await api.post('/logout')
      } finally {
        // Drop the local session even if the server call failed, so the
        // "Esci" button can never leave the UI stuck in a signed-in state.
        this.user = null
      }
    },
  },
})
