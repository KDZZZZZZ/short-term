import { useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import {
  AlertDialog,
  Button,
  Card,
  Pagination,
  Spinner,
  Tabs,
  toast,
} from '@heroui/react'
import { ArrowDownUp, Images, PackageOpen, Pencil, Store } from 'lucide-react'
import { listCurrentUserProducts } from '@/lib/api/users'
import { offShelfProduct, relistProduct } from '@/lib/api/products'
import type { ProductStatus } from '@/lib/types'
import { isApiError } from '@/lib/http'
import { formatDateTime, formatPrice } from '@/lib/format'
import { EmptyState } from '@/components/empty-state'
import { ProductStatusChip } from '@/components/status-chip'

const PAGE_SIZE = 10

const STATUS_TABS: Array<{ id: string; label: string }> = [
  { id: 'ALL', label: '全部' },
  { id: 'ON_SALE', label: '在售' },
  { id: 'RESERVED', label: '已预留' },
  { id: 'SOLD', label: '已售出' },
  { id: 'OFF_SHELF', label: '已下架' },
]

export function MyProductsPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<ProductStatus | 'ALL'>('ALL')
  const [page, setPage] = useState(1)

  const { data, isPending } = useQuery({
    queryKey: ['my-products', { status, page }],
    queryFn: () =>
      listCurrentUserProducts({
        status: status === 'ALL' ? undefined : status,
        page,
        page_size: PAGE_SIZE,
      }),
    placeholderData: keepPreviousData,
  })

  const shelfMutation = useMutation({
    mutationFn: ({ productId, off }: { productId: string; off: boolean }) =>
      off ? offShelfProduct(productId) : relistProduct(productId),
    onSuccess: (updated) => {
      toast.success(updated.status === 'OFF_SHELF' ? '已下架' : '已重新上架')
      void queryClient.invalidateQueries({ queryKey: ['my-products'] })
      void queryClient.invalidateQueries({ queryKey: ['product', updated.id] })
    },
    onError: (err) => {
      toast.danger(isApiError(err) ? err.message : '操作失败，请稍后重试')
    },
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">我的商品</h1>
        <Button size="sm" onPress={() => navigate('/products/new')}>
          <Store className="size-4" />
          发布新品
        </Button>
      </div>

      <Tabs
        selectedKey={status}
        onSelectionChange={(key) => {
          setStatus(key as ProductStatus | 'ALL')
          setPage(1)
        }}
      >
        <Tabs.ListContainer>
          <Tabs.List aria-label="商品状态筛选">
            {STATUS_TABS.map((tab) => (
              <Tabs.Tab id={tab.id} key={tab.id}>
                {tab.label}
                <Tabs.Indicator />
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </Tabs.ListContainer>
      </Tabs>

      {isPending ? (
        <div className="flex h-40 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          actionLabel="发布第一件商品"
          actionTo="/products/new"
          description="发布后的商品会出现在市场列表中"
          icon={<PackageOpen className="size-10" />}
          title="这里还没有商品"
        />
      ) : (
        <>
          <div className="flex flex-col gap-3">
            {data.items.map((product) => (
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
                    <span className="text-xs text-muted">{formatDateTime(product.created_at)}</span>
                  </div>
                  {product.buyer_review ? (
                    <p className="mt-0.5 line-clamp-1 text-xs text-muted">
                      买家评价：{product.buyer_review.content}
                    </p>
                  ) : null}
                </div>

                <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                  {product.status === 'ON_SALE' || product.status === 'OFF_SHELF' ? (
                    <>
                      {product.status === 'ON_SALE' ? (
                        <AlertDialog>
                          <Button isIconOnly aria-label="下架" size="sm" variant="tertiary">
                            <ArrowDownUp className="size-4" />
                          </Button>
                          <AlertDialog.Backdrop>
                            <AlertDialog.Container>
                              <AlertDialog.Dialog className="sm:max-w-[380px]">
                                <AlertDialog.CloseTrigger />
                                <AlertDialog.Header>
                                  <AlertDialog.Icon status="danger" />
                                  <AlertDialog.Heading>下架该商品？</AlertDialog.Heading>
                                </AlertDialog.Header>
                                <AlertDialog.Body>
                                  <p>下架后将从市场列表消失，可随时重新上架。</p>
                                </AlertDialog.Body>
                                <AlertDialog.Footer>
                                  <Button slot="close" variant="tertiary">
                                    取消
                                  </Button>
                                  <Button
                                    slot="close"
                                    variant="danger"
                                    onPress={() => shelfMutation.mutate({ productId: product.id, off: true })}
                                  >
                                    确认下架
                                  </Button>
                                </AlertDialog.Footer>
                              </AlertDialog.Dialog>
                            </AlertDialog.Container>
                          </AlertDialog.Backdrop>
                        </AlertDialog>
                      ) : (
                        <Button
                          isIconOnly
                          aria-label="重新上架"
                          isDisabled={shelfMutation.isPending}
                          size="sm"
                          variant="tertiary"
                          onPress={() => shelfMutation.mutate({ productId: product.id, off: false })}
                        >
                          <ArrowDownUp className="size-4 rotate-180" />
                        </Button>
                      )}
                      <Button
                        isIconOnly
                        aria-label="编辑商品"
                        size="sm"
                        variant="tertiary"
                        onPress={() => navigate(`/products/${product.id}/edit`)}
                      >
                        <Pencil className="size-4" />
                      </Button>
                      <Button
                        isIconOnly
                        aria-label="管理图片"
                        size="sm"
                        variant="tertiary"
                        onPress={() => navigate(`/products/${product.id}/images`)}
                      >
                        <Images className="size-4" />
                      </Button>
                    </>
                  ) : (
                    <span className="text-xs text-muted">预留/售出商品不可编辑</span>
                  )}
                </div>
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
