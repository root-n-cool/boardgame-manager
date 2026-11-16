import { onBeforeUnmount, ref } from 'vue'

/**
 * Vero sotto i 560px, la soglia "telefono" delle liste di giochi (la stessa
 * delle media query in app.css): lì i filtri a tendina passano nel foglio
 * dal basso. Segue la rotazione e il ridimensionamento della finestra.
 */
export function usePhone() {
  const media = window.matchMedia('(max-width: 560px)')
  const isPhone = ref(media.matches)
  const onChange = (e: MediaQueryListEvent) => {
    isPhone.value = e.matches
  }
  media.addEventListener('change', onChange)
  onBeforeUnmount(() => media.removeEventListener('change', onChange))
  return isPhone
}
