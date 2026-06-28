package model

import "time"

type PrivateMessage struct {
	ID        int64     `json:"msgId"`
	SenderID  int64     `json:"fromId"`
	ReceiverID int64    `json:"toId"`
	Content   string    `json:"content"`
	MsgType   int       `json:"msgType"`
	CreatedAt time.Time `json:"timestamp"`
}

type GroupMessage struct {
	ID        int64     `json:"msgId"`
	GroupID   int64     `json:"groupId"`
	SenderID  int64     `json:"fromId"`
	Content   string    `json:"content"`
	MsgType   int       `json:"msgType"`
	GroupSeq  int64     `json:"groupSeq"`
	CreatedAt time.Time `json:"timestamp"`
}

type MsgRevoked struct {
	ID         int64     `json:"id"`
	MsgID      int64     `json:"msg_id"`
	ConvID     string    `json:"conv_id"`
	OperatorID int64     `json:"operator_id"`
	RevokedAt  time.Time `json:"revoked_at"`
}

// InboxMessage — message stored in Redis inbox/outbox ZSet
type InboxMessage struct {
	MsgID      int64  `json:"msgId"`
	ConvID     string `json:"convId"`
	ConvType   int    `json:"convType"`
	FromID     int64  `json:"fromId"`
	ToID       int64  `json:"toId"`
	MsgType    int    `json:"msgType"`
	Content    string `json:"content"`
	ReadStatus int    `json:"readStatus"` // 0=unread, 1=read (private chat only)
	Timestamp  int64  `json:"timestamp"`
}

// ServerAck — returned to sender after message reaches server
type ServerAck struct {
	ClientMsgID string `json:"clientMsgId"`
	ServerMsgID int64  `json:"serverMsgId"`
	GroupSeq    int64  `json:"groupSeq,omitempty"`
	Timestamp   int64  `json:"timestamp"`
}

// DeliverAck — receiver confirms message delivered
type DeliverAck struct {
	ServerMsgID int64 `json:"serverMsgId"`
}

// ReadAck — user marks conversation as read
type ReadAck struct {
	ConvID string `json:"convId"`
}

// SyncReq — client requests offline sync
type SyncReq struct {
	LastSyncTime int64 `json:"lastSyncTime"`
	BatchSize    int   `json:"batchSize"`
}

// SyncBatch — server returns offline messages in batches
type SyncBatch struct {
	Messages []InboxMessage `json:"msgs"`
	HasMore  bool           `json:"hasMore"`
	SyncTime int64          `json:"syncTime,omitempty"`
}

// ConvSync — conversation list + unread counts pushed on sync
type ConvSync struct {
	Conversations []ConvSummary   `json:"conversations"`
	UnreadMap     map[string]int64 `json:"unreadMap"`
}

// ConvSummary — single conversation summary for conv_list ZSet
type ConvSummary struct {
	ConvID       string `json:"convId"`
	ConvType     int    `json:"convType"`
	TargetID     int64  `json:"targetId"`
	TargetName   string `json:"targetName"`
	TargetAvatar string `json:"targetAvatar"`
	LastMsg      string `json:"lastMsg"`
	LastMsgTime  int64  `json:"lastMsgTime"`
}
