import { apiFetch, newIdempotencyKey } from '@/lib/http'
import type { Conversation, ConversationPage, Identifier, MarkReadRequest, Message, MessagePage, SendMessageRequest, UnreadCountData } from '@/lib/types'

/** 以当前用户为买家创建或获取商品会话；重复调用返回原会话。 */
export function getOrCreateProductConversation(productId: Identifier): Promise<Conversation> {
  return apiFetch<Conversation>({
    method: 'POST',
    path: `/products/${encodeURIComponent(productId)}/conversations`,
    idempotencyKey: newIdempotencyKey(),
  })
}

export function listConversations(params: { page?: number; page_size?: number }): Promise<ConversationPage> {
  return apiFetch<ConversationPage>({ path: '/conversations', query: params })
}

export function getConversationUnreadCount(): Promise<UnreadCountData> {
  return apiFetch<UnreadCountData>({ path: '/conversations/unread-count' })
}

export interface ListMessagesParams {
  /** 获取此不透明游标之前的消息；首次请求省略。 */
  before?: Identifier
  limit?: number
}

export function listConversationMessages(
  conversationId: Identifier,
  params: ListMessagesParams = {},
): Promise<MessagePage> {
  return apiFetch<MessagePage>({
    path: `/conversations/${encodeURIComponent(conversationId)}/messages`,
    query: params,
  })
}

export function sendConversationMessage(
  conversationId: Identifier,
  body: SendMessageRequest,
): Promise<Message> {
  return apiFetch<Message>({
    method: 'POST',
    path: `/conversations/${encodeURIComponent(conversationId)}/messages`,
    json: body,
    idempotencyKey: newIdempotencyKey(),
  })
}

export function markConversationRead(
  conversationId: Identifier,
  body: MarkReadRequest,
): Promise<Record<string, never>> {
  return apiFetch({
    method: 'POST',
    path: `/conversations/${encodeURIComponent(conversationId)}/read`,
    json: body,
  })
}
