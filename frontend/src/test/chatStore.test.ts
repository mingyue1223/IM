import { beforeEach, describe, expect, it, vi } from "vitest";
import { useChatStore } from "../stores/chatStore";

describe("chat message state", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useChatStore.setState({ mode: null, connectionState: "idle", conversations: [], messagesByConversation: {}, lastSyncTime: 0 });
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

  it("deduplicates synchronized server messages", () => {
    useChatStore.getState().initializeLive();
    const batch = { msgs: [{ msgId: 9, convId: "p_1_2", convType: 1, fromId: 2, toId: 1, msgType: 1, content: "once", readStatus: 0, timestamp: 100 }], hasMore: false, syncTime: 100 };
    useChatStore.getState().applySyncBatch(batch, 1);
    useChatStore.getState().applySyncBatch(batch, 1);
    expect(useChatStore.getState().messagesByConversation["p_1_2"]).toHaveLength(1);
  });
});
