package redis

import (
	"context"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

// Error codes returned by private_msg_check.lua
const (
	PMErrOK        = 0
	PMErrNotFriend = 1
	PMErrBlocked   = 2
	PMErrDuplicate = 3
)

// Client-facing error codes mapped from Lua error codes.
// These avoid leaking raw Lua integers (1,2,3) alongside HTTP-style codes (400,500,403).
const (
	CodePMNotFriend  = 4001
	CodePMBlocked    = 4002
	CodePMDuplicate  = 4003
)

// MapLuaErrToClientCode translates a private-message Lua error code to a
// client-facing error code. Returns 0 for OK and the original code for
// unrecognized Lua codes (they fall through as "unknown error").
func MapLuaErrToClientCode(luaErrCode int) int {
	switch luaErrCode {
	case PMErrNotFriend:
		return CodePMNotFriend
	case PMErrBlocked:
		return CodePMBlocked
	case PMErrDuplicate:
		return CodePMDuplicate
	default:
		return luaErrCode
	}
}

// PrivateMsgCheckResult holds the result of the private message check Lua script.
type PrivateMsgCheckResult struct {
	ErrCode   int   // 0=ok, 1=not_friend, 2=blocked, 3=duplicate
	MsgID     int64 // allocated global message ID (0 if error)
	IsOnline  bool  // receiver online status
	IsFriend  bool  // friendship exists
	IsBlocked bool  // sender is in receiver's blacklist
}

const luaPrivateMsgCheck = `
local senderID = KEYS[1]
local receiverID = KEYS[2]
local clientMsgID = KEYS[3]

-- 1. Friendship check (bidirectional)
local friend1 = redis.call('EXISTS', 'friend:' .. senderID .. ':' .. receiverID)
local friend2 = redis.call('EXISTS', 'friend:' .. receiverID .. ':' .. senderID)
if friend1 == 0 or friend2 == 0 then
    return {1, 0, 0, 0, 0}
end

-- 2. Blacklist check (receiver blocked sender?)
local isBlocked = redis.call('SISMEMBER', 'blacklist:' .. receiverID, senderID)
if isBlocked == 1 then
    return {2, 0, 0, 1, 1}
end

-- 3. Message dedup
local dedupKey = 'msg_dedup:' .. senderID .. ':' .. clientMsgID
local dedup = redis.call('SET', dedupKey, '1', 'EX', 300, 'NX')
if dedup == false then
    return {3, 0, 0, 0, 0}
end

-- 4. Online status check
local isOnline = redis.call('EXISTS', 'online:' .. receiverID)

-- 5. Allocate global message ID (atomic INCR)
local msgID = redis.call('INCR', 'msg_id_global')

return {0, msgID, isOnline, 1, 0}
`

// ExecPrivateMsgCheck atomically checks friendship, blacklist, dedup,
// allocates a message ID, and checks online status — all in a single
// Redis Lua script execution to avoid race conditions.
func ExecPrivateMsgCheck(rdb *goredis.Client, ctx context.Context, senderID, receiverID int64, clientMsgID string) (*PrivateMsgCheckResult, error) {
	keys := []string{
		strconv.FormatInt(senderID, 10),
		strconv.FormatInt(receiverID, 10),
		clientMsgID,
	}
	result, err := rdb.Eval(ctx, luaPrivateMsgCheck, keys).Slice()
	if err != nil {
		return nil, err
	}

	res := &PrivateMsgCheckResult{}
	res.ErrCode = int(result[0].(int64))
	res.MsgID = result[1].(int64)
	res.IsOnline = result[2].(int64) == 1
	res.IsFriend = result[3].(int64) == 1
	res.IsBlocked = result[4].(int64) == 1

	return res, nil
}
