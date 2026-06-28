package service

// MsgService handles message-related operations.
// This is a placeholder stub — the full implementation is in Task 9.
type MsgService struct{}

// HandleSendMessage processes an incoming chat message from a client.
func (s *MsgService) HandleSendMessage(userID int64, data []byte) {
	// TODO: implement in Task 9 (message service)
}

// HandleDeliverAck processes a delivery acknowledgement from a client.
func (s *MsgService) HandleDeliverAck(userID int64, data []byte) {
	// TODO: implement in Task 9 (message service)
}

// HandleReadAck processes a read acknowledgement from a client.
func (s *MsgService) HandleReadAck(userID int64, data []byte) {
	// TODO: implement in Task 9 (message service)
}

// HandleSyncReq processes a sync request from a client.
func (s *MsgService) HandleSyncReq(userID int64, data []byte) {
	// TODO: implement in Task 9 (message service)
}

// HandleRevokeMsg processes a message revoke request from a client.
func (s *MsgService) HandleRevokeMsg(userID int64, data []byte) {
	// TODO: implement in Task 9 (message service)
}
