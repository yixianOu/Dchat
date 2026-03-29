# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

每次修改代码都要写e2e集成go test（不需要单元测试）。
你可以在测试里面直接启动go内嵌nats服务器进行测试。
如果e2e集成测试失败了，就说明代码逻辑有问题，你要写bug说明文档。
你要检查你写的e2e集成test是否满足docs/refactor.md的要求，你不能照着代码写测试，你要按照需求写测试，如果测试没通过就要报告代码的问题

## Project Overview

**DecentralizedChat (DChat)** is a fully decentralized encrypted chat application built on **NATS LeafNode + JetStream + Wails framework**. It provides secure peer-to-peer communication with end-to-end encryption, offline message support, and cross-platform desktop support (Windows, macOS, Linux).

## Key Features

- ⚡ **LeafNode Architecture** - No NAT traversal required, users only need to connect to public NATS hubs
- 🔒 **End-to-end Encryption** - Direct messages use NaCl Box (X25519 + XSalsa20-Poly1305), group chats use AES-256-GCM
- 🏗️ **Hybrid Decentralization** - Hubs form a decentralized Routes cluster, users connect as LeafNodes
- 💬 **Full-featured Chat** - Direct messages, group chats, message history, search, read receipts
- 📱 **Cross-platform** - Single binary desktop app via Wails (Go + React)
- 📡 **Offline Message Support** - JetStream on hubs stores messages when recipients are offline
- 💾 **Local History** - SQLite for efficient local message storage with full query capabilities

## Tech Stack

- **Frontend**: React 19 + TypeScript + Vite
- **Backend**: Go 1.24.4
- **Framework**: Wails v2.10.2
- **Networking**: NATS Server v2.11.7, NATS.go v1.44.0
- **Storage**: SQLite (modernc.org/sqlite) for local history, JetStream for offline messages
- **Security**: NATS JWT v2, NKeys, Ed25519, NaCl Box, AES-256-GCM

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

### Testing

```bash
# Run integration tests (from test directory)
cd test && go test -v -timeout 60s
```

### Deploy NATS Hub Cluster (Docker)

```bash
# Start local hub cluster for testing
cd docker && docker-compose up -d
```

---

## Project Structure

```
DecentralizedChat/
├── main.go                 # Wails application entry point
├── app.go                  # Main application orchestrator
├── wails.json              # Wails configuration
├── go.mod / go.sum         # Go module dependencies
├── frontend/               # React + TypeScript frontend
│   ├── src/
│   │   ├── App.tsx         # Main application component
│   │   ├── components/     # ChatRoom, ConversationList, KeyManager, etc.
│   │   ├── services/       # Wails API bindings
│   │   └── types/          # TypeScript type definitions
│   └── package.json
├── internal/               # Go backend packages
│   ├── chat/               # Chat service (encryption, messaging, key management)
│   ├── config/             # Configuration management (JSON at ~/.dchat/config.json)
│   ├── nats/               # NATS client service with JetStream offline sync
│   ├── leafnode/           # Local LeafNode server manager
│   ├── nscsetup/           # NSC (NATS Security Center) automatic setup
│   └── storage/            # SQLite storage for local message history
├── test/                   # E2E integration tests
├── docker/                 # Docker config for NATS hub cluster deployment
├── docs/                   # Architecture and design documentation
└── INTERFACE_DOCS.md       # Complete API documentation for frontend ↔ backend
```

---

## Architecture Overview

### High-Level Architecture

```
┌─────────────┐      ┌─────────────┐
│ User Device │      │ User Device │
│  (LeafNode) │      │  (LeafNode) │
└──────┬──────┘      └──────┬──────┘
       │                    │
       ├────────────────────┘
       ↓
┌───────────────────────────────────┐
│       Public Hub Cluster          │
│  (NATS Routes + JetStream)      │
│  Hubs are fully meshed with Routes │
└───────────────────────────────────┘
```

**Why LeafNode instead of full mesh Routes?**
- No NAT traversal required - LeafNodes initiate outbound connections only
- No port forwarding needed - works behind any NAT/firewall
- Simpler configuration - users only need hub URLs
- Better resource utilization - hubs handle clustering, users just connect

### Key Components

1. **Frontend Layer** (React + Wails)
   - `App.tsx`: Main UI state management
   - `components/ChatRoom.tsx`: Real-time chat interface
   - `components/ConversationList.tsx`: Session list with unread counts
   - `services/api.ts`: Wails binding to Go backend methods

2. **Backend Layer** (Go + Wails)
   - `app.go`: Main orchestrator that initializes and manages all services:
     - Loads config from `~/.dchat/config.json`
     - Starts local LeafNode NATS server
     - Initializes SQLite storage
     - Connects NATS client to local LeafNode
     - Creates chat service with encryption
     - Auto-restores previous conversations on startup
   - `internal/leafnode/`: Manages embedded NATS LeafNode server
     - Connects to configured hubs via LeafNode protocol
     - Supports multiple hubs for high availability
     - Optional JetStream enabled locally
   - `internal/chat/`: Core chat service with encryption
     - `Service`: Main chat service orchestrator
     - Handles encryption/decryption for direct and group messages
     - Manages friend public keys and group symmetric keys
     - Emits events to frontend via Wails runtime.EventsEmit
   - `internal/storage/`: SQLite storage for local history
     - `conversations` table: Stores DM/group sessions
     - `messages` table: Stores encrypted/decrypted messages
     - `friends` table: Stores friend public keys
     - `groups` table: Stores group keys
     - Supports full-text search, pagination, mark-as-read
   - `internal/nats/`: NATS client wrapper
     - Wraps connection to local LeafNode
     - Handles publish/subscribe
     - Provides JetStream offline message synchronization
   - `internal/config/`: Configuration loading/saving
   - `internal/nscsetup/`: Automatic NSC/credentials generation

### Message Encryption

| Message Type | Encryption | Key Exchange |
|--------------|------------|--------------|
| Direct Message | NaCl Box (X25519 + XSalsa20-Poly1305) | ECDH with Ed25519 keys |
| Group Chat | AES-256-GCM | Shared symmetric key distributed to all members |

### Message Flow (Direct Message)

```
User A → Frontend → app.SendDirect() → chat.Service.SendDirect()
  ↓
Encrypt with friend's public key (NaCl Box)
  ↓
Publish to nats: dchat.dm.<conversation-id>.msg
  ↓
Local LeafNode → Hub → Other user's LeafNode
  ↓
Other user's nats.Client receives → chat.Service.Handler
  ↓
Decrypt with private key → OnDecrypted event → Frontend → Display
  ↓
Async save to SQLite for local history
```

### Offline Message Flow

```
User B is offline
  ↓
User A sends message → Hub JetStream stores message
  ↓
User B comes online → LeafNode connects → JetStream pulls stored messages
  ↓
Messages delivered to User B → Acknowledged → Hub deletes
```

### NATS Subjects

- Direct Messages: `dchat.dm.<conversation-id>.msg`
- Group Chat: `dchat.grp.<group-id>.msg`

### Configuration

Config file location: `~/.dchat/config.json`

Key sections:

```json
{
  "user": {
    "nickname": "User Name"
  },
  "leafnode": {
    "local_host": "127.0.0.1",
    "local_port": 4222,
    "hub_urls": ["nats://hub1.example.com:7422"],
    "enable_tls": false,
    "enable_jetstream": true
  },
  "keys": {
    "user_creds_path": "~/.dchat/nsc/.../user.creds",
    "user_seed_path": "~/.dchat/nsc/.../user.seed",
    "user_pub_key": "U..."
  },
  "sqlite_path": "~/.dchat/chat.db",
  "log_level": "info"
}
```

---

## Important Documentation

- `README.md`: Project overview, architecture, quick start
- `INTERFACE_DOCS.md`: Complete API documentation (frontend ↔ backend)
- `docs/refactor.md`: LeafNode architecture refactor plan
- `docs/architecture-design.md`: Detailed architecture design
- `docs/offline.md`: Offline message design with JetStream
- `docs/leaf-node/`: LeafNode technical analysis documentation
- `internal/chat/README.md`: Chat service implementation details
- `test/E2E_INTEGRATION_TEST_PLAN.md`: Comprehensive test plan

---

## Port Requirements

- **LeafNode Client Port**: 4222 (local only, 127.0.0.1)
- **Hub Client Port**: 7422 (public NATS hub port)

User devices **do NOT need any open inbound ports** - only outbound connections to hubs.

---

## Development Guidelines

1. **Testing**: Write E2E integration tests, not unit tests. Use embedded NATS server in tests.
2. **Encryption**: All message content must be encrypted before sending over NATS.
3. **Storage**: Local history goes to SQLite, hub offline storage uses JetStream.
4. **Events**: Use `runtime.EventsEmit` for async communication from Go to frontend.
5. **Configuration**: Keep config schema backward compatible when adding new fields.
