package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	db *sql.DB
)

const (
	sessionCookieName = "session_id"
)

type Webhook struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Method      string    `json:"method"`       // "GET" or "POST"
	FilterType  string    `json:"filter_type"`  // "all", "group", "chat"
	FilterValue string    `json:"filter_value"` // Group/Chat ID (empty for "all")
	CreatedAt   time.Time `json:"created_at"`
}

type UserWebhooks struct {
	Webhooks []Webhook `json:"webhooks"`
}

// For recent chats endpoint
type Chat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "group" or "chat"
}

var webhookMu sync.Mutex

// --- Webhook log storage (in-memory, per webhook) ---
type WebhookLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

var webhookLogs = struct {
	mu   sync.Mutex
	logs map[string][]WebhookLogEntry // webhookID -> log entries
}{
	logs: make(map[string][]WebhookLogEntry),
}

// Track recent chats per user
var recentChats = struct {
	mu   sync.Mutex
	data map[string][]Chat // email -> recent chats
}{
	data: make(map[string][]Chat),
}

// --- Per-user WhatsApp session state ---
type UserWAState struct {
	waClient   *whatsmeow.Client
	waStatus   string // "disconnected", "waiting_qr", "connected", "error"
	qrCode     string
	loginState string
	waCancel   context.CancelFunc
	mu         sync.RWMutex
}

// Map of email -> UserWAState
var waUsers = struct {
	mu   sync.Mutex
	data map[string]*UserWAState
}{
	data: make(map[string]*UserWAState),
}

func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && cookie.Value != ""
}

func getUserWebhookFile(email string) string {
	return "webhooks_" + email + ".json"
}

func loadWebhooks(email string) ([]Webhook, error) {
	file := getUserWebhookFile(email)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return []Webhook{}, nil
	}
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var uw UserWebhooks
	if err := json.Unmarshal(data, &uw); err != nil {
		return nil, err
	}
	return uw.Webhooks, nil
}

func saveWebhooks(email string, webhooks []Webhook) error {
	file := getUserWebhookFile(email)
	uw := UserWebhooks{Webhooks: webhooks}
	data, err := json.MarshalIndent(uw, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(file, data, 0644)
}

func generateWebhookID() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, 16)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Helper: get the logged-in user's email from the session cookie
func getUserEmail(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	return cookie.Value
}

// Helper: get or create the UserWAState for a user
func getUserWAState(email string) *UserWAState {
	waUsers.mu.Lock()
	defer waUsers.mu.Unlock()
	state, ok := waUsers.data[email]
	if !ok {
		state = &UserWAState{waStatus: "disconnected"}
		waUsers.data[email] = state
	}
	return state
}

// Send the webhook HTTP request (POST or GET)
func sendWebhook(wh Webhook, payload map[string]interface{}) error {
	var req *http.Request
	var err error
	client := &http.Client{Timeout: 10 * time.Second}

	if wh.Method == "GET" {
		// For GET, encode payload as query params
		urlWithParams := wh.URL
		if len(payload) > 0 {
			q := url.Values{}
			for k, v := range payload {
				q.Set(k, fmt.Sprintf("%v", v))
			}
			if strings.Contains(urlWithParams, "?") {
				urlWithParams += "&" + q.Encode()
			} else {
				urlWithParams += "?" + q.Encode()
			}
		}
		req, err = http.NewRequest("GET", urlWithParams, nil)
	} else {
		// For POST, send JSON body
		data, _ := json.Marshal(payload)
		req, err = http.NewRequest("POST", wh.URL, bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Printf("DEBUG: Webhook %s sent, status: %d\n", wh.ID, resp.StatusCode)
	return nil
}

// Helper: Forward WhatsApp message to all user webhooks
func forwardToWebhooks(email string, payload map[string]interface{}, mediaPath string) {
	fmt.Printf("DEBUG: [FORWARD] user email: %s\n", email)
	userID, err := getUserIDByEmail(email)
	if err != nil {
		fmt.Printf("ERROR: [FORWARD] Could not get user ID for email %s: %v\n", email, err)
		return
	}
	fmt.Printf("DEBUG: [FORWARD] userID: %d\n", userID)

	// Extract message info for filtering and chat tracking
	fromJID, _ := payload["from"].(string)
	fromName, _ := payload["name"].(string)
	fmt.Printf("DEBUG: Message from JID: %s, Name: %s\n", fromJID, fromName)

	// Track recent chat for this user
	if fromJID != "" {
		chatType := "chat"
		if strings.HasSuffix(fromJID, "@g.us") {
			chatType = "group"
		}
		addRecentChat(email, fromJID, fromName, chatType)
	}

	// Load webhooks from the database for this user
	webhooks, err := dbListWebhooks(userID)
	if err != nil {
		fmt.Printf("ERROR: [FORWARD] Could not load webhooks for user %s: %v\n", email, err)
		return
	}
	fmt.Printf("DEBUG: Found %d webhooks for user %s\n", len(webhooks), email)

	for _, wh := range webhooks {
		fmt.Printf("DEBUG: Checking webhook %s with filter_type=%s, filter_value=%s\n",
			wh.ID, wh.FilterType, wh.FilterValue)

		// Check if message should be forwarded to this webhook
		shouldForward := false

		switch wh.FilterType {
		case "all", "":
			shouldForward = true
			fmt.Printf("DEBUG: Webhook %s accepts all messages\n", wh.ID)
		case "group":
			if fromJID != "" && strings.HasSuffix(fromJID, "@g.us") {
				if wh.FilterValue == "" || fromJID == wh.FilterValue {
					shouldForward = true
					fmt.Printf("DEBUG: Webhook %s accepts group message from %s\n", wh.ID, fromJID)
				}
			}
		case "chat":
			if fromJID != "" && strings.HasSuffix(fromJID, "@s.whatsapp.net") {
				if wh.FilterValue == "" || fromJID == wh.FilterValue {
					shouldForward = true
					fmt.Printf("DEBUG: Webhook %s accepts chat message from %s\n", wh.ID, fromJID)
				}
			}
		}

		if shouldForward {
			fmt.Printf("DEBUG: Forwarding to webhook %s (%s) at URL: %s\n", wh.ID, wh.Method, wh.URL)
			addWebhookLog(wh.ID, payload)
			err := sendWebhook(wh, payload)
			if err != nil {
				fmt.Printf("ERROR: Failed to send webhook: %v\n", err)
			}
		} else {
			fmt.Printf("DEBUG: Webhook %s filtered out message from %s\n", wh.ID, fromJID)
		}
	}
}

func addWebhookLog(webhookID string, payload map[string]interface{}) {
	webhookLogs.mu.Lock()
	defer webhookLogs.mu.Unlock()
	entry := WebhookLogEntry{
		Timestamp: time.Now(),
		Payload:   payload,
	}
	entries := webhookLogs.logs[webhookID]
	entries = append(entries, entry)
	if len(entries) > 5 {
		entries = entries[len(entries)-5:]
	}
	webhookLogs.logs[webhookID] = entries
}

func getWebhookLogs(webhookID string) []WebhookLogEntry {
	webhookLogs.mu.Lock()
	defer webhookLogs.mu.Unlock()
	return append([]WebhookLogEntry(nil), webhookLogs.logs[webhookID]...)
}

// Add or update recent chat for a user
func addRecentChat(email string, chatID string, chatName string, chatType string) {
	if chatID == "" {
		return // Skip empty chat IDs
	}

	fmt.Printf("DEBUG: Adding recent chat for %s: %s (%s) - %s\n", email, chatID, chatName, chatType)

	recentChats.mu.Lock()
	defer recentChats.mu.Unlock()

	chats := recentChats.data[email]

	// Check if chat already exists and update it
	for i, existingChat := range chats {
		if existingChat.ID == chatID {
			// Update existing chat (move to front and update name if provided)
			if chatName != "" {
				chats[i].Name = chatName
			}
			// Move to front
			chat := chats[i]
			copy(chats[1:i+1], chats[0:i])
			chats[0] = chat
			recentChats.data[email] = chats
			return
		}
	}

	// Add new chat at the front
	newChat := Chat{
		ID:   chatID,
		Name: chatName,
		Type: chatType,
	}
	chats = append([]Chat{newChat}, chats...)

	// Keep only the 10 most recent chats
	if len(chats) > 10 {
		chats = chats[:10]
	}

	recentChats.data[email] = chats
	fmt.Printf("DEBUG: Now tracking %d recent chats for %s\n", len(chats), email)
}

// Get recent chats for a user
func getRecentChats(email string) []Chat {
	recentChats.mu.Lock()
	defer recentChats.mu.Unlock()

	chats := recentChats.data[email]
	if chats == nil {
		return []Chat{}
	}

	// Return a copy to avoid concurrent modification
	result := make([]Chat, len(chats))
	copy(result, chats)
	return result
}

func initDB() error {
	var err error
	db, err = sql.Open("sqlite", "file:whatsmeow.db?mode=rwc&_pragma=foreign_keys(1)")
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}
	// Add webhooks table for DB-backed storage (now with user_id foreign key)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS webhooks (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		url TEXT NOT NULL,
		method TEXT NOT NULL,
		filter_type TEXT NOT NULL,
		filter_value TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	)`)
	return err
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func checkPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func startServer() {
	if err := initDB(); err != nil {
		panic("Failed to initialize DB: " + err.Error())
	}
	// --- API: Register ---
	http.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var creds struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil || creds.Email == "" || creds.Password == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		pwHash, err := hashPassword(creds.Password)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}
		_, err = db.Exec("INSERT INTO users (email, password_hash) VALUES (?, ?)", creds.Email, pwHash)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				http.Error(w, "Email already registered", http.StatusConflict)
				return
			}
			http.Error(w, "Failed to register", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	})
	// --- API: Login (updated for DB users) ---
	http.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var creds struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		err := json.NewDecoder(r.Body).Decode(&creds)
		if err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		var pwHash string
		row := db.QueryRow("SELECT password_hash FROM users WHERE email = ?", creds.Email)
		err = row.Scan(&pwHash)
		if err == sql.ErrNoRows {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		if checkPassword(pwHash, creds.Password) != nil {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    creds.Email,
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Now().Add(24 * time.Hour),
		})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	})

	// --- API: Logout ---
	http.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Now().Add(-1 * time.Hour),
		})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	})

	// --- API: Session Status ---
	http.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if isAuthenticated(r) {
			w.Write([]byte(`{"authenticated":true}`))
		} else {
			w.Write([]byte(`{"authenticated":false}`))
		}
	})

	// --- API: QR PNG (existing) ---
	http.HandleFunc("/qr.png", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r)
		code := getUserQRCode(email)
		if code == "" {
			http.NotFound(w, r)
			return
		}
		png, err := qrcode.Encode(code, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})

	// --- API: WhatsApp Status ---
	http.HandleFunc("/api/wa/status", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"authenticated":false}`))
			return
		}
		email := getUserEmail(r)

		status := getUserWAStatus(email)
		qr := getUserQRCode(email)
		loginState := getUserLoginState(email)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"status":     status,
			"qr":         qr,
			"loginState": loginState,
		}
		json.NewEncoder(w).Encode(resp)
	})

	// --- API: WhatsMeow Connect ---
	http.HandleFunc("/api/wa/connect", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r)
		if getUserWAStatus(email) == "connected" {
			w.Write([]byte(`{"success":true,"message":"Already connected"}`))
			return
		}

		// Start connection in background
		go startUserWhatsMeowConnection(email)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"message":"Connecting..."}`))
	})

	// --- API: WhatsMeow Disconnect ---
	http.HandleFunc("/api/wa/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r)
		disconnectUserWhatsMeow(email)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"disconnected"}`))
	})

	// --- API: List Webhooks ---
	http.HandleFunc("/api/webhooks", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r)
		userID, err := getUserIDByEmail(email)
		if err != nil {
			fmt.Println("ERROR: Could not get user ID for email", email, err)
			http.Error(w, "Failed to get user ID", http.StatusInternalServerError)
			return
		}
		webhooks, err := dbListWebhooks(userID)
		if err != nil {
			fmt.Println("ERROR: Could not list webhooks for user", userID, err)
			http.Error(w, "Failed to load webhooks", http.StatusInternalServerError)
			return
		}
		if webhooks == nil {
			webhooks = []Webhook{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(webhooks)
	})

	// --- API: Create Webhook ---
	http.HandleFunc("/api/webhooks/create", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("DEBUG: /api/webhooks/create called")
		if !isAuthenticated(r) {
			fmt.Println("DEBUG: Not authenticated for webhook creation")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			URL         string `json:"url"`
			Method      string `json:"method"`
			FilterType  string `json:"filter_type"`
			FilterValue string `json:"filter_value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fmt.Println("DEBUG: Failed to decode request:", err)
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		// Validate method
		if req.Method != "GET" && req.Method != "POST" {
			fmt.Println("DEBUG: Invalid method:", req.Method)
			http.Error(w, "Invalid method", http.StatusBadRequest)
			return
		}
		// Validate filter type
		if req.FilterType != "" && req.FilterType != "all" && req.FilterType != "group" && req.FilterType != "chat" {
			fmt.Println("DEBUG: Invalid filter type:", req.FilterType)
			http.Error(w, "Invalid filter type", http.StatusBadRequest)
			return
		}
		// Default to "all" if no filter type specified
		if req.FilterType == "" {
			req.FilterType = "all"
		}
		email := getUserEmail(r)
		userID, err := getUserIDByEmail(email)
		if err != nil {
			fmt.Println("ERROR: Could not get user ID for email", email, err)
			http.Error(w, "Failed to get user ID", http.StatusInternalServerError)
			return
		}
		fmt.Printf("DEBUG: [CREATE] user email: %s, userID: %d\n", email, userID)
		fmt.Printf("DEBUG: Creating webhook for %s: URL=%s, Method=%s, FilterType=%s, FilterValue=%s\n",
			email, req.URL, req.Method, req.FilterType, req.FilterValue)
		id := generateWebhookID()
		wh := Webhook{
			ID:          id,
			URL:         req.URL,
			Method:      req.Method,
			FilterType:  req.FilterType,
			FilterValue: req.FilterValue,
			CreatedAt:   time.Now(),
		}
		err = dbCreateWebhook(userID, wh)
		if err != nil {
			fmt.Println("ERROR: Could not create webhook in DB", err)
			http.Error(w, "Failed to create webhook", http.StatusInternalServerError)
			return
		}
		fmt.Printf("DEBUG: Webhook created with ID: %s\n", id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           id,
			"url":          req.URL,
			"method":       req.Method,
			"filter_type":  req.FilterType,
			"filter_value": req.FilterValue,
		})
	})

	// --- API: Delete Webhook ---
	http.HandleFunc("/api/webhooks/delete", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		email := getUserEmail(r)
		userID, err := getUserIDByEmail(email)
		if err != nil {
			fmt.Println("ERROR: Could not get user ID for email", email, err)
			http.Error(w, "Failed to get user ID", http.StatusInternalServerError)
			return
		}
		err = dbDeleteWebhook(userID, req.ID)
		if err != nil {
			fmt.Println("ERROR: Could not delete webhook in DB", err)
			http.Error(w, "Failed to delete webhook", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	})

	// --- API: Webhook Logs ---
	http.HandleFunc("/api/webhooks/logs", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id", http.StatusBadRequest)
			return
		}
		logs := getWebhookLogs(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	})

	// --- API: Recent Chats ---
	http.HandleFunc("/api/wa/chats", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("DEBUG: /api/wa/chats called")
		if !isAuthenticated(r) {
			fmt.Println("DEBUG: Not authenticated for chats endpoint")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r)
		fmt.Println("DEBUG: Getting recent chats for:", email)

		// Get recent chats for this user
		chats := getRecentChats(email)
		fmt.Printf("DEBUG: Found %d recent chats for user %s\n", len(chats), email)

		// If no recent chats available, return empty array
		if len(chats) == 0 {
			fmt.Println("DEBUG: No recent chats found, returning empty array")
			chats = []Chat{} // Ensure it's not nil
		}

		// Log the chats being returned for debugging
		for i, chat := range chats {
			fmt.Printf("DEBUG: Chat %d: ID=%s, Name=%s, Type=%s\n", i+1, chat.ID, chat.Name, chat.Type)
		}

		fmt.Printf("DEBUG: Returning %d chats\n", len(chats))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chats)
	})

	// --- Serve media files ---
	http.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		mediaFile := path.Base(r.URL.Path)
		filePath := path.Join("media", mediaFile)
		f, err := os.Open(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		// Set content type based on file extension (simple)
		ext := path.Ext(mediaFile)
		if ext == ".jpg" || ext == ".jpeg" {
			w.Header().Set("Content-Type", "image/jpeg")
		} else if ext == ".png" {
			w.Header().Set("Content-Type", "image/png")
		} else if ext == ".mp3" {
			w.Header().Set("Content-Type", "audio/mpeg")
		} else if ext == ".ogg" {
			w.Header().Set("Content-Type", "audio/ogg")
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		io.Copy(w, f)
	})

	// --- Webhook receiver endpoint (stub) ---
	http.HandleFunc("/webhook/", func(w http.ResponseWriter, r *http.Request) {
		id := path.Base(r.URL.Path)
		if id == "" {
			http.NotFound(w, r)
			return
		}
		// For now, just log the request
		fmt.Printf("Received webhook call for id: %s\n", id)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	})

	// Serve static files from frontend/dist
	staticDir := "frontend/dist"
	fs := http.FileServer(http.Dir(staticDir))

	// Catch-all handler for frontend (except /api/ and /qr.png)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If the request is for an API or QR endpoint, skip
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/qr.png" {
			http.NotFound(w, r)
			return
		}
		// Try to serve static file
		path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		// Fallback: serve index.html for SPA routing
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})

	fmt.Println("Starting web server at http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Failed to start server:", err)
	}
}

// Update QR code for a specific user
func updateUserQRCode(email string, code string) {
	state := getUserWAState(email)
	state.mu.Lock()
	state.qrCode = code
	state.mu.Unlock()
}

// Update login state for a specific user
func updateUserLoginState(email string, state string) {
	userState := getUserWAState(email)
	userState.mu.Lock()
	userState.loginState = state
	userState.mu.Unlock()
}

// Get QR code for a specific user
func getUserQRCode(email string) string {
	state := getUserWAState(email)
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.qrCode
}

// Get login state for a specific user
func getUserLoginState(email string) string {
	state := getUserWAState(email)
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.loginState
}

// Set WhatsApp status for a specific user
func setUserWAStatus(email string, status string) {
	state := getUserWAState(email)
	state.mu.Lock()
	state.waStatus = status
	state.mu.Unlock()
}

// Get WhatsApp status for a specific user
func getUserWAStatus(email string) string {
	state := getUserWAState(email)
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.waStatus
}

// Handle WhatsApp events for a specific user
func handleUserWAEvent(email string, evt interface{}) {
	state := getUserWAState(email)
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return // Ignore own messages
		}
		msg := v.Message
		if msg == nil {
			return
		}
		// Prepare payload
		payload := map[string]interface{}{
			"from":      v.Info.Sender.String(),
			"to":        v.Info.Chat.String(),
			"timestamp": v.Info.Timestamp.Unix(),
			"id":        v.Info.ID,
		}

		// Try to get contact name
		if v.Info.PushName != "" {
			payload["name"] = v.Info.PushName
		} else if v.Info.Sender.User != "" {
			payload["name"] = v.Info.Sender.User
		}

		mediaPath := ""
		// Text message
		if msg.GetConversation() != "" {
			payload["type"] = "text"
			payload["text"] = msg.GetConversation()
		} else if img := msg.GetImageMessage(); img != nil {
			payload["type"] = "image"
			filename := fmt.Sprintf("%d_%s.jpg", time.Now().UnixNano(), v.Info.ID)
			os.MkdirAll("media", 0755)
			f, err := os.Create(path.Join("media", filename))
			if err == nil {
				data, err := state.waClient.Download(context.Background(), img)
				if err == nil {
					f.Write(data)
					f.Close()
					mediaPath = "/media/" + filename
					payload["media_url"] = mediaPath
					payload["caption"] = img.GetCaption()
				}
			}
		} else if audio := msg.GetAudioMessage(); audio != nil {
			payload["type"] = "audio"
			filename := fmt.Sprintf("%d_%s.ogg", time.Now().UnixNano(), v.Info.ID)
			os.MkdirAll("media", 0755)
			f, err := os.Create(path.Join("media", filename))
			if err == nil {
				data, err := state.waClient.Download(context.Background(), audio)
				if err == nil {
					f.Write(data)
					f.Close()
					mediaPath = "/media/" + filename
					payload["media_url"] = mediaPath
				}
			}
		} else if doc := msg.GetDocumentMessage(); doc != nil {
			payload["type"] = "document"
			filename := fmt.Sprintf("%d_%s_%s", time.Now().UnixNano(), v.Info.ID, doc.GetFileName())
			os.MkdirAll("media", 0755)
			f, err := os.Create(path.Join("media", filename))
			if err == nil {
				data, err := state.waClient.Download(context.Background(), doc)
				if err == nil {
					f.Write(data)
					f.Close()
					mediaPath = "/media/" + filename
					payload["media_url"] = mediaPath
					payload["file_name"] = doc.GetFileName()
				}
			}
		}
		// Forward to user's webhooks
		forwardToWebhooks(email, payload, mediaPath)
	}
}

// Start WhatsApp connection for a specific user
func startUserWhatsMeowConnection(email string) {
	fmt.Println("DEBUG: startUserWhatsMeowConnection called for:", email)
	state := getUserWAState(email)

	// Check if already started (with mutex protection)
	state.mu.Lock()
	if state.waClient != nil {
		fmt.Println("DEBUG: WhatsApp client already exists for:", email)
		state.mu.Unlock()
		return // already started
	}
	state.mu.Unlock()

	fmt.Println("DEBUG: Creating new WhatsApp connection for:", email)
	ctx, cancel := context.WithCancel(context.Background())

	// Set cancel function (with mutex protection)
	state.mu.Lock()
	state.waCancel = cancel
	state.mu.Unlock()

	// Use user-specific session file
	sessionFile := fmt.Sprintf("whatsmeow_%s.db", email)
	fmt.Println("DEBUG: Using session file:", sessionFile)
	container, err := sqlstore.New(ctx, "sqlite", fmt.Sprintf("file:%s?mode=rwc&_pragma=foreign_keys(1)", sessionFile), nil)
	if err != nil {
		fmt.Println("DEBUG: Failed to create store:", err)
		setUserWAStatus(email, "error")
		updateUserLoginState(email, "Failed to create store: "+err.Error())
		return
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		fmt.Println("DEBUG: Failed to get device:", err)
		setUserWAStatus(email, "error")
		updateUserLoginState(email, "Failed to get device: "+err.Error())
		return
	}

	fmt.Println("DEBUG: Creating WhatsApp client...")
	client := whatsmeow.NewClient(deviceStore, nil)

	// Set client (with mutex protection)
	state.mu.Lock()
	state.waClient = client
	state.mu.Unlock()

	// Add event handler for this user
	client.AddEventHandler(func(evt interface{}) {
		handleUserWAEvent(email, evt)
	})

	if client.Store.ID == nil {
		fmt.Println("DEBUG: Need to login, getting QR channel...")
		// Need to login
		qrChan, qrErr := client.GetQRChannel(ctx)
		if qrErr != nil {
			fmt.Println("DEBUG: Failed to get QR channel:", qrErr)
			setUserWAStatus(email, "error")
			updateUserLoginState(email, "Failed to get QR channel: "+qrErr.Error())
			return
		}

		fmt.Println("DEBUG: Setting status to waiting_qr")
		setUserWAStatus(email, "waiting_qr")
		updateUserLoginState(email, "Waiting for QR code scan...")

		go func() {
			fmt.Println("DEBUG: Starting client.Connect() in goroutine...")
			err := client.Connect()
			if err != nil {
				fmt.Println("DEBUG: client.Connect() failed:", err)
				setUserWAStatus(email, "error")
				updateUserLoginState(email, "Failed to connect: "+err.Error())
				return
			}
			fmt.Println("DEBUG: client.Connect() successful")
		}()

		go func() {
			fmt.Println("DEBUG: Starting QR code listener...")
			for evt := range qrChan {
				fmt.Println("DEBUG: QR event received:", evt.Event)
				if evt.Event == "code" {
					fmt.Println("DEBUG: Got QR code, updating...")
					updateUserQRCode(email, evt.Code)
					setUserWAStatus(email, "waiting_qr")
					updateUserLoginState(email, "Waiting for QR code scan...")
				} else if evt.Event == "error" {
					fmt.Println("DEBUG: QR channel error:", evt.Error)
					setUserWAStatus(email, "error")
					updateUserLoginState(email, "QR channel error: "+evt.Error.Error())
					break
				} else {
					fmt.Println("DEBUG: Login event:", evt.Event)
					updateUserLoginState(email, "Login event: "+evt.Event)
					if evt.Event == "success" {
						setUserWAStatus(email, "connected")
						updateUserLoginState(email, "Successfully logged in!")
						updateUserQRCode(email, "")
						break
					} else if evt.Event == "timeout" {
						setUserWAStatus(email, "disconnected")
						updateUserLoginState(email, "QR code timed out. Please try again.")
						updateUserQRCode(email, "")
						break
					}
				}
			}
			fmt.Println("DEBUG: QR code listener finished")
		}()
	} else {
		fmt.Println("DEBUG: Already logged in, connecting...")
		// Already logged in
		go func() {
			err := client.Connect()
			if err != nil {
				fmt.Println("DEBUG: Connect failed for existing session:", err)
				setUserWAStatus(email, "error")
				updateUserLoginState(email, "Failed to connect: "+err.Error())
				return
			}
			fmt.Println("DEBUG: Connected with existing session")
			setUserWAStatus(email, "connected")
			updateUserLoginState(email, "Already logged in!")
		}()
	}
	fmt.Println("DEBUG: startUserWhatsMeowConnection finished setup for:", email)
}

// Disconnect WhatsApp for a specific user
func disconnectUserWhatsMeow(email string) {
	state := getUserWAState(email)
	state.mu.Lock()

	if state.waCancel != nil {
		state.waCancel()
		state.waCancel = nil
	}

	if state.waClient != nil {
		state.waClient.Disconnect()
		state.waClient = nil
	}

	state.mu.Unlock()

	// Remove user's session file
	sessionFile := fmt.Sprintf("whatsmeow_%s.db", email)
	os.Remove(sessionFile)

	setUserWAStatus(email, "disconnected")
	updateUserQRCode(email, "")
	updateUserLoginState(email, "Disconnected")
}

// Get user_id from email
func getUserIDByEmail(email string) (int64, error) {
	var id int64
	row := db.QueryRow("SELECT id FROM users WHERE email = ?", email)
	err := row.Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// Create a webhook in the DB
func dbCreateWebhook(userID int64, wh Webhook) error {
	_, err := db.Exec(`INSERT INTO webhooks (id, user_id, url, method, filter_type, filter_value, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		wh.ID, userID, wh.URL, wh.Method, wh.FilterType, wh.FilterValue, wh.CreatedAt)
	return err
}

// List all webhooks for a user from the DB
func dbListWebhooks(userID int64) ([]Webhook, error) {
	rows, err := db.Query(`SELECT id, url, method, filter_type, filter_value, created_at FROM webhooks WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var webhooks []Webhook
	for rows.Next() {
		var wh Webhook
		var createdAt string
		err := rows.Scan(&wh.ID, &wh.URL, &wh.Method, &wh.FilterType, &wh.FilterValue, &createdAt)
		if err != nil {
			return nil, err
		}
		wh.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		webhooks = append(webhooks, wh)
	}
	return webhooks, nil
}

// Delete a webhook by ID for a user
func dbDeleteWebhook(userID int64, webhookID string) error {
	_, err := db.Exec(`DELETE FROM webhooks WHERE user_id = ? AND id = ?`, userID, webhookID)
	return err
}
