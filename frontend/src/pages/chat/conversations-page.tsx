import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { motion } from 'motion/react'
import { Avatar, Badge, Card } from '@heroui/react'
import { LissajousLoader } from '@/components/lissajous-loader'
import { Store } from 'lucide-react'
import { SpeechBubbleAlertIcon } from '@/components/icons/koboyo'
import { listConversations } from '@/lib/api/conversations'
import { formatRelativeTime } from '@/lib/format'
import { nicknameInitial } from '@/lib/format'
import { EmptyState } from '@/components/empty-state'
import { useAuthStore } from '@/stores/auth-store'

const PAGE_SIZE = 20

export function ConversationsPage() {
  const me = useAuthStore((state) => state.user)

  const { data, isPending } = useQuery({
    queryKey: ['conversations', { page: 1 }],
    queryFn: () => listConversations({ page: 1, page_size: PAGE_SIZE }),
    refetchInterval: 5_000,
  })

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <h1 className="text-lg font-bold">消息</h1>

      {isPending ? (
        <div className="flex h-40 items-center justify-center">
          <LissajousLoader className="size-28 text-foreground" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          actionLabel="去逛逛市场"
          actionTo="/"
          description="在商品详情页点击“和卖家聊聊”即可开始对话"
          icon={<SpeechBubbleAlertIcon className="h-14 w-auto" />}
          title="还没有会话"
        />
      ) : (
        <div className="flex flex-col gap-2">
          {data.items.map((conversation, index) => (
            <motion.div
              animate={{ opacity: 1, y: 0 }}
              initial={{ opacity: 0, y: 10 }}
              key={conversation.id}
              transition={{ delay: Math.min(index * 0.04, 0.24), duration: 0.25 }}
            >
            <Link
              className="block outline-none focus-visible:rounded-2xl focus-visible:ring-2 focus-visible:ring-accent"
              key={conversation.id}
              to={`/chats/${conversation.id}`}
            >
              <Card className="card-interactive flex flex-row items-center gap-3 p-3 hover:bg-surface-secondary">
                <Badge.Anchor>
                  <Avatar className="size-11">
                    <Avatar.Fallback>
                      {nicknameInitial(conversation.other_user.nickname)}
                    </Avatar.Fallback>
                  </Avatar>
                  {conversation.unread_count > 0 ? (
                    <Badge color="danger" size="sm">
                      {conversation.unread_count > 99 ? '99+' : conversation.unread_count}
                    </Badge>
                  ) : null}
                </Badge.Anchor>

                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-sm font-semibold text-foreground">
                      {conversation.other_user.nickname}
                    </span>
                    <span className="shrink-0 text-xs text-muted">
                      {conversation.last_message
                        ? formatRelativeTime(conversation.last_message.created_at)
                        : formatRelativeTime(conversation.created_at)}
                    </span>
                  </div>
                  <p className="line-clamp-1 text-sm text-muted">
                    {conversation.last_message
                      ? (conversation.last_message.sender_id === me?.id ? '我：' : '') +
                        conversation.last_message.content
                      : '你们还没有聊过天，打个招呼吧'}
                  </p>
                  <p className="flex items-center gap-1 truncate text-xs text-muted">
                    <Store className="size-3 shrink-0" />
                    {conversation.product.title}
                  </p>
                </div>
              </Card>
            </Link>
            </motion.div>
          ))}
        </div>
      )}
    </div>
  )
}
