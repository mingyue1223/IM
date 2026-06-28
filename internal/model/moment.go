package model

import "time"

type Moment struct {
	ID         int64     `json:"id"`
	AuthorID   int64     `json:"author_id"`
	Content    string    `json:"content"`
	MediaUrls  *string   `json:"media_urls,omitempty"` // stored as JSON string in DB; nullable
	Visibility int       `json:"visibility"` // 1=all, 2=friends only, 3=private
	CreatedAt  time.Time `json:"created_at"`
}

type MomentLike struct {
	ID        int64     `json:"id"`
	MomentID  int64     `json:"moment_id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type MomentComment struct {
	ID        int64     `json:"id"`
	MomentID  int64     `json:"moment_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
