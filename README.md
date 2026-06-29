# GoIM — High-Concurrency Instant Messaging System

A production-grade IM (Instant Messaging) system built in Go, featuring WeChat-style privacy design, Redis-first architecture with async MySQL persistence, and AI-powered conversation intelligence.

## ✨ Features

| Category | Features |
|----------|----------|
| **Messaging** | Private chat (push model), group chat (pull model), message revoke, delivery/read ack, offline sync |
| **Social** | Friend request/accept/reject/block, bidirectional friendship, moment publish/like/comment/feed |
| **Group** | Create/update/leave group, member management (add/remove/kick), role system (owner/admin/member) |
| **AI** | 4-layer memory architecture, AI chat with LLM integration, conversation summaries, user profile extraction |
| **Settings** | Notification preferences, message preview toggle, conversation mute list |
| **Real-time** | WebSocket with JWT auth, single-device policy (kick old connection), heartbeat, push notifications |
| **Reliability** | Redis Lua atomic operations, RabbitMQ async persistence, 3-day TTL with auto-cleanup |

## 🏗 Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌────────────┐
│   Client     │────▶│   Gin HTTP + WS   │────▶│   Redis    │
│  (Browser/   │     │   Server          │     │   (First)  │
│   Mobile)    │◀────│                   │◀────│            │
└─────────────┘     │   ┌──────────┐    │     └────────────┘
                    │   │ MQ Pub   │────│────▶┌────────────┐
                    │   │ (RabbitMQ)│    │     │  RabbitMQ  │
                    │   └──────────┘    │     │  Consumers │
                    │                   │     │ ┌────────┐ │
                    │   ┌──────────┐    │     │ │ MySQL  │ │
                    │   │ Lua EVAL │◀───│     │ │ Persist│ │
                    │   └──────────┘    │     │ └────────┘ │
                    └──────────────────┘     └────────────┘
```

### Core Design Decisions

1. **Redis-First Pattern**: All reads/writes go to Redis first. MySQL persistence happens asynchronously via MQ consumers.
2. **Private Chat Push Model**: Messages pushed to per-user `inbox:{userID}` ZSet. Receiver's connection gets real-time push.
3. **Group Chat Pull Model**: Messages stored in per-group `outbox:{groupID}` ZSet. Members pull on sync request.
4. **WeChat Privacy**: Sender **cannot** see receiver's read status. `readStatus` is only visible to the receiver.
5. **Atomic Lua Operations**: Friend check, dedup, msgID allocation, inbox write, mark-read, and revoke all execute as Redis Lua scripts.
6. **3-Day TTL**: Inbox/outbox/timeline auto-expire via `ZREMRANGEBYSCORE` (time) + `ZREMRANGEBYRANK` (count cap).
7. **Single-Device Policy**: New WS connection kicks old one, sending `{"type":"kick","reason":"new_login"}`.

### Conversation ID Format

- Private: `p_{smallerID}_{largerID}` (e.g., `p_1_2`)
- Group: `g_{groupID}` (e.g., `g_42`)

### AI 4-Layer Memory Architecture

| Layer | Storage | Content | Purpose |
|-------|---------|---------|---------|
| 0 | MySQL `private_messages` | Raw messages | Full conversation history |
| 1 | MySQL `ai_summaries` | Topic, key points, conclusion | Structured summaries |
| 2 | MySQL `ai_user_profiles` | Field, value, confidence, source | User profile with confidence scoring |
| 3 | Redis `ai_memory:{userID}:{key}` | Working memory with TTL | Fast context for AI responses |

## 🚀 Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose
- MySQL 8, Redis 7, RabbitMQ 3 (or use Docker Compose below)

### 1. Start Infrastructure

```bash
docker-compose up -d
```

### 2. Run Database Migrations

```bash
# Apply migrations in order
for f in scripts/migrations/*.sql; do
  mysql -u goim -pgoim123 goim < "$f"
done
```

### 3. Build & Run

```bash
go build -o goim-server ./cmd/server
./goim-server -c configs/config.yaml
```

Or run directly:

```bash
go run ./cmd/server -c configs/config.yaml
```

### 4. Verify

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"goim"}
```

### 5. Run E2E Tests

```bash
# Start Docker services first, then:
go test ./tests/... -v -tags e2e -timeout 120s
```

### 6. Run Unit Tests

```bash
go test ./internal/... -v
```

## 📁 Project Structure

```
GoIM/
├── cmd/server/           # Entry point (main.go)
├── configs/              # Config YAML files
│   ├── config.yaml       # Production config template
│   └── config.test.yaml  # E2E test config
├── docker-compose.yaml   # Docker infrastructure
├── docs/                 # Documentation
│   ├── architecture.md   # System architecture details
│   ├── api_reference.md  # REST + WS API reference
│   └── deployment.md     # Deployment guide
├── scripts/migrations/   # MySQL migration SQL files
│   ├── 001_create_users.sql
│   ├── 002_create_friendships.sql
│   ├── 003_create_groups.sql
│   ├── 004_create_messages.sql
│   ├── 005_create_moments.sql
│   ├── 006_create_misc.sql
│   ├── 007_create_ai.sql
│   └── 008_create_user_settings.sql
├── tests/                # E2E integration tests
│   ├── e2e_helper.go     # Test helpers
│   └── e2e_test.go       # E2E test suites
└── internal/
    ├── api/              # Gin HTTP handlers (7 files)
    ├── config/           # Config loading
    ├── conn/             # ConnectionManager + ClientConnection
    ├── consumer/         # MQ consumers (3 files)
    ├── infra/            # MySQL/Redis/RabbitMQ connections + cleanup
    ├── llm/              # LLM client (OpenAI-compatible)
    ├── middleware/        # JWT auth middleware
    ├── model/            # Data models (7 files)
    ├── protocol/         # WS message types + encode/decode
    ├── redis/            # Lua scripts (4 files + loader)
    ├── repository/       # MySQL/Redis/MQ repo interfaces + impl
    ├── service/          # Business logic (8 services)
    └── ws/               # WS upgrade + message dispatcher
```

## 🛠 Tech Stack

| Component | Technology | Version |
|-----------|------------|---------|
| Language | Go | 1.24+ |
| HTTP Framework | Gin | v1.10+ |
| WebSocket | gorilla/websocket | v1.5 |
| MySQL | go-sql-driver/mysql | v1.8+ |
| Redis | go-redis/v9 | v9.7+ |
| RabbitMQ | amqp091-go | v1.10+ |
| JWT | golang-jwt/jwt/v5 | v5.2+ |
| Config | yaml.v3 | v3 |
| Logging | zap | v1.27+ |
| Password | bcrypt | — |
| Container | Docker Compose | v3.8 |

## 📊 API Overview

See [docs/api_reference.md](docs/api_reference.md) for full details.

| Category | Endpoints | Auth |
|----------|-----------|------|
| Health | `GET /health` | None |
| Auth | register, login, refresh | None |
| Friend | request, accept, reject, list, block, unblock | JWT |
| Group | create, update, info, members, add/remove member, leave | JWT |
| Moment | publish, get, like, comment, feed | JWT |
| AI | chat, profile, summary | JWT |
| Message Ops | revoke, delete, search | JWT |
| Settings | get, update, mute, unmute | JWT |
| WebSocket | `GET /ws?token=JWT` | JWT |

## 🔧 Configuration

See `configs/config.yaml` for the full config template with all fields documented.

Key sections:
- `server`: port, ws_path, upload_dir
- `mysql`: host, port, user, password, db_name
- `redis`: addr, password, db
- `rabbitmq`: url (amqp://)
- `jwt`: secret, access_exp_hours, refresh_exp_days
- `llm`: provider, api_key, base_url, model, max_tokens
- `file`: max_size_mb, allowed_exts, upload_dir

## 📄 License

This project is for educational and demonstration purposes.
