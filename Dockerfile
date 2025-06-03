# --- Stage 1: Build frontend ---
FROM node:20-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --frozen-lockfile
COPY frontend/ ./
RUN npm run build

# --- Stage 2: Build backend ---
FROM golang:1.24.3-alpine AS backend-build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy built frontend into backend context
COPY --from=frontend-build /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -o app .

# --- Stage 3: Final image ---
FROM gcr.io/distroless/base-debian12 AS final
WORKDIR /app
COPY --from=backend-build /app/app ./app
COPY --from=backend-build /app/frontend/dist ./frontend/dist
EXPOSE 8080
ENV PORT=8080
ENV MEDIA_DIR=/app/media
CMD ["/app/app"] 