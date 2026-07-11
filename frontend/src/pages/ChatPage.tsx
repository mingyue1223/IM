import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { BellOff, CheckCheck, ChevronDown, CircleAlert, LoaderCircle, MessageCircle, MoreHorizontal, Plus, RotateCcw, Search, SendHorizontal, Users, WifiOff } from "lucide-react";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { NavLink, useNavigate, useParams } from "react-router-dom";
import { Avatar, Badge, Button, Drawer, EmptyState, IconButton, TextField } from "../components/ui";
import { goimSocket } from "../realtime/socket";
import { useAuthStore } from "../stores/authStore";
import { useChatStore, type ChatMessage } from "../stores/chatStore";
import { CreateGroupDrawer, GroupManagementDrawer } from "../features/groups/GroupManagement";

const emptyMessages: ChatMessage[] = [];

const connectionLabels = {
  idle: "未连接",
  connecting: "正在连接",
  connected: "已连接",
  reconnecting: "正在重新连接",
  offline: "连接中断",
} as const;

export function ChatPage() {
  const { conversationId } = useParams();
  const navigate = useNavigate();
  const conversations = useChatStore((state) => state.conversations);
  const messages = useChatStore((state) => state.messagesByConversation[conversationId ?? ""] ?? emptyMessages);
  const connectionState = useChatStore((state) => state.connectionState);
  const mode = useChatStore((state) => state.mode);
  const sendText = useChatStore((state) => state.sendText);
  const retryMessage = useChatStore((state) => state.retryMessage);
  const markConversationRead = useChatStore((state) => state.markConversationRead);
  const previewMode = useAuthStore((state) => state.previewMode);
  const selected = conversations.find((item) => item.id === conversationId);
  const selectedId = selected?.id;
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [createGroupOpen, setCreateGroupOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const reduceMotion = useReducedMotion();
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (mode === "live" && conversations.length > 0 && !selected) navigate(`/app/chats/${conversations[0].id}`, { replace: true });
  }, [conversations, mode, navigate, selected]);

  useEffect(() => {
    if (!selectedId) return;
    markConversationRead(selectedId);
    if (!previewMode && connectionState === "connected") goimSocket.send({ type: "readAck", data: { convId: selectedId } });
  }, [connectionState, markConversationRead, previewMode, selectedId]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth" });
  }, [conversationId, messages.length, reduceMotion]);

  const sendMessage = () => {
    const content = draft.trim();
    if (!content || !selected) return;
    sendText(selected, content, previewMode);
    setDraft("");
  };

  const handleComposerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      sendMessage();
    }
  };

  return (
    <>
      <aside className="module-sidebar conversation-sidebar">
        <header className="module-sidebar__header"><div><span className="eyebrow">Messages</span><h1>聊天</h1></div><IconButton label="创建群聊" onClick={() => setCreateGroupOpen(true)}><Plus size={18} /></IconButton></header>
        <div className={`connection-status connection-status--${connectionState}`}><span />{previewMode ? "预览连接" : connectionLabels[connectionState]}</div>
        <div className="module-sidebar__search"><TextField aria-label="搜索会话" leadingIcon={<Search size={16} />} placeholder="搜索会话" /></div>
        <div className="sidebar-section-label"><span>最近会话</span><small>{conversations.length}</small></div>
        <nav aria-label="会话列表" className="conversation-nav">
          {conversations.map((item) => (
            <NavLink className={({ isActive }) => isActive ? "conversation-item is-active" : "conversation-item"} key={item.id} to={`/app/chats/${item.id}`}>
              <Avatar name={item.name} online={item.online} />
              <span className="conversation-item__copy"><strong>{item.name}</strong><small>{item.preview}</small></span>
              <span className="conversation-item__meta"><time>{item.time}</time>{item.unread > 0 ? <b>{item.unread}</b> : item.muted ? <BellOff size={12} /> : null}</span>
            </NavLink>
          ))}
        </nav>
      </aside>

      <section className="module-main chat-main">
        {selected ? (
          <>
            <header className="chat-header">
              <div className="chat-header__person"><Avatar name={selected.name} online={selected.online} /><div><h2>{selected.name}</h2><p>{selected.group ? "群聊" : selected.online ? "在线" : "离线"}</p></div></div>
              <div className="chat-header__actions"><IconButton label="搜索聊天记录"><Search size={18} /></IconButton><IconButton label="查看聊天资料" onClick={() => setDrawerOpen(true)}><MoreHorizontal size={19} /></IconButton></div>
            </header>
            {connectionState !== "connected" && !previewMode && <div className="chat-connection-banner"><WifiOff size={14} /><span>{connectionLabels[connectionState]}，未确认的消息会保留并允许重试。</span></div>}
            <div className="message-scroll">
              <div className="message-day"><span>今天</span></div>
              {messages.length === 0 && <div className="conversation-empty"><MessageCircle size={21} /><p>还没有消息，发一句问候吧。</p></div>}
              <AnimatePresence initial={false}>
                {messages.map((message) => (
                  <motion.div animate={{ opacity: 1, y: 0 }} className={`message-row ${message.from === "me" ? "is-me" : ""} ${message.status === "revoked" ? "is-revoked" : ""}`} initial={{ opacity: 0, y: reduceMotion ? 0 : 8 }} key={message.id} transition={{ duration: reduceMotion ? 0 : .2 }}>
                    {message.from === "other" && <Avatar name={selected.name} size="sm" />}
                    <div className="message-stack">
                      <div className="message-bubble">{message.content}</div>
                      <span className={`message-time message-time--${message.status}`}>
                        {message.time}
                        {message.from === "me" && message.status === "sent" && <CheckCheck size={12} />}
                        {message.status === "pending" && <LoaderCircle className="ui-spinner" size={11} />}
                        {message.status === "failed" && <button aria-label="重新发送" onClick={() => retryMessage(selected.id, message.id, previewMode)} title={message.error}><CircleAlert size={11} />发送失败<RotateCcw size={10} /></button>}
                      </span>
                    </div>
                  </motion.div>
                ))}
              </AnimatePresence>
              <div ref={bottomRef} />
            </div>
            <footer className="composer-wrap"><div className="composer"><textarea aria-label="输入消息" onChange={(event) => setDraft(event.target.value)} onKeyDown={handleComposerKeyDown} placeholder="输入消息…" rows={1} value={draft} /><div className="composer__footer"><span>Enter 发送 · Shift + Enter 换行</span><Button aria-label="发送消息" disabled={!draft.trim()} onClick={sendMessage} size="sm"><SendHorizontal size={15} />发送</Button></div></div></footer>
          </>
        ) : conversations.length === 0 && mode === "live" ? (
          <EmptyState description="连接成功后，会话和离线消息将在这里同步。" icon={<LoaderCircle className="ui-spinner" size={25} />} title="正在同步会话" />
        ) : (
          <EmptyState description="从左侧选择一个会话，开始查看消息。" icon={<MessageCircle size={25} />} title="选择一段对话" />
        )}
      </section>

      {selected?.group ? <GroupManagementDrawer conversation={selected} onClose={() => setDrawerOpen(false)} open={drawerOpen} /> : <Drawer description="联系人资料与会话设置" onClose={() => setDrawerOpen(false)} open={drawerOpen} title={selected?.name ?? "聊天资料"}>
        <div className="profile-hero"><Avatar name={selected?.name ?? "?"} online={selected?.online} size="xl" /><h3>{selected?.name}</h3><p>{selected?.group ? "群聊资料将在群组阶段接入" : "联系人基础资料"}</p>{selected?.group && <Badge>群聊</Badge>}</div>
        <div className="detail-list"><button><span><BellOff size={17} />消息免打扰</span><ChevronDown size={16} /></button>{selected?.group && <button><span><Users size={17} />查看全部成员</span><span>—</span></button>}</div>
      </Drawer>}
      <CreateGroupDrawer onClose={() => setCreateGroupOpen(false)} onCreated={(id) => navigate(`/app/chats/${id}`)} open={createGroupOpen} />
    </>
  );
}
