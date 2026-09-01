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
      const status = await api.get<{ needsSetup: boolean }>('/bootstrap/status')
      this.needsSetup = status.needsSetup
      if (!this.needsSetup) {
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
      await api.post('/logout')
      this.user = null
    },
  },
})
