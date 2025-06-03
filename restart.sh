#!/bin/bash

# Kill any existing process using port 8080
echo "Stopping any existing process on port 8080..."
lsof -ti:8080 | xargs kill -9 2>/dev/null

# Remove the main app database (users, webhooks, etc.)
echo "Removing existing WhatsApp app database..."
rm -f whatsmeow.db

# Remove all WhatsApp session files (new and old locations)
echo "Removing all WhatsApp session files..."
rm -rf sessions/
rm -f whatsmeow_*.db

# Remove all media files
echo "Removing all media files..."
rm -rf media/*

# Build the Vue frontend
echo "Building Vue frontend..."
cd frontend && npm run build && cd ..

# Wait a moment to ensure cleanup
sleep 1

# Start the application
echo "Starting fresh WhatsApp connection..."
go run . 