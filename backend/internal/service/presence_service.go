package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/protocol"
	"github.com/goim/goim/internal/repository"
)

type PresenceService struct {
	mysqlRepo repository.MySQLRepo
	cm        *conn.ConnectionManager
	logger    *zap.Logger
}

func NewPresenceService(mysqlRepo repository.MySQLRepo, cm *conn.ConnectionManager, logger *zap.Logger) *PresenceService {
	return &PresenceService{mysqlRepo: mysqlRepo, cm: cm, logger: logger}
}

func (s *PresenceService) NotifyFriends(userID int64, online bool) {
	friends, err := s.mysqlRepo.GetFriendList(context.Background(), userID)
	if err != nil {
		s.logger.Warn("failed to load friends for presence event", zap.Error(err))
		return
	}
	event := model.PresenceEvent{UserID: userID, Online: online}
	if !online {
		event.LastSeenAt = time.Now().UnixMilli()
	}
	payload, err := protocol.EncodeMsg(protocol.TypePresence, &event)
	if err != nil {
		return
	}
	for _, friend := range friends {
		if client, ok := s.cm.Get(friend.FriendID); ok {
			select {
			case client.SendCh <- payload:
			default:
				s.logger.Debug("presence event dropped for slow client", zap.Int64("userID", friend.FriendID))
			}
		}
	}
}
