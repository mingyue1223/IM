import type { ApiId, Page, RevokeMessageRequest, SearchMessagesQuery } from "../../goim-api-types";
import type { InboxMessage } from "../../goim-ws-types";
import type { GoIMApiClient } from "./client";

function searchPath(input: SearchMessagesQuery) {
  const params = new URLSearchParams();
  if (input.q) params.set("q", input.q);
  if (input.convId) params.set("convId", input.convId);
  if (input.startTime) params.set("startTime", String(input.startTime));
  if (input.endTime) params.set("endTime", String(input.endTime));
  params.set("limit", String(input.limit ?? 20));
  params.set("offset", String(input.offset ?? 0));
  return `/msg/search?${params.toString()}`;
}

export interface MessageHistoryResponse { items: InboxMessage[]; has_more: boolean; }

export const createMessagesApi = (client: GoIMApiClient) => ({
  revoke: (input: RevokeMessageRequest) => client.post<void>("/msg/revoke", input),
  remove: (messageId: ApiId, convId: string) => client.delete<void>(`/msg/${messageId}?convId=${encodeURIComponent(convId)}`),
  search: (input: SearchMessagesQuery) => client.get<Page<InboxMessage>>(searchPath(input)),
  history: (convId: string, beforeId = 0, limit = 30) => client.get<MessageHistoryResponse>(`/msg/history?convId=${encodeURIComponent(convId)}&beforeId=${beforeId}&limit=${limit}`),
});
