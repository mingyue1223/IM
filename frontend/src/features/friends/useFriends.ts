import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import type { FriendGroup, FriendRequest } from "../../../goim-api-types";
import { contacts as previewContacts } from "../../mocks/data";
import { friendsApi } from "../../lib/api";
import { useAuthStore } from "../../stores/authStore";
import { useChatStore } from "../../stores/chatStore";
import { buildPrivateConvId } from "../../../goim-ws-types";

export interface FriendView {
  routeId: string;
  userId: number;
  name: string;
  avatarUrl?: string;
  note: string;
  online?: boolean;
  location?: string;
  groups?: number;
  isBlocked: boolean;
  groupId?: number | null;
}

const previewIds: Record<string, number> = { "chen-xi": 2003, "lin-cheng": 2001, "lu-yao": 2004, "zhou-yu": 2002 };
const initialPreviewRequests: FriendRequest[] = [
  { id: 9001, from_user_id: 2010, to_user_id: 10086, message: "你好，想和你聊聊新项目", status: 0, created_at: new Date().toISOString(), updated_at: new Date().toISOString() },
  { id: 9002, from_user_id: 2011, to_user_id: 10086, message: "我们在产品讨论组", status: 0, created_at: new Date().toISOString(), updated_at: new Date().toISOString() },
];

export function useFriends() {
  const previewMode = useAuthStore((state) => state.previewMode);
  const currentUserId = useAuthStore((state) => state.user?.id ?? 0);
  const queryClient = useQueryClient();
  const [localContacts, setLocalContacts] = useState(previewContacts);
  const [localRequests, setLocalRequests] = useState(initialPreviewRequests);
  const [localGroups, setLocalGroups] = useState<FriendGroup[]>([]);

  const friendsQuery = useQuery({ queryKey: ["friends"], queryFn: () => friendsApi.list(100, 0), enabled: !previewMode, refetchInterval: 15_000 });
  const requestsQuery = useQuery({ queryKey: ["friend-requests"], queryFn: () => friendsApi.requests(100, 0), enabled: !previewMode });
  const groupsQuery = useQuery({ queryKey: ["friend-groups"], queryFn: () => friendsApi.groups(), enabled: !previewMode });
  const acceptMutation = useMutation({ mutationFn: (requestId: number) => friendsApi.accept({ request_id: requestId }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friends"] }) });
  const rejectMutation = useMutation({ mutationFn: (requestId: number) => friendsApi.reject({ request_id: requestId }) });
  const sendMutation = useMutation({ mutationFn: (input: { userId: number; message: string }) => friendsApi.request({ to_user_id: input.userId, message: input.message }) });
  const removeMutation = useMutation({ mutationFn: (friendId: number) => friendsApi.remove(friendId), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friends"] }) });
  const blockMutation = useMutation({ mutationFn: (friendId: number) => friendsApi.block({ blocked_id: friendId }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friends"] }) });
  const unblockMutation = useMutation({ mutationFn: (friendId: number) => friendsApi.unblock({ blocked_id: friendId }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friends"] }) });
  const remarkMutation = useMutation({ mutationFn: (input: { friendId: number; remark: string }) => friendsApi.updateRemark(input.friendId, { remark: input.remark }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friends"] }) });
  const createGroupMutation = useMutation({ mutationFn: (name: string) => friendsApi.createGroup({ name }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friend-groups"] }) });
  const renameGroupMutation = useMutation({ mutationFn: (input: { groupId: number; name: string }) => friendsApi.renameGroup(input.groupId, { name: input.name }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friend-groups"] }) });
  const deleteGroupMutation = useMutation({ mutationFn: (groupId: number) => friendsApi.deleteGroup(groupId), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ["friend-groups"] }); void queryClient.invalidateQueries({ queryKey: ["friends"] }); } });
  const moveGroupMutation = useMutation({ mutationFn: (input: { friendId: number; groupId: number | null }) => friendsApi.moveToGroup(input.friendId, { group_id: input.groupId }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["friends"] }) });

  const contacts: FriendView[] = useMemo(() => {
    if (previewMode) return localContacts.map((contact) => ({ routeId: contact.id, userId: previewIds[contact.id], name: contact.name, note: contact.note, online: contact.online, location: contact.location, groups: contact.groups, isBlocked: false, groupId: null }));
    return (friendsQuery.data?.items ?? []).map((friend) => ({ routeId: String(friend.friend_id), userId: friend.friend_id, name: friend.remark || friend.nickname || `用户 #${friend.friend_id}`, avatarUrl: friend.avatar_url, note: friend.remark && friend.nickname ? friend.nickname : "好友", online: friend.online, isBlocked: friend.is_blocked, groupId: friend.group_id }));
  }, [friendsQuery.data, localContacts, previewMode]);

  const requests = previewMode ? localRequests : (requestsQuery.data?.items ?? []);

  const accept = async (requestId: number) => {
    if (previewMode) { setLocalRequests((current) => current.filter((request) => request.id !== requestId)); return; }
    const response = await acceptMutation.mutateAsync(requestId);
    const acceptedFriendID = response.user_id === currentUserId ? response.friend_id : response.user_id;
    const applicant = requestsQuery.data?.items.find((request) => request.id === requestId);
    useChatStore.getState().addPrivateConversation(
      buildPrivateConvId(currentUserId, acceptedFriendID),
      acceptedFriendID,
      applicant?.username || `用户 #${acceptedFriendID}`,
      applicant?.avatar_url,
    );
    await queryClient.invalidateQueries({ queryKey: ["friend-requests"] });
    await queryClient.invalidateQueries({ queryKey: ["friends"] });
  };
  const reject = async (requestId: number) => {
    if (previewMode) { setLocalRequests((current) => current.filter((request) => request.id !== requestId)); return; }
    await rejectMutation.mutateAsync(requestId);
    await queryClient.invalidateQueries({ queryKey: ["friend-requests"] });
  };
  const sendRequest = async (userId: number, message: string) => {
    if (previewMode) return;
    await sendMutation.mutateAsync({ userId, message });
  };
  const remove = async (friend: FriendView) => {
    if (previewMode) setLocalContacts((current) => current.filter((contact) => contact.id !== friend.routeId));
    else await removeMutation.mutateAsync(friend.userId);
    useChatStore.getState().removeConversation(previewMode ? friend.routeId : buildPrivateConvId(currentUserId, friend.userId));
  };
  const block = async (friend: FriendView) => {
    if (previewMode) { setLocalContacts((current) => current.filter((contact) => contact.id !== friend.routeId)); return; }
    await blockMutation.mutateAsync(friend.userId);
  };
  const unblock = async (friendId: number) => {
    if (previewMode) return;
    await unblockMutation.mutateAsync(friendId);
  };
  const updateRemark = async (friendId: number, remark: string) => {
    if (previewMode) return;
    await remarkMutation.mutateAsync({ friendId, remark });
  };
  const createGroup = async (name: string) => {
    if (previewMode) {
      setLocalGroups((current) => [...current, { id: Date.now(), user_id: currentUserId, name, sort_order: current.length, created_at: new Date().toISOString(), updated_at: new Date().toISOString() }]);
      return;
    }
    await createGroupMutation.mutateAsync(name);
  };
  const renameGroup = async (groupId: number, name: string) => {
    if (previewMode) { setLocalGroups((current) => current.map((group) => group.id === groupId ? { ...group, name } : group)); return; }
    await renameGroupMutation.mutateAsync({ groupId, name });
  };
  const deleteGroup = async (groupId: number) => {
    if (previewMode) { setLocalGroups((current) => current.filter((group) => group.id !== groupId)); return; }
    await deleteGroupMutation.mutateAsync(groupId);
  };
  const moveToGroup = async (friendId: number, groupId: number | null) => {
    if (previewMode) return;
    await moveGroupMutation.mutateAsync({ friendId, groupId });
  };

  return {
    contacts,
    friendGroups: previewMode ? localGroups : (groupsQuery.data ?? []),
    requests,
    isLoading: !previewMode && (friendsQuery.isLoading || requestsQuery.isLoading || groupsQuery.isLoading),
    error: friendsQuery.error ?? requestsQuery.error ?? groupsQuery.error,
    isMutating: acceptMutation.isPending || rejectMutation.isPending || sendMutation.isPending || removeMutation.isPending || blockMutation.isPending || unblockMutation.isPending || remarkMutation.isPending || createGroupMutation.isPending || renameGroupMutation.isPending || deleteGroupMutation.isPending || moveGroupMutation.isPending,
    accept,
    reject,
    sendRequest,
    remove,
    block,
    unblock,
    updateRemark,
    createGroup,
    renameGroup,
    deleteGroup,
    moveToGroup,
  };
}
