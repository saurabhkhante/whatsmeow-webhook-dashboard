<template>
  <div class="message-sender">
    <div class="message-card">
      <div class="message-header">
        <h2>📨 Send Message</h2>
        <div class="status-indicator" :class="statusClass">{{ statusText }}</div>
      </div>
      
      <form @submit.prevent="sendMessage" class="message-form">
        <div class="input-group">
          <label for="chat-id">Chat/Group ID</label>
          <div class="chat-input-wrapper">
            <input 
              id="chat-id"
              v-model="chatId" 
              type="text" 
              placeholder="e.g., 1234567890@s.whatsapp.net or 1234567890@g.us"
              required 
              class="chat-input"
              :disabled="sending"
            />
            <button 
              type="button" 
              @click="showChatBrowser" 
              class="browse-btn"
              :disabled="sending"
              title="Browse recent chats"
            >
              📋
            </button>
          </div>
          <div class="input-help">
            Use @s.whatsapp.net for individual chats, @g.us for groups
          </div>
        </div>

        <div class="input-group">
          <label for="message">Message</label>
          <textarea 
            id="message"
            v-model="message" 
            placeholder="Type your message here..."
            required 
            rows="4"
            class="message-textarea"
            :disabled="sending"
            @keydown.ctrl.enter="sendMessage"
          ></textarea>
          <div class="input-help">
            Press Ctrl+Enter to send quickly
          </div>
        </div>

        <div class="form-actions">
          <button 
            type="submit" 
            class="send-btn"
            :disabled="!canSend"
            :class="{ 'sending': sending }"
          >
            <span v-if="sending">⏳ Sending...</span>
            <span v-else>📤 Send Message</span>
          </button>
          <button 
            type="button" 
            @click="clearForm" 
            class="clear-btn"
            :disabled="sending"
          >
            🗑️ Clear
          </button>
        </div>
      </form>

      <div v-if="lastSentMessage" class="last-message">
        <div class="last-message-header">✅ Last sent message:</div>
        <div class="last-message-content">
          <div class="last-message-to">To: <code>{{ lastSentMessage.chatId }}</code></div>
          <div class="last-message-text">"{{ lastSentMessage.message }}"</div>
          <div class="last-message-time">{{ formatTime(lastSentMessage.timestamp) }}</div>
        </div>
      </div>

      <div v-if="error" class="error-message">
        ❌ {{ error }}
      </div>
    </div>

    <!-- Chat Browser Modal -->
    <div v-if="showModal" class="modal-overlay" @click="closeModal">
      <div class="modal-content" @click.stop>
        <div class="modal-header">
          <h3>📝 Select Chat</h3>
          <button @click="closeModal" class="close-btn">&times;</button>
        </div>
        <div class="modal-body">
          <div class="filter-tabs">
            <button 
              @click="chatFilter = 'all'" 
              :class="{ active: chatFilter === 'all' }"
              class="filter-tab"
            >
              All
            </button>
            <button 
              @click="chatFilter = 'chat'" 
              :class="{ active: chatFilter === 'chat' }"
              class="filter-tab"
            >
              👤 Individual
            </button>
            <button 
              @click="chatFilter = 'group'" 
              :class="{ active: chatFilter === 'group' }"
              class="filter-tab"
            >
              👥 Groups
            </button>
          </div>
          
          <div v-if="chatsLoading" class="loading">Loading chats...</div>
          <div v-else-if="chatsError" class="error">{{ chatsError }}</div>
          <div v-else-if="filteredChats.length === 0" class="empty">
            No recent chats found. Try connecting to WhatsApp first.
          </div>
          <div v-else class="chats-list">
            <div 
              v-for="chat in filteredChats" 
              :key="chat.id" 
              class="chat-item" 
              @click="selectChat(chat)"
            >
              <div class="chat-icon">{{ chat.type === 'group' ? '👥' : '👤' }}</div>
              <div class="chat-details">
                <div class="chat-name">{{ chat.name || 'Unknown' }}</div>
                <div class="chat-id">{{ chat.id }}</div>
              </div>
              <div class="select-indicator">→</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: "MessageSender",
  data() {
    return {
      chatId: '',
      message: '',
      sending: false,
      error: '',
      lastSentMessage: null,
      showModal: false,
      chatsLoading: false,
      chatsError: '',
      recentChats: [],
      chatFilter: 'all'
    };
  },
  computed: {
    canSend() {
      return this.chatId.trim() && this.message.trim() && !this.sending;
    },
    statusClass() {
      if (this.sending) return 'status-sending';
      if (this.error) return 'status-error';
      if (this.lastSentMessage) return 'status-success';
      return 'status-ready';
    },
    statusText() {
      if (this.sending) return 'Sending...';
      if (this.error) return 'Error';
      if (this.lastSentMessage) return 'Ready';
      return 'Ready to send';
    },
    filteredChats() {
      if (this.chatFilter === 'all') return this.recentChats;
      return this.recentChats.filter(chat => chat.type === this.chatFilter);
    }
  },
  methods: {
    async sendMessage() {
      if (!this.canSend) return;

      this.sending = true;
      this.error = '';

      try {
        const response = await fetch('/api/messages/send', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            chat_jid: this.chatId.trim(),
            message: this.message.trim()
          })
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || `HTTP ${response.status}`);
        }

        const result = await response.json();
        
        // Store last sent message
        this.lastSentMessage = {
          chatId: this.chatId.trim(),
          message: this.message.trim(),
          timestamp: new Date(),
          messageId: result.message_id
        };

        // Clear the form
        this.chatId = '';
        this.message = '';
        
        // Show success briefly
        setTimeout(() => {
          this.error = '';
        }, 100);

      } catch (error) {
        console.error('Send message error:', error);
        this.error = error.message || 'Failed to send message';
      } finally {
        this.sending = false;
      }
    },

    clearForm() {
      this.chatId = '';
      this.message = '';
      this.error = '';
    },

    async showChatBrowser() {
      this.showModal = true;
      this.chatsLoading = true;
      this.chatsError = '';

      try {
        const response = await fetch('/api/wa/chats');
        if (response.ok) {
          this.recentChats = await response.json();
        } else {
          this.chatsError = 'Failed to load recent chats';
        }
      } catch (error) {
        this.chatsError = error.message || 'Network error';
      } finally {
        this.chatsLoading = false;
      }
    },

    closeModal() {
      this.showModal = false;
    },

    selectChat(chat) {
      this.chatId = chat.id;
      this.closeModal();
    },

    formatTime(timestamp) {
      return new Date(timestamp).toLocaleString();
    }
  }
};
</script>

<style scoped>
.message-sender {
  width: 100%;
  max-width: 1400px;
  margin: 0 auto;
}

.message-card {
  background: #fff;
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  padding: 2rem;
  border: 1px solid #e0e0e0;
}

.message-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2rem;
  padding-bottom: 1rem;
  border-bottom: 2px solid #f0f0f0;
}

.message-header h2 {
  margin: 0;
  color: #333;
  font-size: 1.5rem;
  font-weight: 600;
}

.status-indicator {
  padding: 0.5rem 1rem;
  border-radius: 20px;
  font-size: 0.9rem;
  font-weight: 500;
}

.status-ready {
  background: #e3f2fd;
  color: #1976d2;
}

.status-sending {
  background: #fff3e0;
  color: #f57c00;
}

.status-success {
  background: #e8f5e8;
  color: #4caf50;
}

.status-error {
  background: #ffebee;
  color: #f44336;
}

.message-form {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.input-group {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.input-group label {
  font-weight: 600;
  color: #555;
  font-size: 1rem;
}

.chat-input-wrapper {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.chat-input {
  flex: 1;
  padding: 0.75rem 1rem;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 1rem;
  transition: border-color 0.3s;
}

.chat-input:focus {
  outline: none;
  border-color: #42b983;
}

.browse-btn {
  padding: 0.75rem 1rem;
  background: #f5f5f5;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  cursor: pointer;
  font-size: 1rem;
  transition: all 0.3s;
}

.browse-btn:hover:not(:disabled) {
  background: #e0e0e0;
}

.browse-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.message-textarea {
  padding: 1rem;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 1rem;
  font-family: inherit;
  resize: vertical;
  min-height: 100px;
  transition: border-color 0.3s;
}

.message-textarea:focus {
  outline: none;
  border-color: #42b983;
}

.input-help {
  font-size: 0.85rem;
  color: #888;
  font-style: italic;
}

.form-actions {
  display: flex;
  gap: 1rem;
  align-items: center;
}

.send-btn {
  padding: 1rem 2rem;
  background: #42b983;
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s;
  flex: 1;
  max-width: 200px;
}

.send-btn:hover:not(:disabled) {
  background: #369870;
  transform: translateY(-1px);
}

.send-btn:disabled {
  background: #ccc;
  cursor: not-allowed;
  transform: none;
}

.send-btn.sending {
  background: #ff9800;
}

.clear-btn {
  padding: 1rem 1.5rem;
  background: #f5f5f5;
  color: #666;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 1rem;
  cursor: pointer;
  transition: all 0.3s;
}

.clear-btn:hover:not(:disabled) {
  background: #e0e0e0;
}

.clear-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.last-message {
  margin-top: 2rem;
  padding: 1rem;
  background: #f8f9fa;
  border-radius: 8px;
  border-left: 4px solid #4caf50;
}

.last-message-header {
  font-weight: 600;
  color: #4caf50;
  margin-bottom: 0.5rem;
}

.last-message-content {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.last-message-to {
  font-size: 0.9rem;
  color: #666;
}

.last-message-to code {
  background: #e0e0e0;
  padding: 0.2rem 0.4rem;
  border-radius: 4px;
  font-family: 'Courier New', monospace;
}

.last-message-text {
  font-style: italic;
  color: #333;
}

.last-message-time {
  font-size: 0.85rem;
  color: #888;
}

.error-message {
  margin-top: 1rem;
  padding: 1rem;
  background: #ffebee;
  border: 1px solid #f44336;
  border-radius: 8px;
  color: #d32f2f;
  font-weight: 500;
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
  background: white;
  border-radius: 12px;
  width: 90%;
  max-width: 600px;
  max-height: 80vh;
  overflow: hidden;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.2);
}

.modal-header {
  padding: 1.5rem;
  border-bottom: 1px solid #e0e0e0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #f8f9fa;
}

.modal-header h3 {
  margin: 0;
  color: #333;
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
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.3s;
}

.close-btn:hover {
  background: #e0e0e0;
  color: #333;
}

.modal-body {
  padding: 1.5rem;
  max-height: 60vh;
  overflow-y: auto;
}

.filter-tabs {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.filter-tab {
  padding: 0.5rem 1rem;
  border: 2px solid #e0e0e0;
  background: white;
  border-radius: 20px;
  cursor: pointer;
  transition: all 0.3s;
  font-size: 0.9rem;
}

.filter-tab:hover {
  border-color: #42b983;
}

.filter-tab.active {
  background: #42b983;
  color: white;
  border-color: #42b983;
}

.loading, .error, .empty {
  text-align: center;
  padding: 2rem;
  color: #666;
}

.error {
  color: #f44336;
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
  padding: 1rem;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.3s;
}

.chat-item:hover {
  background: #f8f9fa;
  border-color: #42b983;
  transform: translateX(4px);
}

.chat-icon {
  font-size: 1.5rem;
  min-width: 30px;
}

.chat-details {
  flex: 1;
}

.chat-name {
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.chat-id {
  font-size: 0.85rem;
  color: #666;
  font-family: 'Courier New', monospace;
  background: #f0f0f0;
  padding: 0.2rem 0.4rem;
  border-radius: 4px;
  display: inline-block;
}

.select-indicator {
  font-size: 1.2rem;
  color: #42b983;
  font-weight: bold;
}

/* Responsive */
@media (max-width: 768px) {
  .message-card {
    padding: 1rem;
    margin: 0.5rem;
  }
  
  .message-header {
    flex-direction: column;
    gap: 1rem;
    align-items: flex-start;
  }
  
  .form-actions {
    flex-direction: column;
    align-items: stretch;
  }
  
  .send-btn {
    max-width: none;
  }
  
  .chat-input-wrapper {
    flex-direction: column;
    align-items: stretch;
  }
  
  .browse-btn {
    align-self: stretch;
  }
}
</style>