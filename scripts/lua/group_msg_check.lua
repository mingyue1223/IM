-- group_msg_check.lua
-- Atomic check: group membership + mute status + dedup + message ID + group sequence
-- KEYS[1] = groupID
-- KEYS[2] = senderID
-- KEYS[3] = clientMsgID
-- Returns: {err_code, msgID, groupSeq, isMember, isMuted}
--   err_code: 0=ok, 1=not_member, 2=muted, 3=duplicate
--   msgID: allocated global message ID (0 if error)
--   groupSeq: allocated group sequence number (0 if error)
--   isMember: membership status (1=member, 0=not member)
--   isMuted: mute status (1=muted, 0=not muted)

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
