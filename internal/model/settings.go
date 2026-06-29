package model

import "time"

// UserSettings stores per-user notification and conversation preferences.
type UserSettings struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	NotificationEnabled bool      `json:"notification_enabled"`
	MsgPreviewEnabled   bool      `json:"msg_preview_enabled"`
	MuteList           string    `json:"mute_list"` // JSON array of muted convIDs; empty string = null
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
