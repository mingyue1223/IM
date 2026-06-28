-- private_msg_check.lua
-- Atomic check: friendship + online + blacklist + dedup + message ID allocation
-- KEYS[1] = senderID
-- KEYS[2] = receiverID
-- KEYS[3] = clientMsgID
-- Returns: {err_code, msgID, isOnline, isFriend, isBlocked}
--   err_code: 0=ok, 1=not_friend, 2=blocked, 3=duplicate
--   msgID: allocated global message ID (0 if error)
--   isOnline: receiver online status (1=online, 0=offline)
--   isFriend: friendship status (1=friend, 0=not friend)
--   isBlocked: blacklist status (1=blocked, 0=not blocked)

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
