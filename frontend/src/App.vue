<script setup>
import { ref } from 'vue'
import ProfileDashboard from './components/ProfileDashboard.vue'

// Demo mode - automatically "logged in"
const authenticated = ref(true)
const userEmail = ref("demo@example.com")

function logout() {
  authenticated.value = false
}
</script>

<template>
  <ProfileDashboard v-if="authenticated" :email="userEmail" @logout="logout" />
</template>