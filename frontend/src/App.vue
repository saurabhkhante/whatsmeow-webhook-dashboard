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

function statusMessage() {
  if (waStatus.value === 'waiting_qr') return 'Scan this QR code with WhatsApp to connect.'
  if (waStatus.value === 'connected') return 'WhatsApp Connected!'
  if (waStatus.value === 'disconnected' || !waStatus.value) return 'Not connected.'
  if (waStatus.value === 'error') return waLoginState.value || 'An error occurred.'
  return waLoginState.value || waStatus.value
}
</script>

<template>
  <div class="min-h-screen w-full bg-gray-100">
    <div v-if="!authenticated" class="login-container">
      <h2 v-if="!showRegister">Login</h2>
      <h2 v-else>Register</h2>
      <form v-if="!showRegister" @submit.prevent="login">
        <input v-model="email" type="email" placeholder="Email" required />
        <input v-model="password" type="password" placeholder="Password" required />
        <button type="submit" :disabled="loading">Login</button>
        <div v-if="error" class="error">{{ error }}</div>
        <div class="hint">Don't have an account? <a href="#" @click.prevent="showRegister = true; error = ''">Register</a></div>
      </form>
      <form v-else @submit.prevent="register">
        <input v-model="email" type="email" placeholder="Email" required />
        <input v-model="password" type="password" placeholder="Password" required />
        <button type="submit" :disabled="loading">Register</button>
        <div v-if="error" class="error">{{ error }}</div>
        <div v-if="regSuccess" class="success">Registration successful! Please log in.</div>
        <div class="hint">Already have an account? <a href="#" @click.prevent="showRegister = false; error = ''">Login</a></div>
      </form>
    </div>
    <ProfileDashboard v-else :email="userEmail" @logout="logout" />
  </div>
</template>

<style>
.login-container {
  max-width: 350px;
  margin: 80px auto;
  padding: 2rem;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 2px 16px rgba(0,0,0,0.10);
  text-align: center;
}

.login-container input {
  display: block;
  width: 100%;
  margin: 1rem 0;
  padding: 0.5rem;
  font-size: 1rem;
  border: 1px solid #ddd;
  border-radius: 4px;
}

.login-container button {
  width: 100%;
  padding: 0.5rem;
  font-size: 1rem;
  background: #42b983;
  color: #fff;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.login-container button:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.login-container .error {
  color: #c00;
  margin-top: 1rem;
}

.login-container .success {
  color: #43a047;
  margin-top: 1rem;
}

.hint {
  margin-top: 1rem;
  color: #888;
  font-size: 0.95em;
}

.hint a {
  color: #42b983;
  text-decoration: none;
}

.hint a:hover {
  text-decoration: underline;
}
</style>