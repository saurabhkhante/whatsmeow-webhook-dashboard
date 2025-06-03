# WhatsMeow Webhook Dashboard

## Overview
This app lets you connect a WhatsApp account (via WhatsMeow), create webhooks, and forward WhatsApp messages (including media) to user-defined webhook URLs. It features a modern web dashboard for managing webhooks, viewing logs, and connecting WhatsApp via QR code.

---

## Current Features

- **WhatsApp Integration:**
  - Connect your WhatsApp account by scanning a QR code.
  - Receive and process incoming WhatsApp messages (text, images, audio, documents).
- **Webhook Management:**
  - Create, list, and delete webhooks from a modern web UI.
  - Each webhook has a unique URL and can be set to POST or GET.
  - View the last 5 messages received by each webhook (with payload log).
  - Copy webhook URLs easily.
- **Media Handling:**
  - Media files (images, audio, documents) are saved locally and accessible via a URL.
- **Session Management:**
  - Login/logout system (currently with a test user).
  - WhatsApp connection persists even if you log out of the web UI.
- **Modern UI:**
  - Responsive, clean dashboard with cards, readable logs, and status indicators.

---

## Setup & Usage

### Prerequisites
- Go (1.18+)
- Node.js & npm (for frontend)

### Local Development
1. **Clone the repo and install dependencies:**
   ```sh
   git clone <repo-url>
   cd whatsmeowtest
   cd frontend && npm install && cd ..
   go mod tidy
   ```
2. **Start the app:**
   ```sh
   ./restart.sh
   ```
   This will build the frontend and start the backend server.
3. **Open the app:**
   - Go to [http://localhost:8080](http://localhost:8080)
   - Login with:
     - Email: `test@example.com`
     - Password: `password123`
   - Connect WhatsApp, create webhooks, and send WhatsApp messages to test.

### Docker Usage (Local & Production)

1. **Build the Docker image:**
   ```sh
   docker build -t whatsmeow-dashboard .
   ```

2. **Run the app with environment variables and media volume:**
   ```sh
   docker run --env-file .env.production -v $(pwd)/media:/app/media -p 8080:8080 whatsmeow-dashboard
   ```

   - Use `.env.production` for production settings (never commit this file to git).
   - The `media/` directory will persist WhatsApp media files.

3. **Access the app:**
   - Go to [http://localhost:8080](http://localhost:8080) (or your server's IP in production).

#### **Environment Variables**
- All configuration is managed via environment variables.
- See `.env.example` for a template.

#### **Production Deployment**
- Copy your code and `.env.production` to your server.
- Use Docker as above, mounting a persistent volume for `/app/media`.

### How It Works
- WhatsApp messages are received by the backend and forwarded to all your webhooks.
- Media is saved locally and accessible via a URL in the webhook payload.
- The dashboard shows recent messages for each webhook.
- Logging out does **not** disconnect WhatsApp or stop forwarding.

---

## Roadmap / Planned Features

### 1. **User Authentication & Multi-user Support**
- Real registration/login for multiple users.
- Each user has their own WhatsApp session and webhooks.

### 2. **Database Integration**
- Store users, webhooks, and logs in a database (SQLite/Postgres).
- Persistent storage for all data.

### 3. **Message Filtering**
- Allow users to filter which messages are forwarded (by chat/group ID, etc.).

### 4. **Cloud Media Storage**
- Store media files in a cloud bucket (S3, GCS) for external access (n8n, Zapier, etc.).

### 5. **Dockerization**
- Dockerfile and docker-compose for easy deployment.

### 6. **Cloud Hosting**
- Deploy the app to a cloud provider for 24/7 uptime.

### 7. **Webhook Destination URLs**
- Let users set the actual external URL for each webhook (not just internal endpoints).
- Forward WhatsApp messages to any public endpoint.

### 8. **Webhook Delivery Status & Retry**
- Track delivery status and retry failed webhook calls.

### 9. **Admin & Analytics**
- Admin dashboard for monitoring users and webhooks.
- Analytics for usage, delivery rates, etc.

### 10. **Other Ideas**
- Media previews in the UI.
- Real-time log updates (WebSockets).
- API access for automation.
- Rate limiting and security improvements.
- Mobile-optimized UI.

---

## Contributing / Next Steps
- Pick a feature from the roadmap and start building!
- For questions or to discuss architecture, see the comments in `server.go` and the frontend Vue components.
- PRs and suggestions are welcome.

---

## Credits
- Built with [WhatsMeow](https://github.com/tulir/whatsmeow) for WhatsApp integration.
- Frontend: Vue.js + Vite
- Backend: Go

## Testing

- The backend includes a comprehensive test suite using Go's `testing` and `httptest` packages.
- To run all tests:
  ```sh
  go test ./...
  ```
- Tests cover:
  - User authentication and session management
  - Webhook creation, listing, and deletion
  - Media file saving and serving
  - Webhook forwarding (with mock server)
  - Security and edge cases (invalid input, unauthorized access, etc.)
- All critical logic is covered to ensure reliability and security.

## Recent Changes

- **Dockerization:** Added a multi-stage Dockerfile and .dockerignore for easy, reproducible deployment. Supports both local and production use with environment variables.
- **Environment Variable Management:** Added support for `.env` and `.env.production` files. All secrets and config are now managed via environment variables.
- **Media Volume Handling:** Media files are now stored in a configurable directory (`MEDIA_DIR`), and Docker volumes are recommended for persistence in production.
- **Comprehensive Test Suite:** Added Go tests using the `testing` and `httptest` packages. Tests cover authentication, webhook management, media serving, webhook forwarding, and security edge cases.
- **Security & Validation:** Improved input validation, error handling, and security best practices throughout the backend.
- **Webhook Management System**: Implemented a system to create, list, and delete webhooks from a modern web UI. Each webhook has a unique URL and can be set to POST or GET.
- **WhatsMeow Integration**: Integrated WhatsMeow for real-time message handling, allowing users to connect their WhatsApp account and receive messages.
- **Webhook Forwarding Update**: The `forwardToWebhooks` function has been updated to load webhooks from the database instead of the JSON file. This ensures that messages are forwarded to the correct webhooks for each user. 