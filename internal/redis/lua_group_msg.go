package redis

import (
	"context"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

// Error codes returned by group_msg_check.lua
const (
	GMErrOK        = 0
	GMErrNotMember = 1
	GMErrMuted     = 2
	GMErrDuplicate = 3
)

// GroupMsgCheckResult holds the result of the group message check Lua script.
type GroupMsgCheckResult struct {
	ErrCode  int   // 0=ok, 1=not_member, 2=muted, 3=duplicate
	MsgID    int64 // allocated global message ID (0 if error)
	GroupSeq int64 // allocated group sequence number (0 if error)
	IsMember bool  // membership status
	IsMuted  bool  // mute status
}

const luaGroupMsgCheck = `
local groupID = KEYS[1]
local senderID = KEYS[2]
local clientMsgID = KEYS[3]

-- 1. Membership check
local isMember = redis.call('SISMEMBER', 'group_members:' .. groupID, senderID)
if isMember == 0 then
    return {1, 0, 0, 0, 0}
end

-- 2. Mute status check
local memberInfo = redis.call('HGET', 'group_member_info:' .. groupID, senderID)
if memberInfo then
    local info = cjson.decode(memberInfo)
    if info.muted then
        return {2, 0, 0, 1, 1}
    end
end

-- 3. Message dedup
local dedupKey = 'msg_dedup:' .. senderID .. ':' .. clientMsgID
local dedup = redis.call('SET', dedupKey, '1', 'EX', 300, 'NX')
if dedup == false then
    return {3, 0, 0, 0, 0}
end

-- 4. Allocate global message ID
local msgID = redis.call('INCR', 'msg_id_global')

-- 5. Allocate group sequence number
local groupSeq = redis.call('INCR', 'group_seq:' .. groupID)

return {0, msgID, groupSeq, 1, 0}
`

// ExecGroupMsgCheck atomically checks group membership, mute status, dedup,
// allocates a message ID and group sequence number — all in a single
// Redis Lua script execution to avoid race conditions.
func ExecGroupMsgCheck(rdb *goredis.Client, ctx context.Context, groupID, senderID int64, clientMsgID string) (*GroupMsgCheckResult, error) {
	keys := []string{
		strconv.FormatInt(groupID, 10),
		strconv.FormatInt(senderID, 10),
		clientMsgID,
	}
	result, err := rdb.Eval(ctx, luaGroupMsgCheck, keys).Slice()
	if err != nil {
		return nil, err
	}

	res := &GroupMsgCheckResult{}
	res.ErrCode = int(result[0].(int64))
	res.MsgID = result[1].(int64)
	res.GroupSeq = result[2].(int64)
	res.IsMember = result[3].(int64) == 1
	res.IsMuted = result[4].(int64) == 1

	return res, nil
}
