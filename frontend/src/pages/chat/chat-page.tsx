import { useEffect, useMemo, useRef } from 'react'
import { useAutoResizeTextarea } from '@/hooks/use-auto-resize-textarea'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { Link, useParams } from 'react-router-dom'
import {
  ArrowLeft,
  ImageOff,
  Send,
} from 'lucide-react'
import { Avatar, Button, Card, Spinner, TextArea, toast } from '@heroui/react'
import { LissajousLoader } from '@/components/lissajous-loader'
import {
  listConversationMessages,
  markConversationRead,
  listConversations,
  sendConversationMessage,
} from '@/lib/api/conversations'
import { isApiError } from '@/lib/http'
import { nicknameInitial } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { ProductStatusChip } from '@/components/status-chip'
import type { Message } from '@/lib/types'

const MESSAGE_PAGE_SIZE = 30
const POLL_INTERVAL_MS = 4_000

function messageTime(iso: string): string {
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(
    new Date(iso),
  )
}

export function ChatPage() {
  const { conversationId = '' } = useParams()
  const queryClient = useQueryClient()
  const me = useAuthStore((state) => state.user)
  const bottomRef = useRef<HTMLDivElement>(null)
  const { textareaRef: inputRef, adjustHeight } = useAutoResizeTextarea({ minHeight: 44, maxHeight: 160 })

  // 会话元信息来自会话列表（契约未提供按 id 查询单个会话的接口）
  const { data: conversation } = useQuery({
    queryKey: ['conversations', { page: 1 }],
    queryFn: () => listConversations({ page: 1, page_size: 100 }),
    staleTime: 3_000,
    select: (page) => page.items.find((item) => item.id === conversationId),
  })

  const messagesQuery = useInfiniteQuery({
    queryKey: ['conversations', conversationId, 'messages'],
    queryFn: ({ pageParam }) =>
      listConversationMessages(conversationId, {
        before: pageParam || undefined,
        limit: MESSAGE_PAGE_SIZE,
      }),
    initialPageParam: '' as string,
    getNextPageParam: (lastPage) => lastPage.next_before ?? undefined,
    refetchInterval: POLL_INTERVAL_MS,
  })

  const chronological: Message[] = useMemo(() => {
    const pages = messagesQuery.data?.pages ?? []
    const flat = pages.flatMap((page) => page.items)
    return [...flat].reverse() // 接口按创建时间倒序返回，展示时转为正序
  }, [messagesQuery.data])

  const newest = chronological.at(-1)
  const newestId = newest?.id ?? ''
  const newestFromOther = newest != null && newest.sender.id !== me?.id

  const hasUnreadFromOther = useMemo(() => {
    if (!newestFromOther) return false
    return (conversation?.unread_count ?? 0) > 0 || newest.read_at == null
  }, [newestFromOther, newest, conversation])

  // 打开会话或对方发来新消息时标记已读（接口幂等）
  const markReadMutation = useMutation({
    mutationFn: (lastMessageId: string) =>
      markConversationRead(conversationId, { last_message_id: lastMessageId }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['conversations'] })
    },
  })

  useEffect(() => {
    if (newestId && hasUnreadFromOther && !markReadMutation.isPending) {
      markReadMutation.mutate(newestId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [newestId, hasUnreadFromOther])

  const sendMutation = useMutation({
    mutationFn: (content: string) =>
      sendConversationMessage(conversationId, { content }),
    onSuccess: () => {
      if (inputRef.current) {
        inputRef.current.value = ''
        adjustHeight(true)
      }
      void queryClient.invalidateQueries({ queryKey: ['conversations', conversationId, 'messages'] })
      void queryClient.invalidateQueries({ queryKey: ['conversations'] })
    },
    onError: (err) => {
      toast.danger(isApiError(err) ? err.message : '发送失败，请稍后重试')
    },
  })

  const submitMessage = () => {
    const content = (inputRef.current?.value ?? '').trim()
    if (!content) return
    if (content.length > 1000) {
      toast.warning('消息最长 1000 字')
      return
    }
    sendMutation.mutate(content)
  }

  // 首次加载与收到新消息时滚动到底部
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [chronological.length])

  return (
    <div className="mx-auto flex h-[calc(100vh-8.5rem)] w-full max-w-3xl flex-col gap-3">
      {/* 顶部：返回 + 对方 + 商品 */}
      <div className="flex items-center gap-3">
        <Link to="/chats">
          <Button isIconOnly aria-label="返回消息列表" size="sm" variant="tertiary">
            <ArrowLeft className="size-4" />
          </Button>
        </Link>
        <Avatar className="size-9">
          <Avatar.Fallback>{nicknameInitial(conversation?.other_user.nickname ?? '…')}</Avatar.Fallback>
        </Avatar>
        <div className="flex min-w-0 flex-col">
          <span className="truncate text-sm font-semibold">
            {conversation?.other_user.nickname ?? '会话'}
          </span>
          {conversation ? (
            <Link
              className="flex items-center gap-1 truncate text-xs text-muted hover:underline"
              to={`/products/${conversation.product.id}`}
            >
              {conversation.product.cover_url ? (
                <img
                  alt={conversation.product.title}
                  className="h-4 w-4 shrink-0 rounded object-cover"
                  src={conversation.product.cover_url}
                />
              ) : (
                <ImageOff className="size-3 shrink-0" />
              )}
              <span className="truncate">{conversation.product.title}</span>
              <ProductStatusChip status={conversation.product.status} />
            </Link>
          ) : null}
        </div>
      </div>

      <Card className="flex min-h-0 flex-1 flex-col p-0">
        {/* 更早的消息 */}
        <div className="flex justify-center pt-3">
          {messagesQuery.hasNextPage ? (
            <Button
              isDisabled={messagesQuery.isFetchingNextPage}
              size="sm"
              variant="tertiary"
              onPress={() => void messagesQuery.fetchNextPage()}
            >
              {messagesQuery.isFetchingNextPage ? <Spinner size="sm" /> : '查看更早的消息'}
            </Button>
          ) : null}
        </div>

        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
          {messagesQuery.isPending ? (
            <div className="flex h-full items-center justify-center">
              <LissajousLoader className="size-28 text-foreground" />
            </div>
          ) : chronological.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center gap-2 text-muted">
              <p className="text-sm">还没有消息</p>
              <p className="text-xs">发送第一条消息开始沟通吧</p>
            </div>
          ) : (
            chronological.map((message) => {
              const mine = message.sender.id === me?.id
              return (
                <motion.div
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  className={`flex w-full flex-col gap-1 ${mine ? 'items-end' : 'items-start'}`}
                  initial={{ opacity: 0, scale: 0.96, y: 8 }}
                  key={message.id}
                  transition={{ duration: 0.22, ease: [0.21, 0.68, 0.35, 1] }}
                >
                  <div className={`flex w-full items-end gap-2 ${mine ? 'justify-end' : 'justify-start'}`}>
                    {mine ? null : (
                      <Avatar className="size-7 shrink-0">
                        <Avatar.Fallback>{nicknameInitial(message.sender.nickname)}</Avatar.Fallback>
                      </Avatar>
                    )}
                    <div
                      className={`max-w-[min(75%,32rem)] whitespace-pre-wrap break-words rounded-2xl px-3.5 py-2 text-sm leading-6 ${
                        mine
                          ? 'rounded-br-md bg-accent text-accent-foreground'
                          : 'rounded-bl-md border border-border-secondary bg-surface-secondary text-foreground'
                      }`}
                    >
                      {message.content}
                    </div>
                  </div>
                  <span className="px-1 text-[10px] text-muted">
                    {messageTime(message.created_at)}
                    {mine ? ` · ${message.read_at ? '已读' : '未读'}` : ''}
                  </span>
                </motion.div>
              )
            })
          )}
          <div ref={bottomRef} />
        </div>

        {/* 输入区 */}
        <form
          className="flex items-end gap-2 border-t border-border p-3"
          onSubmit={(event) => {
            event.preventDefault()
            submitMessage()
          }}
        >
          <TextArea
            aria-label="消息内容"
            className="flex-1"
            placeholder="输入消息，回车发送（Shift+回车换行）"
            ref={inputRef}
            rows={1}
            onInput={() => adjustHeight()}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                event.preventDefault()
                submitMessage()
              }
            }}
          />
          <Button
            aria-label="发送"
            isDisabled={sendMutation.isPending}
            isIconOnly
            type="submit"
          >
            {sendMutation.isPending ? <Spinner size="sm" color="current" /> : <Send className="size-4" />}
          </Button>
        </form>
      </Card>
    </div>
  )
}
