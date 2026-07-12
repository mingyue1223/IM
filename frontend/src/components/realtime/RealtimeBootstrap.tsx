import { useEffect } from "react";
import type { ServerWsMessage } from "../../../goim-ws-types";
import { useAuthStore } from "../../stores/authStore";
import { useChatStore } from "../../stores/chatStore";
import { goimSocket } from "../../realtime/socket";
import { friendsApi, groupsApi, settingsApi } from "../../lib/api";
import { configureNotifications, notifyIncomingMessage } from "../../realtime/notifications";

let loadedSettings: Awaited<ReturnType<typeof settingsApi.get>> | null = null;

function applyMutedConversations() {
  if (!loadedSettings) return;
  let mutedIds = new Set<string>();
  try {
    const parsed = JSON.parse(loadedSettings.mute_list) as unknown;
    if (Array.isArray(parsed)) mutedIds = new Set(parsed.filter((item): item is string => typeof item === "string"));
  } catch { /* Invalid legacy values behave as an empty list. */ }
  const chat = useChatStore.getState();
  for (const conversation of chat.conversations) chat.setConversationMuted(conversation.id, mutedIds.has(conversation.id));
}

function handleServerMessage(message: ServerWsMessage, currentUserId: number, clearSession: () => void) {
  const chat = useChatStore.getState();
  switch (message.type) {
    case "serverAck":
      chat.acknowledge(message.data);
      break;
    case "msg":
      if (message.data.fromId !== currentUserId) {
        const conversation = chat.conversations.find((item) => item.id === message.data.convId);
        notifyIncomingMessage({ convId: message.data.convId, title: conversation?.name ?? (message.data.convType === 2 ? "群聊新消息" : "好友新消息"), content: message.data.content });
      }
      chat.receiveMessage(message.data, currentUserId);
      goimSocket.send({ type: "deliverAck", data: { serverMsgId: message.data.msgId } });
      if (message.data.convType === 1) {
        const targetId = message.data.fromId === currentUserId ? message.data.toId : message.data.fromId;
        void friendsApi.list(100, 0).then((page) => {
          const friend = page.items.find((item) => item.friend_id === targetId);
          if (friend) useChatStore.getState().setConversationIdentity(message.data.convId, friend.nickname || `用户 #${targetId}`, friend.avatar_url);
        }).catch(() => undefined);
      } else {
        void groupsApi.get(message.data.toId).then((group) => {
          useChatStore.getState().addGroupConversation(group.id, group.name);
        }).catch(() => undefined);
      }
      break;
    case "syncBatch":
      chat.applySyncBatch(message.data, currentUserId);
      if (message.data.hasMore) {
        const { lastSyncTime, lastSyncMsgId } = useChatStore.getState();
        goimSocket.send({ type: "syncReq", data: { lastSyncTime, lastSyncMsgId, batchSize: 50 } });
      }
      break;
    case "convSync":
      chat.applyConversationSync(message.data.conversations, message.data.unreadMap);
      applyMutedConversations();
      void friendsApi.list(100, 0).then((page) => {
        for (const friend of page.items) {
          const conversation = useChatStore.getState().conversations.find((item) => !item.group && item.targetId === friend.friend_id);
          if (conversation) useChatStore.getState().setConversationIdentity(conversation.id, friend.nickname || `用户 #${friend.friend_id}`, friend.avatar_url);
        }
      }).catch(() => undefined);
      void groupsApi.list().then((groups) => {
        for (const group of groups) useChatStore.getState().addGroupConversation(group.id, group.name);
      }).catch(() => undefined);
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
    case "groupRemoved":
      chat.removeConversation(`g_${message.data.groupId}`);
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
    if (!accessToken || !userId) {
      goimSocket.disconnect();
      useChatStore.getState().resetSession();
      return;
    }

    useChatStore.getState().initializeLive(userId);
    void settingsApi.get().then((settings) => {
      loadedSettings = settings;
      configureNotifications(settings);
      applyMutedConversations();
    }).catch(() => undefined);
    goimSocket.setHandlers({
      onStateChange: (state) => {
        useChatStore.getState().setConnectionState(state);
        if (state === "connected") {
          const { lastSyncTime, lastSyncMsgId } = useChatStore.getState();
          goimSocket.send({ type: "syncReq", data: { lastSyncTime, lastSyncMsgId, batchSize: 50 } });
        }
      },
      onMessage: (message) => handleServerMessage(message, userId, clearSession),
    });
    // React StrictMode performs a synchronous setup/cleanup probe in development.
    // Deferring the real connection avoids opening and immediately closing a socket
    // before its handshake completes during that probe.
    const connectTimer = window.setTimeout(() => goimSocket.connect(accessToken), 0);
    return () => {
      window.clearTimeout(connectTimer);
      goimSocket.disconnect(false);
    };
  }, [accessToken, clearSession, previewMode, userId]);

  return null;
}
