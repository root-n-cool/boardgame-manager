<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface AdminUser {
  id: number
  email: string
  createdAt: string
}

const users = ref<AdminUser[]>([])
const newEmail = ref('')
const newPassword = ref('')
const error = ref('')

async function loadUsers() {
  users.value = await api.get<AdminUser[]>('/users')
}

async function addUser() {
  error.value = ''
  try {
    await api.post('/users', { email: newEmail.value, password: newPassword.value })
    newEmail.value = ''
    newPassword.value = ''
    await loadUsers()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function removeUser(id: number) {
  error.value = ''
  try {
    await api.delete(`/users/${id}`)
    await loadUsers()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadUsers)
</script>

<template>
  <div>
    <h1>Utenti</h1>
    <ul>
      <li v-for="u in users" :key="u.id">
        {{ u.email }}
        <button @click="removeUser(u.id)">Rimuovi</button>
      </li>
    </ul>

    <h2>Aggiungi amministratore</h2>
    <form @submit.prevent="addUser">
      <label>
        Email
        <input v-model="newEmail" type="email" required />
      </label>
      <label>
        Password
        <input v-model="newPassword" type="password" required minlength="8" />
      </label>
      <button type="submit">Aggiungi</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
