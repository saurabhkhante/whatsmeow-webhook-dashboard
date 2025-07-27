package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	db *sql.DB
	// Anti-detection and queue system
	messageQueues = make(map[string]*MessageQueue) // Per user message queues
	queueMutex    sync.RWMutex
)

// --- Anti-detection constants ---
const (
	MESSAGE_DELAY         = 1 * time.Second    // 1 message per second
	BURST_ALLOWANCE      = 5                   // Allow 5 rapid messages
	BURST_COOLDOWN       = 3 * time.Second     // Then 3 second cooldown
	MAX_QUEUE_PER_USER   = 50                  // Max messages in queue per user
	MAX_RETRIES          = 3                   // Retry failed messages 3 times
	MAX_HOURLY_MESSAGES  = 200                 // Per user hourly limit
	MAX_DAILY_MESSAGES   = 1000                // Per user daily limit
)

// --- Message Queue System ---
type QueuedMessage struct {
	ID          string    `json:"id"`
	UserEmail   string    `json:"user_email"`
	ChatJID     string    `json:"chat_jid"`
	Message     string    `json:"message"`
	CallbackURL string    `json:"callback_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Retries     int       `json:"retries"`
	Status      string    `json:"status"` // "queued", "sending", "sent", "failed"
}

type MessageQueue struct {
	UserEmail      string
	Messages       []*QueuedMessage
	LastSent       time.Time
	BurstCount     int
	HourlyCount    int
	DailyCount     int
	HourlyReset    time.Time
	DailyReset     time.Time
	IsProcessing   bool
	mu             sync.RWMutex
}

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

func isAuthenticated(r *http.Request, sessionCookieName string) bool {
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && cookie.Value != ""
}

func generateWebhookID() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, 16)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// --- Queue Management Functions ---

func generateMessageID() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, 12)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return "msg_" + string(b)
}

func getOrCreateQueue(userEmail string) *MessageQueue {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	
	if queue, exists := messageQueues[userEmail]; exists {
		return queue
	}
	
	now := time.Now()
	queue := &MessageQueue{
		UserEmail:   userEmail,
		Messages:    make([]*QueuedMessage, 0),
		HourlyReset: now.Add(time.Hour),
		DailyReset:  now.Add(24 * time.Hour),
	}
	messageQueues[userEmail] = queue
	return queue
}

func (q *MessageQueue) canSendMessage() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	now := time.Now()
	
	// Reset counters if needed
	if now.After(q.HourlyReset) {
		q.HourlyCount = 0
		q.HourlyReset = now.Add(time.Hour)
	}
	if now.After(q.DailyReset) {
		q.DailyCount = 0
		q.DailyReset = now.Add(24 * time.Hour)
	}
	
	// Check daily limit
	if q.DailyCount >= MAX_DAILY_MESSAGES {
		return false
	}
	
	// Check hourly limit
	if q.HourlyCount >= MAX_HOURLY_MESSAGES {
		return false
	}
	
	return true
}

func (q *MessageQueue) addMessage(msg *QueuedMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if len(q.Messages) >= MAX_QUEUE_PER_USER {
		return fmt.Errorf("queue full (max %d messages)", MAX_QUEUE_PER_USER)
	}
	
	q.Messages = append(q.Messages, msg)
	
	// Start processing if not already running
	if !q.IsProcessing {
		q.IsProcessing = true
		go q.processQueue()
	}
	
	return nil
}

func (q *MessageQueue) getQueuePosition(msgID string) int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	for i, msg := range q.Messages {
		if msg.ID == msgID {
			return i + 1
		}
	}
	return -1
}

func (q *MessageQueue) estimateDelay(position int) time.Duration {
	if position <= 0 {
		return 0
	}
	
	baseDelay := time.Duration(position-1) * MESSAGE_DELAY
	
	// Add burst cooldown if we're past burst allowance
	burstCycles := (position - 1) / BURST_ALLOWANCE
	if burstCycles > 0 {
		baseDelay += time.Duration(burstCycles) * BURST_COOLDOWN
	}
	
	return baseDelay
}

// --- Anti-Detection Functions ---

func addHumanDelay() {
	// Random delay between 500ms-2000ms to simulate human typing
	delay := time.Duration(500+rand.Intn(1500)) * time.Millisecond
	time.Sleep(delay)
}

func simulateTyping(client *whatsmeow.Client, chatJID types.JID, message string) {
	if client == nil {
		return
	}
	
	// Calculate typing duration based on message length (simulate ~50 chars per second)
	typingDuration := time.Duration(len(message)*20) * time.Millisecond
	if typingDuration > 5*time.Second {
		typingDuration = 5 * time.Second
	}
	if typingDuration < 500*time.Millisecond {
		typingDuration = 500 * time.Millisecond
	}
	
	// Send typing indicator
	client.SendChatPresence(chatJID, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	time.Sleep(typingDuration)
	client.SendChatPresence(chatJID, types.ChatPresencePaused, types.ChatPresenceMediaText)
	
	// Small pause after typing before sending
	time.Sleep(time.Duration(100+rand.Intn(300)) * time.Millisecond)
}

func isSpamPattern(message string, userEmail string) bool {
	// Convert to lowercase for case-insensitive checking
	lowerMsg := strings.ToLower(message)
	
	// Spam keywords to avoid
	spamKeywords := []string{
		"buy now", "limited time", "click here", "free money", "earn money",
		"get rich", "make money fast", "investment opportunity", "guaranteed profit",
		"call now", "act now", "offer expires", "special deal", "discount",
		"promotion", "sale", "bitcoin", "crypto investment", "trading bot",
		"mlm", "pyramid", "referral bonus", "commission", "affiliate",
	}
	
	// Check for spam keywords
	for _, keyword := range spamKeywords {
		if strings.Contains(lowerMsg, keyword) {
			fmt.Printf("WARNING: Potential spam detected in message from %s: contains '%s'\n", userEmail, keyword)
			return true
		}
	}
	
	// Check for excessive capitalization (more than 70% caps)
	if len(message) > 10 {
		capsCount := 0
		letterCount := 0
		for _, char := range message {
			if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
				letterCount++
				if char >= 'A' && char <= 'Z' {
					capsCount++
				}
			}
		}
		if letterCount > 0 && float64(capsCount)/float64(letterCount) > 0.7 {
			fmt.Printf("WARNING: Excessive capitalization detected in message from %s\n", userEmail)
			return true
		}
	}
	
	// Check for excessive repetition of characters
	if len(message) > 5 {
		for i := 0; i < len(message)-4; i++ {
			char := message[i]
			if char != ' ' {
				repeatCount := 1
				for j := i + 1; j < len(message) && j < i+10; j++ {
					if message[j] == char {
						repeatCount++
					} else {
						break
					}
				}
				if repeatCount >= 5 {
					fmt.Printf("WARNING: Excessive character repetition detected in message from %s\n", userEmail)
					return true
				}
			}
		}
	}
	
	// Check for excessive emojis (more than 30% of message)
	emojiCount := 0
	runeCount := 0
	for _, r := range message {
		runeCount++
		// Simple emoji detection (Unicode ranges)
		if (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		   (r >= 0x1F300 && r <= 0x1F5FF) || // Misc symbols
		   (r >= 0x1F680 && r <= 0x1F6FF) || // Transport
		   (r >= 0x2600 && r <= 0x26FF) ||   // Misc symbols
		   (r >= 0x2700 && r <= 0x27BF) {    // Dingbats
			emojiCount++
		}
	}
	if runeCount > 0 && float64(emojiCount)/float64(runeCount) > 0.3 {
		fmt.Printf("WARNING: Excessive emojis detected in message from %s\n", userEmail)
		return true
	}
	
	return false
}

func sendCallback(callbackURL, queueID, status string, messageID interface{}) {
	if callbackURL == "" {
		return
	}
	
	payload := map[string]interface{}{
		"queue_id": queueID,
		"status":   status,
		"sent_at":  time.Now().UTC().Format(time.RFC3339),
	}
	
	if messageID != nil {
		payload["message_id"] = messageID
	}
	
	payloadBytes, _ := json.Marshal(payload)
	
	go func() {
		resp, err := http.Post(callbackURL, "application/json", bytes.NewBuffer(payloadBytes))
		if err != nil {
			fmt.Printf("ERROR: Failed to send callback to %s: %v\n", callbackURL, err)
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("SUCCESS: Callback sent to %s for queue %s\n", callbackURL, queueID)
		} else {
			fmt.Printf("WARNING: Callback to %s returned status %d for queue %s\n", callbackURL, resp.StatusCode, queueID)
		}
	}()
}

// --- Queue Processing ---

func (q *MessageQueue) processQueue() {
	defer func() {
		q.mu.Lock()
		q.IsProcessing = false
		q.mu.Unlock()
	}()
	
	for {
		q.mu.Lock()
		if len(q.Messages) == 0 {
			q.mu.Unlock()
			break
		}
		
		// Get the next message
		msg := q.Messages[0]
		q.Messages = q.Messages[1:]
		q.mu.Unlock()
		
		// Check if we can send (rate limiting)
		if !q.canSendMessage() {
			// Put message back at front and wait
			q.mu.Lock()
			q.Messages = append([]*QueuedMessage{msg}, q.Messages...)
			q.mu.Unlock()
			time.Sleep(time.Minute) // Wait a minute before retrying
			continue
		}
		
		// Apply rate limiting delays
		q.mu.Lock()
		now := time.Now()
		
		// Check if we need burst cooldown
		if q.BurstCount >= BURST_ALLOWANCE {
			timeSinceLastBurst := now.Sub(q.LastSent)
			if timeSinceLastBurst < BURST_COOLDOWN {
				waitTime := BURST_COOLDOWN - timeSinceLastBurst
				q.mu.Unlock()
				fmt.Printf("INFO: Burst cooldown, waiting %v for user %s\n", waitTime, q.UserEmail)
				time.Sleep(waitTime)
				q.mu.Lock()
				q.BurstCount = 0 // Reset burst count after cooldown
			} else {
				q.BurstCount = 0 // Reset if enough time has passed
			}
		}
		
		// Apply normal message delay
		if !q.LastSent.IsZero() {
			timeSinceLastMessage := now.Sub(q.LastSent)
			if timeSinceLastMessage < MESSAGE_DELAY {
				waitTime := MESSAGE_DELAY - timeSinceLastMessage
				q.mu.Unlock()
				time.Sleep(waitTime)
				q.mu.Lock()
			}
		}
		
		q.mu.Unlock()
		
		// Send the message
		success := q.sendMessage(msg)
		
		q.mu.Lock()
		if success {
			q.LastSent = time.Now()
			q.BurstCount++
			q.HourlyCount++
			q.DailyCount++
			msg.Status = "sent"
			fmt.Printf("SUCCESS: Sent queued message %s for user %s\n", msg.ID, q.UserEmail)
		} else {
			msg.Retries++
			if msg.Retries < MAX_RETRIES {
				// Put back in queue for retry
				q.Messages = append(q.Messages, msg)
				msg.Status = "retrying"
				fmt.Printf("RETRY: Message %s failed, retry %d/%d for user %s\n", msg.ID, msg.Retries, MAX_RETRIES, q.UserEmail)
			} else {
				msg.Status = "failed"
				fmt.Printf("FAILED: Message %s failed permanently after %d retries for user %s\n", msg.ID, MAX_RETRIES, q.UserEmail)
				sendCallback(msg.CallbackURL, msg.ID, "failed", nil)
			}
		}
		q.mu.Unlock()
		
		// Random delay between messages to appear more human
		addHumanDelay()
	}
}

func (q *MessageQueue) sendMessage(msg *QueuedMessage) bool {
	// Get WhatsApp client for this user
	state := getUserWAState(msg.UserEmail)
	state.mu.RLock()
	client := state.waClient
	state.mu.RUnlock()
	
	if client == nil {
		fmt.Printf("ERROR: WhatsApp client not connected for user %s\n", msg.UserEmail)
		return false
	}
	
	// Parse chat JID
	chatJID, err := types.ParseJID(msg.ChatJID)
	if err != nil {
		fmt.Printf("ERROR: Invalid chat JID %s: %v\n", msg.ChatJID, err)
		return false
	}
	
	// Anti-detection: simulate human behavior
	simulateTyping(client, chatJID, msg.Message)
	
	// Send the message
	msgID, err := client.SendMessage(context.Background(), chatJID, &waProto.Message{
		Conversation: &msg.Message,
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to send message %s: %v\n", msg.ID, err)
		return false
	}
	
	// Send success callback
	sendCallback(msg.CallbackURL, msg.ID, "sent", msgID)
	
	return true
}

// Helper: get the logged-in user's email from the session cookie
func getUserEmail(r *http.Request, sessionCookieName string) string {
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
func sendWebhook(wh Webhook, payload map[string]interface{}, webhookURL string, method string) error {
	var req *http.Request
	var err error
	client := &http.Client{Timeout: 10 * time.Second}

	if method == "GET" {
		// For GET, encode payload as query params
		urlWithParams := webhookURL
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
		req, err = http.NewRequest("POST", webhookURL, bytes.NewBuffer(data))
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
func forwardToWebhooks(email string, payload map[string]interface{}, mediaPath string, mediaDir string) {
	fmt.Printf("DEBUG: [FORWARD] user email: %s\n", email)
	userID, err := getUserIDByEmail(email)
	if err != nil {
		fmt.Printf("ERROR: [FORWARD] Could not get user ID for email %s: %v\n", email, err)
		return
	}
	fmt.Printf("DEBUG: [FORWARD] userID: %d\n", userID)

	// Extract message info for filtering and chat tracking
	fromJID, _ := payload["from"].(string)     // Individual sender
	chatJID, _ := payload["to"].(string)       // Chat/Group where message was sent
	fromName, _ := payload["name"].(string)
	fmt.Printf("DEBUG: Message from JID: %s, in Chat: %s, Name: %s\n", fromJID, chatJID, fromName)

	// Track recent chat for this user (use chatJID for tracking, not fromJID)
	if chatJID != "" {
		chatType := "chat"
		if strings.HasSuffix(chatJID, "@g.us") {
			chatType = "group"
		}
		addRecentChat(email, chatJID, fromName, chatType)
	}

	// Load webhooks from the database for this user
	webhooks, err := dbListWebhooks(userID)
	if err != nil {
		fmt.Printf("ERROR: [FORWARD] Could not load webhooks for user %s: %v\n", email, err)
		return
	}
	fmt.Printf("DEBUG: Found %d webhooks for user %s\n", len(webhooks), email)

	// Load BASE_URL from environment
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		fmt.Println("ERROR: BASE_URL environment variable is not set. Media URLs will be invalid for external services.")
	}

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
			// For group filter, compare chatJID (where message was sent) with filter_value
			if chatJID != "" && strings.HasSuffix(chatJID, "@g.us") {
				if wh.FilterValue == "" || chatJID == wh.FilterValue {
					shouldForward = true
					fmt.Printf("DEBUG: Webhook %s accepts group message in chat %s\n", wh.ID, chatJID)
				} else {
					fmt.Printf("DEBUG: Webhook %s rejects group message - expected %s, got %s\n", wh.ID, wh.FilterValue, chatJID)
				}
			}
		case "chat":
			// For chat filter, compare chatJID (where message was sent) with filter_value
			if chatJID != "" && strings.HasSuffix(chatJID, "@s.whatsapp.net") {
				if wh.FilterValue == "" || chatJID == wh.FilterValue {
					shouldForward = true
					fmt.Printf("DEBUG: Webhook %s accepts chat message in chat %s\n", wh.ID, chatJID)
				} else {
					fmt.Printf("DEBUG: Webhook %s rejects chat message - expected %s, got %s\n", wh.ID, wh.FilterValue, chatJID)
				}
			}
		}

		if shouldForward {
			// If media_url is present, make it absolute
			if murl, ok := payload["media_url"].(string); ok && murl != "" && baseURL != "" {
				if !strings.HasPrefix(murl, "http://") && !strings.HasPrefix(murl, "https://") {
					payload["media_url"] = strings.TrimRight(baseURL, "/") + murl
				}
			}
			fmt.Printf("DEBUG: Forwarding to webhook %s (%s) at URL: %s\n", wh.ID, wh.Method, wh.URL)
			addWebhookLog(wh.ID, payload)
			err := sendWebhook(wh, payload, wh.URL, wh.Method)
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

func initDB(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite", dbPath)
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

// Start media cleanup goroutine
func startMediaCleanup(mediaDir string) {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			now := time.Now()
			filepath.Walk(mediaDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				// Delete files older than 24 hours
				if now.Sub(info.ModTime()) > 24*time.Hour {
					os.Remove(path)
					fmt.Printf("Deleted expired media file: %s\n", path)
				}
				return nil
			})
		}
	}()
}

// Refactor startServer to accept a *http.ServeMux argument and register all handlers on it
func startServer(mux *http.ServeMux, port, sessionCookieName, dbPath, mediaDir, waSessionPrefix string) {
	if err := initDB(dbPath); err != nil {
		panic("Failed to initialize DB: " + err.Error())
	}

	// Start media cleanup goroutine
	startMediaCleanup(mediaDir)

	// Register all handlers on mux instead of http.DefaultServeMux
	mux.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if isAuthenticated(r, sessionCookieName) {
			w.Write([]byte(`{"authenticated":true}`))
		} else {
			w.Write([]byte(`{"authenticated":false}`))
		}
	})

	// --- API: QR PNG (existing) ---
	mux.HandleFunc("/qr.png", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r, sessionCookieName)
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
	mux.HandleFunc("/api/wa/status", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"authenticated":false}`))
			return
		}
		email := getUserEmail(r, sessionCookieName)

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
	mux.HandleFunc("/api/wa/connect", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r, sessionCookieName)
		if getUserWAStatus(email) == "connected" {
			w.Write([]byte(`{"success":true,"message":"Already connected"}`))
			return
		}

		// Start connection in background
		go startUserWhatsMeowConnection(email, mediaDir, waSessionPrefix)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"message":"Connecting..."}`))
	})

	// --- API: WhatsMeow Disconnect ---
	mux.HandleFunc("/api/wa/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r, sessionCookieName)
		disconnectUserWhatsMeow(email, mediaDir, waSessionPrefix)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"disconnected"}`))
	})

	// --- API: List Webhooks ---
	mux.HandleFunc("/api/webhooks", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r, sessionCookieName)
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
	mux.HandleFunc("/api/webhooks/create", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("DEBUG: /api/webhooks/create called")
		if !isAuthenticated(r, sessionCookieName) {
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
		// Validate required fields
		if req.URL == "" {
			http.Error(w, "Missing URL", http.StatusBadRequest)
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
		email := getUserEmail(r, sessionCookieName)
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
	mux.HandleFunc("/api/webhooks/delete", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
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
		email := getUserEmail(r, sessionCookieName)
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
	mux.HandleFunc("/api/webhooks/logs", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
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

	// --- API: Generate Automation URL ---
	mux.HandleFunc("/api/automation/generate", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		email := getUserEmail(r, sessionCookieName)
		userID, err := dbGetUserIDByEmail(email)
		if err != nil {
			fmt.Printf("ERROR: Failed to get user ID for email %s: %v\n", email, err)
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		// Create automation webhook (no URL needed for forwarding)
		webhook := Webhook{
			ID:          generateWebhookID(),
			URL:         "", // Empty - no forwarding needed
			Method:      "POST",
			FilterType:  "all",
			FilterValue: "",
			CreatedAt:   time.Now(),
		}

		err = dbCreateWebhook(userID, webhook)
		if err != nil {
			fmt.Printf("ERROR: Failed to create automation webhook: %v\n", err)
			http.Error(w, "Failed to create automation URL", http.StatusInternalServerError)
			return
		}

		// Get base URL from request
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
		automationURL := fmt.Sprintf("%s/webhook/%s", baseURL, webhook.ID)

		fmt.Printf("SUCCESS: Generated automation URL for user %s: %s\n", email, automationURL)
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":        true,
			"automation_url": automationURL,
			"webhook_id":     webhook.ID,
			"message":        "Automation URL generated successfully",
		})
	})

	// --- API: Queue Status ---
	mux.HandleFunc("/api/queue/status", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		email := getUserEmail(r, sessionCookieName)
		
		// Get queue for this user
		queueMutex.RLock()
		queue, exists := messageQueues[email]
		queueMutex.RUnlock()
		
		if !exists {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"queue_length":    0,
				"messages":        []interface{}{},
				"hourly_count":    0,
				"daily_count":     0,
				"hourly_limit":    MAX_HOURLY_MESSAGES,
				"daily_limit":     MAX_DAILY_MESSAGES,
			})
			return
		}

		queue.mu.RLock()
		
		// Prepare queue status
		messages := make([]map[string]interface{}, len(queue.Messages))
		for i, msg := range queue.Messages {
			messages[i] = map[string]interface{}{
				"id":         msg.ID,
				"chat_jid":   msg.ChatJID,
				"message":    msg.Message,
				"status":     msg.Status,
				"created_at": msg.CreatedAt,
				"retries":    msg.Retries,
				"position":   i + 1,
			}
		}
		
		response := map[string]interface{}{
			"queue_length":     len(queue.Messages),
			"messages":         messages,
			"hourly_count":     queue.HourlyCount,
			"daily_count":      queue.DailyCount,
			"hourly_limit":     MAX_HOURLY_MESSAGES,
			"daily_limit":      MAX_DAILY_MESSAGES,
			"hourly_remaining": MAX_HOURLY_MESSAGES - queue.HourlyCount,
			"daily_remaining":  MAX_DAILY_MESSAGES - queue.DailyCount,
			"is_processing":    queue.IsProcessing,
			"last_sent":        queue.LastSent,
		}
		
		queue.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// --- API: Specific Message Status ---
	mux.HandleFunc("/api/queue/message/", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		messageID := path.Base(r.URL.Path)
		if messageID == "" {
			http.Error(w, "Missing message ID", http.StatusBadRequest)
			return
		}

		email := getUserEmail(r, sessionCookieName)
		
		// Get queue for this user
		queueMutex.RLock()
		queue, exists := messageQueues[email]
		queueMutex.RUnlock()
		
		if !exists {
			http.Error(w, "Queue not found", http.StatusNotFound)
			return
		}

		queue.mu.RLock()
		defer queue.mu.RUnlock()
		
		// Find the message
		for i, msg := range queue.Messages {
			if msg.ID == messageID {
				response := map[string]interface{}{
					"id":         msg.ID,
					"chat_jid":   msg.ChatJID,
					"message":    msg.Message,
					"status":     msg.Status,
					"created_at": msg.CreatedAt,
					"retries":    msg.Retries,
					"position":   i + 1,
					"estimated_delay": queue.estimateDelay(i + 1).Seconds(),
				}
				
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}
		}
		
		http.Error(w, "Message not found in queue", http.StatusNotFound)
	})

	// --- API: Recent Chats ---
	mux.HandleFunc("/api/wa/chats", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("DEBUG: /api/wa/chats called")
		if !isAuthenticated(r, sessionCookieName) {
			fmt.Println("DEBUG: Not authenticated for chats endpoint")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		email := getUserEmail(r, sessionCookieName)
		fmt.Println("DEBUG: Getting chats for:", email)

		// Get WhatsApp client for this user
		state := getUserWAState(email)
		state.mu.RLock()
		client := state.waClient
		state.mu.RUnlock()

		var allChats []Chat

		if client != nil && client.Store.ID != nil {
			fmt.Println("DEBUG: WhatsApp client available, fetching contacts and groups")

			// Get contacts from the store
			contacts, err := client.Store.Contacts.GetAllContacts(context.Background())
			if err == nil {
				fmt.Printf("DEBUG: Found %d contacts\n", len(contacts))
				for jid, contact := range contacts {
					if jid.Server == "s.whatsapp.net" { // Individual contacts
						name := contact.FullName
						if name == "" {
							name = contact.FirstName
						}
						if name == "" {
							name = contact.PushName
						}
						if name == "" {
							name = jid.User // Use phone number as fallback
						}
						
						allChats = append(allChats, Chat{
							ID:   jid.String(),
							Name: name,
							Type: "chat",
						})
					}
				}
			} else {
				fmt.Printf("DEBUG: Error getting contacts: %v\n", err)
			}

			// Get groups from the store
			groups, err := client.GetJoinedGroups()
			if err == nil {
				fmt.Printf("DEBUG: Found %d groups\n", len(groups))
				for _, group := range groups {
					groupName := group.Name
					if groupName == "" {
						groupName = "Unnamed Group"
					}
					
					allChats = append(allChats, Chat{
						ID:   group.JID.String(),
						Name: groupName,
						Type: "group",
					})
				}
			} else {
				fmt.Printf("DEBUG: Error getting groups: %v\n", err)
			}
		} else {
			fmt.Println("DEBUG: WhatsApp client not available or not connected")
		}

		// If no chats from WhatsApp client, fall back to recent chats
		if len(allChats) == 0 {
			fmt.Println("DEBUG: No chats from WhatsApp client, falling back to recent chats")
			allChats = getRecentChats(email)
		}

		// Ensure we return an empty array instead of null
		if allChats == nil {
			allChats = []Chat{}
		}

		// Log the chats being returned for debugging
		fmt.Printf("DEBUG: Returning %d chats for user %s\n", len(allChats), email)
		for i, chat := range allChats {
			if i < 5 { // Only log first 5 to avoid spam
				fmt.Printf("DEBUG: Chat %d: ID=%s, Name=%s, Type=%s\n", i+1, chat.ID, chat.Name, chat.Type)
			}
		}
		if len(allChats) > 5 {
			fmt.Printf("DEBUG: ... and %d more chats\n", len(allChats)-5)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(allChats)
	})

	// --- API: Health ---
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("[API] /api/health called")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// --- API: Delete Message ---
	mux.HandleFunc("/api/messages/delete", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ChatJID   string `json:"chat_jid"`
			MessageID string `json:"message_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.ChatJID == "" || req.MessageID == "" {
			http.Error(w, "Missing chat_jid or message_id", http.StatusBadRequest)
			return
		}

		email := getUserEmail(r, sessionCookieName)
		state := getUserWAState(email)

		state.mu.RLock()
		client := state.waClient
		state.mu.RUnlock()

		if client == nil {
			http.Error(w, "WhatsApp client not connected", http.StatusServiceUnavailable)
			return
		}

		// Parse chat JID
		chatJID, err := types.ParseJID(req.ChatJID)
		if err != nil {
			http.Error(w, "Invalid chat JID", http.StatusBadRequest)
			return
		}

		// Delete the message
		_, err = client.RevokeMessage(chatJID, req.MessageID)
		if err != nil {
			fmt.Printf("ERROR: Failed to delete message %s in chat %s: %v\n", req.MessageID, req.ChatJID, err)
			http.Error(w, "Failed to delete message", http.StatusInternalServerError)
			return
		}

		fmt.Printf("SUCCESS: Deleted message %s in chat %s\n", req.MessageID, req.ChatJID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"message":    "Message deleted successfully",
			"message_id": req.MessageID,
			"chat_jid":   req.ChatJID,
		})
	})

	// --- API: Send Message (with Queue System) ---
	mux.HandleFunc("/api/messages/send", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r, sessionCookieName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ChatJID     string `json:"chat_jid"`
			Message     string `json:"message"`
			CallbackURL string `json:"callback_url,omitempty"` // Optional callback URL
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.ChatJID == "" || req.Message == "" {
			http.Error(w, "Missing chat_jid or message", http.StatusBadRequest)
			return
		}

		email := getUserEmail(r, sessionCookieName)
		
		// Check for spam patterns
		if isSpamPattern(req.Message, email) {
			fmt.Printf("WARNING: Blocked potential spam message from %s\n", email)
			http.Error(w, "Message blocked: potential spam detected", http.StatusBadRequest)
			return
		}
		
		// Check if WhatsApp is connected
		state := getUserWAState(email)
		state.mu.RLock()
		client := state.waClient
		state.mu.RUnlock()

		if client == nil {
			http.Error(w, "WhatsApp client not connected", http.StatusServiceUnavailable)
			return
		}

		// Validate chat JID
		_, err := types.ParseJID(req.ChatJID)
		if err != nil {
			http.Error(w, "Invalid chat JID", http.StatusBadRequest)
			return
		}

		// Get or create queue for this user
		queue := getOrCreateQueue(email)
		
		// Check if queue can accept messages
		if !queue.canSendMessage() {
			http.Error(w, "Daily or hourly message limit reached", http.StatusTooManyRequests)
			return
		}

		// Create queued message
		queuedMsg := &QueuedMessage{
			ID:          generateMessageID(),
			UserEmail:   email,
			ChatJID:     req.ChatJID,
			Message:     req.Message,
			CallbackURL: req.CallbackURL,
			CreatedAt:   time.Now(),
			Status:      "queued",
		}

		// Debug logging
		if req.CallbackURL != "" {
			fmt.Printf("DEBUG: Callback URL received: %s for message %s\n", req.CallbackURL, queuedMsg.ID)
		} else {
			fmt.Printf("DEBUG: No callback URL provided for message %s\n", queuedMsg.ID)
		}

		// Add to queue
		err = queue.addMessage(queuedMsg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		// Get queue position and estimated delay
		position := queue.getQueuePosition(queuedMsg.ID)
		estimatedDelay := queue.estimateDelay(position)

		fmt.Printf("SUCCESS: Queued message %s for user %s (position: %d)\n", queuedMsg.ID, email, position)
		
		// Return immediate response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":         true,
			"status":          "queued",
			"queue_id":        queuedMsg.ID,
			"position":        position,
			"estimated_delay": fmt.Sprintf("%.0f seconds", estimatedDelay.Seconds()),
			"message":         "Message queued successfully",
		})
	})

	// --- Serve media files ---
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		mediaFile := path.Base(r.URL.Path)
		filePath := path.Join(mediaDir, mediaFile)
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

	// --- Webhook receiver endpoint ---
	mux.HandleFunc("/webhook/", func(w http.ResponseWriter, r *http.Request) {
		id := path.Base(r.URL.Path)
		if id == "" {
			http.NotFound(w, r)
			return
		}

		fmt.Printf("Received webhook call for id: %s\n", id)

		// Check if this is a POST request with a body (from n8n)
		if r.Method == "POST" && r.Body != nil {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				fmt.Printf("DEBUG: Received JSON payload: %+v\n", payload)
				// This is likely from n8n - extract message and send to WhatsApp
				if message, ok := payload["message"].(string); ok && message != "" {
					fmt.Printf("Received message from webhook %s: %s\n", id, message)

					// Get the webhook owner
					userID, err := dbGetWebhookOwner(id)
					if err != nil {
						fmt.Printf("ERROR: Failed to find webhook owner for ID %s: %v\n", id, err)
						http.Error(w, "Webhook not found", http.StatusNotFound)
						return
					}

					userEmail, err := dbGetUserEmailByID(userID)
					if err != nil {
						fmt.Printf("ERROR: Failed to find user email for ID %d: %v\n", userID, err)
						http.Error(w, "User not found", http.StatusNotFound)
						return
					}

					fmt.Printf("DEBUG: Webhook %s belongs to user %s\n", id, userEmail)

					// Check for spam patterns
					if isSpamPattern(message, userEmail) {
						fmt.Printf("WARNING: Blocked potential spam message from webhook %s (user %s)\n", id, userEmail)
						http.Error(w, "Message blocked: potential spam detected", http.StatusBadRequest)
						return
					}

					// Get the WhatsApp client for this specific user
					state := getUserWAState(userEmail)
					state.mu.RLock()
					connectedClient := state.waClient
					waStatus := state.waStatus
					state.mu.RUnlock()

					if connectedClient == nil || waStatus != "connected" {
						fmt.Printf("ERROR: User %s WhatsApp not connected (status: %s)\n", userEmail, waStatus)
						http.Error(w, "WhatsApp not connected for this user", http.StatusServiceUnavailable)
						return
					}

					// Try to get chat JID from payload
					var chatJID types.JID
					if chatID, ok := payload["chat_id"].(string); ok && chatID != "" {
						if parsedJID, err := types.ParseJID(chatID); err == nil {
							chatJID = parsedJID
						} else {
							fmt.Printf("ERROR: Invalid chat_id format: %s\n", chatID)
							http.Error(w, "Invalid chat_id format", http.StatusBadRequest)
							return
						}
					} else if groupID, ok := payload["groupId"].(string); ok && groupID != "" {
						// Support legacy groupId field
						if parsedJID, err := types.ParseJID(groupID); err == nil {
							chatJID = parsedJID
						}
					} else {
						fmt.Printf("ERROR: No chat_id or groupId provided in payload\n")
						http.Error(w, "Missing chat_id field", http.StatusBadRequest)
						return
					}

					// Get or create queue for this user
					queue := getOrCreateQueue(userEmail)
					
					// Check if queue can accept messages
					if !queue.canSendMessage() {
						http.Error(w, "Daily or hourly message limit reached for this user", http.StatusTooManyRequests)
						return
					}

					// Check for optional callback URL in payload
					callbackURL := ""
					if callback, ok := payload["callback_url"].(string); ok {
						callbackURL = callback
					}

					// Create queued message
					queuedMsg := &QueuedMessage{
						ID:          generateMessageID(),
						UserEmail:   userEmail,
						ChatJID:     chatJID.String(),
						Message:     message,
						CallbackURL: callbackURL,
						CreatedAt:   time.Now(),
						Status:      "queued",
					}

					// Add to queue
					err = queue.addMessage(queuedMsg)
					if err != nil {
						http.Error(w, err.Error(), http.StatusServiceUnavailable)
						return
					}

					// Get queue position and estimated delay
					position := queue.getQueuePosition(queuedMsg.ID)
					estimatedDelay := queue.estimateDelay(position)

					fmt.Printf("SUCCESS: Queued webhook message %s for user %s (position: %d)\n", queuedMsg.ID, userEmail, position)
					
					// Return immediate queue response
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"success":         true,
						"status":          "queued",
						"queue_id":        queuedMsg.ID,
						"position":        position,
						"estimated_delay": fmt.Sprintf("%.0f seconds", estimatedDelay.Seconds()),
						"message":         "Message queued successfully",
						"chat_id":         chatJID.String(),
					})
					return
				} else {
					fmt.Printf("DEBUG: No message field found in payload\n")
					http.Error(w, "Missing message field", http.StatusBadRequest)
					return
				}
			} else {
				fmt.Printf("DEBUG: Failed to parse JSON body: %v\n", err)
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
		} else {
			fmt.Printf("DEBUG: Not a POST request or no body\n")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	})

	// Serve static files from frontend/dist
	staticDir := "frontend/dist"
	fs := http.FileServer(http.Dir(staticDir))

	// Catch-all handler for frontend (except /api/ and /qr.png)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
func handleUserWAEvent(email string, evt interface{}, mediaDir string, waSessionPrefix string) {
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
		forwardToWebhooks(email, payload, mediaPath, mediaDir)
	}
}

// Start WhatsApp connection for a specific user
func startUserWhatsMeowConnection(email string, mediaDir string, waSessionPrefix string) {
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

	// Ensure sessions directory exists
	os.MkdirAll("sessions", 0755)
	// Use user-specific session file in sessions dir
	sessionFile := fmt.Sprintf("sessions/%s%s.db", waSessionPrefix, email)
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
		handleUserWAEvent(email, evt, mediaDir, waSessionPrefix)
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
func disconnectUserWhatsMeow(email string, mediaDir string, waSessionPrefix string) {
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

	// Remove user's session file from sessions dir
	sessionFile := fmt.Sprintf("sessions/%s%s.db", waSessionPrefix, email)
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

// Get webhook owner by webhook ID
func dbGetWebhookOwner(webhookID string) (int64, error) {
	var userID int64
	err := db.QueryRow(`SELECT user_id FROM webhooks WHERE id = ?`, webhookID).Scan(&userID)
	return userID, err
}

// Get user email by user ID
func dbGetUserEmailByID(userID int64) (string, error) {
	var email string
	err := db.QueryRow(`SELECT email FROM users WHERE id = ?`, userID).Scan(&email)
	return email, err
}

// Get user ID by email
func dbGetUserIDByEmail(email string) (int64, error) {
	var userID int64
	err := db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&userID)
	return userID, err
}

// CORS middleware
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[CORS] %s %s from %s\n", r.Method, r.URL.Path, r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == "OPTIONS" {
			fmt.Println("[CORS] Preflight OPTIONS request handled")
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
