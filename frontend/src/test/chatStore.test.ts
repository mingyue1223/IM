import { beforeEach, describe, expect, it, vi } from "vitest";
import { useChatStore } from "../stores/chatStore";

describe("chat message state", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useChatStore.setState({ mode: null, liveUserId: null, connectionState: "idle", syncCompleted: false, conversations: [], messagesByConversation: {}, typingByConversation: {}, lastSyncTime: 0, lastSyncMsgId: 0 });
  });

  it("hydrates the preview conversations", () => {
    useChatStore.getState().initializePreview();
    expect(useChatStore.getState().conversations.length).toBeGreaterThan(0);
    expect(useChatStore.getState().messagesByConversation["lin-cheng"]).toHaveLength(4);
  });

  it("moves a preview message from pending to sent after acknowledgement", () => {
    useChatStore.getState().initializePreview();
    const conversation = useChatStore.getState().conversations[0];
    useChatStore.getState().sendText(conversation, "hello", true);
    expect(useChatStore.getState().messagesByConversation[conversation.id].at(-1)?.status).toBe("pending");
    vi.advanceTimersByTime(500);
    expect(useChatStore.getState().messagesByConversation[conversation.id].at(-1)?.status).toBe("sent");
  });

  it("moves the conversation to the top as soon as a local message is sent", () => {
    useChatStore.getState().initializePreview();
    const target = useChatStore.getState().conversations[2];

    useChatStore.getState().sendText(target, "hello", true);

    expect(useChatStore.getState().conversations[0]).toMatchObject({
      id: target.id,
      preview: "hello",
    });
  });

  it("deduplicates synchronized server messages", () => {
    useChatStore.getState().initializeLive(1);
    const batch = { msgs: [{ msgId: 9, convId: "p_1_2", convType: 1, fromId: 2, toId: 1, msgType: 1, content: "once", readStatus: 0, timestamp: 100 }], hasMore: false, syncTime: 100, syncMsgId: 9 };
    useChatStore.getState().applySyncBatch(batch, 1);
    useChatStore.getState().applySyncBatch(batch, 1);
    expect(useChatStore.getState().messagesByConversation["p_1_2"]).toHaveLength(1);
    expect(useChatStore.getState().lastSyncTime).toBe(100);
    expect(useChatStore.getState().lastSyncMsgId).toBe(9);
  });

  it("clears live messages when the authenticated user changes", () => {
    useChatStore.getState().initializeLive(1);
    useChatStore.getState().applySyncBatch({ msgs: [{ msgId: 10, convId: "p_1_2", convType: 1, fromId: 2, toId: 1, msgType: 1, content: "private", readStatus: 0, timestamp: 101 }], hasMore: false, syncTime: 101, syncMsgId: 10 }, 1);
    expect(useChatStore.getState().messagesByConversation["p_1_2"]).toHaveLength(1);

    useChatStore.getState().initializeLive(2);
    expect(useChatStore.getState().liveUserId).toBe(2);
    expect(useChatStore.getState().messagesByConversation).toEqual({});
    expect(useChatStore.getState().lastSyncTime).toBe(0);
    expect(useChatStore.getState().lastSyncMsgId).toBe(0);
  });

  it("marks an empty conversation synchronization as completed", () => {
    useChatStore.getState().initializeLive(1);
    expect(useChatStore.getState().syncCompleted).toBe(false);

    useChatStore.getState().applyConversationSync([], {});
    expect(useChatStore.getState().syncCompleted).toBe(true);
    expect(useChatStore.getState().conversations).toEqual([]);
  });

  it("creates and surfaces a conversation for an incoming message", () => {
    useChatStore.getState().initializeLive(1);
    useChatStore.getState().receiveMessage({ msgId: 21, convId: "p_1_2", convType: 1, fromId: 2, toId: 1, msgType: 1, content: "hello", readStatus: 0, timestamp: 200 }, 1);
    expect(useChatStore.getState().conversations[0]).toMatchObject({ id: "p_1_2", targetId: 2, unread: 1, preview: "hello" });
    expect(useChatStore.getState().messagesByConversation["p_1_2"]).toHaveLength(1);
    useChatStore.getState().setConversationIdentity("p_1_2", "Alice");
    expect(useChatStore.getState().conversations[0].name).toBe("Alice");
  });

  it("uses the group ID as the target for a conversation received from another member", () => {
    useChatStore.getState().initializeLive(226);
    useChatStore.getState().receiveMessage({ msgId: 22, convId: "g_22", convType: 2, fromId: 225, toId: 22, msgType: 1, content: "group hello", readStatus: 0, groupSeq: 1, timestamp: 201 }, 226);
    expect(useChatStore.getState().conversations[0]).toMatchObject({ id: "g_22", targetId: 22, group: true });
    expect(useChatStore.getState().messagesByConversation["g_22"][0]).toMatchObject({ senderId: 225 });
  });

  it("hydrates an existing group conversation name", () => {
    useChatStore.getState().initializeLive(226);
    useChatStore.getState().receiveMessage({ msgId: 23, convId: "g_22", convType: 2, fromId: 225, toId: 22, msgType: 1, content: "group hello", readStatus: 0, groupSeq: 1, timestamp: 202 }, 226);
    useChatStore.getState().addGroupConversation(22, "联调群");
    expect(useChatStore.getState().conversations[0]).toMatchObject({ name: "联调群", targetId: 22 });
  });

  it("removes a group conversation and its messages after leaving or removal", () => {
    useChatStore.getState().initializeLive(227);
    useChatStore.getState().receiveMessage({ msgId: 24, convId: "g_22", convType: 2, fromId: 225, toId: 22, msgType: 1, content: "group hello", readStatus: 0, groupSeq: 1, timestamp: 203 }, 227);
    useChatStore.getState().removeConversation("g_22");
    expect(useChatStore.getState().conversations).toEqual([]);
    expect(useChatStore.getState().messagesByConversation["g_22"]).toBeUndefined();
  });

  it("parses image attachment metadata and keeps reply references", () => {
    useChatStore.getState().initializeLive(1);
    const content = JSON.stringify({ id: 7, url: "/uploads/attachments/picture.png", name: "picture.png", size: 128, mimeType: "image/png", kind: "image" });
    useChatStore.getState().receiveMessage({ msgId: 30, convId: "p_1_2", convType: 1, fromId: 2, toId: 1, msgType: 2, content, replyToMsgId: 29, readStatus: 0, timestamp: 300 }, 1);
    expect(useChatStore.getState().messagesByConversation["p_1_2"][0]).toMatchObject({ replyToMsgId: 29, attachment: { name: "picture.png", kind: "image" } });
    expect(useChatStore.getState().conversations[0].preview).toBe("[图片]");
  });

  it("prepends history without duplicating live messages", () => {
    useChatStore.getState().initializeLive(1);
    const latest = { msgId: 41, convId: "p_1_2", convType: 1 as const, fromId: 2, toId: 1, msgType: 1 as const, content: "latest", readStatus: 0, timestamp: 410 };
    useChatStore.getState().receiveMessage(latest, 1);
    useChatStore.getState().prependHistory("p_1_2", [{ ...latest, msgId: 40, content: "older", timestamp: 400 }, latest], 1);
    expect(useChatStore.getState().messagesByConversation["p_1_2"].map((message) => message.serverMsgId)).toEqual([40, 41]);
  });

  it("applies typing and presence events to conversation state", () => {
    useChatStore.getState().initializeLive(1);
    useChatStore.getState().receiveMessage({ msgId: 50, convId: "p_1_2", convType: 1, fromId: 2, toId: 1, msgType: 1, content: "hello", readStatus: 0, timestamp: 500 }, 1);
    useChatStore.getState().setTyping("p_1_2", true);
    useChatStore.getState().setPresence(2, true);
    expect(useChatStore.getState().typingByConversation["p_1_2"]).toBeTypeOf("number");
    expect(useChatStore.getState().conversations[0].online).toBe(true);
  });
});
