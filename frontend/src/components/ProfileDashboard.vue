<template>
  <div class="min-h-screen w-full">
    <!-- Top Navigation Bar -->
    <nav class="bg-white shadow w-full">
      <div class="w-full px-4 sm:px-6 lg:px-8">
        <div class="flex h-16 justify-between">
          <div class="flex items-center">
            <div class="flex-shrink-0">
              <div class="text-2xl font-bold text-primary-600">WhatsApp Dashboard</div>
            </div>
          </div>
          <div class="flex items-center gap-4">
            <div class="text-sm text-gray-500">{{ email }}</div>
            <button @click="$emit('logout')"
              class="rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50">
              Logout
            </button>
          </div>
        </div>
      </div>
    </nav>

    <main class="w-full px-4 sm:px-6 lg:px-8 py-6">
      <!-- Rest of the component remains the same -->
      <!-- WhatsApp Connection Status -->
      <div class="mb-6 rounded-lg bg-white p-6 shadow">
        <!-- ... existing WhatsApp status section ... -->
      </div>

      <!-- Webhooks Section -->
      <div class="rounded-lg bg-white p-6 shadow">
        <!-- ... existing webhooks section ... -->
      </div>
    </main>

    <!-- Recent Chats Modal -->
    <!-- ... existing modal section ... -->
  </div>
</template>

<script>
// ... existing script section ...
</script>