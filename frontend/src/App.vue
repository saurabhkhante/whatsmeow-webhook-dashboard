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
  <div class="w-full min-h-screen">
    <ProfileDashboard v-if="authenticated" :email="userEmail" @logout="logout" />
  </div>
</template>