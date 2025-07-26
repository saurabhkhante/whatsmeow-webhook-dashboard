package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	_ = godotenv.Load()

	port := getEnv("PORT", "8080")
	sessionCookieName := getEnv("SESSION_COOKIE_NAME", "session_id")
	dbPath := getEnv("DB_PATH", "whatsmeow.db")
	mediaDir := getEnv("MEDIA_DIR", "media")
	waSessionPrefix := getEnv("WA_SESSION_PREFIX", "whatsmeow_")

	fmt.Println("main.go: main() is running, about to call startServer()...")
	mux := http.NewServeMux()
	startServer(mux, port, sessionCookieName, dbPath, mediaDir, waSessionPrefix)
	fmt.Printf("Starting web server at http://localhost:%s\n", port)
	http.ListenAndServe(":"+port, withCORS(mux))
}
