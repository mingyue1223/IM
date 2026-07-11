import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { Check, ChevronRight, LoaderCircle, MapPin, MessageCircle, MoreHorizontal, Search, Trash2, UserRoundPlus, UsersRound, X } from "lucide-react";
import { useEffect, useState } from "react";
import { NavLink, useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../../api/client";
import { buildPrivateConvId } from "../../goim-ws-types";
import { Avatar, Badge, Button, ConfirmDialog, IconButton, TextField } from "../components/ui";
import { useFriends, type FriendView } from "../features/friends/useFriends";
import { useAuthStore } from "../stores/authStore";
import { useChatStore } from "../stores/chatStore";

type DangerAction = "block" | "remove" | null;

export function ContactsPage() {
  const { contactId } = useParams();
  const navigate = useNavigate();
  const reduceMotion = useReducedMotion();
  const userId = useAuthStore((state) => state.user?.id ?? 0);
  const previewMode = useAuthStore((state) => state.previewMode);
  const addPrivateConversation = useChatStore((state) => state.addPrivateConversation);
  const { contacts, requests, isLoading, error, isMutating, accept, reject, sendRequest, remove, block } = useFriends();
  const [requestOpen, setRequestOpen] = useState(false);
  const [dangerAction, setDangerAction] = useState<DangerAction>(null);
  const [targetUserId, setTargetUserId] = useState("");
  const [requestMessage, setRequestMessage] = useState("");
  const [requestError, setRequestError] = useState<string | null>(null);
  const [requestSent, setRequestSent] = useState(false);
  const selected = contacts.find((contact) => contact.routeId === contactId) ?? contacts[0];

  useEffect(() => {
    if (contacts.length && !contacts.some((contact) => contact.routeId === contactId)) navigate(`/app/contacts/${contacts[0].routeId}`, { replace: true });
  }, [contactId, contacts, navigate]);

  const startChat = (friend: FriendView) => {
    const convId = previewMode ? friend.routeId : buildPrivateConvId(userId, friend.userId);
    addPrivateConversation(convId, friend.userId, friend.name);
    navigate(`/app/chats/${convId}`);
  };

  const submitFriendRequest = async () => {
    const id = Number(targetUserId);
    if (!Number.isSafeInteger(id) || id <= 0) { setRequestError("请输入有效的用户 ID"); return; }
    setRequestError(null);
    try {
      await sendRequest(id, requestMessage.trim());
      setRequestSent(true);
      setTargetUserId("");
      setRequestMessage("");
    } catch (requestFailure) {
      setRequestError(requestFailure instanceof ApiError ? requestFailure.message : "发送申请失败");
    }
  };

  const confirmDangerAction = async () => {
    if (!selected || !dangerAction) return;
    dangerAction === "block" ? await block(selected) : await remove(selected);
    setDangerAction(null);
  };

  return (
    <>
      <aside className="module-sidebar contacts-sidebar">
        <header className="module-sidebar__header"><div><span className="eyebrow">Contacts</span><h1>联系人</h1></div><IconButton label="添加好友" onClick={() => setRequestOpen(true)}><UserRoundPlus size={18} /></IconButton></header>
        <div className="module-sidebar__search"><TextField aria-label="搜索联系人" leadingIcon={<Search size={16} />} placeholder="搜索联系人" /></div>
        <button className="friend-request-entry" onClick={() => setRequestOpen(true)}><span className="friend-request-entry__icon"><UserRoundPlus size={18} /></span><span><strong>好友申请</strong><small>{requests.length ? `有 ${requests.length} 条待处理申请` : "暂无新的申请"}</small></span>{requests.length > 0 && <b>{requests.length}</b>}<ChevronRight size={16} /></button>
        <div className="sidebar-section-label"><span>联系人</span><small>{contacts.length}</small></div>
        {isLoading ? <div className="sidebar-loading"><LoaderCircle className="ui-spinner" size={17} />正在加载联系人</div> : error ? <div className="sidebar-error">联系人加载失败，稍后重试</div> : (
          <nav aria-label="联系人列表" className="contact-nav">{contacts.map((contact, index) => <div className="contact-group" key={contact.routeId}>{(index === 0 || contact.name[0] !== contacts[index - 1]?.name[0]) && <span className="contact-letter">{contact.name[0]}</span>}<NavLink className={({ isActive }) => isActive ? "contact-item is-active" : "contact-item"} to={`/app/contacts/${contact.routeId}`}><Avatar name={contact.name} online={contact.online} src={contact.avatarUrl} /><span><strong>{contact.name}</strong><small>{contact.note}</small></span></NavLink></div>)}</nav>
        )}
      </aside>

      <section className="module-main contact-main">
        {selected ? <AnimatePresence mode="wait"><motion.div animate={{ opacity: 1, y: 0 }} className="contact-profile" initial={{ opacity: 0, y: reduceMotion ? 0 : 8 }} key={selected.routeId} transition={{ duration: reduceMotion ? 0 : .24 }}>
          <div className="contact-profile__top"><Avatar name={selected.name} online={selected.online} size="xl" src={selected.avatarUrl} /><div className="contact-profile__identity"><div><h2>{selected.name}</h2>{selected.online && <Badge>在线</Badge>}</div><p>用户 #{selected.userId}</p></div><IconButton label="更多联系人操作" onClick={() => setDangerAction("block")}><MoreHorizontal size={19} /></IconButton></div>
          <p className="contact-profile__note">{selected.note}</p>
          <Button leadingIcon={<MessageCircle size={17} />} onClick={() => startChat(selected)} size="lg">发消息</Button>
          <div className="contact-info-grid"><div><span><MapPin size={16} />所在地</span><strong>{selected.location ?? "未提供"}</strong></div><div><span><UsersRound size={16} />共同群聊</span><strong>{selected.groups === undefined ? "暂不可用" : `${selected.groups} 个`}</strong></div></div>
          <div className="contact-profile__section"><header><h3>联系人管理</h3></header><div className="contact-danger-row"><button onClick={() => setDangerAction("remove")}><Trash2 size={15} />删除好友</button><button onClick={() => setDangerAction("block")}>加入黑名单</button></div></div>
        </motion.div></AnimatePresence> : <div className="contact-empty"><UsersRound size={24} /><h2>还没有联系人</h2><p>通过用户 ID 发送好友申请，建立第一段连接。</p><Button onClick={() => setRequestOpen(true)} size="sm">添加好友</Button></div>}
      </section>

      <div className={`request-panel ${requestOpen ? "is-open" : ""}`} aria-hidden={!requestOpen}><button aria-label="关闭好友申请" className="request-panel__backdrop" onClick={() => setRequestOpen(false)} /><motion.aside animate={{ x: requestOpen ? 0 : 30, opacity: requestOpen ? 1 : 0 }} className="request-panel__content">
        <header><div><h2>好友</h2><p>添加联系人或处理收到的申请</p></div><IconButton label="关闭" onClick={() => setRequestOpen(false)}><X size={18} /></IconButton></header>
        <section className="add-friend-form"><h3>添加好友</h3><TextField label="用户 ID" onChange={(event) => setTargetUserId(event.target.value)} placeholder="输入数字用户 ID" type="number" value={targetUserId} /><TextField label="验证信息（选填）" onChange={(event) => setRequestMessage(event.target.value)} placeholder="介绍一下自己" value={requestMessage} />{requestError && <p className="inline-error">{requestError}</p>}{requestSent && <p className="inline-success">好友申请已发送</p>}<Button disabled={isMutating || !targetUserId} onClick={submitFriendRequest} size="sm">发送申请</Button></section>
        <section className="request-list"><h3>收到的申请 <span>{requests.length}</span></h3>{requests.length === 0 && <p className="request-list__empty">暂无待处理申请</p>}{requests.map((request) => <div className="friend-request-card" key={request.id}><Avatar name={`用户 ${request.from_user_id}`} /><div><strong>用户 #{request.from_user_id}</strong><p>{request.message || "希望添加你为好友"}</p><span><Button leadingIcon={<Check size={14} />} onClick={() => void accept(request.id)} size="sm">接受</Button><Button onClick={() => void reject(request.id)} size="sm" variant="ghost">拒绝</Button></span></div></div>)}</section>
      </motion.aside></div>

      <ConfirmDialog confirmLabel={dangerAction === "remove" ? "删除好友" : "确认拉黑"} description={dangerAction === "remove" ? `删除后，你与 ${selected?.name ?? "该用户"} 将不再是好友。` : `拉黑后，${selected?.name ?? "该用户"} 将无法向你发送消息。`} destructive onClose={() => setDangerAction(null)} onConfirm={() => void confirmDangerAction()} open={Boolean(dangerAction)} title={dangerAction === "remove" ? "删除这个联系人？" : "拉黑这个联系人？"} />
    </>
  );
}
