-- inbox_mark_read.lua
-- Mark all unread messages in a specific conversation as read
-- KEYS[1] = userID
-- KEYS[2] = convID
-- Logic: Scan inbox:{userID} ZSet for messages matching convID with readStatus=0,
--   for each: ZREM old entry + ZADD new entry with readStatus changed to 1,
--   then clear unread:{userID} convID field to 0
-- Returns: count of messages marked read

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
