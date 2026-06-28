-- revoke_msg.lua
-- Revoke a message within 2 minutes: find by msgID, check timestamp, replace with revoked version
-- KEYS[1] = userID (the user performing revoke, used to locate inbox/outbox)
-- KEYS[2] = convID (conversation ID, e.g. p_100_200 or g_5)
-- KEYS[3] = msgID (the message ID to revoke)
-- ARGV[1] = revokeMsgJSON (the replacement message with msgType=6)
-- ARGV[2] = nowTimestamp (current timestamp in milliseconds, for 2-min check)
-- Returns: 1 (success) or 0 (failed - not found or too late)

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
