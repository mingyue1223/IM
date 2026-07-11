import { useEffect } from "react";
import type { ServerWsMessage } from "../../../goim-ws-types";
import { useAuthStore } from "../../stores/authStore";
import { useChatStore } from "../../stores/chatStore";
import { goimSocket } from "../../realtime/socket";

function handleServerMessage(message: ServerWsMessage, currentUserId: number, clearSession: () => void) {
  const chat = useChatStore.getState();
  switch (message.type) {
    case "serverAck":
      chat.acknowledge(message.data);
      break;
    case "msg":
      chat.receiveMessage(message.data, currentUserId);
      goimSocket.send({ type: "deliverAck", data: { serverMsgId: message.data.msgId } });
      break;
    case "syncBatch":
      chat.applySyncBatch(message.data, currentUserId);
      if (message.data.hasMore) goimSocket.send({ type: "syncReq", data: { lastSyncTime: useChatStore.getState().lastSyncTime, batchSize: 50 } });
      break;
    case "convSync":
      chat.applyConversationSync(message.data.conversations, message.data.unreadMap);
      break;
    case "msgRevoked":
      chat.revokeMessage(message.data.convId, message.data.serverMsgId);
      break;
    case "error":
      chat.failLatestPending(message.data.message);
      break;
    case "kick":
      goimSocket.disconnect();
      clearSession();
      break;
  }
}

export function RealtimeBootstrap() {
  const accessToken = useAuthStore((state) => state.accessToken);
  const previewMode = useAuthStore((state) => state.previewMode);
  const userId = useAuthStore((state) => state.user?.id ?? 0);
  const clearSession = useAuthStore((state) => state.clearSession);

  useEffect(() => {
    if (previewMode) {
      goimSocket.disconnect();
      useChatStore.getState().initializePreview();
      return;
    }
    if (!accessToken) return;

    useChatStore.getState().initializeLive();
    goimSocket.setHandlers({
      onStateChange: (state) => {
        useChatStore.getState().setConnectionState(state);
        if (state === "connected") {
          goimSocket.send({ type: "syncReq", data: { lastSyncTime: useChatStore.getState().lastSyncTime, batchSize: 50 } });
        }
      },
      onMessage: (message) => handleServerMessage(message, userId, clearSession),
    });
    goimSocket.connect(accessToken);
    return () => goimSocket.disconnect(false);
  }, [accessToken, clearSession, previewMode, userId]);

  return null;
}
