package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/goim/goim/internal/model"
)

// MySQLRepo defines all MySQL CRUD operations needed by services and consumers.
// The interface allows mocking in tests. Methods will be fleshed out in Tasks 12-17.
type MySQLRepo interface {
	// ── Messages ──
	InsertPrivateMessage(ctx context.Context, msg *model.PrivateMessage) error
	InsertGroupMessage(ctx context.Context, msg *model.GroupMessage) error
	InsertMsgRevoked(ctx context.Context, revoked *model.MsgRevoked) error

	// ── Users ──
	GetUserByID(ctx context.Context, userID int64) (*model.User, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	CreateUser(ctx context.Context, user *model.User) error
	UpdateUser(ctx context.Context, user *model.User) error

	// ── Friendships ──
	CreateFriendRequest(ctx context.Context, req *model.FriendRequest) error
	UpdateFriendRequest(ctx context.Context, req *model.FriendRequest) error
	GetFriendRequestByID(ctx context.Context, id int64) (*model.FriendRequest, error)
	GetFriendRequestsByUser(ctx context.Context, userID int64) ([]model.FriendRequest, error)
	CreateFriendship(ctx context.Context, fs *model.Friendship) error
	DeleteFriendship(ctx context.Context, userID, friendID int64) error
	GetFriendList(ctx context.Context, userID int64) ([]model.Friendship, error)
	IsFriend(ctx context.Context, userID, friendID int64) (bool, error)
	CreateBlacklist(ctx context.Context, bl *model.Blacklist) error
	DeleteBlacklist(ctx context.Context, userID, blockedID int64) error
	IsBlocked(ctx context.Context, userID, blockedID int64) (bool, error)

	// ── Groups ──
	CreateGroup(ctx context.Context, group *model.Group) (int64, error)
	UpdateGroup(ctx context.Context, group *model.Group) error
	GetGroupByID(ctx context.Context, groupID int64) (*model.Group, error)
	AddGroupMember(ctx context.Context, member *model.GroupMember) error
	RemoveGroupMember(ctx context.Context, groupID, userID int64) error
	GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error)
	UpdateGroupMemberRole(ctx context.Context, groupID, userID, role int) error

	// ── Moments ──
	CreateMoment(ctx context.Context, moment *model.Moment) error
	GetMomentByID(ctx context.Context, id int64) (*model.Moment, error)
	GetMomentsByUser(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error)
	CreateMomentLike(ctx context.Context, like *model.MomentLike) error
	DeleteMomentLike(ctx context.Context, momentID, userID int64) error
	CreateMomentComment(ctx context.Context, comment *model.MomentComment) error
	GetMomentCommentByID(ctx context.Context, id int64) (*model.MomentComment, error)
	DeleteMomentComment(ctx context.Context, id int64) error

	// ── AI ──
	CreateAISummary(ctx context.Context, summary *model.AISummary) error
	CreateAIProfileItem(ctx context.Context, item *model.AIProfileItem) error
	GetAIProfileByUser(ctx context.Context, userID int64) ([]model.AIProfileItem, error)
}

// ──────────────────────────────────────────────────────
// MySQLRepoImpl — concrete implementation using database/sql
// ──────────────────────────────────────────────────────

type MySQLRepoImpl struct {
	db *sql.DB
}

func NewMySQLRepo(db *sql.DB) *MySQLRepoImpl {
	return &MySQLRepoImpl{db: db}
}

// ── Messages ──

func (m *MySQLRepoImpl) InsertPrivateMessage(ctx context.Context, msg *model.PrivateMessage) error {
	query := `INSERT INTO private_messages (id, sender_id, receiver_id, content, msg_type, created_at)
	          VALUES (?, ?, ?, ?, ?, ?)`
	_, err := m.db.ExecContext(ctx, query,
		msg.ID,
		msg.SenderID,
		msg.ReceiverID,
		msg.Content,
		msg.MsgType,
		msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert private_messages: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) InsertGroupMessage(ctx context.Context, msg *model.GroupMessage) error {
	query := `INSERT INTO group_messages (id, group_id, sender_id, content, msg_type, group_seq, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := m.db.ExecContext(ctx, query,
		msg.ID,
		msg.GroupID,
		msg.SenderID,
		msg.Content,
		msg.MsgType,
		msg.GroupSeq,
		msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert group_messages: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) InsertMsgRevoked(ctx context.Context, revoked *model.MsgRevoked) error {
	return fmt.Errorf("stub: InsertMsgRevoked not yet implemented")
}

// ── Users (stub implementations — fleshed out in Task 12) ──

func (m *MySQLRepoImpl) GetUserByID(ctx context.Context, userID int64) (*model.User, error) {
	query := `SELECT id, username, password_hash, nickname, avatar_url, sign, gender, created_at, updated_at
	          FROM users WHERE id = ?`
	row := m.db.QueryRowContext(ctx, query, userID)
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.AvatarURL, &u.Sign, &u.Gender, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

func (m *MySQLRepoImpl) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `SELECT id, username, password_hash, nickname, avatar_url, sign, gender, created_at, updated_at
	          FROM users WHERE username = ?`
	row := m.db.QueryRowContext(ctx, query, username)
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Nickname, &u.AvatarURL, &u.Sign, &u.Gender, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}

func (m *MySQLRepoImpl) CreateUser(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (username, password_hash, nickname, avatar_url, sign, gender)
	          VALUES (?, ?, ?, ?, ?, ?)`
	result, err := m.db.ExecContext(ctx, query,
		user.Username,
		user.PasswordHash,
		user.Nickname,
		user.AvatarURL,
		user.Sign,
		user.Gender,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create user lastInsertId: %w", err)
	}
	user.ID = id
	return nil
}

func (m *MySQLRepoImpl) UpdateUser(ctx context.Context, user *model.User) error {
	query := `UPDATE users SET nickname=?, avatar_url=?, sign=?, gender=? WHERE id=?`
	_, err := m.db.ExecContext(ctx, query, user.Nickname, user.AvatarURL, user.Sign, user.Gender, user.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// ── Friendships (stub — fleshed out in Task 13) ──

func (m *MySQLRepoImpl) CreateFriendRequest(ctx context.Context, req *model.FriendRequest) error {
	return fmt.Errorf("stub: CreateFriendRequest not yet implemented")
}

func (m *MySQLRepoImpl) UpdateFriendRequest(ctx context.Context, req *model.FriendRequest) error {
	return fmt.Errorf("stub: UpdateFriendRequest not yet implemented")
}

func (m *MySQLRepoImpl) GetFriendRequestByID(ctx context.Context, id int64) (*model.FriendRequest, error) {
	return nil, fmt.Errorf("stub: GetFriendRequestByID not yet implemented")
}

func (m *MySQLRepoImpl) GetFriendRequestsByUser(ctx context.Context, userID int64) ([]model.FriendRequest, error) {
	return nil, fmt.Errorf("stub: GetFriendRequestsByUser not yet implemented")
}

func (m *MySQLRepoImpl) CreateFriendship(ctx context.Context, fs *model.Friendship) error {
	return fmt.Errorf("stub: CreateFriendship not yet implemented")
}

func (m *MySQLRepoImpl) DeleteFriendship(ctx context.Context, userID, friendID int64) error {
	return fmt.Errorf("stub: DeleteFriendship not yet implemented")
}

func (m *MySQLRepoImpl) GetFriendList(ctx context.Context, userID int64) ([]model.Friendship, error) {
	return nil, fmt.Errorf("stub: GetFriendList not yet implemented")
}

func (m *MySQLRepoImpl) IsFriend(ctx context.Context, userID, friendID int64) (bool, error) {
	return false, fmt.Errorf("stub: IsFriend not yet implemented")
}

func (m *MySQLRepoImpl) CreateBlacklist(ctx context.Context, bl *model.Blacklist) error {
	return fmt.Errorf("stub: CreateBlacklist not yet implemented")
}

func (m *MySQLRepoImpl) DeleteBlacklist(ctx context.Context, userID, blockedID int64) error {
	return fmt.Errorf("stub: DeleteBlacklist not yet implemented")
}

func (m *MySQLRepoImpl) IsBlocked(ctx context.Context, userID, blockedID int64) (bool, error) {
	return false, fmt.Errorf("stub: IsBlocked not yet implemented")
}

// ── Groups (stub — fleshed out in Task 14) ──

func (m *MySQLRepoImpl) CreateGroup(ctx context.Context, group *model.Group) (int64, error) {
	return 0, fmt.Errorf("stub: CreateGroup not yet implemented")
}

func (m *MySQLRepoImpl) UpdateGroup(ctx context.Context, group *model.Group) error {
	return fmt.Errorf("stub: UpdateGroup not yet implemented")
}

func (m *MySQLRepoImpl) GetGroupByID(ctx context.Context, groupID int64) (*model.Group, error) {
	return nil, fmt.Errorf("stub: GetGroupByID not yet implemented")
}

func (m *MySQLRepoImpl) AddGroupMember(ctx context.Context, member *model.GroupMember) error {
	return fmt.Errorf("stub: AddGroupMember not yet implemented")
}

func (m *MySQLRepoImpl) RemoveGroupMember(ctx context.Context, groupID, userID int64) error {
	return fmt.Errorf("stub: RemoveGroupMember not yet implemented")
}

func (m *MySQLRepoImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	return nil, fmt.Errorf("stub: GetGroupMembers not yet implemented")
}

func (m *MySQLRepoImpl) UpdateGroupMemberRole(ctx context.Context, groupID, userID, role int) error {
	return fmt.Errorf("stub: UpdateGroupMemberRole not yet implemented")
}

// ── Moments ──

func (m *MySQLRepoImpl) CreateMoment(ctx context.Context, moment *model.Moment) error {
	query := `INSERT INTO moments (author_id, content, media_urls, visibility, created_at)
	          VALUES (?, ?, ?, ?, ?)`
	result, err := m.db.ExecContext(ctx, query,
		moment.AuthorID,
		moment.Content,
		moment.MediaUrls,
		moment.Visibility,
		moment.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert moment: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("insert moment lastInsertId: %w", err)
	}
	moment.ID = id
	return nil
}

func (m *MySQLRepoImpl) GetMomentByID(ctx context.Context, id int64) (*model.Moment, error) {
	query := `SELECT id, author_id, content, media_urls, visibility, created_at
	          FROM moments WHERE id = ?`
	row := m.db.QueryRowContext(ctx, query, id)
	var moment model.Moment
	err := row.Scan(&moment.ID, &moment.AuthorID, &moment.Content, &moment.MediaUrls, &moment.Visibility, &moment.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get moment by id: %w", err)
	}
	return &moment, nil
}

func (m *MySQLRepoImpl) GetMomentsByUser(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error) {
	query := `SELECT id, author_id, content, media_urls, visibility, created_at
	          FROM moments WHERE author_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := m.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get moments by user: %w", err)
	}
	defer rows.Close()

	moments := make([]model.Moment, 0)
	for rows.Next() {
		var moment model.Moment
		if err := rows.Scan(&moment.ID, &moment.AuthorID, &moment.Content, &moment.MediaUrls, &moment.Visibility, &moment.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan moment: %w", err)
		}
		moments = append(moments, moment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return moments, nil
}

func (m *MySQLRepoImpl) CreateMomentLike(ctx context.Context, like *model.MomentLike) error {
	query := `INSERT INTO moment_likes (moment_id, user_id, created_at) VALUES (?, ?, ?)`
	result, err := m.db.ExecContext(ctx, query, like.MomentID, like.UserID, like.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert moment_like: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("insert moment_like lastInsertId: %w", err)
	}
	like.ID = id
	return nil
}

func (m *MySQLRepoImpl) DeleteMomentLike(ctx context.Context, momentID, userID int64) error {
	query := `DELETE FROM moment_likes WHERE moment_id = ? AND user_id = ?`
	_, err := m.db.ExecContext(ctx, query, momentID, userID)
	if err != nil {
		return fmt.Errorf("delete moment_like: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) CreateMomentComment(ctx context.Context, comment *model.MomentComment) error {
	query := `INSERT INTO moment_comments (id, moment_id, user_id, content, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := m.db.ExecContext(ctx, query, comment.ID, comment.MomentID, comment.UserID, comment.Content, comment.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert moment_comment: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) GetMomentCommentByID(ctx context.Context, id int64) (*model.MomentComment, error) {
	query := `SELECT id, moment_id, user_id, content, created_at
	          FROM moment_comments WHERE id = ?`
	row := m.db.QueryRowContext(ctx, query, id)
	var comment model.MomentComment
	err := row.Scan(&comment.ID, &comment.MomentID, &comment.UserID, &comment.Content, &comment.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get moment_comment by id: %w", err)
	}
	return &comment, nil
}

func (m *MySQLRepoImpl) DeleteMomentComment(ctx context.Context, id int64) error {
	query := `DELETE FROM moment_comments WHERE id = ?`
	_, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete moment_comment: %w", err)
	}
	return nil
}

// ── AI (stub — fleshed out in Task 16) ──

func (m *MySQLRepoImpl) CreateAISummary(ctx context.Context, summary *model.AISummary) error {
	return fmt.Errorf("stub: CreateAISummary not yet implemented")
}

func (m *MySQLRepoImpl) CreateAIProfileItem(ctx context.Context, item *model.AIProfileItem) error {
	return fmt.Errorf("stub: CreateAIProfileItem not yet implemented")
}

func (m *MySQLRepoImpl) GetAIProfileByUser(ctx context.Context, userID int64) ([]model.AIProfileItem, error) {
	return nil, fmt.Errorf("stub: GetAIProfileByUser not yet implemented")
}
