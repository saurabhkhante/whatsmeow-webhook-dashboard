<template>
  <div class="min-h-screen w-full">
    <!-- Top Navigation Bar -->
    <nav class="bg-white shadow">
      <div class="mx-auto max-w-full px-4 sm:px-6 lg:px-8">
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

    <main class="mx-auto max-w-full px-4 sm:px-6 lg:px-8 py-6">
      <!-- WhatsApp Connection Status -->
      <div class="mb-6 rounded-lg bg-white p-6 shadow">
        <div class="text-center">
          <div v-if="waStatus === 'waiting_qr'">
            <div class="mx-auto max-w-sm rounded-lg bg-gray-50 p-6">
              <img v-if="waQR" :src="'/qr.png?'+Date.now()" alt="QR Code" class="mx-auto h-64 w-64" />
              <div class="mt-4 text-sm text-gray-600">{{ waStatusMessage() }}</div>
              <button @click="disconnectWA" :disabled="waLoading"
                class="mt-4 rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50">
                Cancel
              </button>
            </div>
          </div>
          <div v-else-if="waStatus === 'connected'" class="space-y-4">
            <div class="text-lg font-medium text-primary-600">{{ waStatusMessage() }}</div>
            <button @click="disconnectWA" :disabled="waLoading"
              class="rounded-md bg-red-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-red-500">
              Disconnect WhatsApp
            </button>
          </div>
          <div v-else class="space-y-4">
            <div class="text-sm text-gray-600">{{ waStatusMessage() }}</div>
            <button v-if="waStatus === 'disconnected' || !waStatus" @click="connectWA" :disabled="waLoading"
              class="rounded-md bg-primary-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-500">
              Connect WhatsApp
            </button>
          </div>
        </div>
      </div>

      <!-- Webhooks Section -->
      <div class="rounded-lg bg-white p-6 shadow">
        <div class="mb-6 flex items-center justify-between">
          <h2 class="text-lg font-medium text-gray-900">Your Webhooks</h2>
        </div>

        <form @submit.prevent="createWebhook" class="mb-8 space-y-4">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label class="block text-sm font-medium text-gray-700">URL</label>
              <input v-model="newURL" type="url" placeholder="https://your-endpoint.com/webhook" required
                class="mt-1 block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6 px-3" />
            </div>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium text-gray-700">Method</label>
                <select v-model="newMethod"
                  class="mt-1 block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6">
                  <option value="POST">POST</option>
                  <option value="GET">GET</option>
                </select>
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700">Filter</label>
                <select v-model="newFilterType" @change="onFilterTypeChange"
                  class="mt-1 block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6">
                  <option value="all">All Messages</option>
                  <option value="group">Specific Group</option>
                  <option value="chat">Specific Chat</option>
                </select>
              </div>
            </div>
          </div>

          <div v-if="newFilterType !== 'all'" class="flex gap-4">
            <div class="flex-1">
              <label class="block text-sm font-medium text-gray-700">{{ newFilterType === 'group' ? 'Group' : 'Chat' }}
                ID</label>
              <input v-model="newFilterValue" type="text" :placeholder="getFilterPlaceholder()"
                class="mt-1 block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6 px-3" />
            </div>
            <div class="flex items-end">
              <button type="button" @click="showRecentChats"
                class="rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50">
                Browse {{ newFilterType === 'group' ? 'Groups' : 'Chats' }}
              </button>
            </div>
          </div>

          <div class="flex justify-end">
            <button type="submit"
              class="rounded-md bg-primary-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-500">
              Create Webhook
            </button>
          </div>
        </form>

        <div v-if="loading" class="text-sm text-gray-500">Loading webhooks...</div>
        <div v-else-if="webhooks.length === 0" class="text-sm text-gray-500">No webhooks yet.</div>
        <div v-else class="space-y-6">
          <div v-for="wh in webhooks" :key="wh.id"
            class="relative rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
            <div class="space-y-3">
              <div class="text-sm text-gray-500">ID: <span class="font-mono text-gray-900">{{ wh.id }}</span></div>
              <div class="text-sm text-gray-500">Method: <span class="font-mono text-gray-900">{{ wh.method }}</span></div>
              <div class="text-sm text-gray-500">
                URL:
                <div class="mt-1 flex items-center gap-2">
                  <code class="block rounded bg-gray-50 p-2 text-xs font-mono break-all">{{ wh.url || fullWebhookUrl(wh.id)
                  }}</code>
                  <button @click="copyUrl(wh.id)"
                    class="rounded-md bg-white px-2 py-1 text-xs font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50">
                    Copy
                  </button>
                </div>
              </div>
              <div class="text-sm text-gray-500">
                Filter: <span class="font-mono text-gray-900">{{ getFilterDisplayText(wh) }}</span>
              </div>
            </div>

            <div class="mt-4">
              <div class="text-xs text-gray-500">Recent Messages</div>
              <div class="mt-2 space-y-2">
                <div v-if="logs[wh.id] && logs[wh.id].length > 0">
                  <div v-for="log in logs[wh.id]" :key="log.timestamp" class="rounded-md bg-gray-50 p-3">
                    <div class="text-xs text-primary-600">{{ formatTime(log.timestamp) }}</div>
                    <pre class="mt-1 overflow-x-auto text-xs text-gray-600 break-all">{{ log.payload }}</pre>
                  </div>
                </div>
                <div v-else class="text-xs text-gray-400">No messages yet.</div>
              </div>
            </div>

            <button @click="deleteWebhook(wh.id)"
              class="absolute right-4 top-4 rounded-md bg-white px-2 py-1 text-xs font-semibold text-red-600 shadow-sm ring-1 ring-inset ring-red-300 hover:bg-red-50">
              Delete
            </button>
          </div>
        </div>
      </div>
    </main>

    <!-- Recent Chats Modal -->
    <div v-if="showChatsModal" class="fixed inset-0 z-10 overflow-y-auto" @click="closeChatsModal">
      <div class="flex min-h-screen items-end justify-center px-4 pb-20 pt-4 text-center sm:block sm:p-0">
        <div class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"></div>
        <div class="inline-block transform overflow-hidden rounded-lg bg-white text-left align-bottom shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-lg sm:align-middle"
          @click.stop>
          <div class="bg-white px-4 pb-4 pt-5 sm:p-6 sm:pb-4">
            <div class="sm:flex sm:items-start">
              <div class="mt-3 w-full text-center sm:ml-4 sm:mt-0 sm:text-left">
                <h3 class="text-base font-semibold leading-6 text-gray-900">
                  Recent {{ newFilterType === 'group' ? 'Groups' : 'Chats' }}
                </h3>
                <div class="mt-4">
                  <div v-if="chatsLoading" class="text-sm text-gray-500">
                    Loading {{ newFilterType === 'group' ? 'groups' : 'chats' }}...
                  </div>
                  <div v-else-if="chatsError" class="text-sm text-red-600">{{ chatsError }}</div>
                  <div v-else-if="filteredChats.length === 0" class="text-sm text-gray-500">
                    No recent {{ newFilterType === 'group' ? 'groups' : 'chats' }} found.
                  </div>
                  <div v-else class="mt-2 space-y-2">
                    <button v-for="chat in filteredChats" :key="chat.id" @click="selectChat(chat)"
                      class="flex w-full items-center gap-3 rounded-lg border border-gray-200 p-3 text-left hover:bg-gray-50">
                      <span class="text-2xl">{{ chat.type === 'group' ? '👥' : '👤' }}</span>
                      <div>
                        <div class="font-medium text-gray-900">{{ chat.name }}</div>
                        <div class="text-xs font-mono text-gray-500">{{ chat.id }}</div>
                      </div>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="bg-gray-50 px-4 py-3 sm:flex sm:flex-row-reverse sm:px-6">
            <button type="button" @click="closeChatsModal"
              class="mt-3 inline-flex w-full justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 sm:mt-0 sm:w-auto">
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: "ProfileDashboard",
  props: ["email"],
  data() {
    return {
      webhooks: [
        {
          id: "demo1",
          method: "POST",
          url: "https://example.com/webhook1",
          filter_type: "all",
          filter_value: "",
          created_at: new Date().toISOString()
        },
        {
          id: "demo2",
          method: "POST",
          url: "https://example.com/webhook2",
          filter_type: "group",
          filter_value: "123456789@g.us",
          created_at: new Date().toISOString()
        }
      ],
      loading: false,
      newMethod: "POST",
      error: "",
      logs: {
        demo1: [
          {
            timestamp: new Date().toISOString(),
            payload: JSON.stringify({
              from: "1234567890@s.whatsapp.net",
              type: "text",
              text: "Hello, World!"
            }, null, 2)
          }
        ]
      },
      waStatus: 'connected',
      waQR: '',
      waLoginState: '',
      waLoading: false,
      newURL: '',
      newFilterType: 'all',
      newFilterValue: '',
      showChatsModal: false,
      chatsLoading: false,
      chatsError: '',
      recentChats: [
        { id: "123456789@g.us", name: "Demo Group", type: "group" },
        { id: "987654321@s.whatsapp.net", name: "Demo Contact", type: "chat" }
      ],
      filteredChats: []
    };
  },
  methods: {
    async fetchWebhooks() {
      // Demo mode - do nothing
    },
    async createWebhook() {
      // Demo mode - add to local array
      this.webhooks.unshift({
        id: "demo" + (this.webhooks.length + 1),
        method: this.newMethod,
        url: this.newURL,
        filter_type: this.newFilterType,
        filter_value: this.newFilterValue,
        created_at: new Date().toISOString()
      });
      this.newURL = '';
      this.newFilterType = 'all';
      this.newFilterValue = '';
    },
    async deleteWebhook(id) {
      // Demo mode - remove from local array
      this.webhooks = this.webhooks.filter(wh => wh.id !== id);
    },
    fullWebhookUrl(id) {
      return window.location.origin + "/webhook/" + id;
    },
    formatTime(ts) {
      return new Date(ts).toLocaleString();
    },
    copyUrl(id) {
      const url = this.fullWebhookUrl(id);
      if (navigator.clipboard) {
        navigator.clipboard.writeText(url);
      }
    },
    waStatusMessage() {
      return 'WhatsApp Connected!';
    },
    onFilterTypeChange() {
      this.newFilterValue = '';
    },
    getFilterPlaceholder() {
      if (this.newFilterType === 'group') return 'Enter group ID';
      if (this.newFilterType === 'chat') return 'Enter chat ID';
      return '';
    },
    showRecentChats() {
      this.showChatsModal = true;
      this.filteredChats = this.recentChats.filter(chat =>
        this.newFilterType === 'group' ? chat.type === 'group' : true
      );
    },
    closeChatsModal() {
      this.showChatsModal = false;
    },
    selectChat(chat) {
      this.newFilterValue = chat.id;
      this.closeChatsModal();
    },
    getFilterDisplayText(webhook) {
      if (webhook.filter_type === 'group') return `Group: ${webhook.filter_value || 'All Groups'}`;
      if (webhook.filter_type === 'chat') return `Chat: ${webhook.filter_value || 'All Chats'}`;
      return 'All Messages';
    }
  }
};
</script>