package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestServer() (*httptest.Server, func()) {
	// Use a temporary DB and media dir for tests
	tmpDB := "test_whatsmeow.db"
	tmpMedia := "test_media"
	os.Remove(tmpDB)
	os.RemoveAll(tmpMedia)
	os.Mkdir(tmpMedia, 0755)

	mux := http.NewServeMux()
	startServer(mux, "8081", "test_session_id", tmpDB, tmpMedia, "test_whatsmeow_")
	ts := httptest.NewServer(mux)

	teardown := func() {
		ts.Close()
		os.Remove(tmpDB)
		os.RemoveAll(tmpMedia)
	}
	return ts, teardown
}

func TestRegisterLoginLogoutSession(t *testing.T) {
	ts, teardown := setupTestServer()
	defer teardown()

	client := &http.Client{}

	// Register
	regBody := map[string]string{"email": "testuser@example.com", "password": "testpass123"}
	regJSON, _ := json.Marshal(regBody)
	resp, err := client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(regJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Register failed: %v, status: %d", err, resp.StatusCode)
	}

	// Login
	loginBody := map[string]string{"email": "testuser@example.com", "password": "testpass123"}
	loginJSON, _ := json.Marshal(loginBody)
	resp, err = client.Post(ts.URL+"/api/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Login failed: %v, status: %d", err, resp.StatusCode)
	}

	// Check session (should be authenticated)
	req, _ := http.NewRequest("GET", ts.URL+"/api/session", nil)
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	sessResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Session check failed: %v", err)
	}
	var sessData map[string]interface{}
	json.NewDecoder(sessResp.Body).Decode(&sessData)
	if !sessData["authenticated"].(bool) {
		t.Fatalf("Session not authenticated after login")
	}

	// Logout
	logoutReq, _ := http.NewRequest("POST", ts.URL+"/api/logout", nil)
	for _, c := range resp.Cookies() {
		logoutReq.AddCookie(c)
	}
	logoutResp, err := client.Do(logoutReq)
	if err != nil || logoutResp.StatusCode != 200 {
		t.Fatalf("Logout failed: %v, status: %d", err, logoutResp.StatusCode)
	}

	// Check session (should be unauthenticated)
	req2, _ := http.NewRequest("GET", ts.URL+"/api/session", nil)
	sessResp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("Session check after logout failed: %v", err)
	}
	var sessData2 map[string]interface{}
	json.NewDecoder(sessResp2.Body).Decode(&sessData2)
	if sessData2["authenticated"].(bool) {
		t.Fatalf("Session still authenticated after logout")
	}
}

func TestWebhookManagement(t *testing.T) {
	ts, teardown := setupTestServer()
	defer teardown()

	client := &http.Client{}

	// Register and login
	regBody := map[string]string{"email": "webhookuser@example.com", "password": "webhookpass123"}
	regJSON, _ := json.Marshal(regBody)
	resp, err := client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(regJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Register failed: %v, status: %d", err, resp.StatusCode)
	}
	loginBody := map[string]string{"email": "webhookuser@example.com", "password": "webhookpass123"}
	loginJSON, _ := json.Marshal(loginBody)
	resp, err = client.Post(ts.URL+"/api/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Login failed: %v, status: %d", err, resp.StatusCode)
	}
	cookies := resp.Cookies()

	// List webhooks (should be empty)
	req, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	listResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("List webhooks failed: %v", err)
	}
	var webhooks []map[string]interface{}
	json.NewDecoder(listResp.Body).Decode(&webhooks)
	if len(webhooks) != 0 {
		t.Fatalf("Expected 0 webhooks, got %d", len(webhooks))
	}

	// Create webhook
	createBody := map[string]string{
		"url":          "https://example.com/webhook",
		"method":       "POST",
		"filter_type":  "all",
		"filter_value": "",
	}
	createJSON, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/create", bytes.NewBuffer(createJSON))
	createReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		createReq.AddCookie(c)
	}
	createResp, err := client.Do(createReq)
	if err != nil || createResp.StatusCode != 200 {
		t.Fatalf("Create webhook failed: %v, status: %d", err, createResp.StatusCode)
	}
	var createData map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createData)
	id, ok := createData["id"].(string)
	if !ok || id == "" {
		t.Fatalf("Webhook ID not returned after creation")
	}

	// List webhooks (should have 1)
	req2, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	listResp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("List webhooks after create failed: %v", err)
	}
	var webhooks2 []map[string]interface{}
	json.NewDecoder(listResp2.Body).Decode(&webhooks2)
	if len(webhooks2) != 1 {
		t.Fatalf("Expected 1 webhook, got %d", len(webhooks2))
	}

	// Delete webhook
	deleteBody := map[string]string{"id": id}
	deleteJSON, _ := json.Marshal(deleteBody)
	deleteReq, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/delete", bytes.NewBuffer(deleteJSON))
	deleteReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		deleteReq.AddCookie(c)
	}
	deleteResp, err := client.Do(deleteReq)
	if err != nil || deleteResp.StatusCode != 200 {
		t.Fatalf("Delete webhook failed: %v, status: %d", err, deleteResp.StatusCode)
	}

	// List webhooks (should be empty again)
	req3, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	for _, c := range cookies {
		req3.AddCookie(c)
	}
	listResp3, err := client.Do(req3)
	if err != nil {
		t.Fatalf("List webhooks after delete failed: %v", err)
	}
	var webhooks3 []map[string]interface{}
	json.NewDecoder(listResp3.Body).Decode(&webhooks3)
	if len(webhooks3) != 0 {
		t.Fatalf("Expected 0 webhooks after delete, got %d", len(webhooks3))
	}
}

func TestEdgeCasesAndSecurity(t *testing.T) {
	ts, teardown := setupTestServer()
	defer teardown()

	client := &http.Client{}

	// 1. Unauthorized access
	endpoints := []string{"/api/webhooks", "/api/webhooks/create", "/api/wa/status"}
	for _, ep := range endpoints {
		req, _ := http.NewRequest("GET", ts.URL+ep, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("Request to %s failed: %v", ep, err)
		}
		if resp.StatusCode != 401 {
			t.Errorf("Expected 401 for %s, got %d", ep, resp.StatusCode)
		}
	}

	// 2. Invalid registration (missing fields)
	regBody := map[string]string{"email": ""}
	regJSON, _ := json.Marshal(regBody)
	resp, err := client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(regJSON))
	if err != nil || resp.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid registration, got %d", resp.StatusCode)
	}

	// 3. Double registration
	regBody2 := map[string]string{"email": "edgecase@example.com", "password": "edgepass"}
	regJSON2, _ := json.Marshal(regBody2)
	resp, err = client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(regJSON2))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("First registration failed: %v, status: %d", err, resp.StatusCode)
	}
	resp, err = client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(regJSON2))
	if err != nil || resp.StatusCode != 409 {
		t.Errorf("Expected 409 for duplicate registration, got %d", resp.StatusCode)
	}

	// 4. Invalid webhook creation (missing URL)
	// Login first
	loginBody := map[string]string{"email": "edgecase@example.com", "password": "edgepass"}
	loginJSON, _ := json.Marshal(loginBody)
	resp, err = client.Post(ts.URL+"/api/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Login failed: %v, status: %d", err, resp.StatusCode)
	}
	cookies := resp.Cookies()
	createBody := map[string]string{
		"method":       "POST",
		"filter_type":  "all",
		"filter_value": "",
	}
	createJSON, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/create", bytes.NewBuffer(createJSON))
	createReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		createReq.AddCookie(c)
	}
	createResp, err := client.Do(createReq)
	if err != nil || createResp.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid webhook creation, got %d", createResp.StatusCode)
	}

	// 5. Invalid webhook creation (invalid method)
	createBody2 := map[string]string{
		"url":          "https://example.com/webhook",
		"method":       "INVALID",
		"filter_type":  "all",
		"filter_value": "",
	}
	createJSON2, _ := json.Marshal(createBody2)
	createReq2, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/create", bytes.NewBuffer(createJSON2))
	createReq2.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		createReq2.AddCookie(c)
	}
	createResp2, err := client.Do(createReq2)
	if err != nil || createResp2.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid webhook method, got %d", createResp2.StatusCode)
	}

	// 6. Invalid webhook deletion (missing ID)
	deleteBody := map[string]string{}
	deleteJSON, _ := json.Marshal(deleteBody)
	deleteReq, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/delete", bytes.NewBuffer(deleteJSON))
	deleteReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		deleteReq.AddCookie(c)
	}
	deleteResp, err := client.Do(deleteReq)
	if err != nil || deleteResp.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid webhook delete, got %d", deleteResp.StatusCode)
	}
}

func TestMediaServing(t *testing.T) {
	ts, teardown := setupTestServer()
	defer teardown()

	// Simulate saving a file to the media directory
	mediaDir := "test_media"
	filename := "testfile.txt"
	fileContent := []byte("hello, media!")
	filePath := filepath.Join(mediaDir, filename)
	err := ioutil.WriteFile(filePath, fileContent, 0644)
	if err != nil {
		t.Fatalf("Failed to write test media file: %v", err)
	}

	// Request the file via /media/filename
	resp, err := http.Get(ts.URL + "/media/" + filename)
	if err != nil {
		t.Fatalf("Failed to GET media file: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200 for media file, got %d", resp.StatusCode)
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read media response body: %v", err)
	}
	if string(respBody) != string(fileContent) {
		t.Fatalf("Media file content mismatch: got %q, want %q", string(respBody), string(fileContent))
	}
}

func TestWebhookForwarding(t *testing.T) {
	ts, teardown := setupTestServer()
	defer teardown()

	// Start a mock server to act as the webhook endpoint
	received := make(chan map[string]interface{}, 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		received <- payload
		w.WriteHeader(200)
	}))
	defer mockServer.Close()

	// Register and login
	client := &http.Client{}
	regBody := map[string]string{"email": "forwarduser@example.com", "password": "forwardpass123"}
	regJSON, _ := json.Marshal(regBody)
	resp, err := client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(regJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Register failed: %v, status: %d", err, resp.StatusCode)
	}
	loginBody := map[string]string{"email": "forwarduser@example.com", "password": "forwardpass123"}
	loginJSON, _ := json.Marshal(loginBody)
	resp, err = client.Post(ts.URL+"/api/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Login failed: %v, status: %d", err, resp.StatusCode)
	}
	cookies := resp.Cookies()

	// Create webhook pointing to mock server
	createBody := map[string]string{
		"url":          mockServer.URL,
		"method":       "POST",
		"filter_type":  "all",
		"filter_value": "",
	}
	createJSON, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/create", bytes.NewBuffer(createJSON))
	createReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		createReq.AddCookie(c)
	}
	createResp, err := client.Do(createReq)
	if err != nil || createResp.StatusCode != 200 {
		t.Fatalf("Create webhook failed: %v, status: %d", err, createResp.StatusCode)
	}

	// Call forwardToWebhooks directly with a sample payload
	email := "forwarduser@example.com"
	payload := map[string]interface{}{
		"from": "12345@s.whatsapp.net",
		"name": "Test User",
		"type": "text",
		"text": "Hello, webhook!",
	}
	forwardToWebhooks(email, payload, "", "test_media")

	// Assert that the mock server received the correct payload
	select {
	case got := <-received:
		if got["text"] != "Hello, webhook!" {
			t.Fatalf("Expected payload text 'Hello, webhook!', got %v", got["text"])
		}
		if got["from"] != "12345@s.whatsapp.net" {
			t.Fatalf("Expected payload from '12345@s.whatsapp.net', got %v", got["from"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for webhook to be received")
	}
}
