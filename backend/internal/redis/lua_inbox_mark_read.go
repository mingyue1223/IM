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
    -- 仅修改匹配目标会话且未读的消息
    if decoded.convId == convID and decoded.readStatus == 0 then
        decoded.readStatus = 1
        local newMsg = cjson.encode(decoded)
        -- 删除旧条目，然后用相同分数但新的 JSON 值重新添加
        redis.call('ZREM', inboxKey, msg)
        redis.call('ZADD', inboxKey, decoded.timestamp, newMsg)
        modified = modified + 1
    end
end

-- 将该会话的未读数更新为 0
if modified > 0 then
    redis.call('HSET', unreadKey, convID, 0)
end

return modified
`

// ExecInboxMarkRead 原子性地将指定会话中所有未读消息标记为已读。
// 它扫描收件箱 ZSet，查找 convID 匹配且 readStatus=0 的消息，
// 将其替换为 readStatus=1 的版本，并重置未读计数器。
// 返回被标记为已读的消息数量。
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
