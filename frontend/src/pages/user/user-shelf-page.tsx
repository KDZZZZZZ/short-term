import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { motion } from 'motion/react'
import { Pagination, Spinner } from '@heroui/react'
import { Store } from 'lucide-react'
import { listUserProducts } from '@/lib/api/users'
import { EmptyState } from '@/components/empty-state'
import { ProductCard } from '@/components/product-card'

const PAGE_SIZE = 12

/** 公开卖家货架：该用户的在售与已售出商品（RESERVED/OFF_SHELF 不对外）。 */
export function UserShelfPage() {
  const { userId = '' } = useParams()
  const [page, setPage] = useState(1)

  const { data, isPending } = useQuery({
    queryKey: ['user-shelf', userId, page],
    queryFn: () => listUserProducts(userId, { page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  })

  const nickname = data?.items[0]?.seller.nickname
  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-lg font-bold">{nickname ? `${nickname} 的商品` : 'TA 的商品'}</h1>
        <p className="mt-1 text-xs text-muted">展示在售与已售出商品；已售出商品可在详情页查看买家评价</p>
      </div>

      {isPending ? (
        <div className="flex h-40 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          description="该用户暂无在售或已售出的商品"
          icon={<Store className="size-10" />}
          title="还没有公开商品"
        />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            {data.items.map((product, index) => (
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
  )
}
