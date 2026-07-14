import { useQuery } from "@tanstack/react-query";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { ArrowLeft, ArrowUp, BellOff, CheckCheck, CircleAlert, Download, FileText, ImagePlus, LoaderCircle, MessageCircle, MoreHorizontal, Paperclip, Plus, Reply, RotateCcw, Search, WifiOff, X } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from "react";
import { NavLink, useNavigate, useParams } from "react-router-dom";
import type { InboxMessage } from "../../goim-ws-types";
import { MsgType } from "../../goim-ws-types";
import { Avatar, Badge, Button, Drawer, EmptyState, IconButton, Switch, TextField } from "../components/ui";
import { env } from "../config/env";
import { CreateGroupDrawer, GroupManagementDrawer } from "../features/groups/GroupManagement";
import { goimSocket } from "../realtime/socket";
import { useAuthStore } from "../stores/authStore";
import { useChatStore, type ChatAttachment, type ChatMessage } from "../stores/chatStore";
import { groupsApi, messagesApi, settingsApi, uploadApi } from "../lib/api";

const emptyMessages: ChatMessage[] = [];

const connectionLabels = {
  idle: "未连接",
  connecting: "正在连接",
  connected: "已连接",
  reconnecting: "正在重新连接",
  offline: "连接中断",
} as const;

function absoluteUploadUrl(path: string) {
  try { return new URL(path, env.apiBaseUrl).toString(); } catch { return path; }
}

function formatFileSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function messageLabel(message: ChatMessage) {
  if (message.msgType === MsgType.Image) return "[图片]";
  if (message.msgType === MsgType.File) return message.attachment?.name || "[文件]";
  return message.content;
}

function MessageBody({ message }: { message: ChatMessage }) {
  if (message.msgType === MsgType.Image && message.attachment) {
    const url = absoluteUploadUrl(message.attachment.url);
    return <a className="message-image" href={url} rel="noreferrer" target="_blank"><img alt={message.attachment.name} src={url} /></a>;
  }
  if (message.msgType === MsgType.File && message.attachment) {
    const url = absoluteUploadUrl(message.attachment.url);
    return <a className="message-file" href={url} rel="noreferrer" target="_blank"><FileText size={22} /><span><strong>{message.attachment.name}</strong><small>{formatFileSize(message.attachment.size)}</small></span><Download size={16} /></a>;
  }
  return <div className={`message-bubble ${message.msgType === MsgType.System ? "message-bubble--system" : ""}`}>{message.content}</div>;
}

function SearchResultContent({ message }: { message: InboxMessage }) {
  if (message.msgType === MsgType.Image) return <>[图片]</>;
  if (message.msgType === MsgType.File) {
    try { return <>[文件] {(JSON.parse(message.content) as ChatAttachment).name}</>; } catch { return <>[文件]</>; }
  }
  return <>{message.content}</>;
}

export function ChatPage() {
  const { conversationId } = useParams();
  const navigate = useNavigate();
  const conversations = useChatStore((state) => state.conversations);
  const messages = useChatStore((state) => state.messagesByConversation[conversationId ?? ""] ?? emptyMessages);
  const connectionState = useChatStore((state) => state.connectionState);
  const syncCompleted = useChatStore((state) => state.syncCompleted);
  const mode = useChatStore((state) => state.mode);
  const sendText = useChatStore((state) => state.sendText);
  const sendAttachment = useChatStore((state) => state.sendAttachment);
  const retryMessage = useChatStore((state) => state.retryMessage);
  const prependHistory = useChatStore((state) => state.prependHistory);
  const markConversationRead = useChatStore((state) => state.markConversationRead);
  const setConversationMuted = useChatStore((state) => state.setConversationMuted);
  const typingStamp = useChatStore((state) => state.typingByConversation[conversationId ?? ""]);
  const previewMode = useAuthStore((state) => state.previewMode);
  const currentUserId = useAuthStore((state) => state.user?.id ?? 0);
  const selected = conversations.find((item) => item.id === conversationId);
  const selectedId = selected?.id;
  const groupMembersQuery = useQuery({ queryKey: ["group-members", selected?.targetId], queryFn: () => groupsApi.members(selected!.targetId, 500, 0), enabled: Boolean(selected?.group && !previewMode) });
  const groupMembers = groupMembersQuery.data?.items ?? [];
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [createGroupOpen, setCreateGroupOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [replyingTo, setReplyingTo] = useState<ChatMessage | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyHasMore, setHistoryHasMore] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchStart, setSearchStart] = useState("");
  const [searchEnd, setSearchEnd] = useState("");
  const [searchResults, setSearchResults] = useState<InboxMessage[]>([]);
  const [searching, setSearching] = useState(false);
  const [muteSaving, setMuteSaving] = useState(false);
  const [muteError, setMuteError] = useState<string | null>(null);
  const reduceMotion = useReducedMotion();
  const messageScrollRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const typingTimerRef = useRef<number | null>(null);
  const initiallyPositionedConversationRef = useRef<string | null>(null);
  const suppressAutoScrollRef = useRef(false);

  useEffect(() => {
    if (conversationId && mode === "live" && conversations.length > 0 && !selected) navigate(`/app/chats/${conversations[0].id}`, { replace: true });
  }, [conversationId, conversations, mode, navigate, selected]);

  useEffect(() => {
    setHistoryHasMore(true);
    setReplyingTo(null);
    setSearchResults([]);
  }, [selectedId]);

  useEffect(() => {
    if (!selectedId) return;
    markConversationRead(selectedId);
    if (!previewMode && connectionState === "connected") goimSocket.send({ type: "readAck", data: { convId: selectedId } });
  }, [connectionState, markConversationRead, messages.length, previewMode, selectedId]);

  useEffect(() => () => {
    if (typingTimerRef.current !== null) window.clearTimeout(typingTimerRef.current);
  }, []);

  useLayoutEffect(() => {
    if (!selectedId || (mode === "live" && !syncCompleted)) return;
    const scrollContainer = messageScrollRef.current;
    if (!scrollContainer) return;
    if (suppressAutoScrollRef.current) {
      suppressAutoScrollRef.current = false;
      return;
    }
    const isInitialPosition = initiallyPositionedConversationRef.current !== selectedId;
    if (isInitialPosition) {
      scrollContainer.scrollTop = scrollContainer.scrollHeight;
      initiallyPositionedConversationRef.current = selectedId;
      return;
    }
    scrollContainer.scrollTo({ top: scrollContainer.scrollHeight, behavior: reduceMotion ? "auto" : "smooth" });
  }, [messages.length, mode, reduceMotion, selectedId, syncCompleted]);

  const stopTyping = () => {
    if (!selected || previewMode || connectionState !== "connected") return;
    goimSocket.send({ type: "typing", data: { convId: selected.id, convType: selected.convType, toId: selected.targetId, typing: false } });
  };

  const handleDraftChange = (value: string) => {
    setDraft(value);
    if (!selected || previewMode || connectionState !== "connected") return;
    goimSocket.send({ type: "typing", data: { convId: selected.id, convType: selected.convType, toId: selected.targetId, typing: value.trim().length > 0 } });
    if (typingTimerRef.current !== null) window.clearTimeout(typingTimerRef.current);
    typingTimerRef.current = window.setTimeout(stopTyping, 1_200);
  };

  const sendMessage = () => {
    const content = draft.trim();
    if (!content || !selected) return;
    sendText(selected, content, previewMode, replyingTo?.serverMsgId);
    setDraft("");
    setReplyingTo(null);
    stopTyping();
  };

  const handleComposerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      sendMessage();
    }
  };

  const uploadAttachment = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file || !selected || uploading) return;
    setUploadError(null);
    setUploading(true);
    try {
      const attachment = previewMode
        ? { id: Date.now(), url: URL.createObjectURL(file), name: file.name, size: file.size, mimeType: file.type, kind: file.type.startsWith("image/") ? "image" as const : "file" as const }
        : await uploadApi.chat(file);
      sendAttachment(selected, attachment, previewMode, replyingTo?.serverMsgId);
      setReplyingTo(null);
    } catch {
      setUploadError("文件上传失败，请检查格式和大小");
    } finally {
      setUploading(false);
    }
  };

  const loadHistory = async () => {
    if (!selected || previewMode || historyLoading || !historyHasMore) return;
    setHistoryLoading(true);
    const scroll = messageScrollRef.current;
    const previousHeight = scroll?.scrollHeight ?? 0;
    try {
      const firstServerMessage = messages.find((message) => message.serverMsgId);
      const response = await messagesApi.history(selected.id, firstServerMessage?.serverMsgId ?? 0, 30);
      suppressAutoScrollRef.current = true;
      prependHistory(selected.id, response.items, currentUserId);
      setHistoryHasMore(response.has_more);
      window.requestAnimationFrame(() => {
        if (scroll) scroll.scrollTop = scroll.scrollHeight - previousHeight;
      });
    } finally {
      setHistoryLoading(false);
    }
  };

  const runSearch = async () => {
    if (!selected || (!searchQuery.trim() && !searchStart && !searchEnd)) return;
    setSearching(true);
    try {
      const response = await messagesApi.search({ q: searchQuery.trim() || undefined, convId: selected.id, startTime: searchStart ? new Date(searchStart).getTime() : undefined, endTime: searchEnd ? new Date(searchEnd).getTime() : undefined, limit: 100 });
      setSearchResults(response.items);
    } finally {
      setSearching(false);
    }
  };

  const revokeMessage = (message: ChatMessage) => {
    if (!message.serverMsgId || !selected) return;
    goimSocket.send({ type: "revokeMsg", data: { convId: selected.id, serverMsgId: message.serverMsgId } });
  };

  const toggleMute = async (muted: boolean) => {
    if (!selected || muteSaving) return;
    setMuteError(null);
    setMuteSaving(true);
    try {
      if (!previewMode) {
        if (muted) await settingsApi.mute({ convId: selected.id });
        else await settingsApi.unmute(selected.id);
      }
      setConversationMuted(selected.id, muted);
    } catch {
      setMuteError("设置免打扰失败，请稍后重试");
    } finally {
      setMuteSaving(false);
    }
  };

  return (
    <>
      <aside className={`module-sidebar conversation-sidebar ${conversationId ? "conversation-sidebar--mobile-hidden" : ""}`}>
        <header className="module-sidebar__header"><div><span className="eyebrow">Messages</span><h1>聊天</h1></div><IconButton label="创建群聊" onClick={() => setCreateGroupOpen(true)}><Plus size={18} /></IconButton></header>
        <div className={`connection-status connection-status--${connectionState}`}><span />{previewMode ? "预览连接" : connectionLabels[connectionState]}</div>
        <div className="module-sidebar__search"><TextField aria-label="搜索会话" leadingIcon={<Search size={16} />} placeholder="搜索会话" /></div>
        <div className="sidebar-section-label"><span>最近会话</span><small>{conversations.length}</small></div>
        <nav aria-label="会话列表" className="conversation-nav">
          {conversations.map((item) => <NavLink className={({ isActive }) => isActive ? "conversation-item is-active" : "conversation-item"} key={item.id} to={`/app/chats/${item.id}`}><Avatar name={item.name} online={item.online} src={item.avatarUrl} /><span className="conversation-item__copy"><strong>{item.name}</strong><small>{item.preview}</small></span><span className="conversation-item__meta"><time>{item.time}</time>{item.unread > 0 ? <b>{item.unread}</b> : item.muted ? <BellOff size={12} /> : null}</span></NavLink>)}
        </nav>
      </aside>

      <section className={`module-main chat-main ${conversationId ? "" : "chat-main--mobile-hidden"}`}>
        {selected ? <>
          <header className="chat-header">
            <IconButton label="返回会话列表" className="chat-header__back" onClick={() => navigate("/app/chats")}><ArrowLeft size={19} /></IconButton>
            <div className="chat-header__person"><Avatar name={selected.name} online={selected.online} src={selected.avatarUrl} /><div><h2>{selected.name}</h2><p>{typingStamp ? "正在输入…" : selected.group ? "群聊" : selected.online ? "在线" : "离线"}</p></div></div>
            <div className="chat-header__actions"><IconButton label="搜索消息" onClick={() => setSearchOpen(true)}><Search size={18} /></IconButton><IconButton label="查看聊天资料" onClick={() => setDrawerOpen(true)}><MoreHorizontal size={19} /></IconButton></div>
          </header>
          {connectionState !== "connected" && !previewMode && <div className="chat-connection-banner"><WifiOff size={14} /><span>{connectionLabels[connectionState]}，未确认的消息会保留并允许重试。</span></div>}
          <div className="message-scroll" ref={messageScrollRef}>
            {!previewMode && <div className="history-loader"><Button disabled={historyLoading || !historyHasMore} loading={historyLoading} onClick={() => void loadHistory()} size="sm" variant="ghost">{historyHasMore ? "加载更早消息" : "没有更早消息"}</Button></div>}
            {messages.length === 0 && <div className="conversation-empty"><MessageCircle size={21} /><p>还没有消息，发一句问候吧。</p></div>}
            <AnimatePresence initial={false}>
              {messages.map((message) => {
                const repliedMessage = message.replyToMsgId ? messages.find((item) => item.serverMsgId === message.replyToMsgId) : undefined;
                return <motion.div animate={{ opacity: 1, y: 0 }} className={`message-row ${message.from === "me" ? "is-me" : ""} ${message.status === "revoked" ? "is-revoked" : ""} ${message.msgType === MsgType.System ? "is-system" : ""}`} initial={{ opacity: 0, y: reduceMotion ? 0 : 8 }} key={message.id} transition={{ duration: reduceMotion ? 0 : .2 }}>
                  {message.from === "other" && message.msgType !== MsgType.System && (() => { const sender = selected.group ? groupMembers.find((member) => member.user_id === message.senderId) : undefined; return <Avatar name={sender?.username ?? (selected.group ? `用户 ${message.senderId}` : selected.name)} size="sm" src={selected.group ? sender?.avatar_url : selected.avatarUrl} />; })()}
                  <div className="message-stack">
                    {message.replyToMsgId && <div className="message-reply-preview"><Reply size={12} /><span>{repliedMessage ? messageLabel(repliedMessage) : `消息 #${message.replyToMsgId}`}</span></div>}
                    <MessageBody message={message} />
                    {message.msgType !== MsgType.System && <span className={`message-time message-time--${message.status}`}>{message.time}{message.from === "me" && message.status === "sent" && <CheckCheck size={12} />}{message.status === "pending" && <LoaderCircle className="ui-spinner" size={11} />}{message.status === "failed" && <button aria-label="重新发送" onClick={() => retryMessage(selected.id, message.id, previewMode)} title={message.error}><CircleAlert size={11} />发送失败<RotateCcw size={10} /></button>}{message.status !== "revoked" && message.serverMsgId && <button className="message-icon-action" onClick={() => setReplyingTo(message)} title="引用回复" type="button"><Reply size={12} /></button>}{message.from === "me" && message.status === "sent" && message.serverMsgId && Date.now() - message.timestamp <= 120_000 && <button className="message-revoke" onClick={() => revokeMessage(message)} type="button">撤回</button>}</span>}
                  </div>
                </motion.div>;
              })}
            </AnimatePresence>
          </div>
          <footer className="composer-wrap">
            {replyingTo && <div className="composer-reply"><Reply size={14} /><span><strong>回复</strong>{messageLabel(replyingTo)}</span><IconButton label="取消引用" onClick={() => setReplyingTo(null)}><X size={14} /></IconButton></div>}
            {uploadError && <p className="composer-error">{uploadError}</p>}
            <div className="composer-tools"><IconButton disabled={uploading} label="发送图片" onClick={() => fileInputRef.current?.click()}><ImagePlus size={18} /></IconButton><IconButton disabled={uploading} label="发送文件" onClick={() => fileInputRef.current?.click()}><Paperclip size={18} /></IconButton>{uploading && <LoaderCircle className="ui-spinner" size={15} />}</div>
            <div className="composer"><textarea aria-label="输入消息" maxLength={2000} onChange={(event) => handleDraftChange(event.target.value)} onKeyDown={handleComposerKeyDown} placeholder="输入消息…" rows={1} value={draft} /><span className="composer__count">{draft.length}/2000</span><button aria-label="发送消息" className="composer__send" disabled={!draft.trim()} onClick={sendMessage} type="button"><ArrowUp size={20} /></button></div>
            <input accept="image/*,.pdf,.txt,.md,.csv,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.zip,.rar,.7z" hidden onChange={(event) => void uploadAttachment(event)} ref={fileInputRef} type="file" />
          </footer>
        </> : mode === "live" && !syncCompleted ? <EmptyState description="连接成功后，会话和离线消息将在这里同步。" icon={<LoaderCircle className="ui-spinner" size={25} />} title="正在同步会话" /> : mode === "live" && conversations.length === 0 ? <EmptyState description="添加好友或创建群聊后，就可以开始第一段对话。" icon={<MessageCircle size={25} />} title="暂无会话" /> : <EmptyState description="从左侧选择一个会话，开始查看消息。" icon={<MessageCircle size={25} />} title="选择一段会话" />}
      </section>

      {selected?.group ? <GroupManagementDrawer conversation={selected} onClose={() => setDrawerOpen(false)} open={drawerOpen} /> : <Drawer description="联系人资料与会话设置" onClose={() => setDrawerOpen(false)} open={drawerOpen} title={selected?.name ?? "聊天资料"}><div className="profile-hero"><Avatar name={selected?.name ?? "?"} online={selected?.online} size="xl" src={selected?.avatarUrl} /><h3>{selected?.name}</h3><p>{selected?.online ? "在线" : "离线"}</p>{selected?.group && <Badge>群聊</Badge>}</div><div className="group-notification-setting"><Switch checked={Boolean(selected?.muted)} description="开启后，该会话不会触发声音与桌面通知。" disabled={muteSaving} label="消息免打扰" onCheckedChange={(checked) => void toggleMute(checked)} />{muteError && <p className="drawer-setting-error">{muteError}</p>}</div></Drawer>}
      <Drawer description="按关键词和时间范围查找当前会话" onClose={() => setSearchOpen(false)} open={searchOpen} title="搜索聊天记录"><div className="message-search-form"><TextField label="关键词" onChange={(event) => setSearchQuery(event.target.value)} placeholder="输入消息内容" value={searchQuery} /><label className="ui-field"><span className="ui-field__label">开始时间</span><span className="ui-field__control"><input onChange={(event) => setSearchStart(event.target.value)} type="datetime-local" value={searchStart} /></span></label><label className="ui-field"><span className="ui-field__label">结束时间</span><span className="ui-field__control"><input onChange={(event) => setSearchEnd(event.target.value)} type="datetime-local" value={searchEnd} /></span></label><Button disabled={searching || (!searchQuery.trim() && !searchStart && !searchEnd)} loading={searching} onClick={() => void runSearch()}>搜索</Button></div><div className="message-search-results">{searchResults.map((message) => <button key={message.msgId} onClick={() => setSearchOpen(false)}><span><strong>{message.fromId === currentUserId ? "我" : `用户 #${message.fromId}`}</strong><time>{new Date(message.timestamp).toLocaleString("zh-CN")}</time></span><p><SearchResultContent message={message} /></p></button>)}{!searching && searchResults.length === 0 && <p className="message-search-empty">暂无搜索结果</p>}</div></Drawer>
      <CreateGroupDrawer onClose={() => setCreateGroupOpen(false)} onCreated={(id) => navigate(`/app/chats/${id}`)} open={createGroupOpen} />
    </>
  );
}
