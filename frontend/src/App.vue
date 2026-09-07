<script setup lang="ts">
import { useRoute } from 'vue-router'
import AppShell from './components/AppShell.vue'

const route = useRoute()
</script>

<template>
  <!-- Setup, login e accettazione invito sono pagine a schermo pieno sul
       feltro: la shell (topbar + sidebar) non le riguarda. -->
  <router-view v-if="route.meta.bare" />
  <AppShell v-else>
    <router-view />
  </AppShell>
</template>
