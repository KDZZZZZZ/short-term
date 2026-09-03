import { apiFetch } from '@/lib/http'
import type { ProductPage, ProductStatusFilter } from '@/lib/types'

export function listCurrentUserProducts(params: {
  status?: ProductStatusFilter
  page?: number
  page_size?: number
}): Promise<ProductPage> {
  return apiFetch<ProductPage>({ path: '/users/me/products', query: params })
}
