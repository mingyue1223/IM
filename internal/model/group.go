package model

import "time"

type Group struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Notice     string    `json:"notice"`
	OwnerID    int64     `json:"owner_id"`
	MaxMembers int       `json:"max_members"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type GroupMember struct {
	ID        int64     `json:"id"`
	GroupID   int64     `json:"group_id"`
	UserID    int64     `json:"user_id"`
	Role      int       `json:"role"` // 0=member, 1=admin, 2=owner
	MutedUntil *time.Time `json:"muted_until,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
}
