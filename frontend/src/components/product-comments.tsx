import { useState } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Avatar, Button, Card, Label, TextArea, TextField, toast } from '@heroui/react'
import { createProductComment, listProductComments } from '@/lib/api/comments'
import { isApiError } from '@/lib/http'
import { formatRelativeTime, nicknameInitial } from '@/lib/format'
import type { Identifier } from '@/lib/types'

const PAGE_SIZE = 10

/**
 * 商品公开评论：任何已认证用户可评论任何商品、可多条、发布后不可改删；
 * 列表按 created_at DESC 分页，「加载更多」按页追加。
 */
export function ProductComments({ productId }: { productId: Identifier }) {
  const queryClient = useQueryClient()
  const [content, setContent] = useState('')

  const { data, isPending, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ['comments', productId],
    queryFn: ({ pageParam }) => listProductComments(productId, { page: pageParam, page_size: PAGE_SIZE }),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.page_size < last.total ? last.page + 1 : undefined),
  })

  const createMutation = useMutation({
    mutationFn: () => createProductComment(productId, { content: content.trim() }),
    onSuccess: () => {
      toast.success('评论已发布')
      setContent('')
      void queryClient.invalidateQueries({ queryKey: ['comments', productId] })
    },
    onError: (err) => {
      if (isApiError(err)) toast.danger(err.message)
    },
  })

  const trimmed = content.trim()
  const canSubmit = trimmed.length >= 1 && !createMutation.isPending
  const total = data?.pages[0]?.total ?? 0
  const comments = data?.pages.flatMap((pageData) => pageData.items) ?? []

  return (
    <section aria-label="用户评论" className="flex flex-col gap-3">
      <h2 className="text-base font-bold text-foreground">用户评论{total > 0 ? `（${total}）` : ''}</h2>

      <Card className="flex flex-col gap-2 p-4">
        <TextField className="w-full" value={content} onChange={setContent}>
          <Label>发表评论（公开可见，发布后不可删除）</Label>
          <TextArea maxLength={500} placeholder="1-500 字，任何人都可以评论" rows={3} />
        </TextField>
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted">{content.length}/500</span>
          <Button
            isDisabled={!canSubmit}
            isPending={createMutation.isPending}
            size="sm"
            onPress={() => createMutation.mutate()}
          >
            发布评论
          </Button>
        </div>
      </Card>

      {isPending ? (
        <p className="py-6 text-center text-sm text-muted">加载中…</p>
      ) : comments.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted">还没有评论，来抢第一条</p>
      ) : (
        <>
          <ul className="flex flex-col gap-3">
            {comments.map((comment) => (
              <li key={comment.id}>
                <Card className="flex items-start gap-3 p-4">
                  <Avatar className="size-8 shrink-0">
                    <Avatar.Fallback>{nicknameInitial(comment.user.nickname)}</Avatar.Fallback>
                  </Avatar>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2 text-xs">
                      <Link
                        className="font-medium text-foreground underline-offset-4 hover:text-accent hover:underline"
                        to={`/users/${comment.user.id}`}
                      >
                        {comment.user.nickname}
                      </Link>
                      <span className="text-muted">{formatRelativeTime(comment.created_at)}</span>
                    </div>
                    <p className="mt-1 whitespace-pre-wrap text-sm leading-6 text-foreground/90">
                      {comment.content}
                    </p>
                  </div>
                </Card>
              </li>
            ))}
          </ul>
          {hasNextPage ? (
            <Button
              className="self-center"
              isPending={isFetchingNextPage}
              size="sm"
              variant="outline"
              onPress={() => fetchNextPage()}
            >
              加载更多
            </Button>
          ) : null}
        </>
      )}
    </section>
  )
}
