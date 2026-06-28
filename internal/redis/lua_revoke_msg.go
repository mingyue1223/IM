package redis

import (
	"context"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

const luaRevokeMsg = `
local userID = KEYS[1]
local convID = KEYS[2]
local msgID = tonumber(KEYS[3])
local revokeMsgJSON = ARGV[1]
local nowTimestamp = tonumber(ARGV[2])

-- Determine if this is a private or group conversation
local isGroup = false
if string.sub(convID, 1, 2) == 'g_' then
    isGroup = true
end

local zsetKey
if isGroup then
    -- Group: outbox:{groupID}
    local groupID = string.sub(convID, 3)
    zsetKey = 'outbox:' .. groupID
else
    -- Private: inbox:{userID}
    zsetKey = 'inbox:' .. userID
end

-- Scan the ZSet to find the message by msgID
local msgs = redis.call('ZRANGE', zsetKey, 0, -1)
for i, msg in ipairs(msgs) do
    local decoded = cjson.decode(msg)
    if decoded.msgId == msgID then
        -- Check 2-minute window
        if nowTimestamp - decoded.timestamp > 120000 then
            return 0 -- Too late, cannot revoke
        end

        -- ZREM the original message
        redis.call('ZREM', zsetKey, msg)

        -- ZADD the revoked replacement (same score to preserve position)
        redis.call('ZADD', zsetKey, decoded.timestamp, revokeMsgJSON)

        return 1 -- Success
    end
end

return 0 -- Message not found
`

// ExecRevokeMsg atomically revokes a message within 2 minutes.
// It finds the message by msgID in the inbox/outbox ZSet, checks the
// 2-minute timestamp window, and replaces the original message with a
// revoked version (msgType=6). Returns true if successful, false if
// the message was not found or the 2-minute window has expired.
func ExecRevokeMsg(rdb *goredis.Client, ctx context.Context, userID int64, convID string, msgID int64, revokeMsgJSON string, nowTimestamp int64) (bool, error) {
	keys := []string{
		strconv.FormatInt(userID, 10),
		convID,
		strconv.FormatInt(msgID, 10),
	}
	args := []interface{}{
		revokeMsgJSON,
		strconv.FormatInt(nowTimestamp, 10),
	}
	result, err := rdb.Eval(ctx, luaRevokeMsg, keys, args...).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
