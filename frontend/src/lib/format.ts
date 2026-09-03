import type { ProductCategory, ProductStatus, TradeStatus } from '@/lib/types'

export const CATEGORY_OPTIONS = [
  { value: 'TEXTBOOK', label: '教材教辅' },
  { value: 'DIGITAL', label: '数码电子' },
  { value: 'LIFE', label: '生活用品' },
  { value: 'OTHER', label: '其他' },
] as const satisfies ReadonlyArray<{ value: ProductCategory; label: string }>

export function categoryLabel(category: ProductCategory): string {
  return CATEGORY_OPTIONS.find((option) => option.value === category)?.label ?? category
}

export const PRODUCT_STATUS_OPTIONS: ReadonlyArray<{ value: ProductStatus; label: string }> = [
  { value: 'ON_SALE', label: '在售' },
  { value: 'RESERVED', label: '已预留' },
  { value: 'SOLD', label: '已售出' },
  { value: 'OFF_SHELF', label: '已下架' },
]

export function productStatusLabel(status: ProductStatus): string {
  return PRODUCT_STATUS_OPTIONS.find((option) => option.value === status)?.label ?? status
}

export const TRADE_STATUS_OPTIONS: ReadonlyArray<{ value: TradeStatus; label: string }> = [
  { value: 'PENDING', label: '待处理' },
  { value: 'ACCEPTED', label: '已接受' },
  { value: 'COMPLETED', label: '已完成' },
  { value: 'CANCELLED', label: '已取消' },
]

export function tradeStatusLabel(status: TradeStatus): string {
  return TRADE_STATUS_OPTIONS.find((option) => option.value === status)?.label ?? status
}

/** 契约要求金额为十进制字符串，展示时原样使用，不做浮点运算。 */
export function formatPrice(price: string): string {
  return `¥${price}`
}

export function formatDateTime(iso: string): string {
  const date = new Date(iso)
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

export function formatRelativeTime(iso: string): string {
  const then = new Date(iso).getTime()
  const diff = Date.now() - then
  if (diff < 60_000) return '刚刚'
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days} 天前`
  return formatDateTime(iso)
}

export function nicknameInitial(nickname: string): string {
  return Array.from(nickname.trim())[0]?.toUpperCase() ?? '?'
}

/** 契约价格格式：^(0|[1-9][0-9]{0,7})(\.[0-9]{1,2})?$ */
export const PRICE_PATTERN = /^(0|[1-9][0-9]{0,7})(\.[0-9]{1,2})?$/

/** QQ：^[0-9]{5,20}$；微信：非空白 1..64 字符。 */
export const QQ_PATTERN = /^[0-9]{5,20}$/
export const WECHAT_PATTERN = /^\S{1,64}$/

export const MAX_IMAGES = 3
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024
export const ALLOWED_IMAGE_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp'])
