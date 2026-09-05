import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { motion } from 'motion/react'
import { Pagination, Spinner } from '@heroui/react'
import { Store, Star, UserX } from 'lucide-react'
import { getUserProfile, listUserProducts } from '@/lib/api/users'
import { nicknameInitial } from '@/lib/format'
import { isApiError } from '@/lib/http'
import { EmptyState } from '@/components/empty-state'
import { ProductCard } from '@/components/product-card'

const PAGE_SIZE = 12

/** 公开用户主页：资料头部（昵称 + 卖家平均分）与该用户的在售/已售出商品。 */
export function UserShelfPage() {
  const { userId = '' } = useParams()
  const [page, setPage] = useState(1)

  const profile = useQuery({
    queryKey: ['user-profile', userId],
    queryFn: () => getUserProfile(userId),
    retry: false,
  })

  const shelf = useQuery({
    queryKey: ['user-shelf', userId, page],
    queryFn: () => listUserProducts(userId, { page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
    enabled: !profile.isError,
  })

  if (profile.isPending) {
    return (
      <div className="flex h-40 items-center justify-center">
        <Spinner size="lg" />
      </div>
    )
  }

  if (profile.isError) {
    return (
      <EmptyState
        description={isApiError(profile.error) && profile.error.code === 'RESOURCE_NOT_FOUND'
          ? '这个用户可能已经不存在了'
          : '加载用户资料失败，请稍后重试'}
        icon={<UserX className="size-10" />}
        title="找不到该用户"
      />
    )
  }

  const user = profile.data
  const totalPages = shelf.data ? Math.max(1, Math.ceil(shelf.data.total / shelf.data.page_size)) : 1

  return (
    <div className="flex flex-col gap-6">
      {/* 资料头部 */}
      <section className="flex items-center gap-4 rounded-2xl border border-border-secondary bg-surface-tertiary p-5">
        <div className="flex size-14 shrink-0 items-center justify-center rounded-full bg-accent-soft text-lg font-semibold text-accent-soft-foreground">
          {nicknameInitial(user.nickname)}
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-xl font-bold tracking-tight text-foreground">{user.nickname}</h1>
          <p className="mt-1.5 flex items-center gap-1.5 text-xs text-muted">
            <Star className="size-3.5 text-highlight" />
            {user.average_score != null ? (
              <span className="tabular-nums">卖家评分 {user.average_score}</span>
            ) : (
              <span>暂无买家评分</span>
            )}
          </p>
        </div>
      </section>

      {/* 商品区 */}
      <div className="flex flex-col gap-4">
        <h2 className="text-sm font-semibold text-foreground">TA 的商品</h2>
        {shelf.isPending ? (
          <div className="flex h-40 items-center justify-center">
            <Spinner size="lg" />
          </div>
        ) : !shelf.data || shelf.data.items.length === 0 ? (
          <EmptyState
            description="该用户暂无在售或已售出的商品"
            icon={<Store className="size-10" />}
            title="还没有公开商品"
          />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
              {shelf.data.items.map((product, index) => (
                <motion.div
                  animate={{ opacity: 1, y: 0 }}
                  initial={{ opacity: 0, y: 12 }}
                  key={product.id}
                  transition={{
                    delay: Math.min(index * 0.04, 0.24),
                    duration: 0.28,
                    ease: [0.21, 0.68, 0.35, 1],
                  }}
                >
                  <ProductCard product={product} />
                </motion.div>
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
    </div>
  )
}
