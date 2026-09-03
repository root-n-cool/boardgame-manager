<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'

/**
 * La mappina del luogo di un evento. Vive solo quando ci sono le
 * coordinate: un indirizzo scritto a mano resta una riga di testo.
 *
 * Lo zoom con la rotella e con due dita è spento di proposito. Questa
 * pagina si legge in piedi al tavolo, col pollice: una mappa che cattura
 * lo scorrimento è una pagina che si blocca a metà. Chi vuole muoversi
 * davvero apre la mappa vera con il link sotto.
 */
const props = defineProps<{
  lat: number
  lon: number
  label: string
}>()

const ZOOM = 16

const container = ref<HTMLDivElement | null>(null)
const titleId = useId()
let map: L.Map | undefined
let marker: L.Marker | undefined

// Il puntino è un divIcon: l'icona di serie di Leaflet è un PNG con percorsi
// relativi che il bundle non risolve, e un cerchio in CSS eredita i colori
// del tema invece di importarne di suoi.
const pinIcon = L.divIcon({
  className: 'event-map-pin',
  html: '<span class="event-map-pin-dot"></span>',
  iconSize: [22, 22],
  iconAnchor: [11, 11],
})

function draw() {
  if (!container.value) {
    return
  }
  // Chi ha chiesto meno movimento non vuole la mappa che sfuma e scivola a
  // ogni zoom: resta la mappa, sparisce l'animazione.
  const calmly = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  map = L.map(container.value, {
    center: [props.lat, props.lon],
    zoom: ZOOM,
    scrollWheelZoom: false,
    // Trascinare con un dito deve scorrere la pagina, non la mappa; con due
    // dita la mappa si muove, ed è il gesto che chi vuole guardarla fa.
    dragging: !L.Browser.mobile,
    keyboard: false,
    // I controlli di serie sono in inglese: qui l'interfaccia è in italiano.
    zoomControl: false,
    zoomAnimation: !calmly,
    fadeAnimation: !calmly,
    markerZoomAnimation: !calmly,
  })
  L.control.zoom({ zoomInTitle: 'Ingrandisci', zoomOutTitle: 'Riduci' }).addTo(map)
  L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  }).addTo(map)
  marker = L.marker([props.lat, props.lon], { icon: pinIcon, keyboard: false, alt: props.label })
  marker.addTo(map)
}

watch(
  () => [props.lat, props.lon],
  ([lat, lon]) => {
    map?.setView([lat, lon], ZOOM)
    marker?.setLatLng([lat, lon])
  },
)

onMounted(draw)
onBeforeUnmount(() => {
  map?.remove()
  map = undefined
  marker = undefined
})
</script>

<template>
  <figure class="event-map">
    <!-- Niente role="img": dentro la mappa restano i bottoni dello zoom e il
         link di attribuzione, che con quel ruolo sparirebbero dallo screen
         reader pur restando raggiungibili col tab. Il gruppo è etichettato
         dalla didascalia, che dice di che posto è la mappa. -->
    <div ref="container" class="event-map-canvas" role="group" :aria-labelledby="titleId"></div>
    <!-- Il nome del posto sta già sopra la mappa: qui resta per chi non la
         vede, mentre a schermo la didascalia porta solo la via d'uscita. -->
    <figcaption :id="titleId" class="event-map-caption">
      <span class="visually-hidden">Mappa di {{ label }}</span>
      <a
        class="event-map-link"
        :href="`https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=17/${lat}/${lon}`"
        target="_blank"
        rel="noopener noreferrer"
      >
        Apri in mappe
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M14 4h6v6" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          <path d="M20 4 11 13" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
          <path
            d="M18 14.5V19a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h4.5"
            stroke="currentColor"
            stroke-width="1.7"
            stroke-linecap="round"
          />
        </svg>
      </a>
    </figcaption>
  </figure>
</template>
