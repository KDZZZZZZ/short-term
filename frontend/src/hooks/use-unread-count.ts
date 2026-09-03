import { useQuery } from '@tanstack/react-query'
import { getConversationUnreadCount } from '@/lib/api/conversations'
import { useAuthStore } from '@/stores/auth-store'

export const UNREAD_POLL_INTERVAL_MS = 5_000

/** 轮询当前用户未读消息总数；未登录时禁用。 */
export function useUnreadCount() {
  const token = useAuthStore((state) => state.token)
  return useQuery({
    queryKey: ['conversations', 'unread-count'],
    queryFn: getConversationUnreadCount,
    enabled: Boolean(token),
    refetchInterval: UNREAD_POLL_INTERVAL_MS,
    staleTime: 3_000,
  })
}
