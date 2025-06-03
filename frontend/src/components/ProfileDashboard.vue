<template>
  <div class="min-h-screen w-full">
    <!-- Header -->
    <header class="w-full bg-white shadow">
      <div class="w-full px-4 py-4 flex justify-between items-center">
        <h1 class="text-2xl font-bold text-gray-800">WhatsApp Dashboard</h1>
        <div class="flex items-center gap-4">
          <span class="text-gray-600">{{ email }}</span>
          <button @click="$emit('logout')" class="bg-red-500 text-white px-4 py-2 rounded hover:bg-red-600">
            Logout
          </button>
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <main class="w-full p-4">
      <!-- WhatsApp Connection Status -->
      <div class="w-full bg-white rounded-lg shadow-md p-6 mb-6">
        <div class="wa-section text-center">
          <div v-if="waStatus === 'waiting_qr'" class="qr-area">
            <img v-if="waQR" :src="'/qr.png?'+Date.now()" alt="QR Code" class="qr-img mx-auto" />
            <div class="wa-status">{{ waStatusMessage() }}</div>
            <button @click="disconnectWA" :disabled="waLoading" class="wa-btn wa-btn-secondary mt-4">
              Cancel
            </button>
          </div>
          <div v-else-if="waStatus === 'connected'" class="text-center">
            <div class="wa-status success text-xl font-bold text-green-600 mb-4">
              {{ waStatusMessage() }}
            </div>
            <button @click="disconnectWA" :disabled="waLoading" class="wa-btn wa-btn-danger">
              Disconnect WhatsApp
            </button>
          </div>
          <div v-else class="text-center">
            <div class="wa-status text-lg text-gray-700 mb-4">
              {{ waStatusMessage() }}
            </div>
            <button 
              v-if="waStatus === 'disconnected' || !waStatus" 
              @click="connectWA" 
              :disabled="waLoading"
              class="wa-btn wa-btn-primary"
            >
              Connect WhatsApp
            </button>
          </div>
        </div>
      </div>

      <!-- Webhooks Section -->
      <div class="w-full bg-white rounded-lg shadow-md p-6">
        <div class="webhooks-header mb-6">
          <h2 class="text-xl font-bold text-gray-800">Your Webhooks</h2>
          <form @submit.prevent="createWebhook" class="mt-4 space-y-4">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium text-gray-700">URL:</label>
                <input 
                  v-model="newURL" 
                  type="url" 
                  placeholder="https://your-endpoint.com/webhook" 
                  required
                  class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2"
                />
              </div>
              <div class="grid grid-cols-2 gap-4">
                <div>
                  <label class="block text-sm font-medium text-gray-700">Method:</label>
                  <select 
                    v-model="newMethod"
                    class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2"
                  >
                    <option value="POST">POST</option>
                    <option value="GET">GET</option>
                  </select>
                </div>
                <div>
                  <label class="block text-sm font-medium text-gray-700">Filter:</label>
                  <select 
                    v-model="newFilterType"
                    class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2"
                  >
                    <option value="all">All Messages</option>
                    <option value="group">Specific Group</option>
                    <option value="chat">Specific Chat</option>
                  </select>
                </div>
              </div>
            </div>
            <button type="submit" class="wa-btn wa-btn-primary">Create Webhook</button>
          </form>
        </div>

        <div v-if="loading" class="text-gray-600">Loading webhooks...</div>
        <div v-else-if="webhooks.length === 0" class="text-gray-500">No webhooks yet.</div>
        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <div v-for="wh in webhooks" :key="wh.id" class="webhook-card">
            <!-- Webhook card content -->
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script>
export default {
  name: "ProfileDashboard",
  props: ["email"],
  data() {
    return {
      webhooks: [],
      loading: true,
      error: "",
      newURL: "",
      newMethod: "POST",
      newFilterType: "all",
      newFilterValue: "",
      waStatus: "",
      waQR: "",
      waLoginState: "",
      waLoading: false
    }
  },
  mounted() {
    this.fetchWebhooks()
    this.fetchWAStatus()
  },
  methods: {
    // ... existing methods ...
  }
}
</script>

<style scoped>
.wa-btn {
  @apply px-4 py-2 rounded-md font-medium transition-colors duration-200;
}

.wa-btn-primary {
  @apply bg-green-600 text-white hover:bg-green-700;
}

.wa-btn-secondary {
  @apply bg-gray-200 text-gray-700 hover:bg-gray-300;
}

.wa-btn-danger {
  @apply bg-red-500 text-white hover:bg-red-600;
}

.qr-img {
  @apply w-64 h-64 bg-white p-4 rounded-lg shadow-md;
}

.webhook-card {
  @apply bg-white rounded-lg shadow-md p-4;
}
</style>