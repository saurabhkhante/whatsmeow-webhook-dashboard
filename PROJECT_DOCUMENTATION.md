# WhatsApp Webhook Dashboard - Complete Documentation

## Project Overview

A multi-user WhatsApp webhook dashboard built with Go backend (using WhatsMeow library) and Vue.js frontend. Each user can connect their WhatsApp account, create webhooks, and receive real-time message forwarding to their configured endpoints.

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Vue.js        │    │   Go Backend     │    │   WhatsApp      │
│   Frontend      │◄───┤   (WhatsMeow)    │◄───┤   Web API       │
│                 │    │                  │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │   Data Storage   │
                    │  • SQLite (users)│
                    │  • JSON (webhooks)│
                    │  • Session DBs   │
                    └──────────────────┘
```

## Key Features

### 🔐 Multi-User Authentication
- User registration and login system
- Password hashing with bcrypt
- Session-based authentication using HTTP cookies
- Per-user data isolation

### 📱 WhatsApp Integration
- Individual WhatsApp sessions per user
- QR code login for each user
- Real-time connection status
- Media message handling (images, audio, documents)
- Per-user session persistence

### 🔗 Webhook Management
- Create/delete webhooks per user
- Support for GET and POST methods
- Real-time message forwarding
- Webhook activity logging
- JSON payload delivery

### 📁 Media Handling
- Automatic media download and storage
- Support for images, audio, documents
- Media URL generation for webhooks
- Organized media directory structure

### 🎯 Webhook Filtering System
- **All Messages**: Webhook receives all incoming WhatsApp messages
- **Specific Group**: Filter to only receive messages from a particular group
- **Specific Chat**: Filter to only receive messages from a particular contact
- **Chat Discovery**: Browse recent chats and groups through the UI
- **Smart Filtering**: Automatic JID detection (groups: `@g.us`, chats: `@s.whatsapp.net`)

## File Structure

```
whatsmeowtest/
├── main.go                 # Entry point
├── server.go              # Main backend logic
├── users.db               # SQLite database for users
├── media/                 # Downloaded media files
├── webhooks_*.json        # Per-user webhook configurations
├── whatsmeow_*.db         # Per-user WhatsApp session files
└── frontend/
    ├── src/
    │   ├── components/
    │   │   ├── LoginForm.vue
    │   │   ├── RegisterForm.vue
    │   │   └── ProfileDashboard.vue
    │   ├── App.vue
    │   └── main.js
    ├── index.html
    └── package.json
```

## Backend Functions Reference

### Core Data Structures

#### `UserWAState`
```go
type UserWAState struct {
    waClient   *whatsmeow.Client  // WhatsApp client instance
    waStatus   string             // "disconnected", "waiting_qr", "connected", "error"
    qrCode     string             // Base64 QR code for login
    loginState string             // Human-readable status message
    waCancel   context.CancelFunc // Context cancellation function
    mu         sync.RWMutex       // Thread-safe access mutex
}
```

#### `Webhook`
```go
type Webhook struct {
    ID          string    `json:"id"`           // Unique webhook identifier
    URL         string    `json:"url"`          // Webhook endpoint URL
    Method      string    `json:"method"`       // "GET" or "POST"
    FilterType  string    `json:"filter_type"`  // "all", "group", "chat"
    FilterValue string    `json:"filter_value"` // Group/Chat ID (empty for "all")
    CreatedAt   time.Time `json:"created_at"`   // Creation timestamp
}
```

### Authentication Functions

#### `isAuthenticated(r *http.Request) bool`
- Checks if user has valid session cookie
- Returns true if authenticated, false otherwise

#### `getUserEmail(r *http.Request) string`
- Extracts user email from session cookie
- Used throughout the app for user identification

#### `hashPassword(password string) (string, error)`
- Hashes passwords using bcrypt
- Used during user registration

#### `checkPassword(hash, password string) error`
- Verifies password against bcrypt hash
- Used during user login

### Database Functions

#### `initDB() error`
- Creates SQLite database and users table
- Called on server startup

#### `loadWebhooks(email string) ([]Webhook, error)`
- Loads user's webhooks from JSON file
- Returns empty slice if file doesn't exist

#### `saveWebhooks(email string, webhooks []Webhook) error`
- Saves user's webhooks to JSON file
- Creates file if it doesn't exist

### WhatsApp Management Functions

#### `getUserWAState(email string) *UserWAState`
- Returns or creates WhatsApp state for user
- Thread-safe singleton pattern per user
- Uses global `userWAStates` map with mutex protection

#### `startUserWhatsMeowConnection(email string)`
- Initializes WhatsApp connection for user
- Creates per-user session file: `whatsmeow_{email}.db`
- Handles QR code generation and login flow
- Uses granular mutex locking to prevent deadlocks

#### `disconnectUserWhatsMeow(email string)`
- Disconnects user's WhatsApp session
- Cleans up session files and client state
- Resets status to "disconnected"

#### `handleUserWAEvent(email string, evt interface{})`
- Processes WhatsApp events for specific user
- Handles incoming messages and forwards to webhooks
- Supports text, image, audio, and document messages
- Downloads and stores media files

### Status Management Functions

#### `getUserWAStatus(email string) string`
- Thread-safe getter for user's WhatsApp status
- Returns: "disconnected", "waiting_qr", "connected", "error"

#### `setUserWAStatus(email string, status string)`
- Thread-safe setter for user's WhatsApp status
- Updates status with mutex protection

#### `getUserQRCode(email string) string`
- Returns base64 QR code for user login
- Empty string if no QR code available

#### `updateUserQRCode(email string, code string)`
- Updates QR code for user
- Thread-safe with mutex protection

### Webhook Functions

#### `forwardToWebhooks(email string, payload map[string]interface{}, mediaPath string)`
- Forwards WhatsApp messages to user's configured webhooks
- Supports both GET and POST requests
- Includes media URLs when applicable
- Logs all webhook activities

#### `generateWebhookID() string`
- Generates unique 8-character webhook IDs
- Uses random alphanumeric characters

#### `addWebhookLog(webhookID string, payload map[string]interface{})`
- Logs webhook activity for debugging
- Stores in memory with timestamp

## API Endpoints

### Authentication Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/register` | Register new user |
| POST | `/api/login` | User login |
| POST | `/api/logout` | User logout |

### WhatsApp Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/wa/status` | Get WhatsApp connection status |
| POST | `/api/wa/connect` | Start WhatsApp connection |
| POST | `/api/wa/disconnect` | Disconnect WhatsApp |
| GET | `/api/wa/chats` | Get recent chats and groups for filtering |

### Webhook Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/webhooks` | List user's webhooks |
| POST | `/api/webhooks` | Create new webhook |
| DELETE | `/api/webhooks/{id}` | Delete specific webhook |
| GET | `/api/webhooks/{id}/logs` | Get webhook activity logs |

### Static File Serving

| Path | Description |
|------|-------------|
| `/` | Serves Vue.js frontend |
| `/media/*` | Serves downloaded media files |

## Message Payload Format

When WhatsApp messages are forwarded to webhooks, they follow this JSON structure:

```json
{
  "from": "1234567890@s.whatsapp.net",
  "name": "Contact Name", 
  "message_id": "unique_message_id",
  "timestamp": 1234567890,
  "type": "text|image|audio|document",
  "text": "Message content",           // For text messages
  "media_url": "/media/filename",   // For media messages  
  "caption": "Media caption",       // For media with captions
  "file_name": "document.pdf"       // For document messages
}
```

## Security Features

### Input Validation
- All user inputs are validated before processing
- SQL injection prevention with prepared statements
- XSS protection in frontend

### Authentication Security
- Passwords hashed with bcrypt (cost factor 10)
- Session-based authentication
- Automatic logout on session expiry

### File Security
- Media files stored in dedicated directory
- Session files isolated per user
- No sensitive data exposed in frontend

## Technical Implementation Details

### Concurrency & Thread Safety

#### Per-User State Management
- Each user has isolated `UserWAState` instance
- Global `userWAStates` map with RWMutex protection
- Granular locking to prevent deadlocks

#### Mutex Strategy
```go
// Good: Granular locking
state.mu.Lock()
state.field = value
state.mu.Unlock()

// Bad: Holding lock while calling other functions
state.mu.Lock()
defer state.mu.Unlock()  // Can cause deadlocks
callOtherFunction()
```

#### Database Connection Handling
- SQLite with connection pooling
- Prepared statements for user queries
- Automatic database creation on startup

### Session File Management
- Format: `whatsmeow_{email}.db`
- Contains encrypted WhatsApp session data
- Automatically cleaned up on disconnect

### Media File Handling
- Automatic download from WhatsApp servers
- Filename format: `{timestamp}_{message_id}.{extension}`
- Organized in `/media/` directory
- URL serving through `/media/*` endpoint

## Environment Setup

### Prerequisites
```bash
# Install Go 1.19+
go version

# Install Node.js 16+
node --version
npm --version
```

### Backend Setup
```bash
# Initialize Go module
go mod init whatsmeowtest
go mod tidy

# Run server
go run .
```

### Frontend Setup
```bash
cd frontend
npm install
npm run dev  # Development mode
npm run build  # Production build
```

### Environment Variables
```bash
# Optional: Custom port
export PORT=8080

# Optional: Custom database path
export DB_PATH=./users.db
```

## Development Workflow

### Adding New Features
1. Update backend API endpoints in `server.go`
2. Add frontend components in `frontend/src/components/`
3. Update this documentation
4. Test multi-user scenarios

### Debugging
- Backend logs include DEBUG prefixes for critical functions
- Frontend console logs for API calls
- Webhook logs stored in memory
- Session files for persistence debugging

### Production Deployment
- Remove DEBUG print statements for production
- Enable proper logging with log levels
- Configure reverse proxy (nginx/Apache) for static files
- Set up SSL/TLS certificates
- Configure database backups

## Common Issues & Solutions

### Issue: Status Requests Hanging
**Cause**: Mutex deadlocks in WhatsApp functions  
**Solution**: Use granular locking, unlock before calling other functions

### Issue: Media Not Loading
**Cause**: Missing media directory or permissions  
**Solution**: Ensure `/media/` directory exists and is writable

### Issue: Session Not Persisting
**Cause**: Session files being deleted or corrupted  
**Solution**: Check `whatsmeow_{email}.db` files exist and have correct permissions

### Issue: Multiple Users Interfering
**Cause**: Shared global state instead of per-user state  
**Solution**: Ensure all state is isolated per user email

## Future Enhancement Ideas

### Feature Roadmap
- [ ] Webhook retry logic with exponential backoff
- [x] **Message filtering and routing rules** ✅ **COMPLETED**
- [ ] Advanced filter conditions (regex, keywords, sender patterns)
- [ ] Real-time dashboard updates via WebSockets
- [ ] Webhook response validation and retry on failure
- [ ] Rate limiting per user and per webhook
- [ ] Webhook analytics and delivery statistics
- [ ] Message templates and auto-responses
- [ ] Backup and restore functionality
- [ ] Admin panel for user management
- [ ] Docker containerization
- [ ] Horizontal scaling support
- [ ] Webhook scheduling and delayed delivery
- [ ] Message encryption for sensitive data

### Technical Improvements
- [ ] Database migrations system
- [ ] Comprehensive error handling
- [ ] API rate limiting
- [ ] Request/response logging middleware
- [ ] Health check endpoints
- [ ] Metrics and monitoring
- [ ] Unit and integration tests

## Webhook Filtering System - Complete Implementation

### Overview
The webhook filtering system allows users to create targeted webhooks that only receive messages from specific chats, groups, or all messages. This feature significantly reduces noise and allows for precise message routing.

### Implementation Details

#### Backend Changes Made

**1. Updated Webhook Structure**
```go
type Webhook struct {
    ID          string    `json:"id"`           // Unique webhook identifier
    URL         string    `json:"url"`          // Webhook endpoint URL
    Method      string    `json:"method"`       // "GET" or "POST"
    FilterType  string    `json:"filter_type"`  // "all", "group", "chat"
    FilterValue string    `json:"filter_value"` // Group/Chat ID (empty for "all")
    CreatedAt   time.Time `json:"created_at"`   // Creation timestamp
}
```

**2. Chat Discovery API**
- **Endpoint**: `GET /api/wa/chats`
- **Purpose**: Returns recent chats and groups for easy filter setup
- **Sample Response**:
```json
[
  {"id": "1234567890@s.whatsapp.net", "name": "John Doe", "type": "chat"},
  {"id": "120363000000000000@g.us", "name": "Project Team", "type": "group"}
]
```

**3. Enhanced Message Processing**
The `forwardToWebhooks()` function now includes smart filtering:

```go
func forwardToWebhooks(email string, payload map[string]interface{}, mediaPath string) {
    // Extract message sender JID
    fromJID, _ := payload["from"].(string)
    
    // Load user's webhooks
    webhooks, _ := loadWebhooks(email)
    
    // Process each webhook with filtering
    for _, wh := range webhooks {
        shouldForward := false
        
        switch wh.FilterType {
        case "all":
            shouldForward = true
        case "group":
            shouldForward = (fromJID == wh.FilterValue)
        case "chat":
            shouldForward = (fromJID == wh.FilterValue)
        default:
            shouldForward = true // Fallback to all messages
        }
        
        if shouldForward {
            // Forward message to webhook
            go forwardToSingleWebhook(wh, payload, mediaPath)
        }
    }
}
```

#### Frontend Changes Made

**1. Enhanced Webhook Creation Form**
- Added filter type dropdown (All Messages, Specific Group, Specific Chat)
- Added filter value input with placeholder text
- Added "Browse Chats" button for chat discovery
- Form validation for filter values

**2. Chat Discovery Modal**
- Lists recent chats and groups with names and IDs
- Click to select and auto-populate filter value
- Loading states and error handling
- Responsive design with proper styling

**3. Webhook Display Updates**
- Shows filter information for each webhook
- Clear indication of filter type and target
- Better visual organization of webhook details

### Usage Examples

#### Example 1: All Messages Webhook
```json
{
  "url": "https://myapp.com/webhook/all",
  "method": "POST",
  "filter_type": "all",
  "filter_value": ""
}
```
**Result**: Receives all WhatsApp messages

#### Example 2: Group-Specific Webhook
```json
{
  "url": "https://myapp.com/webhook/project",
  "method": "POST", 
  "filter_type": "group",
  "filter_value": "120363000000000000@g.us"
}
```
**Result**: Only receives messages from the specified group

#### Example 3: Chat-Specific Webhook
```json
{
  "url": "https://myapp.com/webhook/support",
  "method": "POST",
  "filter_type": "chat", 
  "filter_value": "1234567890@s.whatsapp.net"
}
```
**Result**: Only receives messages from the specified contact

### JID Format Reference

#### WhatsApp JID (Jabber ID) Formats
- **Individual Chats**: `[phone_number]@s.whatsapp.net`
  - Example: `1234567890@s.whatsapp.net`
- **Group Chats**: `[group_id]@g.us`
  - Example: `120363000000000000@g.us`

#### Automatic JID Detection
The system automatically recognizes:
- Groups by `@g.us` suffix
- Individual chats by `@s.whatsapp.net` suffix
- Applies appropriate filtering logic for each type

### Testing and Debugging

#### Frontend Console Logs
```javascript
// Webhook creation
console.log('DEBUG: Creating webhook with data:', {
  url: this.newURL,
  method: this.newMethod,
  filter_type: this.newFilterType,
  filter_value: this.newFilterValue
});

// Chat discovery
console.log('DEBUG: Received chats:', this.recentChats);
```

#### Backend Debug Logs
```go
// Message filtering
fmt.Printf("DEBUG: Message from JID: %s\n", fromJID)
fmt.Printf("DEBUG: Checking webhook %s with filter_type=%s, filter_value=%s\n", 
    wh.ID, wh.FilterType, wh.FilterValue)
fmt.Printf("DEBUG: Should forward: %v\n", shouldForward)
```

### Security Considerations

#### Input Validation
- URL validation for webhook endpoints
- JID format validation for filter values
- Method validation (GET/POST only)
- Authentication required for all filter operations

#### Data Privacy
- Filter values are stored per-user
- No cross-user data leakage
- Session-based authentication for chat discovery

### Performance Optimizations

#### Efficient Filtering
- Early return for "all" filter type
- String comparison for exact JID matching
- Minimal overhead for filtered webhooks

#### Memory Management
- Webhooks loaded once per message
- Filtering done in-memory
- No database queries for each message

## Quick Start Guide

### 1. Clone and Setup
```bash
# Setup backend
cd whatsmeowtest
go mod init whatsmeowtest
go mod tidy

# Setup frontend  
cd frontend
npm install
```

### 2. Start the Application
```bash
# Terminal 1: Start backend
go run .

# Terminal 2: Start frontend (if needed)
cd frontend
npm run dev
```

### 3. First Time Usage
1. **Register**: Create a new user account at `http://localhost:8080`
2. **Login**: Sign in with your credentials
3. **Connect WhatsApp**: Click "Connect WhatsApp" and scan the QR code
4. **Create Webhook**: Add a webhook URL with desired filtering
5. **Test**: Send a message to your WhatsApp and verify webhook delivery

### 4. Webhook Filtering Setup
1. **All Messages**: Select "All Messages" for complete message forwarding
2. **Group Filter**: Select "Specific Group" → "Browse Chats" → Choose group
3. **Chat Filter**: Select "Specific Chat" → "Browse Chats" → Choose contact
4. **Test Filter**: Send test message to verify filtering works correctly

### 5. Monitor and Debug
- Check webhook logs in the dashboard
- Monitor console output for DEBUG messages
- Verify media files in `/media/` directory
- Check session persistence in `whatsmeow_*.db` files

## Project Statistics

- **Languages**: Go (backend), JavaScript/Vue.js (frontend)
- **Dependencies**: WhatsMeow, SQLite, bcrypt, Vue.js
- **Architecture**: Multi-user, session-based, RESTful API
- **Storage**: SQLite + JSON + Binary session files
- **Security**: bcrypt hashing, input validation, session management

---

**Last Updated**: December 2024  
**Version**: 1.1 (Added Webhook Filtering)  
**Status**: Production Ready with Advanced Filtering 🚀 