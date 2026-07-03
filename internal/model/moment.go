package model

import "time"

type Moment struct {
	ID         int64     `json:"id"`
	AuthorID   int64     `json:"author_id"`
	Content    string    `json:"content"`
	MediaUrls  *string   `json:"media_urls,omitempty"` // 以JSON字符串形式存储在数据库中；可为空
	Visibility int       `json:"visibility"` // 1=所有人可见，2=仅好友可见，3=私密
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

// FeedEntry 是 Feed ZSet（收件箱/寄件箱）中的一条记录：
// 动态 ID 及其发布时间戳（毫秒），用于推拉合并时的排序与游标分页。
type FeedEntry struct {
	MomentID int64
	Ts       int64 // 发布时间戳（毫秒），即 ZSet 的 score
}
