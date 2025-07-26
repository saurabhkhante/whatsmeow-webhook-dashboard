<template>
  <div class="profile-dashboard">
    <div class="profile-card">
      <div class="profile-row">
        <div class="avatar" v-if="email && email.length">{{ email[0].toUpperCase() }}</div>
        <div class="avatar" v-else>?</div>
        <div class="info">
          <div class="email">{{ email }}</div>
        </div>
        <button class="logout-btn" @click="$emit('logout')">Logout</button>
      </div>
    </div>
    <main>
      <div class="wa-card">
        <section class="wa-section">
          <div v-if="waStatus === 'waiting_qr'">
            <div class="qr-area">
              <img v-if="waQR" :src="'/qr.png?'+Date.now()" alt="QR Code" class="qr-img" />
              <div class="wa-status">{{ waStatusMessage() }}</div>
              <button @click="disconnectWA" :disabled="waLoading" class="wa-btn wa-btn-secondary" style="margin-top:1rem;">Cancel</button>
            </div>
          </div>
          <div v-else-if="waStatus === 'connected'">
            <div class="wa-status wa-status-success">{{ waStatusMessage() }}</div>
            <button @click="disconnectWA" :disabled="waLoading" class="wa-btn wa-btn-danger">Disconnect WhatsApp</button>
          </div>
          <div v-else>
            <div class="wa-status wa-status-error">{{ waStatusMessage() }}</div>
            <button v-if="waStatus === 'disconnected' || !waStatus" @click="connectWA" :disabled="waLoading" class="wa-btn wa-btn-primary">Connect WhatsApp</button>
          </div>
        </section>
      </div>
      <!-- Quick Automation Section -->
      <section class="automation-quick-section">
        <div class="automation-quick-card">
          <div class="automation-quick-header">
            <h3>🚀 Quick Automation URL</h3>
            <p class="automation-quick-description">Generate a URL for external tools (n8n, Zapier, etc.) to send WhatsApp messages</p>
          </div>
          
          <div class="generate-section">
            <button @click="generateAutomationURL" class="generate-btn" :disabled="generating">
              <span v-if="generating">⏳ Generating...</span>
              <span v-else>⚡ Generate Automation URL</span>
            </button>
          </div>
          
          <div v-if="automationURLs.length > 0" class="generated-urls">
            <div class="urls-title">📋 Your Automation URLs:</div>
            <div v-for="url in automationURLs" :key="url.id" class="automation-url-item">
              <code class="automation-url-code">{{ url.full_url }}</code>
              <div class="automation-url-actions">
                <button @click="copyToClipboard(url.full_url)" class="copy-automation-btn" title="Copy URL">📋</button>
                <button @click="deleteAutomationURL(url.id)" class="delete-automation-btn" title="Delete URL">🗑️</button>
              </div>
            </div>
            <div class="automation-usage-example">
              <div class="usage-example-title">💡 Usage Example:</div>
              <pre class="usage-example-code">curl -X POST "{{ automationURLs[0]?.full_url }}" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Hello from automation!",
    "chat_id": "1234567890@s.whatsapp.net"
  }'</pre>
              <button @click="copyUsageExample" class="copy-example-btn">📋 Copy Example</button>
            </div>
          </div>
        </div>
      </section>

      <section class="webhooks-section">
        <div class="webhooks-header">
          <h2>Webhook Forwarding (Optional)</h2>
          <p class="webhooks-description">Forward incoming WhatsApp messages to your own endpoints</p>
          <form @submit.prevent="createWebhook" class="create-form">
            <div class="form-row">
              <label>URL:
                <input v-model="newURL" type="url" placeholder="https://your-endpoint.com/webhook" required />
              </label>
            </div>
            <div class="form-row">
              <label>Method:
                <select v-model="newMethod">
                  <option value="POST">POST</option>
                  <option value="GET">GET</option>
                </select>
              </label>
              <label>Filter:
                <select v-model="newFilterType" @change="onFilterTypeChange">
                  <option value="all">All Messages</option>
                  <option value="group">Specific Group</option>
                  <option value="chat">Specific Chat</option>
                </select>
              </label>
            </div>
            <div v-if="newFilterType !== 'all'" class="form-row">
              <label>Chat/Group ID:
                <input v-model="newFilterValue" type="text" :placeholder="getFilterPlaceholder()" />
              </label>
              <button type="button" @click="showRecentChats" class="wa-btn wa-btn-secondary">
                {{ newFilterType === 'group' ? 'Browse Groups' : 'Browse Chats' }}
              </button>
            </div>
            <button type="submit" class="wa-btn wa-btn-primary">Save Webhook URL</button>
          </form>
        </div>
        
        <!-- Recent Chats Modal -->
        <div v-if="showChatsModal" class="modal-overlay" @click="closeChatsModal">
          <div class="modal-content" @click.stop>
            <div class="modal-header">
              <h3>Recent {{ newFilterType === 'group' ? 'Groups' : 'Chats' }}</h3>
              <div class="modal-actions">
                <button @click="refreshChats" class="refresh-btn" :disabled="chatsLoading" title="Refresh from server">
                  <span v-if="chatsLoading">⏳</span>
                  <span v-else>🔄</span>
                </button>
                <button @click="closeChatsModal" class="close-btn">&times;</button>
              </div>
            </div>
            <div class="modal-body">
              <div v-if="chatsLoading" class="loading">Loading {{ newFilterType === 'group' ? 'groups' : 'chats' }}...</div>
              <div v-else-if="chatsError" class="error">{{ chatsError }}</div>
              <div v-else-if="filteredChats.length === 0" class="empty">No recent {{ newFilterType === 'group' ? 'groups' : 'chats' }} found.</div>
              <div v-else class="chats-list">
                <div v-for="chat in filteredChats" :key="chat.id" class="chat-item" @click="selectChat(chat)">
                  <div class="chat-type">{{ chat.type === 'group' ? '👥' : '👤' }}</div>
                  <div class="chat-info">
                    <div class="chat-name">{{ chat.name }}</div>
                    <div class="chat-id">{{ chat.id }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="loading" class="loading">Loading webhooks...</div>
        <div v-else-if="webhooks.length === 0" class="empty">No webhooks yet.</div>
        <div class="webhook-grid">
          <div v-for="wh in webhooks" :key="wh.id" class="webhook-card">
            <div class="webhook-info">
              <div class="webhook-id">ID: <span class="mono">{{ wh.id }}</span></div>
              <div class="webhook-method">Method: <span class="mono">{{ wh.method }}</span></div>
              <div class="webhook-url">
                URL:
                <code ref="urlRefs[wh.id]" class="mono">{{ wh.url || fullWebhookUrl(wh.id) }}</code>
                <button class="copy-btn wa-btn wa-btn-secondary" @click="copyUrl(wh.id)">Copy</button>
              </div>
              <div class="webhook-filter">
                Filter: <span class="mono">{{ getFilterDisplayText(wh) }}</span>
              </div>
            </div>
            <div class="webhook-logs">
              <div class="logs-title">Recent Messages</div>
              <div v-if="logs[wh.id] && logs[wh.id].length > 0">
                <div v-for="log in logs[wh.id]" :key="log.timestamp" class="log-entry">
                  <div class="log-time">{{ formatTime(log.timestamp) }}</div>
                  <pre class="log-payload">{{ log.payload }}</pre>
                </div>
              </div>
              <div v-else class="no-logs">No messages yet.</div>
            </div>
            <button class="delete-btn wa-btn wa-btn-danger" @click="deleteWebhook(wh.id)">Delete</button>
          </div>
        </div>
        <div v-if="error" class="error">{{ error }}</div>
      </section>
      
      <!-- Message Sender Section -->
      <section class="message-section">
        <MessageSender />
      </section>

    </main>
  </div>
</template>

<script>
import MessageSender from './MessageSender.vue';

export default {
  name: "ProfileDashboard",
  components: {
    MessageSender
  },
  props: ["email"],
  data() {
    return {
      webhooks: [],
      loading: true,
      newMethod: "POST",
      error: "",
      logs: {},
      urlRefs: {},
      logsInterval: null,
      // WhatsApp connection state
      waStatus: '',
      waQR: '',
      waLoginState: '',
      waLoading: false,
      showDebug: false,
      newURL: '',
      newFilterType: 'all',
      newFilterValue: '',
      showChatsModal: false,
      chatsLoading: false,
      chatsError: '',
      recentChats: [],
      filteredChats: [],
      cacheKey: 'whatsapp_chats_cache',
      cacheTimestampKey: 'whatsapp_chats_timestamp',
      // Automation URLs
      automationURLs: [],
      generating: false
    };
  },
  mounted() {
    this.fetchWebhooks();
    this.loadAutomationURLs();
    this.logsInterval = setInterval(this.fetchAllLogs, 5000);
    this.fetchWAStatus();
    this.waPollInterval = setInterval(this.fetchWAStatus, 2000);
  },
  beforeUnmount() {
    clearInterval(this.logsInterval);
    clearInterval(this.waPollInterval);
  },
  methods: {
    async fetchWebhooks() {
      this.loading = true;
      this.error = "";
      try {
        const res = await fetch("/api/webhooks");
        if (!res.ok) throw new Error("Failed to load webhooks");
        this.webhooks = await res.json();
        this.loadAutomationURLs(); // Load automation URLs after fetching webhooks
        this.fetchAllLogs();
      } catch (e) {
        this.error = e.message;
      } finally {
        this.loading = false;
      }
    },
    async createWebhook() {
      this.error = "";
      try {
        const res = await fetch("/api/webhooks/create", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ 
            method: this.newMethod, 
            url: this.newURL, 
            filter_type: this.newFilterType, 
            filter_value: this.newFilterValue 
          })
        });
        if (!res.ok) throw new Error("Failed to create webhook");
        await this.fetchWebhooks();
      } catch (e) {
        this.error = e.message;
      }
    },
    async deleteWebhook(id) {
      this.error = "";
      try {
        const res = await fetch("/api/webhooks/delete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ id })
        });
        if (!res.ok) throw new Error("Failed to delete webhook");
        await this.fetchWebhooks();
      } catch (e) {
        this.error = e.message;
      }
    },
    fullWebhookUrl(id) {
      return window.location.origin + "/webhook/" + id;
    },
    async fetchAllLogs() {
      for (const wh of this.webhooks) {
        try {
          const res = await fetch(`/api/webhooks/logs?id=${wh.id}`);
          if (res.ok) {
            const logs = await res.json();
            // Format payload as pretty JSON string
            this.logs[wh.id] = logs.map(l => ({
              timestamp: l.timestamp,
              payload: JSON.stringify(l.payload, null, 2)
            }));
          }
        } catch (e) {
          // Ignore log fetch errors for now
        }
      }
    },
    formatTime(ts) {
      return new Date(ts).toLocaleString();
    },
    copyUrl(id) {
      const url = this.fullWebhookUrl(id);
      if (navigator.clipboard) {
        navigator.clipboard.writeText(url);
      } else {
        // fallback
        const el = document.createElement('textarea');
        el.value = url;
        document.body.appendChild(el);
        el.select();
        document.execCommand('copy');
        document.body.removeChild(el);
      }
    },
    async fetchWAStatus() {
      try {
        const res = await fetch('/api/wa/status');
        if (res.ok) {
          const data = await res.json();
          this.waStatus = data.status || '';
          this.waQR = data.qr || '';
          this.waLoginState = data.loginState || '';
        } else {
          this.waStatus = 'error';
          this.waLoginState = 'Failed to fetch status';
        }
      } catch (e) {
        this.waStatus = 'error';
        this.waLoginState = 'Network error';
      }
    },
    async connectWA() {
      console.log('connectWA: Starting connection...');
      this.waLoading = true;
      try {
        console.log('connectWA: Sending POST to /api/wa/connect...');
        const response = await fetch('/api/wa/connect', { method: 'POST' });
        console.log('connectWA: Got response:', response.status, response.statusText);
        
        if (!response.ok) {
          console.error('connectWA: Response not OK:', await response.text());
        } else {
          console.log('connectWA: Success, fetching status...');
          await this.fetchWAStatus();
        }
      } catch (error) {
        console.error('connectWA: Error occurred:', error);
      }
      this.waLoading = false;
      console.log('connectWA: Finished.');
    },
    async disconnectWA() {
      this.waLoading = true;
      await fetch('/api/wa/disconnect', { method: 'POST' });
      this.waLoading = false;
      this.fetchWAStatus();
    },
    waStatusMessage() {
      if (this.waStatus === 'waiting_qr') return 'Scan this QR code with WhatsApp to connect.';
      if (this.waStatus === 'connected') return 'WhatsApp Connected!';
      if (this.waStatus === 'disconnected' || !this.waStatus) return 'Not connected.';
      if (this.waStatus === 'error') return this.waLoginState || 'An error occurred.';
      return this.waLoginState || this.waStatus;
    },
    onFilterTypeChange() {
      if (this.newFilterType === 'group') {
        this.newFilterValue = '';
      } else if (this.newFilterType === 'chat') {
        this.newFilterValue = '';
      }
    },
    getFilterPlaceholder() {
      if (this.newFilterType === 'group') return 'Enter group ID';
      if (this.newFilterType === 'chat') return 'Enter chat ID';
      return '';
    },
    async showRecentChats(forceRefresh = false) {
      console.log('DEBUG: Fetching chats for filter type:', this.newFilterType, 'forceRefresh:', forceRefresh);
      this.showChatsModal = true;
      this.chatsError = '';
      
      // Try to load from cache first (unless force refresh)
      if (!forceRefresh) {
        const cachedData = this.loadChatsFromCache();
        if (cachedData) {
          console.log('DEBUG: Using cached data:', cachedData.length, 'chats');
          this.recentChats = cachedData;
          this.applyFiltering();
          return;
        }
      }
      
      // Fetch from server
      this.chatsLoading = true;
      try {
        console.log('DEBUG: Fetching fresh data from server');
        const res = await fetch('/api/wa/chats', {
          method: 'GET',
          credentials: 'include',
          headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json'
          }
        });
        
        if (res.ok) {
          const data = await res.json();
          const chats = Array.isArray(data) ? data : [];
          console.log('DEBUG: Fetched', chats.length, 'chats from server');
          
          // Save to cache
          this.saveChatsToCache(chats);
          
          this.recentChats = chats;
          this.applyFiltering();
        } else {
          const errorText = await res.text();
          this.chatsError = `Failed to load chats (${res.status}): ${errorText}`;
          console.error('DEBUG: Failed to fetch chats:', res.status, errorText);
        }
      } catch (e) {
        this.chatsError = `Network error: ${e.message}`;
        console.error('DEBUG: Error fetching chats:', e);
      } finally {
        this.chatsLoading = false;
      }
    },

    loadChatsFromCache() {
      try {
        const cached = localStorage.getItem(this.cacheKey);
        const timestamp = localStorage.getItem(this.cacheTimestampKey);
        
        if (cached && timestamp) {
          const cacheAge = Date.now() - parseInt(timestamp);
          const maxAge = 10 * 60 * 1000; // 10 minutes
          
          if (cacheAge < maxAge) {
            console.log('DEBUG: Cache hit, age:', Math.round(cacheAge / 1000), 'seconds');
            return JSON.parse(cached);
          } else {
            console.log('DEBUG: Cache expired, age:', Math.round(cacheAge / 1000), 'seconds');
            this.clearCache();
          }
        }
      } catch (e) {
        console.warn('DEBUG: Error loading cache:', e);
        this.clearCache();
      }
      return null;
    },

    saveChatsToCache(chats) {
      try {
        localStorage.setItem(this.cacheKey, JSON.stringify(chats));
        localStorage.setItem(this.cacheTimestampKey, Date.now().toString());
        console.log('DEBUG: Saved', chats.length, 'chats to cache');
      } catch (e) {
        console.warn('DEBUG: Error saving to cache:', e);
      }
    },

    clearCache() {
      localStorage.removeItem(this.cacheKey);
      localStorage.removeItem(this.cacheTimestampKey);
      console.log('DEBUG: Cache cleared');
    },

    applyFiltering() {
      if (this.newFilterType === 'group') {
        this.filteredChats = this.recentChats.filter(chat => chat.type === 'group');
      } else if (this.newFilterType === 'chat') {
        this.filteredChats = this.recentChats.filter(chat => chat.type === 'chat');
      } else {
        this.filteredChats = this.recentChats;
      }
      
      console.log('DEBUG: Applied filtering for', this.newFilterType, ':', this.filteredChats.length, 'results');
      
      if (this.filteredChats.length === 0) {
        if (this.recentChats.length === 0) {
          this.chatsError = 'No contacts or groups found. Make sure WhatsApp is connected and you have some contacts.';
        } else {
          this.chatsError = `No ${this.newFilterType === 'group' ? 'groups' : 'individual chats'} found. Try switching the filter.`;
        }
      }
    },

    refreshChats() {
      console.log('DEBUG: Manually refreshing chats');
      this.showRecentChats(true); // Force refresh
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
    },


    copyToClipboard(text) {
      if (navigator.clipboard) {
        navigator.clipboard.writeText(text).then(() => {
          // Could add a toast notification here
          console.log('Copied to clipboard');
        });
      } else {
        // Fallback for older browsers
        const el = document.createElement('textarea');
        el.value = text;
        document.body.appendChild(el);
        el.select();
        document.execCommand('copy');
        document.body.removeChild(el);
        console.log('Copied to clipboard');
      }
    },

    copyUsageExample() {
      if (this.automationURLs.length > 0) {
        const example = `curl -X POST "${this.automationURLs[0].full_url}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "message": "Hello from automation!",
    "chat_id": "1234567890@s.whatsapp.net"
  }'`;
        this.copyToClipboard(example);
      }
    },

    loadAutomationURLs() {
      // Filter webhooks that are automation URLs (empty URL field)
      this.automationURLs = this.webhooks
        .filter(webhook => !webhook.url || webhook.url === '')
        .map(webhook => ({
          id: webhook.id,
          full_url: `${window.location.origin}/webhook/${webhook.id}`
        }));
    },

    async generateAutomationURL() {
      this.generating = true;
      this.error = '';
      try {
        const res = await fetch('/api/automation/generate', {
          method: 'POST',
          credentials: 'include',
          headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json'
          }
        });
        
        if (res.ok) {
          const data = await res.json();
          this.automationURLs.push({
            id: data.webhook_id,
            full_url: data.automation_url
          });
          console.log('Generated automation URL:', data.automation_url);
          // Refresh webhooks to update the list
          await this.fetchWebhooks();
        } else {
          const errorText = await res.text();
          this.error = `Failed to generate automation URL: ${errorText}`;
        }
      } catch (e) {
        this.error = `Network error: ${e.message}`;
        console.error('Error generating automation URL:', e);
      } finally {
        this.generating = false;
      }
    },

    async deleteAutomationURL(id) {
      this.error = '';
      try {
        const res = await fetch('/api/webhooks/delete', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id })
        });
        
        if (res.ok) {
          this.automationURLs = this.automationURLs.filter(url => url.id !== id);
          await this.fetchWebhooks();
          console.log('Deleted automation URL:', id);
        } else {
          throw new Error('Failed to delete automation URL');
        }
      } catch (e) {
        this.error = e.message;
        console.error('Error deleting automation URL:', e);
      }
    }
  }
};
</script>

<style scoped>
body, .profile-dashboard {
  min-height: 100vh;
  background: #fff;
  color: #222;
  font-family: 'Inter', 'Segoe UI', Arial, sans-serif;
  margin: 0;
}
.profile-dashboard {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0;
  background: #fff;
}
.profile-card {
  width: 100%;
  max-width: 1400px;
  margin: 40px auto 24px auto;
  background: #fff;
  border-radius: 18px;
  box-shadow: 0 4px 24px #0001;
  padding: 2rem 2.5rem 1.5rem 2.5rem;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.profile-row {
  display: flex;
  align-items: center;
  width: 100%;
  gap: 1.5rem;
  justify-content: space-between;
}
.avatar {
  width: 56px;
  height: 56px;
  background: #42b983;
  color: #fff;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 2.2rem;
  font-weight: bold;
  margin-right: 1rem;
  box-shadow: 0 2px 8px #0002;
}
.info .email {
  font-size: 1.2rem;
  font-weight: 600;
  margin-bottom: 2px;
}
.logout-btn {
  background: #e53935;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 0.7rem 2rem;
  font-size: 1.1rem;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.2s;
  margin-left: auto;
}
.logout-btn:hover {
  background: #b71c1c;
}
main {
  width: 100%;
  max-width: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0 2rem 3rem 2rem;
}
.wa-card {
  width: 100%;
  max-width: 1400px;
  margin-bottom: 32px;
  background: #fff;
  border-radius: 16px;
  box-shadow: 0 2px 12px #0001;
  padding: 2rem 2.5rem 1.5rem 2.5rem;
  display: flex;
  flex-direction: column;
  align-items: center;
}
.wa-section {
  width: 100%;
  text-align: center;
}
.qr-area {
  margin: 2rem auto 1rem auto;
  padding: 1.5rem 1.5rem 1rem 1.5rem;
  background: #f8f8f8;
  border-radius: 12px;
  display: inline-block;
  box-shadow: 0 2px 8px #0001;
}
.qr-img {
  display: block;
  margin: 0 auto 1rem auto;
  background: #fff;
  padding: 16px;
  border-radius: 8px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
  width: 220px;
  height: 220px;
}
.wa-status {
  margin-top: 1rem;
  font-size: 1.1em;
  color: #222;
}
.wa-status-success {
  color: #43a047;
  font-weight: bold;
  font-size: 1.2em;
}
.wa-status-error {
  color: #e53935;
  font-weight: bold;
  font-size: 1.1em;
}
.wa-btn {
  border: none;
  border-radius: 6px;
  padding: 0.5rem 1.5rem;
  font-size: 1rem;
  font-weight: 500;
  cursor: pointer;
  margin: 0.5rem 0.5rem 0 0;
  transition: background 0.2s;
}
.wa-btn-primary {
  background: #42b983;
  color: #fff;
}
.wa-btn-primary:hover {
  background: #2e8c6a;
}
.wa-btn-danger {
  background: #e53935;
  color: #fff;
}
.wa-btn-danger:hover {
  background: #b71c1c;
}
.wa-btn-secondary {
  background: #e0e0e0;
  color: #222;
}
.wa-btn-secondary:hover {
  background: #bdbdbd;
}
.webhooks-section {
  width: 100%;
  max-width: 1400px;
  margin-top: 0;
}
.message-section {
  width: 100%;
  max-width: 1400px;
  margin-top: 2rem;
}
.automation-quick-section {
  width: 100%;
  max-width: 1400px;
  margin-top: 2rem;
}
.automation-quick-card {
  background: #fff;
  border-radius: 16px;
  box-shadow: 0 2px 12px #0001;
  padding: 2rem;
  border: 1px solid #e0e0e0;
  border-left: 4px solid #42b983;
}
.automation-quick-header h3 {
  margin: 0 0 0.5rem 0;
  color: #333;
  font-size: 1.4rem;
  font-weight: 600;
}
.automation-quick-description {
  color: #666;
  margin-bottom: 1.5rem;
  line-height: 1.5;
}
.generate-section {
  margin-bottom: 2rem;
}
.generate-btn {
  background: #42b983;
  color: white;
  border: none;
  border-radius: 8px;
  padding: 1rem 2rem;
  font-size: 1.1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s;
  box-shadow: 0 2px 8px rgba(66, 185, 131, 0.2);
}
.generate-btn:hover:not(:disabled) {
  background: #369870;
  transform: translateY(-2px);
  box-shadow: 0 4px 16px rgba(66, 185, 131, 0.3);
}
.generate-btn:disabled {
  background: #ccc;
  cursor: not-allowed;
  transform: none;
  box-shadow: none;
}
.generated-urls {
  border-top: 1px solid #e0e0e0;
  padding-top: 1.5rem;
}
.urls-title {
  font-weight: 600;
  color: #333;
  margin-bottom: 1rem;
  font-size: 1.1rem;
}
.automation-url-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1rem;
  background: #f8f9fa;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 1rem;
}
.automation-url-code {
  flex: 1;
  background: #fff;
  border: 1px solid #ddd;
  border-radius: 6px;
  padding: 0.75rem 1rem;
  font-family: 'Courier New', monospace;
  font-size: 0.9rem;
  color: #333;
  word-break: break-all;
}
.automation-url-actions {
  display: flex;
  gap: 0.5rem;
}
.copy-automation-btn {
  background: #42b983;
  color: white;
  border: none;
  border-radius: 6px;
  padding: 0.75rem;
  cursor: pointer;
  transition: background 0.3s;
  font-size: 1rem;
}
.copy-automation-btn:hover {
  background: #369870;
}
.delete-automation-btn {
  background: #e53935;
  color: white;
  border: none;
  border-radius: 6px;
  padding: 0.75rem;
  cursor: pointer;
  transition: background 0.3s;
  font-size: 1rem;
}
.delete-automation-btn:hover {
  background: #c62828;
}
.automation-usage-example {
  background: #f8f9fa;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 1rem;
  margin-top: 1rem;
}
.usage-example-title {
  font-weight: 600;
  color: #333;
  margin-bottom: 0.5rem;
}
.usage-example-code {
  background: #2d3748;
  color: #e2e8f0;
  padding: 1rem;
  border-radius: 6px;
  font-family: 'Courier New', monospace;
  font-size: 0.85rem;
  overflow-x: auto;
  white-space: pre;
  margin: 0 0 1rem 0;
  line-height: 1.4;
}
.copy-example-btn {
  background: #4a5568;
  color: white;
  border: none;
  border-radius: 6px;
  padding: 0.75rem 1rem;
  cursor: pointer;
  transition: background 0.3s;
  font-size: 0.9rem;
  font-weight: 500;
}
.copy-example-btn:hover {
  background: #2d3748;
}
.webhooks-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  gap: 1rem;
}
.create-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.form-row {
  display: flex;
  gap: 1rem;
  align-items: end;
}
.form-row label {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  flex: 1;
}
.form-row input, .form-row select {
  padding: 0.5rem;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 1rem;
}
.create-form button {
  min-width: 140px;
  align-self: flex-start;
}
.webhook-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
  gap: 1.5rem;
}
.webhook-card {
  background: #fff;
  border-radius: 14px;
  box-shadow: 0 2px 12px #0001;
  padding: 1.5rem 1.2rem 1.2rem 1.2rem;
  display: flex;
  flex-direction: column;
  min-height: 320px;
  position: relative;
  border: 1px solid #f0f0f0;
}
.webhook-info {
  margin-bottom: 1rem;
}
.webhook-id, .webhook-method, .webhook-url, .webhook-filter {
  font-size: 1em;
  margin-bottom: 2px;
}
.webhook-url {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.copy-btn {
  min-width: 60px;
}
.mono {
  font-family: 'Fira Mono', 'Menlo', 'Consolas', 'Liberation Mono', monospace;
  font-size: 0.98em;
  color: #2e3a59;
}
.webhook-logs {
  background: #f8f8f8;
  border-radius: 6px;
  padding: 0.7rem 0.7rem 0.5rem 0.7rem;
  margin-bottom: 1rem;
  min-height: 120px;
  max-height: 180px;
  overflow-y: auto;
  font-size: 1.01em;
}
.logs-title {
  font-size: 1em;
  color: #888;
  margin-bottom: 0.3rem;
}
.log-entry {
  margin-bottom: 0.7rem;
}
.log-time {
  font-size: 0.97em;
  color: #43a047;
  margin-bottom: 2px;
  font-family: 'Fira Mono', 'Menlo', 'Consolas', 'Liberation Mono', monospace;
}
.log-payload {
  background: #f0f0f0;
  color: #222;
  font-size: 1.01em;
  border-radius: 3px;
  padding: 0.3em 0.5em;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: 'Fira Mono', 'Menlo', 'Consolas', 'Liberation Mono', monospace;
}
.no-logs {
  color: #bbb;
  font-size: 0.99em;
}
.delete-btn {
  position: absolute;
  bottom: 1.2rem;
  right: 1.2rem;
  min-width: 90px;
}
.loading {
  color: #888;
  margin-bottom: 1rem;
}
.empty {
  color: #bbb;
  margin-bottom: 1rem;
}
.error {
  color: #e53935;
  margin-top: 1rem;
  font-weight: 500;
}
@media (max-width: 768px) {
  .profile-card, .wa-card {
    max-width: 95vw;
    margin: 20px auto;
    padding: 1.5rem 1rem;
  }
  main {
    padding: 0 1rem 2rem 1rem;
  }
  .webhook-grid {
    grid-template-columns: 1fr;
    gap: 1rem;
  }
  .webhooks-header {
    flex-direction: column;
    align-items: stretch;
    gap: 1rem;
  }
  .form-row {
    flex-direction: column;
    gap: 0.5rem;
  }
}

@media (min-width: 1200px) {
  .webhook-grid {
    grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
  }
}

@media (min-width: 1600px) {
  .profile-card, .wa-card, .webhooks-section, .message-section {
    max-width: 1600px;
  }
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1000;
}
.modal-content {
  background: #fff;
  border-radius: 12px;
  padding: 0;
  width: 90%;
  max-width: 800px;
  max-height: 80vh;
  overflow: hidden;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
}
.modal-header {
  padding: 1.5rem;
  border-bottom: 1px solid #eee;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.modal-header h3 {
  margin: 0;
  color: #333;
}
.modal-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.refresh-btn {
  background: #f0f8ff;
  border: 1px solid #42b983;
  color: #42b983;
  border-radius: 50%;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
  font-size: 1rem;
}
.refresh-btn:hover:not(:disabled) {
  background: #42b983;
  color: white;
  transform: rotate(45deg);
}
.refresh-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none;
}
.close-btn {
  background: none;
  border: none;
  font-size: 1.5rem;
  cursor: pointer;
  color: #666;
  padding: 0;
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.close-btn:hover {
  color: #333;
}
.modal-body {
  padding: 1.5rem;
  max-height: 50vh;
  overflow-y: auto;
}
.chats-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.chat-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem;
  border: 1px solid #eee;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s;
}
.chat-item:hover {
  background: #f5f5f5;
}
.chat-type {
  font-size: 1.5rem;
  min-width: 30px;
}
.chat-info {
  flex: 1;
}
.chat-name {
  font-weight: 500;
  margin-bottom: 0.25rem;
}
.chat-id {
  font-size: 0.9rem;
  color: #666;
  font-family: 'Fira Mono', 'Menlo', 'Consolas', 'Liberation Mono', monospace;
}
</style> 