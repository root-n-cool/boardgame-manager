import { defineStore } from 'pinia'
import { api } from '../api/client'

interface SiteResponse {
  siteTitle: string
  logoFilename: string
}

// Letto senza autenticazione: l'header mostra marchio e titolo anche su
// /login, /setup e le pagine pubbliche, prima che una sessione esista.
export const useSiteStore = defineStore('site', {
  state: () => ({
    siteTitle: 'BoardGames Manager',
    logoFilename: '',
    loaded: false,
  }),
  actions: {
    async load() {
      try {
        const site = await api.get<SiteResponse>('/site', { skipAuthRedirect: true })
        this.siteTitle = site.siteTitle
        this.logoFilename = site.logoFilename
      } catch (e) {
        // Il backend irraggiungibile non deve rompere l'header: restano i
        // valori di default già nello stato iniziale.
        console.error('could not load site branding', e)
      } finally {
        this.loaded = true
      }
    },
  },
})
