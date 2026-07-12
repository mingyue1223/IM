package service

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/protocol"
	redislua "github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/repository"
)

// ── Lua 检查结果的错误消息 ──

const (
	ErrNotFriend  = "不是好友"
	ErrBlocked    = "你已被对方拉黑"
	ErrDuplicate  = "消息重复"
	ErrNotMember  = "不是该群组的成员"
	ErrMuted      = "你已被禁言"
	ErrRevokeFail = "无法撤回此消息"
)

// MsgService 处理所有与消息相关的 WebSocket 操作：发送、
// 送达确认、已读确认、离线同步以及消息撤回。
type MsgService struct {
	redisRepo repository.RedisRepo
	mqRepo    repository.MQRepo
	cm        *conn.ConnectionManager
	logger    *zap.Logger
}

// NewMsgService 创建一个具有所有必要依赖项的 MsgService。
func NewMsgService(redisRepo repository.RedisRepo, mqRepo repository.MQRepo, cm *conn.ConnectionManager, logger *zap.Logger) *MsgService {
	return &MsgService{
		redisRepo: redisRepo,
		mqRepo:    mqRepo,
		cm:        cm,
		logger:    logger,
	}
}

// ──────────────────────────────────────────────────────
// HandleSendMessage
// ──────────────────────────────────────────────────────

// HandleSendMessage 处理来自客户端的聊天消息。
// 私聊（convType=1）使用收件箱推送模式。
// 群聊（convType=2）使用发件箱拉取模式。
func (s *MsgService) HandleSendMessage(userID int64, data []byte) {
	var req model.SendMessage
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("解析 SendMessage 失败", zap.Error(err))
		s.sendError(userID, 400, "消息格式无效")
		return
	}

	ctx := context.Background()

	switch req.ConvType {
	case model.ConvTypePrivate:
		s.handlePrivateMsg(ctx, userID, &req)
	case model.ConvTypeGroup:
		s.handleGroupMsg(ctx, userID, &req)
	default:
		s.sendError(userID, 400, "未知的会话类型")
	}
}

// handlePrivateMsg 处理私聊（一对一）消息。
// 流程：Lua 检查 → 发布到 MQ。
// serverAck 与接收方推送统一由 MQ 消费者在消息可操作后发送。
func (s *MsgService) handlePrivateMsg(ctx context.Context, senderID int64, req *model.SendMessage) {
	checkResult, err := s.redisRepo.ExecPrivateMsgCheck(ctx, senderID, req.ToID, req.ClientMsgID)
	if err != nil {
		s.logger.Error("ExecPrivateMsgCheck 执行失败", zap.Error(err))
		s.sendError(senderID, 500, "服务器内部错误")
		return
	}

	switch checkResult.ErrCode {
	case redislua.PMErrOK:
		// 继续执行
	case redislua.PMErrNotFriend:
		s.sendError(senderID, redislua.CodePMNotFriend, ErrNotFriend)
		return
	case redislua.PMErrBlocked:
		s.sendError(senderID, redislua.CodePMBlocked, ErrBlocked)
		return
	case redislua.PMErrDuplicate:
		s.sendError(senderID, redislua.CodePMDuplicate, ErrDuplicate)
		return
	default:
		s.sendError(senderID, redislua.MapLuaErrToClientCode(checkResult.ErrCode), "未知错误")
		return
	}

	// 构建 PrivateMessage 用于 MQ 发布
	pm := &model.PrivateMessage{
		ID:          checkResult.MsgID,
		ClientMsgID: req.ClientMsgID,
		SenderID:    senderID,
		ReceiverID:  req.ToID,
		Content:     req.Content,
		MsgType:     req.MsgType,
		CreatedAt:   time.UnixMilli(req.Timestamp),
	}

	// 发布到 MQ
	if err := s.mqRepo.PublishPrivateMsg(ctx, pm); err != nil {
		s.logger.Error("PublishPrivateMsg 发布失败", zap.Error(err))
		s.sendError(senderID, 500, "消息发布失败")
		return
	}

	s.logger.Debug("私聊消息已发布",
		zap.Int64("msgID", checkResult.MsgID),
		zap.Int64("senderID", senderID),
		zap.Int64("receiverID", req.ToID),
	)

}

// handleGroupMsg 处理群聊消息。
// 流程：Lua 检查 → 构建 GroupMessage → 发布到 MQ。
// 带 groupSeq 的 serverAck 由 MQ 消费者在消息可操作后发送。
func (s *MsgService) handleGroupMsg(ctx context.Context, senderID int64, req *model.SendMessage) {
	checkResult, err := s.redisRepo.ExecGroupMsgCheck(ctx, req.ToID, senderID, req.ClientMsgID)
	if err != nil {
		s.logger.Error("ExecGroupMsgCheck 执行失败", zap.Error(err))
		s.sendError(senderID, 500, "服务器内部错误")
		return
	}

	switch checkResult.ErrCode {
	case redislua.GMErrOK:
		// 继续执行
	case redislua.GMErrNotMember:
		s.sendError(senderID, redislua.CodeGMNotMember, ErrNotMember)
		return
	case redislua.GMErrMuted:
		s.sendError(senderID, redislua.CodeGMMuted, ErrMuted)
		return
	case redislua.GMErrDuplicate:
		s.sendError(senderID, redislua.CodeGMDuplicate, ErrDuplicate)
		return
	default:
		s.sendError(senderID, redislua.MapGroupLuaErrToClientCode(checkResult.ErrCode), "未知错误")
		return
	}

	// 构建 GroupMessage 用于 MQ 发布
	gm := &model.GroupMessage{
		ID:          checkResult.MsgID,
		ClientMsgID: req.ClientMsgID,
		GroupID:     req.ToID,
		SenderID:    senderID,
		Content:     req.Content,
		MsgType:     req.MsgType,
		GroupSeq:    checkResult.GroupSeq,
		CreatedAt:   time.UnixMilli(req.Timestamp),
	}

	// 发布到 MQ
	if err := s.mqRepo.PublishGroupMsg(ctx, gm); err != nil {
		s.logger.Error("PublishGroupMsg 发布失败", zap.Error(err))
		s.sendError(senderID, 500, "消息发布失败")
		return
	}

	s.logger.Debug("群聊消息已发布",
		zap.Int64("msgID", checkResult.MsgID),
		zap.Int64("groupID", req.ToID),
		zap.Int64("groupSeq", checkResult.GroupSeq),
	)

}

// ──────────────────────────────────────────────────────
// HandleDeliverAck
// ──────────────────────────────────────────────────────

// HandleDeliverAck 处理来自客户端的送达确认。
// 此操作用于通知——客户端确认已收到消息。
func (s *MsgService) HandleDeliverAck(userID int64, data []byte) {
	var ack model.DeliverAck
	if err := json.Unmarshal(data, &ack); err != nil {
		s.logger.Warn("解析 DeliverAck 失败", zap.Error(err))
		return
	}

	s.logger.Debug("收到送达确认",
		zap.Int64("userID", userID),
		zap.Int64("serverMsgID", ack.ServerMsgID),
	)
}

// ──────────────────────────────────────────────────────
// HandleReadAck
// ──────────────────────────────────────────────────────

// HandleReadAck 处理来自客户端的已读确认。
// 它通过 Lua 脚本原子性地将指定会话中所有未读消息标记为已读，
// 并清除未读计数器。
func (s *MsgService) HandleReadAck(userID int64, data []byte) {
	var req model.ReadAck
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("解析 ReadAck 失败", zap.Error(err))
		s.sendError(userID, 400, "readAck 格式无效")
		return
	}

	ctx := context.Background()

	count, err := s.redisRepo.ExecInboxMarkRead(ctx, userID, req.ConvID)
	if err != nil {
		s.logger.Error("ExecInboxMarkRead 执行失败",
			zap.Int64("userID", userID),
			zap.String("convID", req.ConvID),
			zap.Error(err),
		)
		s.sendError(userID, 500, "标记已读失败")
		return
	}
	// 群消息存放在共享 outbox 而非个人 inbox，所以上面的 Lua 对群聊会返回 0。
	// 无论会话类型如何，收到 readAck 都应清空该会话的服务端未读计数。
	if err := s.redisRepo.ClearUnread(ctx, userID, req.ConvID); err != nil {
		s.logger.Error("ClearUnread 执行失败",
			zap.Int64("userID", userID),
			zap.String("convID", req.ConvID),
			zap.Error(err),
		)
		s.sendError(userID, 500, "标记已读失败")
		return
	}

	s.logger.Debug("收件箱已标记为已读",
		zap.Int64("userID", userID),
		zap.String("convID", req.ConvID),
		zap.Int64("count", count),
	)
}

// ──────────────────────────────────────────────────────
// HandleSyncReq
// ──────────────────────────────────────────────────────

// HandleSyncReq 处理来自客户端的离线同步请求。
// 它从用户的收件箱（私聊）和群发件箱中拉取新消息，
// 合并排序后，通过 WebSocket 推送 SyncBatch 和 ConvSync。
func (s *MsgService) HandleSyncReq(userID int64, data []byte) {
	var req model.SyncReq
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("解析 SyncReq 失败", zap.Error(err))
		s.sendError(userID, 400, "syncReq 格式无效")
		return
	}

	if req.BatchSize <= 0 {
		req.BatchSize = 50 // 默认批量大小
	}

	ctx := context.Background()

	// ── 1. 从收件箱拉取私聊消息 ──
	privateMsgs, err := s.redisRepo.ReadInbox(ctx, userID, req.LastSyncTime, req.LastSyncMsgID, req.BatchSize+1)
	if err != nil {
		s.logger.Error("ReadInbox 读取失败", zap.Error(err))
		s.sendError(userID, 500, "同步失败")
		return
	}

	// ── 2. 从群发件箱拉取群聊消息 ──
	groupIDs, err := s.redisRepo.GetGroupMemberships(ctx, userID)
	if err != nil {
		s.logger.Error("GetGroupMemberships 查询失败", zap.Error(err))
		// 只使用私聊消息继续执行
	}

	var groupMsgs []model.InboxMessage
	for _, gid := range groupIDs {
		// 历史同步只由客户端复合游标控制。群已读位置用于未读语义，
		// 不能用来过滤历史，否则客户端清空本地状态后重新登录会永久缺消息。
		outboxMsgs, err := s.redisRepo.ReadOutbox(ctx, gid, req.LastSyncTime, req.LastSyncMsgID, req.BatchSize+1)
		if err != nil {
			s.logger.Warn("ReadOutbox 读取失败", zap.Int64("groupID", gid), zap.Error(err))
			continue
		}
		groupMsgs = append(groupMsgs, outboxMsgs...)
	}

	// ── 3. 合并并排序所有消息 ──
	allMsgs := append(privateMsgs, groupMsgs...)
	sort.Slice(allMsgs, func(i, j int) bool {
		if allMsgs[i].Timestamp != allMsgs[j].Timestamp {
			return allMsgs[i].Timestamp < allMsgs[j].Timestamp
		}
		return allMsgs[i].MsgID < allMsgs[j].MsgID
	})

	// ── 4. 判断 hasMore ──
	// 如果符合条件的消息数量 >= batchSize，则后续可能还有更多消息。
	// 这处理了群聊过滤导致数量低于 batchSize 的情况，
	// 即使实际上还有更多符合条件的条目存在。
	hasMore := len(allMsgs) > req.BatchSize
	if hasMore {
		allMsgs = allMsgs[:req.BatchSize]
	}

	// ── 5. 确定 syncTime ──
	var syncTime int64
	var syncMsgID int64
	if len(allMsgs) > 0 {
		syncTime = allMsgs[len(allMsgs)-1].Timestamp
		syncMsgID = allMsgs[len(allMsgs)-1].MsgID
	} else {
		syncTime = time.Now().UnixMilli()
	}

	// ── 6. 推送 SyncBatch ──
	batch := &model.SyncBatch{
		Messages:  allMsgs,
		HasMore:   hasMore,
		SyncTime:  syncTime,
		SyncMsgID: syncMsgID,
	}
	s.pushToUser(userID, protocol.TypeSyncBatch, batch)

	// ── 7. 推送 ConvSync ──
	convList, err := s.redisRepo.GetConvList(ctx, userID)
	if err != nil {
		s.logger.Warn("GetConvList 查询失败", zap.Error(err))
	}

	unreadMap, err := s.redisRepo.GetUnreadMap(ctx, userID)
	if err != nil {
		s.logger.Warn("GetUnreadMap 查询失败", zap.Error(err))
	}

	convSync := &model.ConvSync{
		Conversations: convList,
		UnreadMap:     unreadMap,
	}
	s.pushToUser(userID, protocol.TypeConvSync, convSync)

	s.logger.Debug("同步完成",
		zap.Int64("userID", userID),
		zap.Int("msgCount", len(allMsgs)),
		zap.Bool("hasMore", hasMore),
	)
}

// ──────────────────────────────────────────────────────
// HandleRevokeMsg
// ──────────────────────────────────────────────────────

// HandleRevokeMsg 处理来自客户端的消息撤回请求。
// 它通过 Lua 脚本原子性地将收件箱/发件箱 ZSet 中的原始消息替换为"已撤回"版本
// （msgType=6），然后向对方推送 msgRevoked 通知。
func (s *MsgService) HandleRevokeMsg(userID int64, data []byte) {
	var req model.RevokeMsgReq
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("解析 RevokeMsgReq 失败", zap.Error(err))
		s.sendError(userID, 400, "revokeMsg 格式无效")
		return
	}

	ctx := context.Background()

	// 根据 convID 前缀确定会话类型
	var convType int
	if strings.HasPrefix(req.ConvID, "p_") {
		convType = model.ConvTypePrivate
	} else if strings.HasPrefix(req.ConvID, "g_") {
		convType = model.ConvTypeGroup
	} else {
		s.sendError(userID, 400, "convId 格式无效")
		return
	}

	// 构建已撤回的替换消息
	now := time.Now().UnixMilli()
	revokedMsg := model.InboxMessage{
		MsgID:     req.ServerMsgID,
		ConvID:    req.ConvID,
		ConvType:  convType,
		FromID:    userID,
		MsgType:   model.MsgTypeRevoked,
		Content:   "消息已撤回",
		Timestamp: now,
	}
	revokeMsgJSON, err := json.Marshal(revokedMsg)
	if err != nil {
		s.logger.Error("序列化撤回消息失败", zap.Error(err))
		s.sendError(userID, 500, "服务器内部错误")
		return
	}

	// 执行 Lua 撤回脚本
	ok, err := s.redisRepo.ExecRevokeMsg(ctx, userID, req.ConvID, req.ServerMsgID, string(revokeMsgJSON), now)
	if err != nil {
		s.logger.Error("ExecRevokeMsg 执行失败", zap.Error(err))
		s.sendError(userID, 500, "撤回失败")
		return
	}

	if !ok {
		s.sendError(userID, 403, ErrRevokeFail)
		return
	}

	s.logger.Debug("消息已撤回",
		zap.Int64("userID", userID),
		zap.String("convID", req.ConvID),
		zap.Int64("msgID", req.ServerMsgID),
	)

	// 推送 msgRevoked 通知
	notification := &model.RevokedNotification{
		ConvID:      req.ConvID,
		ServerMsgID: req.ServerMsgID,
		OperatorID:  userID,
	}

	// 通知发送方以确认
	s.pushToUser(userID, protocol.TypeMsgRevoked, notification)

	// 私聊情况下，推送给对方
	if convType == model.ConvTypePrivate {
		otherID := getOtherPartyID(req.ConvID, userID)
		if otherID > 0 {
			s.pushToUser(otherID, protocol.TypeMsgRevoked, notification)
		}
	}
	// 群聊情况下，成员通过同步或 MQ 消费者广播发现撤回
}

// ──────────────────────────────────────────────────────
// 辅助方法
// ──────────────────────────────────────────────────────

// sendError 通过 WebSocket 向指定用户推送 WsError。
func (s *MsgService) sendError(userID int64, code int, message string) {
	wsErr := &model.WsError{Code: code, Message: message}
	s.pushToUser(userID, protocol.TypeError, wsErr)
}

// pushToUser 编码消息并将其推送到用户的 WebSocket SendCh。
// 如果用户不在线，消息将被静默丢弃（用户将通过同步获取）。
func (s *MsgService) pushToUser(userID int64, msgType string, data interface{}) {
	encoded, err := protocol.EncodeMsg(msgType, data)
	if err != nil {
		s.logger.Error("EncodeMsg 编码失败", zap.String("type", msgType), zap.Error(err))
		return
	}

	client, ok := s.cm.Get(userID)
	if !ok {
		// 用户不在线——将通过同步获取消息
		return
	}

	select {
	case client.SendCh <- encoded:
		// 发送成功
	default:
		// 缓冲区已满——丢弃消息（用户稍后会同步）
		s.logger.Warn("SendCh 缓冲区已满，丢弃消息",
			zap.Int64("userID", userID),
			zap.String("type", msgType),
		)
	}
}

// getOtherPartyID 从私聊会话 ID 中提取对方的用户 ID。
// 私聊 convID 格式："p_{较小的ID}_{较大的ID}"
func getOtherPartyID(convID string, userID int64) int64 {
	if !strings.HasPrefix(convID, "p_") {
		return 0
	}
	parts := strings.Split(convID[2:], "_")
	if len(parts) != 2 {
		return 0
	}
	id1, err1 := strconv.ParseInt(parts[0], 10, 64)
	id2, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	if id1 == userID {
		return id2
	}
	if id2 == userID {
		return id1
	}
	return 0
}
