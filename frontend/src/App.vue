<script setup>
import { ref, onMounted, watch } from 'vue'
import WebhookManager from './components/WebhookManager.vue'
import ProfileDashboard from './components/ProfileDashboard.vue'

const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)
const authenticated = ref(false)
const userEmail = ref("")
const showRegister = ref(false)
const regSuccess = ref(false)

// WhatsApp connection state
const waStatus = ref('')
const waQR = ref('')
const waLoginState = ref('')
const waLoading = ref(false)
let waPollInterval = null

const showDebug = ref(false)

async function checkSession() {
  const res = await fetch('/api/session')
  const data = await res.json()
  authenticated.value = !!data.authenticated
  if (authenticated.value) {
    userEmail.value = data.email || ''
    fetchWAStatus()
  }
}

async function login() {
  error.value = ''
  loading.value = true
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email.value, password: password.value })
    })
    if (res.ok) {
      authenticated.value = true
      userEmail.value = email.value
      email.value = ''
      password.value = ''
      fetchWAStatus()
    } else {
      const data = await res.text()
      error.value = data || 'Login failed'
    }
  } catch (e) {
    error.value = 'Network error'
  }
  loading.value = false
}

async function register() {
  error.value = ''
  loading.value = true
  try {
    const res = await fetch('/api/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email.value, password: password.value })
    })
    if (res.ok) {
      regSuccess.value = true
      showRegister.value = false
      error.value = ''
      email.value = ''
      password.value = ''
    } else {
      const data = await res.text()
      error.value = data || 'Registration failed'
    }
  } catch (e) {
    error.value = 'Network error'
  }
  loading.value = false
}

async function logout() {
  await fetch('/api/logout', { method: 'POST' })
  authenticated.value = false
  userEmail.value = ''
  stopWAPoll()
}

async function fetchWAStatus() {
  if (!authenticated.value) return
  try {
    const res = await fetch('/api/wa/status')
    if (res.ok) {
      const data = await res.json()
      waStatus.value = data.status || ''
      waQR.value = data.qr || ''
      waLoginState.value = data.loginState || ''
    } else {
      waStatus.value = 'error'
      waLoginState.value = 'Failed to fetch status'
    }
  } catch (e) {
    waStatus.value = 'error'
    waLoginState.value = 'Network error'
  }
}

function startWAPoll() {
  stopWAPoll()
  waPollInterval = setInterval(fetchWAStatus, 2000)
}

function stopWAPoll() {
  if (waPollInterval) clearInterval(waPollInterval)
  waPollInterval = null
}

async function connectWA() {
  waLoading.value = true
  await fetch('/api/wa/connect', { method: 'POST' })
  waLoading.value = false
  startWAPoll()
}

async function disconnectWA() {
  waLoading.value = true
  await fetch('/api/wa/disconnect', { method: 'POST' })
  waLoading.value = false
  fetchWAStatus()
  stopWAPoll()
}

watch(authenticated, (val) => {
  if (val) {
    fetchWAStatus()
  } else {
    stopWAPoll()
  }
})

onMounted(() => {
  checkSession()
})
</script>

<template>
  <div class="min-h-screen w-full bg-gray-50">
    <div v-if="!authenticated" class="flex min-h-screen flex-col justify-center">
      <div class="sm:mx-auto sm:w-full sm:max-w-md px-4">
        <h2 class="text-center text-2xl font-bold leading-9 tracking-tight text-gray-900">
          {{ !showRegister ? 'Sign in to your account' : 'Create a new account' }}
        </h2>
      </div>

      <div class="mt-10 sm:mx-auto sm:w-full sm:max-w-[480px] px-6">
        <div class="bg-white px-6 py-12 shadow sm:rounded-lg sm:px-12">
          <form v-if="!showRegister" @submit.prevent="login" class="space-y-6">
            <div>
              <label for="email" class="block text-sm font-medium leading-6 text-gray-900">Email address</label>
              <div class="mt-2">
                <input v-model="email" id="email" name="email" type="email" autocomplete="email" required
                  class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6 px-3" />
              </div>
            </div>

            <div>
              <label for="password" class="block text-sm font-medium leading-6 text-gray-900">Password</label>
              <div class="mt-2">
                <input v-model="password" id="password" name="password" type="password" autocomplete="current-password" required
                  class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6 px-3" />
              </div>
            </div>

            <div>
              <button type="submit" :disabled="loading"
                class="flex w-full justify-center rounded-md bg-primary-600 px-3 py-1.5 text-sm font-semibold leading-6 text-white shadow-sm hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 disabled:opacity-50 disabled:cursor-not-allowed">
                {{ loading ? 'Signing in...' : 'Sign in' }}
              </button>
            </div>
          </form>

          <form v-else @submit.prevent="register" class="space-y-6">
            <div>
              <label for="reg-email" class="block text-sm font-medium leading-6 text-gray-900">Email address</label>
              <div class="mt-2">
                <input v-model="email" id="reg-email" name="email" type="email" autocomplete="email" required
                  class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6 px-3" />
              </div>
            </div>

            <div>
              <label for="reg-password" class="block text-sm font-medium leading-6 text-gray-900">Password</label>
              <div class="mt-2">
                <input v-model="password" id="reg-password" name="password" type="password" autocomplete="new-password" required
                  class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6 px-3" />
              </div>
            </div>

            <div>
              <button type="submit" :disabled="loading"
                class="flex w-full justify-center rounded-md bg-primary-600 px-3 py-1.5 text-sm font-semibold leading-6 text-white shadow-sm hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 disabled:opacity-50 disabled:cursor-not-allowed">
                {{ loading ? 'Creating account...' : 'Create account' }}
              </button>
            </div>
          </form>

          <div v-if="error" class="mt-4 text-center text-sm text-red-600">{{ error }}</div>
          <div v-if="regSuccess" class="mt-4 text-center text-sm text-primary-600">Registration successful! Please log in.</div>
        </div>

        <p class="mt-10 text-center text-sm text-gray-500">
          {{ !showRegister ? 'Not a member?' : 'Already have an account?' }}
          <a href="#" @click.prevent="showRegister = !showRegister; error = ''"
            class="font-semibold leading-6 text-primary-600 hover:text-primary-500">
            {{ !showRegister ? 'Create an account' : 'Sign in' }}
          </a>
        </p>
      </div>
    </div>
    <ProfileDashboard v-else :email="userEmail" @logout="logout" />
  </div>
</template>