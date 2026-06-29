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

// ── Groups (fleshed out in Task 14) ──

func (m *MySQLRepoImpl) CreateGroup(ctx context.Context, group *model.Group) (int64, error) {
	query := `INSERT INTO groups (name, notice, owner_id, max_members, created_at, updated_at)
	          VALUES (?, ?, ?, 500, NOW(), NOW())`
	result, err := m.db.ExecContext(ctx, query,
		group.Name,
		group.Notice,
		group.OwnerID,
	)
	if err != nil {
		return 0, fmt.Errorf("create group: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create group lastInsertId: %w", err)
	}
	return id, nil
}

func (m *MySQLRepoImpl) UpdateGroup(ctx context.Context, group *model.Group) error {
	query := `UPDATE groups SET name=?, notice=?, updated_at=NOW() WHERE id=?`
	_, err := m.db.ExecContext(ctx, query, group.Name, group.Notice, group.ID)
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) GetGroupByID(ctx context.Context, groupID int64) (*model.Group, error) {
	query := `SELECT id, name, notice, owner_id, max_members, created_at, updated_at
	          FROM groups WHERE id = ?`
	row := m.db.QueryRowContext(ctx, query, groupID)
	var g model.Group
	err := row.Scan(&g.ID, &g.Name, &g.Notice, &g.OwnerID, &g.MaxMembers, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get group by id: %w", err)
	}
	return &g, nil
}

func (m *MySQLRepoImpl) AddGroupMember(ctx context.Context, member *model.GroupMember) error {
	query := `INSERT INTO group_members (group_id, user_id, role, muted_until, joined_at)
	          VALUES (?, ?, ?, ?, NOW())`
	_, err := m.db.ExecContext(ctx, query,
		member.GroupID,
		member.UserID,
		member.Role,
		member.MutedUntil,
	)
	if err != nil {
		return fmt.Errorf("add group member: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) RemoveGroupMember(ctx context.Context, groupID, userID int64) error {
	query := `DELETE FROM group_members WHERE group_id=? AND user_id=?`
	_, err := m.db.ExecContext(ctx, query, groupID, userID)
	if err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	return nil
}

func (m *MySQLRepoImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	query := `SELECT id, group_id, user_id, role, muted_until, joined_at
	          FROM group_members WHERE group_id = ?`
	rows, err := m.db.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group members: %w", err)
	}
	defer rows.Close()

	members := make([]model.GroupMember, 0)
	for rows.Next() {
		var gm model.GroupMember
		err := rows.Scan(&gm.ID, &gm.GroupID, &gm.UserID, &gm.Role, &gm.MutedUntil, &gm.JoinedAt)
		if err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		members = append(members, gm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group members: %w", err)
	}
	return members, nil
}

func (m *MySQLRepoImpl) UpdateGroupMemberRole(ctx context.Context, groupID, userID, role int) error {
	query := `UPDATE group_members SET role=? WHERE group_id=? AND user_id=?`
	_, err := m.db.ExecContext(ctx, query, role, groupID, userID)
	if err != nil {
		return fmt.Errorf("update group member role: %w", err)
	}
	return nil
}

// ── Moments (stub — fleshed out in Task 15) ──

func (m *MySQLRepoImpl) CreateMoment(ctx context.Context, moment *model.Moment) error {
	return fmt.Errorf("stub: CreateMoment not yet implemented")
}

func (m *MySQLRepoImpl) GetMomentByID(ctx context.Context, id int64) (*model.Moment, error) {
	return nil, fmt.Errorf("stub: GetMomentByID not yet implemented")
}

func (m *MySQLRepoImpl) GetMomentsByUser(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error) {
	return nil, fmt.Errorf("stub: GetMomentsByUser not yet implemented")
}

func (m *MySQLRepoImpl) CreateMomentLike(ctx context.Context, like *model.MomentLike) error {
	return fmt.Errorf("stub: CreateMomentLike not yet implemented")
}

func (m *MySQLRepoImpl) DeleteMomentLike(ctx context.Context, momentID, userID int64) error {
	return fmt.Errorf("stub: DeleteMomentLike not yet implemented")
}

func (m *MySQLRepoImpl) CreateMomentComment(ctx context.Context, comment *model.MomentComment) error {
	return fmt.Errorf("stub: CreateMomentComment not yet implemented")
}

func (m *MySQLRepoImpl) DeleteMomentComment(ctx context.Context, id int64) error {
	return fmt.Errorf("stub: DeleteMomentComment not yet implemented")
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
