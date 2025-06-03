<template>
  <div class="webhook-manager">
    <h2>Webhook Management</h2>
    <div v-if="loading">Loading webhooks...</div>
    <div v-else>
      <div v-if="webhooks.length === 0" class="empty">No webhooks yet.</div>
      <ul class="webhook-list">
        <li v-for="wh in webhooks" :key="wh.id" class="webhook-item">
          <span class="webhook-id">ID: {{ wh.id }}</span>
          <span class="webhook-method">Method: {{ wh.method }}</span>
          <span class="webhook-url">URL: <code>/webhook/{{ wh.id }}</code></span>
          <button @click="deleteWebhook(wh.id)" class="delete-btn">Delete</button>
        </li>
      </ul>
      <form @submit.prevent="createWebhook" class="create-form">
        <label>
          Method:
          <select v-model="newMethod">
            <option value="POST">POST</option>
            <option value="GET">GET</option>
          </select>
        </label>
        <button type="submit">Create Webhook</button>
      </form>
      <div v-if="error" class="error">{{ error }}</div>
    </div>
  </div>
</template>

<script>
export default {
  name: "WebhookManager",
  data() {
    return {
      webhooks: [],
      loading: true,
      newMethod: "POST",
      error: ""
    };
  },
  mounted() {
    this.fetchWebhooks();
  },
  methods: {
    async fetchWebhooks() {
      this.loading = true;
      this.error = "";
      try {
        const res = await fetch("/api/webhooks");
        if (!res.ok) throw new Error("Failed to load webhooks");
        this.webhooks = await res.json();
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
          body: JSON.stringify({ method: this.newMethod })
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
    }
  }
};
</script>

<style scoped>
.webhook-manager {
  background: #222;
  color: #fff;
  border-radius: 8px;
  padding: 2rem;
  margin: 2rem auto;
  max-width: 500px;
  box-shadow: 0 2px 12px #0008;
}
.webhook-list {
  list-style: none;
  padding: 0;
  margin-bottom: 1rem;
}
.webhook-item {
  display: flex;
  flex-direction: column;
  background: #333;
  margin-bottom: 0.5rem;
  padding: 0.5rem 1rem;
  border-radius: 4px;
}
.webhook-id, .webhook-method, .webhook-url {
  font-size: 0.95em;
  margin-bottom: 2px;
}
.delete-btn {
  align-self: flex-end;
  background: #c00;
  color: #fff;
  border: none;
  border-radius: 3px;
  padding: 0.2rem 0.7rem;
  cursor: pointer;
  margin-top: 4px;
}
.create-form {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-top: 1rem;
}
.create-form label {
  color: #fff;
}
.create-form button {
  background: #0a0;
  color: #fff;
  border: none;
  border-radius: 3px;
  padding: 0.3rem 1rem;
  cursor: pointer;
}
.error {
  color: #f66;
  margin-top: 1rem;
}
.empty {
  color: #aaa;
  margin-bottom: 1rem;
}
</style> 