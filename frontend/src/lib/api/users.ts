import { apiFetch } from '@/lib/http'
import type { Identifier, MyProductPage, ProductPage, ProductStatusFilter, UserProfile } from '@/lib/types'

/** 其他用户的公开资料：昵称与卖家平均分。 */
export function getUserProfile(userId: Identifier): Promise<UserProfile> {
  return apiFetch<UserProfile>({ path: `/users/${encodeURIComponent(userId)}` })
}

export function listCurrentUserProducts(params: {
  status?: ProductStatusFilter
  page?: number
  page_size?: number
}): Promise<MyProductPage> {
  return apiFetch<MyProductPage>({ path: '/users/me/products', query: params })
}

/** 其他用户的公开货架：在售与已售出商品（RESERVED/OFF_SHELF 仅卖家本人可见）。 */
export function listUserProducts(
  userId: Identifier,
  params: { page?: number; page_size?: number } = {},
): Promise<ProductPage> {
  return apiFetch<ProductPage>({
    path: `/users/${encodeURIComponent(userId)}/products`,
    query: params,
  })
}
