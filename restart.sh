#!/bin/bash

# Kill any existing process using port 8080
echo "Stopping any existing process on port 8080..."
lsof -ti:8080 | xargs kill -9 2>/dev/null

# Remove the existing database
echo "Removing existing WhatsApp session..."
rm -f whatsmeow.db

# Build the Vue frontend
echo "Building Vue frontend..."
cd frontend && npm run build && cd ..

# Wait a moment to ensure cleanup
sleep 1

# Start the application
echo "Starting fresh WhatsApp connection..."
go run . 