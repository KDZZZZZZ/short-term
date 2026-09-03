import type { ProductStatus, TradeStatus } from '@/lib/types'
import { productStatusLabel, tradeStatusLabel } from '@/lib/format'
import { Chip } from '@heroui/react'

const PRODUCT_TONE: Record<ProductStatus, { color: 'success' | 'warning' | 'danger' | 'default'; variant: 'soft' | 'primary' }> = {
  ON_SALE: { color: 'success', variant: 'soft' },
  RESERVED: { color: 'warning', variant: 'soft' },
  SOLD: { color: 'default', variant: 'primary' },
  OFF_SHELF: { color: 'default', variant: 'soft' },
}

export function ProductStatusChip({ status }: { status: ProductStatus }) {
  const tone = PRODUCT_TONE[status]
  return (
    <Chip color={tone.color} size="sm" variant={tone.variant}>
      <Chip.Label>{productStatusLabel(status)}</Chip.Label>
    </Chip>
  )
}

const TRADE_TONE: Record<TradeStatus, { color: 'warning' | 'accent' | 'success' | 'danger' }> = {
  PENDING: { color: 'warning' },
  ACCEPTED: { color: 'accent' },
  COMPLETED: { color: 'success' },
  CANCELLED: { color: 'danger' },
}

export function TradeStatusChip({ status }: { status: TradeStatus }) {
  return (
    <Chip color={TRADE_TONE[status].color} size="sm" variant="soft">
      <Chip.Label>{tradeStatusLabel(status)}</Chip.Label>
    </Chip>
  )
}
