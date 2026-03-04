# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.
每次修改代码都要写e2e集成go test（不需要单元测试）

## Project Overview

**DecentralizedChat** is a fully decentralized chat application built on **NATS Routes clusters + Wails framework**. It provides secure, peer-to-peer communication with automatic network discovery, end-to-end encryption, and cross-platform support (Windows, macOS, Linux).

## Key Features

- ⚡ Automatic network discovery - nodes form a full mesh without manual configuration
- 🔒 NSC (NATS Security Center) integration with Ed25519 signatures and TLS
- 💬 Direct messaging (DM) and group chat with AES-256 encryption
- 📱 Cross-platform desktop app via Wails (Go + React)
- 🏗️ No central server - all nodes are equal peers

## Tech Stack

- **Frontend**: React 19 + TypeScript + Vite
- **Backend**: Go 1.24.4
- **Framework**: Wails v2.10.2
- **Networking**: NATS Server v2.11.7, NATS.go v1.44.0
- **Security**: NATS JWT v2, NKeys, Ed25519, AES-256-GCM

---

## Common Commands

### Development

```bash
# Start Wails development server (hot reload)
wails dev

# Install frontend dependencies
cd frontend && pnpm install
```

### Build

```bash
# Build for current platform
wails build

# Build for specific platforms
wails build -platform windows/amd64
wails build -platform darwin/universal
wails build -platform linux/amd64
```

### Go Dependencies

```bash
# Tidy Go modules
go mod tidy
```

### P2P/NAT Traversal Tests

```bash
# Run tests (from test directory)
cd test && go test -v -timeout 60s

# Build Docker images for P2P testing
cd test && make build

# Export Docker images
cd test && make export

# Clean build artifacts
cd test && make clean
```

### STUN Server

```bash
# Start STUN server for NAT traversal
./stun-docker-run.sh
```

---

## Project Structure

```
DecentralizedChat/
├── main.go                 # Wails application entry point
├── app.go                  # Main Wails application logic
├── wails.json              # Wails configuration
├── go.mod / go.sum         # Go module dependencies
├── frontend/               # React + TypeScript frontend
│   ├── src/
│   │   ├── App.tsx         # Main application component
│   │   ├── components/     # ChatRoom, Sidebar, etc.
│   │   ├── services/       # API integration
│   │   └── types/          # TypeScript definitions
│   └── package.json
├── internal/               # Go backend packages
│   ├── chat/               # Chat service (encryption, messaging)
│   ├── config/             # Configuration management
│   ├── nats/               # NATS client service
│   ├── nscsetup/           # NSC setup and key management
│   └── routes/             # NATS Routes cluster management
├── test/                   # P2P + NAT traversal testing
│   ├── cmd/                # Signal server and P2P node implementations
│   ├── Dockerfile.server   # Signal server Docker config
│   ├── Dockerfile.client   # P2P node Docker config
│   ├── Makefile            # Build automation
│   ├── TESTING_GUIDE.md    # Test documentation
│   └── DOCKER_DEPLOY.md    # Deployment guide
├── docker/                 # Docker config for NATS cluster
├── docs/                   # Architecture documentation
└── stun-docker-run.sh      # STUN server deployment script
```

---

## Architecture Overview

### High-Level Architecture

```
User Device A (Routes) ←→ User Device B (Routes) ←→ User Device C (Routes)
        ↓                        ↓                        ↓
        └────────────────────────┼────────────────────────┘
                                 │
                         NATS Mesh Network
```

### Key Components

1. **Frontend Layer** (React + Wails)
   - `App.tsx`: Main UI state and user interactions
   - ChatRoom component: Real-time chat interface
   - Services layer: API integration with Go backend via Wails bindings

2. **Backend Layer** (Go + Wails)
   - `app.go`: Main orchestrator managing NATS service, chat service, node manager, config, and SSL
   - `internal/chat/`: Encryption (AES-256, Ed25519), message handling, NSC key management
   - `internal/config/`: Config loading/saving (JSON format at `~/.dchat/config.json`)
   - `internal/nats/`: NATS client connection and pub/sub
   - `internal/nscsetup/`: NSC integration for user auth
   - `internal/routes/`: NATS Routes cluster management

### NAT Traversal Strategy

- **STUN**: Public STUN server (121.199.173.116:3478) for public address discovery
- **UDP Hole Punching**: Direct P2P communication through NAT
- **Signal Server**: HTTP server for initial peer discovery and address exchange

### NATS Subjects

- Direct Messages: `dchat.dm.<conversation-id>.msg`
- Group Chat: `dchat.grp.<group-id>.msg`
- Inbox: `_INBOX.<unique-id>` (for replies)

### Configuration

Config file location: `~/.dchat/config.json`

Key config sections:
- `user`: User ID, nickname, avatar
- `network`: Auto-discovery, seed nodes, local IP
- `server`: Client port (4222), cluster port (6222), cluster name
- `keys`: NSC operator, keys directory, user credentials paths

---

## Important Documentation

- `README.md`: Project overview, architecture, quick start
- `test/TESTING_GUIDE.md`: Complete test guide for all features
- `test/DOCKER_DEPLOY.md`: Docker deployment instructions
- `docs/decentralized-chat-architecture.md`: Deep architectural dive
- `internal/chat/README.md`: Chat service implementation details
- `internal/routes/Note.md`: NATS Routes cluster management notes

---

## Port Requirements

- **Client Port**: 4222 (NATS client connections)
- **Cluster Port**: 6222 (NATS Routes cluster communication)
- **STUN Port**: 3478 (UDP, for NAT traversal)
- **Signal Server**: 8080 (HTTP, for peer discovery)

---

## Docker Deployment

```bash
# Build and export images
cd test && make build && make export

# Load on target machine
docker load -i dist/p2p-signal-server.tar
docker load -i dist/p2p-node.tar

# Start signal server
docker run -d --name signal-server -p 8080:8080 --restart unless-stopped p2p-signal-server

# Start P2P nodes (MUST use --network host)
docker run -it --rm --network host --name p2p-node p2p-node /p2p_node \
  -node-id Alice \
  -listen-port 10001 \
  -signal-server http://<signal-server-ip>:8080
```
