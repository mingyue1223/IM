package redis

import (
	"context"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

const luaInboxMarkRead = `
local userID = KEYS[1]
local convID = KEYS[2]
local inboxKey = 'inbox:' .. userID
local unreadKey = 'unread:' .. userID

local msgs = redis.call('ZRANGE', inboxKey, 0, -1)
local modified = 0

for i, msg in ipairs(msgs) do
    local decoded = cjson.decode(msg)
    -- Only modify messages matching the target conversation and unread
    if decoded.convId == convID and decoded.readStatus == 0 then
        decoded.readStatus = 1
        local newMsg = cjson.encode(decoded)
        -- ZREM old entry, then ZADD with same score but new JSON value
        redis.call('ZREM', inboxKey, msg)
        redis.call('ZADD', inboxKey, decoded.timestamp, newMsg)
        modified = modified + 1
    end
end

-- Update unread count to 0 for this conversation
if modified > 0 then
    redis.call('HSET', unreadKey, convID, 0)
end

return modified
`

// ExecInboxMarkRead atomically marks all unread messages in a specific
// conversation as read. It scans the inbox ZSet, finds messages matching
// convID with readStatus=0, replaces them with readStatus=1 versions,
// and resets the unread counter. Returns the count of messages marked read.
func ExecInboxMarkRead(rdb *goredis.Client, ctx context.Context, userID int64, convID string) (int64, error) {
	keys := []string{
		strconv.FormatInt(userID, 10),
		convID,
	}
	result, err := rdb.Eval(ctx, luaInboxMarkRead, keys).Int64()
	if err != nil {
		return 0, err
	}
	return result, nil
}
