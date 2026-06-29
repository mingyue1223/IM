# GoIM System Architecture

## Overview

GoIM is a high-concurrency instant messaging system built on a **Redis-first + MQ async persistence** pattern. The core design principle: **all reads and writes hit Redis first**, and MySQL persistence happens asynchronously via RabbitMQ consumers. This ensures sub-millisecond response times while maintaining data durability.

## Architecture Diagram

```
                        ┌───────────────────────────────────┐
                        │          GoIM Server              │
                        │                                   │
   Client ──HTTP──────▶│  ┌───────────────────────────┐   │
   (REST API)           │  │     Gin Router             │   │──▶ MySQL (async via MQ)
                        │  │  ├─ Public: /auth/*        │   │
                        │  │  ├─ Protected: /friend/*   │   │
   Client ──WS────────▶│  │  │  /group/*  /moment/*   │   │──▶ Redis (first read/write)
   (WebSocket)          │  │  │  /ai/*  /msg/*  /set*  │   │
                        │  │  ├─ WS: /ws?token=JWT      │   │──▶ RabbitMQ (async pub)
                        │  └───────────────────────────┘   │
                        │                                   │
                        │  ┌────────────┐  ┌────────────┐  │
                        │  │  Services  │  │ Consumers  │  │
                        │  │ (8 total)  │  │ (3 total)  │  │
                        │  └────────────┘  └────────────┘  │
                        └───────────────────────────────────┘
```

## Data Flow

### Private Chat (Push Model)

```
Sender                          GoIM                        Receiver
  │                               │                            │
  │── WS: {type:"msg"} ────────▶│                            │
  │                               │── Lua: PrivateMsgCheck    │
  │                               │   (friend check + dedup   │
  │                               │    + msgID alloc + inbox) │
  │                               │── MQ: PublishPrivateMsg   │
  │◀─ WS: {type:"serverAck"} ──│                            │
  │                               │                            │
  │                               │── Consumer picks up msg   │
  │                               │── WriteInbox(receiver)     │
  │                               │── WriteInbox(sender,read=1)│
  │                               │── UpdateConvList(both)     │
  │                               │── IncrementUnread(receiver)│
  │                               │── Push to online receiver  │
  │                               │── InsertMySQL(msg)         │
  │                               │──▶ WS: {type:"msg"} ──────│
  │                               │                            │
```

### Group Chat (Pull Model)

```
Sender                          GoIM                        Members
  │                               │                            │
  │── WS: {type:"msg"} ────────▶│                            │
  │                               │── Lua: GroupMsgCheck      │
  │                               │   (member check + dedup   │
  │                               │    + msgID + outbox write) │
  │                               │── MQ: PublishGroupMsg     │
  │◀─ WS: {type:"serverAck"} ──│                            │
  │                               │                            │
  │                               │── Consumer picks up msg   │
  │                               │── WriteOutbox(group)       │
  │                               │── For each member:         │
  │                               │    UpdateConvList          │
  │                               │    IncrementUnread         │
  │                               │    Push to online member   │
  │                               │── InsertMySQL(msg)         │
  │                               │──▶ WS: {type:"msg"} ──────│ (online members)
  │                               │                            │
  │                               │ Offline member syncs later │
  │                               │── WS: {type:"syncReq"} ──▶│
  │                               │── ReadOutbox(group) ──────▶│
```

### Moment Feed Fan-Out

```
Author                          GoIM                        Friends
  │                               │                            │
  │── POST /moment ────────────▶│                            │
  │                               │── InsertMySQL(moment)     │
  │                               │── MQ: PublishMomentPush   │
  │◀── {moment_id: 42} ────────│                            │
  │                               │                            │
  │                               │── Consumer picks up event │
  │                               │── GetFriendList(author)    │
  │                               │── For each friend:         │
  │                               │    PublishMomentFeed       │
  │                               │    (Redis timeline ZSet)   │
  │                               │                            │
  │                               │── Friend requests feed ──▶│
  │                               │── GET /moment/feed ──────▶│
  │                               │── GetMomentFeed(timeline)  │
```

## Redis Key Schema

| Key Pattern | Type | TTL | Purpose |
|-------------|------|-----|---------|
| `inbox:{userID}` | ZSet (score=timestamp) | 3 days | Private chat inbox per user |
| `outbox:{groupID}` | ZSet (score=timestamp) | 3 days | Group chat outbox per group |
| `conv_list:{userID}` | ZSet (score=timestamp) | 3 days | Conversation list per user |
| `unread:{userID}` | Hash (convID→count) | — | Unread message counts |
| `timeline:{userID}` | ZSet (score=timestamp) | 3 days | Moment feed per user |
| `group_members:{groupID}` | Set (userID) | — | Group member cache |
| `user_groups:{userID}` | Set (groupID) | — | User's group membership cache |
| `msg_id_global` | String (INCR counter) | — | Global message ID generator |
| `online:{userID}` | String "1" | 60s | Online status indicator |
| `conn:{userID}` | String "ws:{nano}" | 60s | Active connection identifier |
| `ai_memory:{userID}:{key}` | String (JSON) | 30min | AI working memory |
| `dedup:{convID}:{clientMsgID}` | String "1" | 5min | Message deduplication |

## Lua Scripts (Atomic Operations)

| Script | Purpose | Keys Accessed |
|--------|---------|---------------|
| `privateMsgCheck` | Friend check + dedup + msgID + inbox write | friendship, dedup, msg_id_global, inbox, conv_list, unread |
| `groupMsgCheck` | Member check + dedup + msgID + outbox write | group_members, dedup, msg_id_global, outbox, conv_list, unread |
| `inboxMarkRead` | Clear unread count + mark inbox messages read | unread, inbox |
| `revokeMsg` | Verify sender + mark msg revoked in inbox/outbox | inbox/outbox, check senderID |

All Lua scripts execute atomically in Redis, preventing race conditions.

## Message Lifecycle

### Message ID Generation
- Global counter: `INCR msg_id_global` (atomic, monotonic)
- Returned as `serverMsgID` in `serverAck`

### Delivery Confirmation
1. Sender receives `serverAck` (msg reached server)
2. Receiver's client sends `deliverAck` (msg displayed)
3. Receiver's client sends `readAck` (user read the conversation)

### WeChat Privacy Design
- Sender sees: message sent, delivery confirmed
- Sender **cannot** see: whether receiver read the message
- `readStatus` in inbox is only updated when the **receiver** sends `readAck`

### Revocation
- Sender can revoke within 2 minutes (configurable)
- Lua script verifies: 1) sender owns the message, 2) within time limit
- Both sender and receiver see `msgRevoked` notification
- Revoked message content replaced with "Message revoked"

## Conversation Sync

### Offline Sync (WebSocket)

On reconnect, client sends `syncReq` with `lastSyncTime`:
- Server reads inbox from `lastSyncTime` onward
- Returns `syncBatch` with messages and `hasMore` flag
- Also returns `convSync` with conversation list + unread counts

### Group Read Position
- `group_read_pos:{userID}:{groupID}` = last read groupSeq
- Used to calculate unread count for group conversations

## Cleanup (3-Day TTL)

Background goroutine runs every 1 hour:
- `ZREMRANGEBYSCORE inbox:* 0 {3_days_ago}` — remove messages older than 3 days
- `ZREMRANGEBYRANK inbox:* 0 -(max-1000)` — cap inbox at 1000 messages
- `ZREMRANGEBYRANK outbox:* 0 -(max-500)` — cap outbox at 500 messages
- `ZREMRANGEBYRANK timeline:* 0 -(max-100)` — cap timeline at 100 moments
- Similar trimming for conv_list keys

## AI 4-Layer Memory

```
Layer 0: Raw Messages (MySQL private_messages)
    ↓ AIService.GenerateSummary()
Layer 1: Structured Summary (MySQL ai_summaries)
    topic, key_points, conclusion, user_intent
    ↓ Profile extraction
Layer 2: Confidence-Graded Profile (MySQL ai_user_profiles)
    field_name, value, confidence (0-1), source
    ON DUPLICATE KEY UPDATE confidence
    ↓ Load into working memory
Layer 3: Redis Working Memory (ai_memory:{userID}:{key})
    TTL = 30 minutes, auto-refresh on use
```

AI message flow:
1. User sends `{type:"aiStream", data:{content:"..."}}`
2. Load working memory from Redis (Layer 3)
3. If missing, load profile from MySQL (Layer 2)
4. Call LLM with context + memory
5. Parse response, extract profile items
6. Update Layer 2 (MySQL) and Layer 3 (Redis)
7. Return AI response to user

## Security

- **JWT Authentication**: Access token (2h) + Refresh token (7d)
- **bcrypt Password Hashing**: Cost factor 10
- **Single-Device Policy**: New connection kicks old one
- **Message Dedup**: Client-generated msgId + Redis SETNX with 5min TTL
- **Sender Authorization**: Revoke Lua script checks senderID matches
- **Friend/Member Verification**: Private/group msg Lua scripts verify relationship exists
