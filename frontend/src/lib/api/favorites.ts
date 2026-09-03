import { apiFetch } from '@/lib/http'
import type { FavoritePage, Identifier } from '@/lib/types'

export function listFavorites(params: { page?: number; page_size?: number }): Promise<FavoritePage> {
  return apiFetch<FavoritePage>({ path: '/favorites', query: params })
}

/** 幂等：重复收藏返回成功。 */
export function addFavorite(productId: Identifier): Promise<Record<string, never>> {
  return apiFetch({ method: 'PUT', path: `/favorites/${encodeURIComponent(productId)}` })
}

/** 幂等：未收藏时也返回成功。 */
export function removeFavorite(productId: Identifier): Promise<Record<string, never>> {
  return apiFetch({ method: 'DELETE', path: `/favorites/${encodeURIComponent(productId)}` })
}
