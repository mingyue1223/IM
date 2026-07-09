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

-- 判断是私聊还是群聊
local isGroup = false
if string.sub(convID, 1, 2) == 'g_' then
    isGroup = true
end

local zsetKey
if isGroup then
    -- 群聊：outbox:{groupID}
    local groupID = string.sub(convID, 3)
    zsetKey = 'outbox:' .. groupID
else
    -- 私聊：inbox:{userID}
    zsetKey = 'inbox:' .. userID
end

-- 扫描 ZSet 根据 msgID 查找消息
local msgs = redis.call('ZRANGE', zsetKey, 0, -1)
for i, msg in ipairs(msgs) do
    local decoded = cjson.decode(msg)
    if decoded.msgId == msgID then
        -- 授权：只有原始发送者才能撤回
        if decoded.fromId ~= tonumber(userID) then
            return 0 -- 未授权：请求者不是消息发送者
        end

        -- 检查 2 分钟撤回窗口
        if nowTimestamp - decoded.timestamp > 120000 then
            return 0 -- 已超时，无法撤回
        end

        -- ZREM 删除原消息
        redis.call('ZREM', zsetKey, msg)

        -- ZADD 添加撤回替换消息（保持相同分数以保留位置）
        redis.call('ZADD', zsetKey, decoded.timestamp, revokeMsgJSON)

        return 1 -- 成功
    end
end

return 0 -- 未找到消息
`

// ExecRevokeMsg 在 2 分钟内原子性地撤回消息。
// 它在收件箱/发件箱 ZSet 中根据 msgID 查找消息，检查
// 2 分钟时间戳窗口，并用撤回版本（msgType=6）替换原消息。
// 成功返回 true，未找到消息或超出 2 分钟窗口返回 false。
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
