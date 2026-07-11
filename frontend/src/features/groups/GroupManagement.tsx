import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Crown, LogOut, ShieldCheck, UserPlus, Users, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { Group, GroupMember } from "../../../goim-api-types";
import { ApiError } from "../../../api/client";
import { Avatar, Button, ConfirmDialog, Drawer, IconButton, TextField } from "../../components/ui";
import { groupsApi } from "../../lib/api";
import { useAuthStore } from "../../stores/authStore";
import { useChatStore, type ChatConversation } from "../../stores/chatStore";

const previewMembers: GroupMember[] = [
  { id: 1, group_id: 3001, user_id: 10086, role: 2, joined_at: new Date().toISOString() },
  { id: 2, group_id: 3001, user_id: 2001, role: 1, joined_at: new Date().toISOString() },
  { id: 3, group_id: 3001, user_id: 2002, role: 0, joined_at: new Date().toISOString() },
  { id: 4, group_id: 3001, user_id: 2003, role: 0, joined_at: new Date().toISOString() },
];

const roleNames = { 0: "成员", 1: "管理员", 2: "群主" } as const;

interface CreateGroupDrawerProps {
  open: boolean;
  onClose: () => void;
  onCreated: (conversationId: string) => void;
}

export function CreateGroupDrawer({ open, onClose, onCreated }: CreateGroupDrawerProps) {
  const previewMode = useAuthStore((state) => state.previewMode);
  const addGroupConversation = useChatStore((state) => state.addGroupConversation);
  const [name, setName] = useState("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState<string | null>(null);
  const createMutation = useMutation({ mutationFn: () => groupsApi.create({ name: name.trim(), notice: notice.trim() }) });

  const createGroup = async () => {
    if (!name.trim()) return;
    setError(null);
    try {
      const groupId = previewMode ? Date.now() % 1_000_000 : (await createMutation.mutateAsync()).group_id;
      addGroupConversation(groupId, name.trim());
      const id = previewMode ? `preview-group-${groupId}` : `g_${groupId}`;
      setName(""); setNotice(""); onClose(); onCreated(id);
    } catch (failure) {
      setError(failure instanceof ApiError ? failure.message : "创建群聊失败");
    }
  };

  return <Drawer description="创建后你将自动成为群主。" onClose={onClose} open={open} title="创建群聊"><div className="group-form"><TextField label="群名称" maxLength={50} onChange={(event) => setName(event.target.value)} placeholder="例如：项目讨论组" value={name} /><label className="ui-field"><span className="ui-field__label">群公告（选填）</span><span className="ui-field__control group-textarea"><textarea maxLength={300} onChange={(event) => setNotice(event.target.value)} placeholder="介绍这个群聊的用途" rows={4} value={notice} /></span></label>{error && <p className="inline-error">{error}</p>}<Button disabled={!name.trim() || createMutation.isPending} loading={createMutation.isPending} onClick={() => void createGroup()} size="lg">创建群聊</Button></div></Drawer>;
}

interface GroupManagementDrawerProps {
  conversation: ChatConversation;
  open: boolean;
  onClose: () => void;
}

export function GroupManagementDrawer({ conversation, open, onClose }: GroupManagementDrawerProps) {
  const previewMode = useAuthStore((state) => state.previewMode);
  const currentUserId = useAuthStore((state) => state.user?.id ?? 0);
  const queryClient = useQueryClient();
  const groupId = conversation.targetId;
  const [localGroup, setLocalGroup] = useState<Group>({ id: groupId, name: conversation.name, notice: "保持信息透明，重要结论及时同步。", owner_id: currentUserId, max_members: 500, created_at: new Date().toISOString(), updated_at: new Date().toISOString() });
  const [localMembers, setLocalMembers] = useState(previewMembers.map((member) => ({ ...member, group_id: groupId })));
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(conversation.name);
  const [notice, setNotice] = useState(localGroup.notice);
  const [newMemberId, setNewMemberId] = useState("");
  const [danger, setDanger] = useState<{ type: "remove" | "leave"; memberId?: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const groupQuery = useQuery({ queryKey: ["group", groupId], queryFn: () => groupsApi.get(groupId), enabled: open && !previewMode });
  const membersQuery = useQuery({ queryKey: ["group-members", groupId], queryFn: () => groupsApi.members(groupId, 100, 0), enabled: open && !previewMode });
  const actionMutation = useMutation({ mutationFn: async (action: () => Promise<unknown>) => action(), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ["group", groupId] }); void queryClient.invalidateQueries({ queryKey: ["group-members", groupId] }); } });
  const group = previewMode ? localGroup : groupQuery.data;
  const members = previewMode ? localMembers : (membersQuery.data?.items ?? []);
  const currentMember = members.find((member) => member.user_id === currentUserId);
  const canManage = currentMember?.role === 1 || currentMember?.role === 2;
  const isOwner = group?.owner_id === currentUserId || currentMember?.role === 2;

  useEffect(() => {
    if (!groupQuery.data) return;
    setName(groupQuery.data.name); setNotice(groupQuery.data.notice);
  }, [groupQuery.data]);

  const run = async (previewAction: () => void, liveAction: () => Promise<unknown>) => {
    setError(null);
    try { previewMode ? previewAction() : await actionMutation.mutateAsync(liveAction); }
    catch (failure) { setError(failure instanceof ApiError ? failure.message : "操作失败，请稍后重试"); }
  };

  const saveInfo = () => void run(() => setLocalGroup((current) => ({ ...current, name: name.trim(), notice: notice.trim() })), () => groupsApi.update(groupId, { name: name.trim(), notice: notice.trim() })).then(() => setEditing(false));
  const addMember = () => { const memberId = Number(newMemberId); if (!memberId) return; void run(() => setLocalMembers((current) => [...current, { id: Date.now(), group_id: groupId, user_id: memberId, role: 0, joined_at: new Date().toISOString() }]), () => groupsApi.addMember(groupId, { member_id: memberId })).then(() => setNewMemberId("")); };
  const updateRole = (member: GroupMember) => void run(() => setLocalMembers((current) => current.map((item) => item.user_id === member.user_id ? { ...item, role: member.role === 1 ? 0 : 1 } : item)), () => groupsApi.updateRole(groupId, member.user_id, { role: member.role === 1 ? 0 : 1 }));
  const confirmDanger = async () => {
    if (!danger) return;
    if (danger.type === "remove" && danger.memberId) await run(() => setLocalMembers((current) => current.filter((member) => member.user_id !== danger.memberId)), () => groupsApi.removeMember(groupId, danger.memberId!));
    if (danger.type === "leave") await run(() => undefined, () => groupsApi.leave(groupId));
    setDanger(null);
    if (danger.type === "leave") onClose();
  };

  const sortedMembers = useMemo(() => [...members].sort((a, b) => b.role - a.role), [members]);

  return <><Drawer description={`${members.length} 位成员 · 最多 ${group?.max_members ?? 500} 人`} onClose={onClose} open={open} title="群聊资料"><div className="group-profile-head"><Avatar name={group?.name ?? conversation.name} size="xl" /><div><h3>{group?.name ?? conversation.name}</h3><p>{group?.notice || "暂无群公告"}</p></div>{canManage && <Button onClick={() => setEditing((value) => !value)} size="sm" variant="secondary">{editing ? "取消编辑" : "编辑资料"}</Button>}</div>
    {error && <p className="inline-error">{error}</p>}
    {editing && <div className="group-edit-panel"><TextField label="群名称" onChange={(event) => setName(event.target.value)} value={name} /><TextField label="群公告" onChange={(event) => setNotice(event.target.value)} value={notice} /><Button disabled={!name.trim()} onClick={saveInfo} size="sm">保存更改</Button></div>}
    {canManage && <div className="group-add-member"><TextField label="邀请成员" onChange={(event) => setNewMemberId(event.target.value)} placeholder="输入用户 ID" type="number" value={newMemberId} /><Button disabled={!newMemberId} leadingIcon={<UserPlus size={14} />} onClick={addMember} size="sm">添加</Button></div>}
    <section className="group-members"><header><h3><Users size={16} />群成员</h3><span>{members.length}</span></header>{sortedMembers.map((member) => <div className="group-member-row" key={member.user_id}><Avatar name={`用户 ${member.user_id}`} size="sm" /><div><strong>{member.user_id === currentUserId ? "我" : `用户 #${member.user_id}`}</strong><small>{roleNames[member.role]}</small></div><span className={`role-badge role-badge--${member.role}`}>{member.role === 2 ? <Crown size={11} /> : member.role === 1 ? <ShieldCheck size={11} /> : null}{roleNames[member.role]}</span>{isOwner && member.role !== 2 && <IconButton label={member.role === 1 ? "取消管理员" : "设为管理员"} onClick={() => updateRole(member)}><ShieldCheck size={15} /></IconButton>}{canManage && member.role !== 2 && member.user_id !== currentUserId && <IconButton label="移除成员" onClick={() => setDanger({ type: "remove", memberId: member.user_id })}><X size={15} /></IconButton>}</div>)}</section>
    <Button disabled={isOwner} leadingIcon={<LogOut size={15} />} onClick={() => setDanger({ type: "leave" })} variant="danger">{isOwner ? "群主需先转让身份才能退出" : "退出群聊"}</Button>
  </Drawer><ConfirmDialog confirmLabel={danger?.type === "leave" ? "退出群聊" : "移除成员"} description={danger?.type === "leave" ? "退出后将不再接收这个群的消息。" : `确认将用户 #${danger?.memberId ?? ""} 移出群聊？`} destructive onClose={() => setDanger(null)} onConfirm={() => void confirmDanger()} open={Boolean(danger)} title={danger?.type === "leave" ? "确定退出群聊？" : "移除这位成员？"} /></>;
}
