# GoIM API Reference

## Base URL

```
http://localhost:8080/api/v1
```

## Authentication

All protected endpoints require a JWT access token via `Authorization` header:

```
Authorization: Bearer <access_token>
```

WebSocket connections authenticate via query parameter:

```
GET /ws?token=<access_token>
```

Tokens are obtained from `/auth/login` or `/auth/refresh`.

---

## Health Check

### GET /health

No auth required.

**Response:**
```json
{
  "status": "ok",
  "service": "goim"
}
```

---

## Auth (Public — No JWT Required)

### POST /auth/register

**Request:**
```json
{
  "username": "alice",
  "password": "pass1234"
}
```

**Response (201):**
```json
{
  "user_id": 1,
  "username": "alice"
}
```

**Errors:**
- `400`: username/password too short (min 3 chars each)
- `409`: username already taken

### POST /auth/login

**Request:**
```json
{
  "username": "alice",
  "password": "pass1234"
}
```

**Response (200):**
```json
{
  "access_token": "eyJhbG...",
  "refresh_token": "eyJhbG...",
  "expires_in": 7200
}
```

**Errors:**
- `401`: user not found or wrong password

### POST /auth/refresh

**Request:**
```json
{
  "refresh_token": "eyJhbG..."
}
```

**Response (200):**
```json
{
  "access_token": "eyJhbG...",
  "expires_in": 7200
}
```

**Errors:**
- `401`: invalid or expired refresh token

---

## Friend (JWT Required)

### POST /friend/request

Send a friend request.

**Request:**
```json
{
  "to_user_id": 2,
  "message": "Let's be friends!"
}
```

**Response (201):**
```json
{
  "request_id": 1,
  "from_user_id": 1,
  "to_user_id": 2,
  "status": 0
}
```

**Errors:**
- `400`: self-request
- `403`: target has blocked you
- `409`: already friends or duplicate request

### POST /friend/accept

**Request:**
```json
{
  "request_id": 1
}
```

**Response (200):**
```json
{
  "user_id": 2,
  "friend_id": 1
}
```

**Errors:**
- `403`: not the request target
- `404`: request not found

### POST /friend/reject

**Request:**
```json
{
  "request_id": 1
}
```

**Response (200):**
```json
{
  "message": "friend request rejected"
}
```

### GET /friend/requests

**Response (200):**
```json
{
  "requests": [
    {
      "id": 1,
      "from_user_id": 3,
      "to_user_id": 1,
      "message": "Hi!",
      "status": 0,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### GET /friend/list

**Response (200):**
```json
{
  "friends": [
    {
      "id": 1,
      "user_id": 1,
      "friend_id": 2,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### DELETE /friend/:friendID

Remove a friendship.

**Response (200):**
```json
{
  "message": "friend deleted"
}
```

### POST /friend/block

**Request:**
```json
{
  "blocked_id": 5
}
```

**Response (200):**
```json
{
  "message": "user blocked"
}
```

**Errors:**
- `409`: already blocked

### POST /friend/unblock

**Request:**
```json
{
  "blocked_id": 5
}
```

**Response (200):**
```json
{
  "message": "user unblocked"
}
```

---

## Group (JWT Required)

### POST /group

Create a group. Creator becomes owner (role=2).

**Request:**
```json
{
  "name": "My Group",
  "notice": "Welcome!"
}
```

**Response (201):**
```json
{
  "group_id": 1
}
```

### PUT /group/:groupID

Update group name/notice. Only owner or admin can update.

**Request:**
```json
{
  "name": "Updated Name",
  "notice": "Updated notice"
}
```

**Response (200):**
```json
{
  "message": "group updated"
}
```

**Errors:**
- `403`: not owner or admin
- `404`: group not found

### GET /group/:groupID

**Response (200):**
```json
{
  "id": 1,
  "name": "My Group",
  "notice": "Welcome!",
  "owner_id": 1,
  "max_members": 500,
  "created_at": "2024-01-01T00:00:00Z"
}
```

### POST /group/:groupID/member

Add a member. Only owner or admin can add.

**Request:**
```json
{
  "member_id": 3
}
```

**Response (200):**
```json
{
  "message": "member added"
}
```

**Errors:**
- `403`: not owner/admin
- `404`: group not found
- `409`: already member or group full (max 500)

### DELETE /group/:groupID/member/:memberID

Remove/kick a member. Owner cannot be removed.

**Response (200):**
```json
{
  "message": "member removed"
}
```

**Errors:**
- `403`: not owner/admin or trying to remove owner
- `404`: group not found

### GET /group/:groupID/members

**Response (200):**
```json
{
  "members": [
    {
      "group_id": 1,
      "user_id": 1,
      "role": 2,
      "joined_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### PUT /group/:groupID/member/:memberID/role

Update member role. Only owner can change roles.

**Request:**
```json
{
  "role": 1
}
```

Roles: 0=member, 1=admin, 2=owner

**Response (200):**
```json
{
  "message": "member role updated"
}
```

### POST /group/:groupID/leave

Leave a group. Owner cannot leave (must transfer first).

**Response (200):**
```json
{
  "message": "left group"
}
```

**Errors:**
- `403`: owner cannot leave
- `404`: group not found

---

## Moment (JWT Required)

### POST /moment

Publish a moment.

**Request:**
```json
{
  "content": "Great day today!",
  "media_urls": "https://img.example.com/1.jpg",
  "visibility": 1
}
```

Visibility: 1=all, 2=friends only, 3=private

**Response (201):**
```json
{
  "moment_id": 1
}
```

### GET /moment/:momentID

**Response (200):**
```json
{
  "id": 1,
  "author_id": 1,
  "content": "Great day today!",
  "media_urls": "https://img.example.com/1.jpg",
  "visibility": 1,
  "created_at": "2024-01-01T00:00:00Z"
}
```

### GET /moment/user/:userID?limit=20&offset=0

Get moments by a specific user.

**Response (200):**
```json
{
  "moments": [...]
}
```

### POST /moment/:momentID/like

**Response (200):**
```json
{
  "ok": true
}
```

**Errors:**
- `404`: moment not found
- `409`: already liked

### DELETE /moment/:momentID/like

**Response (200):**
```json
{
  "ok": true
}
```

### POST /moment/:momentID/comment

**Request:**
```json
{
  "content": "Nice post!"
}
```

**Response (201):**
```json
{
  "comment_id": 1
}
```

### DELETE /moment/comment/:commentID

Only comment author can delete.

**Response (200):**
```json
{
  "ok": true
}
```

**Errors:**
- `403`: not comment owner
- `404`: comment not found

### GET /moment/feed?last_sync_time=0&limit=20

Get the user's moment feed (from friends' timeline).

**Response (200):**
```json
{
  "moments": [...]
}
```

---

## AI (JWT Required)

### POST /ai/chat

Send a message to the AI assistant.

**Request:**
```json
{
  "content": "What are my hobbies?"
}
```

**Response (200):**
```json
{
  "response": "Based on our conversations, you enjoy hiking, photography, and cooking."
}
```

### GET /ai/profile

Get the AI's understanding of the user (Layer 2 memory).

**Response (200):**
```json
{
  "items": [
    {
      "field_name": "hobbies",
      "value": "hiking, photography",
      "confidence": 0.85,
      "source": "conversation_summary"
    }
  ]
}
```

### POST /ai/summary/:convID

Generate an AI summary for a conversation.

**Response (200):**
```json
{
  "id": 1,
  "topic": "Travel planning",
  "key_points": "Discussed trip to Japan in March",
  "conclusion": "Decided on Tokyo and Kyoto itinerary",
  "user_intent": "Plan spring trip to Japan",
  "message_range": "msg1-msg45"
}
```

---

## Message Operations (JWT Required)

### POST /msg/revoke

Revoke a message (within 2 minutes of sending, sender only).

**Request:**
```json
{
  "convId": "p_1_2",
  "msgId": 100
}
```

**Response (200):**
```json
{
  "message": "message revoked"
}
```

**Errors:**
- `400`: message not revocable (too old or already revoked)
- `403`: not the message sender

### DELETE /msg/:msgID?convId=p_1_2

Delete a message (local deletion only, other party still sees it).

**Response (200):**
```json
{
  "message": "message deleted"
}
```

### GET /msg/search?q=hello&limit=20&offset=0

Search private messages.

**Response (200):**
```json
{
  "messages": [...]
}
```

---

## Settings (JWT Required)

### GET /settings

**Response (200):**
```json
{
  "user_id": 1,
  "notification_enabled": true,
  "msg_preview_enabled": true,
  "mute_list": "",
  "created_at": "2024-01-01T00:00:00Z"
}
```

### PUT /settings

**Request:**
```json
{
  "notification_enabled": true,
  "msg_preview_enabled": false,
  "mute_list": ""
}
```

**Response (200):**
```json
{
  "message": "settings updated"
}
```

### POST /settings/mute

Mute a conversation.

**Request:**
```json
{
  "convId": "p_1_2"
}
```

**Response (200):**
```json
{
  "message": "conversation muted"
}
```

**Errors:**
- `409`: already muted

### DELETE /settings/mute/:convID

**Response (200):**
```json
{
  "message": "conversation unmuted"
}
```

**Errors:**
- `404`: not muted

---

## WebSocket Protocol

Connect to `GET /ws?token=<access_token>`.

All messages use JSON envelope format:

```json
{
  "type": "<message_type>",
  "data": { ... }
}
```

### Client → Server Messages

| Type | Purpose | Data Fields |
|------|---------|-------------|
| `msg` | Send chat message | `msgId` (client ID), `convType` (1=private, 2=group), `toId`, `msgType` (1=text), `content`, `timestamp` |
| `deliverAck` | Confirm delivery | `serverMsgId` |
| `readAck` | Mark conversation read | `convId` |
| `syncReq` | Request offline sync | `lastSyncTime`, `batchSize` |
| `revokeMsg` | Revoke a message | `convId`, `serverMsgId` |
| `aiStream` | AI chat stream | `content` |
| `friendApply` | Friend apply via WS | (placeholder) |
| `ping` | Heartbeat | — |

### Server → Client Messages

| Type | Purpose | Data Fields |
|------|---------|-------------|
| `serverAck` | Message acknowledged | `clientMsgId`, `serverMsgId`, `groupSeq` (group), `timestamp` |
| `msg` | Incoming chat message | `msgId`, `convId`, `convType`, `fromId`, `toId`, `msgType`, `content`, `readStatus` (private), `groupSeq` (group), `timestamp` |
| `syncBatch` | Offline sync batch | `msgs[]`, `hasMore`, `syncTime` |
| `convSync` | Conversation sync | `conversations[]`, `unreadMap` |
| `msgRevoked` | Message revoked notification | `convId`, `serverMsgId`, `operatorId` |
| `kick` | Connection kicked | `reason: "new_login"` |
| `friendAccepted` | Friend request accepted | — |
| `presence` | Online status change | — |
| `error` | Error notification | `code`, `message` |
| `pong` | Heartbeat response | — |

### Message Types

| Constant | Value | Description |
|----------|-------|-------------|
| MsgTypeText | 1 | Plain text message |
| MsgTypeImage | 2 | Image message |
| MsgTypeVideo | 3 | Video message |
| MsgTypeAI | 4 | AI-generated message |
| MsgTypeSystem | 5 | System notification |
| MsgTypeRevoked | 6 | Revoked placeholder |

### Conversation Types

| Constant | Value | ID Format |
|----------|-------|-----------|
| ConvTypePrivate | 1 | `p_{smallerID}_{largerID}` |
| ConvTypeGroup | 2 | `g_{groupID}` |

### Single-Device Policy

When a user connects via WebSocket, any existing connection for the same user ID is kicked. The old connection receives:

```json
{
  "type": "kick",
  "reason": "new_login"
}
```

The old connection's WebSocket is then closed.
