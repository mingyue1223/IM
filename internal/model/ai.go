package model

import "time"

type AISummary struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Topic        string    `json:"topic"`
	KeyPoints    string    `json:"key_points"` // 以 JSON 字符串形式存储在数据库中
	Conclusion   string    `json:"conclusion"`
	UserIntent   string    `json:"user_intent"`
	MessageRange string    `json:"message_range"` // 以 JSON 字符串形式存储在数据库中
	CreatedAt    time.Time `json:"created_at"`
}

type AIProfileItem struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	FieldName  string    `json:"field_name"`
	Value      string    `json:"value"`
	Confidence float32   `json:"confidence"`
	Source     string    `json:"source"`
	UpdatedAt  time.Time `json:"updated_at"`
}
