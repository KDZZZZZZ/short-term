import { apiFetch, newIdempotencyKey } from '@/lib/http'
import type {
  Identifier,
  ReasonRequest,
  Trade,
  TradeCreateRequest,
  TradePage,
  TradeReview,
  TradeReviewCreateRequest,
  TradeRole,
  TradeStatus,
} from '@/lib/types'

/** 创建或获取商品购买意向：首次 201，已存在 200，接口语义由服务端保证。 */
export function createTrade(productId: Identifier, body?: TradeCreateRequest): Promise<Trade> {
  return apiFetch<Trade>({
    method: 'POST',
    path: `/products/${encodeURIComponent(productId)}/trades`,
    json: body ?? {},
    idempotencyKey: newIdempotencyKey(),
  })
}

export interface ListTradesParams {
  as: TradeRole
  status?: TradeStatus
  page?: number
  page_size?: number
}

export function listTrades(params: ListTradesParams): Promise<TradePage> {
  return apiFetch<TradePage>({ path: '/trades', query: params })
}

export function getTrade(tradeId: Identifier): Promise<Trade> {
  return apiFetch<Trade>({ path: `/trades/${encodeURIComponent(tradeId)}` })
}

export function acceptTrade(tradeId: Identifier): Promise<Trade> {
  return apiFetch<Trade>({
    method: 'POST',
    path: `/trades/${encodeURIComponent(tradeId)}/accept`,
    idempotencyKey: newIdempotencyKey(),
  })
}

export function rejectTrade(tradeId: Identifier, body: ReasonRequest): Promise<Trade> {
  return apiFetch<Trade>({
    method: 'POST',
    path: `/trades/${encodeURIComponent(tradeId)}/reject`,
    json: body,
    idempotencyKey: newIdempotencyKey(),
  })
}

export function cancelTrade(tradeId: Identifier, body: ReasonRequest): Promise<Trade> {
  return apiFetch<Trade>({
    method: 'POST',
    path: `/trades/${encodeURIComponent(tradeId)}/cancel`,
    json: body,
    idempotencyKey: newIdempotencyKey(),
  })
}

export function confirmTrade(tradeId: Identifier): Promise<Trade> {
  return apiFetch<Trade>({
    method: 'POST',
    path: `/trades/${encodeURIComponent(tradeId)}/confirm`,
    idempotencyKey: newIdempotencyKey(),
  })
}

/** 发布买家评价：仅限买家本人且交易已 COMPLETED；每笔交易最多一条，重复发布 409。 */
export function createTradeReview(tradeId: Identifier, body: TradeReviewCreateRequest): Promise<TradeReview> {
  return apiFetch<TradeReview>({
    method: 'POST',
    path: `/trades/${encodeURIComponent(tradeId)}/review`,
    json: body,
  })
}
