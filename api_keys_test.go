package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPerUserAPIKeys(t *testing.T) {
	// Setup test server
	tmpDB := "test_api_keys.db"
	tmpMedia := "test_api_media"
	os.Remove(tmpDB)
	os.RemoveAll(tmpMedia)
	os.Mkdir(tmpMedia, 0755)

	mux := http.NewServeMux()
	startServer(mux, "8082", "api_test_session", tmpDB, tmpMedia, "api_test_")
	ts := httptest.NewServer(mux)
	defer func() {
		ts.Close()
		os.Remove(tmpDB)
		os.RemoveAll(tmpMedia)
	}()

	client := &http.Client{}

	// Register two users
	user1 := map[string]string{"email": "user1@example.com", "password": "pass123"}
	user2 := map[string]string{"email": "user2@example.com", "password": "pass456"}

	for _, user := range []map[string]string{user1, user2} {
		userJSON, _ := json.Marshal(user)
		resp, err := client.Post(ts.URL+"/api/register", "application/json", bytes.NewBuffer(userJSON))
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("Register failed for %s: %v, status: %d", user["email"], err, resp.StatusCode)
		}
	}

	// Login both users and get their API keys
	var user1APIKey, user2APIKey string

	// User 1
	loginJSON, _ := json.Marshal(user1)
	resp, err := client.Post(ts.URL+"/api/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Login failed for user1: %v, status: %d", err, resp.StatusCode)
	}
	user1Cookies := resp.Cookies()

	// Generate API key for user 1
	req, _ := http.NewRequest("POST", ts.URL+"/api/user/api-key", nil)
	for _, c := range user1Cookies {
		req.AddCookie(c)
	}
	apiResp, err := client.Do(req)
	if err != nil || apiResp.StatusCode != 200 {
		t.Fatalf("Get API key failed for user1: %v, status: %d", err, apiResp.StatusCode)
	}
	var apiData map[string]interface{}
	json.NewDecoder(apiResp.Body).Decode(&apiData)
	user1APIKey = apiData["api_key"].(string)

	// User 2
	loginJSON, _ = json.Marshal(user2)
	resp, err = client.Post(ts.URL+"/api/login", "application/json", bytes.NewBuffer(loginJSON))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("Login failed for user2: %v, status: %d", err, resp.StatusCode)
	}
	user2Cookies := resp.Cookies()

	// Generate API key for user 2
	req, _ = http.NewRequest("POST", ts.URL+"/api/user/api-key", nil)
	for _, c := range user2Cookies {
		req.AddCookie(c)
	}
	apiResp, err = client.Do(req)
	if err != nil || apiResp.StatusCode != 200 {
		t.Fatalf("Get API key failed for user2: %v, status: %d", err, apiResp.StatusCode)
	}
	json.NewDecoder(apiResp.Body).Decode(&apiData)
	user2APIKey = apiData["api_key"].(string)

	// Test 1: API keys are different
	if user1APIKey == user2APIKey {
		t.Fatalf("API keys should be different between users")
	}

	// Test 2: API keys work for their respective users
	// User 1 creates a webhook
	createBody := map[string]string{
		"url":          "https://user1.example.com/webhook",
		"method":       "POST",
		"filter_type":  "all",
		"filter_value": "",
	}
	createJSON, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest("POST", ts.URL+"/api/webhooks/create", bytes.NewBuffer(createJSON))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", user1APIKey)
	createResp, err := client.Do(createReq)
	if err != nil || createResp.StatusCode != 200 {
		t.Fatalf("Create webhook failed for user1: %v, status: %d", err, createResp.StatusCode)
	}

	// User 2 creates a webhook
	createBody["url"] = "https://user2.example.com/webhook"
	createJSON, _ = json.Marshal(createBody)
	createReq, _ = http.NewRequest("POST", ts.URL+"/api/webhooks/create", bytes.NewBuffer(createJSON))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", user2APIKey)
	createResp, err = client.Do(createReq)
	if err != nil || createResp.StatusCode != 200 {
		t.Fatalf("Create webhook failed for user2: %v, status: %d", err, createResp.StatusCode)
	}

	// Test 3: Each user can only see their own webhooks
	// User 1 lists webhooks
	listReq, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	listReq.Header.Set("X-API-Key", user1APIKey)
	listResp, err := client.Do(listReq)
	if err != nil || listResp.StatusCode != 200 {
		t.Fatalf("List webhooks failed for user1: %v, status: %d", err, listResp.StatusCode)
	}
	var user1Webhooks []map[string]interface{}
	json.NewDecoder(listResp.Body).Decode(&user1Webhooks)
	if len(user1Webhooks) != 1 {
		t.Fatalf("User1 should see 1 webhook, got %d", len(user1Webhooks))
	}
	if user1Webhooks[0]["url"] != "https://user1.example.com/webhook" {
		t.Fatalf("User1 should see their own webhook URL")
	}

	// User 2 lists webhooks
	listReq, _ = http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	listReq.Header.Set("X-API-Key", user2APIKey)
	listResp, err = client.Do(listReq)
	if err != nil || listResp.StatusCode != 200 {
		t.Fatalf("List webhooks failed for user2: %v, status: %d", err, listResp.StatusCode)
	}
	var user2Webhooks []map[string]interface{}
	json.NewDecoder(listResp.Body).Decode(&user2Webhooks)
	if len(user2Webhooks) != 1 {
		t.Fatalf("User2 should see 1 webhook, got %d", len(user2Webhooks))
	}
	if user2Webhooks[0]["url"] != "https://user2.example.com/webhook" {
		t.Fatalf("User2 should see their own webhook URL")
	}

	// Test 4: Invalid API key fails
	invalidReq, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	invalidReq.Header.Set("X-API-Key", "invalid_key")
	invalidResp, err := client.Do(invalidReq)
	if err != nil {
		t.Fatalf("Request with invalid API key failed: %v", err)
	}
	if invalidResp.StatusCode != 401 {
		t.Fatalf("Expected 401 for invalid API key, got %d", invalidResp.StatusCode)
	}

	// Test 5: Missing API key fails
	noKeyReq, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	noKeyResp, err := client.Do(noKeyReq)
	if err != nil {
		t.Fatalf("Request without API key failed: %v", err)
	}
	if noKeyResp.StatusCode != 401 {
		t.Fatalf("Expected 401 for missing API key, got %d", noKeyResp.StatusCode)
	}

	// Test 6: API key regeneration works
	oldKey := user1APIKey
	req, _ = http.NewRequest("POST", ts.URL+"/api/user/api-key", nil)
	for _, c := range user1Cookies {
		req.AddCookie(c)
	}
	newKeyResp, err := client.Do(req)
	if err != nil || newKeyResp.StatusCode != 200 {
		t.Fatalf("API key regeneration failed: %v, status: %d", err, newKeyResp.StatusCode)
	}
	json.NewDecoder(newKeyResp.Body).Decode(&apiData)
	newKey := apiData["api_key"].(string)

	if oldKey == newKey {
		t.Fatalf("New API key should be different from old key")
	}

	// Old key should not work
	oldKeyReq, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	oldKeyReq.Header.Set("X-API-Key", oldKey)
	oldKeyResp, err := client.Do(oldKeyReq)
	if err != nil {
		t.Fatalf("Request with old API key failed: %v", err)
	}
	if oldKeyResp.StatusCode != 401 {
		t.Fatalf("Expected 401 for old API key, got %d", oldKeyResp.StatusCode)
	}

	// New key should work
	newKeyListReq, _ := http.NewRequest("GET", ts.URL+"/api/webhooks", nil)
	newKeyListReq.Header.Set("X-API-Key", newKey)
	newKeyListResp, err := client.Do(newKeyListReq)
	if err != nil || newKeyListResp.StatusCode != 200 {
		t.Fatalf("New API key should work: %v, status: %d", err, newKeyListResp.StatusCode)
	}
}
