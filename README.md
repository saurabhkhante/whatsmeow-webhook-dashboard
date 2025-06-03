# WhatsMeow Webhook Dashboard

A modern, multi-user WhatsApp webhook dashboard built with Go (WhatsMeow) and Vue.js.

## Features

- Connect WhatsApp via QR code
- Create/manage webhooks (with filtering)
- Real-time message and media forwarding
- User authentication
- Dockerized for easy deployment

## Quick Start

### 1. Clone the repo

```sh
git clone https://github.com/yourusername/yourrepo.git
cd yourrepo
```

### 2. Build and run with Docker

```sh
docker build -t whatsmeow-dashboard .
docker run --env-file .env.production -v $(pwd)/media:/app/media -p 8080:8080 whatsmeow-dashboard
```

- See `.env.example` for environment variables.
- Media files are stored in `/app/media` (use a Docker volume for persistence).

### 3. Access the app

- Go to [http://localhost:8080](http://localhost:8080) (or your deployed domain)

## Documentation

- [Setup & Usage](instructions.md)
- [Full Project Documentation](PROJECT_DOCUMENTATION.md)

## Testing

```sh
go test ./...
```

## License

MIT (or your license here) 