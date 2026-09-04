import { motion } from 'motion/react'
import { ImageOff } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button, Card } from '@heroui/react'
import type { ProductSummary } from '@/lib/types'
import { categoryLabel, formatPrice, formatRelativeTime } from '@/lib/format'
import { ProductStatusChip } from '@/components/status-chip'

interface ProductCardProps {
  product: ProductSummary
  /** 进场动画：false 关闭；传对象可自定义延迟（网格错落入场）。 */
  animate?: boolean | { delay?: number }
}

export function ProductCard({ product, animate = true }: ProductCardProps) {
  const navigate = useNavigate()

  const open = () => navigate(`/products/${product.id}`)

  const body = (
    <Card className="card-interactive group flex h-full w-full flex-col gap-0 overflow-hidden p-0">
      <button
        aria-label={`查看 ${product.title}`}
        className="relative aspect-[3/4] w-full cursor-pointer overflow-hidden bg-surface-secondary text-left outline-none focus-visible:ring-2 focus-visible:ring-accent"
        onClick={open}
        type="button"
      >
        {product.cover_url ? (
          <img
            alt={product.title}
            className="pointer-events-none absolute inset-0 h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
            src={product.cover_url}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted">
            <ImageOff className="size-8 opacity-40" />
          </div>
        )}
        {product.status !== 'ON_SALE' ? (
          <div className="absolute top-2 start-2">
            <ProductStatusChip status={product.status} />
          </div>
        ) : null}
      </button>

      <div className="flex flex-1 flex-col items-center gap-1 px-4 pb-4 pt-3 text-center">
        <button
          className="line-clamp-1 cursor-pointer text-base font-semibold text-foreground outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent"
          onClick={open}
          type="button"
        >
          {product.title}
        </button>
        <p className="text-xs text-muted">{categoryLabel(product.category)}</p>
        <p className="tabular-nums text-lg font-bold text-foreground">{formatPrice(product.price)}</p>
        <p className="text-xs text-muted">
          {product.seller.nickname} · {formatRelativeTime(product.created_at)}
        </p>
        <Button
          className="mt-2 w-full"
          size="sm"
          variant="outline"
          onPress={open}
        >
          查看详情
        </Button>
      </div>
    </Card>
  )

  if (!animate) return body
  const delay = typeof animate === 'object' ? (animate.delay ?? 0) : 0
  return (
    <motion.div
      animate={{ opacity: 1, y: 0 }}
      initial={{ opacity: 0, y: 12 }}
      transition={{ delay, duration: 0.28, ease: [0.21, 0.68, 0.35, 1] }}
    >
      {body}
    </motion.div>
  )
}

/** 市场骨架屏（与商品卡同构） */
export function ProductCardSkeleton() {
  return (
    <Card className="flex h-full w-full flex-col gap-0 overflow-hidden p-0">
      <div className="aspect-[3/4] w-full animate-pulse bg-surface-secondary" />
      <div className="flex flex-col items-center gap-2 px-4 pb-4 pt-3">
        <div className="h-4 w-3/4 animate-pulse rounded bg-surface-secondary" />
        <div className="h-3 w-1/3 animate-pulse rounded bg-surface-secondary" />
        <div className="h-6 w-1/2 animate-pulse rounded bg-surface-secondary" />
        <div className="mt-2 h-9 w-full animate-pulse rounded-lg bg-surface-secondary" />
      </div>
    </Card>
  )
}
