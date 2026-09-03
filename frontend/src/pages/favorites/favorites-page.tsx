import { useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Button, Card, Pagination, Spinner, toast } from '@heroui/react'
import { HeartOff, Store, HeartCrack } from 'lucide-react'
import { listFavorites, removeFavorite } from '@/lib/api/favorites'
import { isApiError } from '@/lib/http'
import { formatPrice, formatRelativeTime } from '@/lib/format'
import { EmptyState } from '@/components/empty-state'
import { ProductStatusChip } from '@/components/status-chip'

const PAGE_SIZE = 10

export function FavoritesPage() {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)

  const { data, isPending } = useQuery({
    queryKey: ['favorites', { page }],
    queryFn: () => listFavorites({ page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  })

  const removeMutation = useMutation({
    mutationFn: (productId: string) => removeFavorite(productId),
    onSuccess: () => {
      toast.success('已取消收藏')
      void queryClient.invalidateQueries({ queryKey: ['favorites'] })
    },
    onError: (err) => {
      toast.danger(isApiError(err) ? err.message : '操作失败，请稍后重试')
    },
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-lg font-bold">我的收藏</h1>

      {isPending ? (
        <div className="flex h-40 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          actionLabel="去逛逛市场"
          actionTo="/"
          description="在商品详情页点击“收藏”，即可在这里找到"
          icon={<HeartCrack className="size-10" />}
          title="还没有收藏任何商品"
        />
      ) : (
        <>
          <div className="flex flex-col gap-3">
            {data.items.map(({ product, favorited_at }) => (
              <Card className="flex flex-row items-center gap-4 p-3" key={product.id}>
                <Link
                  className="h-20 w-20 shrink-0 overflow-hidden rounded-xl bg-surface"
                  to={`/products/${product.id}`}
                >
                  {product.cover_url ? (
                    <img
                      alt={product.title}
                      className="h-full w-full object-cover"
                      loading="lazy"
                      src={product.cover_url}
                    />
                  ) : (
                    <div className="flex h-full w-full items-center justify-center text-muted">
                      <Store className="size-6 opacity-50" />
                    </div>
                  )}
                </Link>

                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <Link
                    className="line-clamp-1 text-sm font-medium text-foreground hover:underline"
                    to={`/products/${product.id}`}
                  >
                    {product.title}
                  </Link>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="tabular-nums text-base font-bold text-accent">
                      {formatPrice(product.price)}
                    </span>
                    <ProductStatusChip status={product.status} />
                    <span className="text-xs text-muted">
                      收藏于 {formatRelativeTime(favorited_at)}
                    </span>
                  </div>
                </div>

                <Button
                  isIconOnly
                  aria-label="取消收藏"
                  isDisabled={removeMutation.isPending}
                  size="sm"
                  variant="tertiary"
                  onPress={() => removeMutation.mutate(product.id)}
                >
                  <HeartOff className="size-4" />
                </Button>
              </Card>
            ))}
          </div>

          {totalPages > 1 ? (
            <Pagination className="justify-center">
              <Pagination.Content>
                <Pagination.Item>
                  <Pagination.Previous isDisabled={page === 1} onPress={() => setPage((p) => p - 1)}>
                    <Pagination.PreviousIcon />
                  </Pagination.Previous>
                </Pagination.Item>
                {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
                  <Pagination.Item key={p}>
                    <Pagination.Link isActive={p === page} onPress={() => setPage(p)}>
                      {p}
                    </Pagination.Link>
                  </Pagination.Item>
                ))}
                <Pagination.Item>
                  <Pagination.Next isDisabled={page === totalPages} onPress={() => setPage((p) => p + 1)}>
                    <Pagination.NextIcon />
                  </Pagination.Next>
                </Pagination.Item>
              </Pagination.Content>
            </Pagination>
          ) : null}
        </>
      )}
    </div>
  )
}
