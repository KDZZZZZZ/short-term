import { apiFetch } from '@/lib/http'
import type { Comment, CommentCreateRequest, CommentPage, Identifier } from '@/lib/types'

export interface ListProductCommentsParams {
  page?: number
  page_size?: number
}

/** 商品评论固定按 created_at DESC 排列，任何状态的现存商品都可读。 */
export function listProductComments(
  productId: Identifier,
  params: ListProductCommentsParams = {},
): Promise<CommentPage> {
  return apiFetch<CommentPage>({
    path: `/products/${encodeURIComponent(productId)}/comments`,
    query: params,
  })
}

/** 任何已认证用户可对任何现存商品发布评论，可多条，发布后不可修改删除。 */
export function createProductComment(productId: Identifier, body: CommentCreateRequest): Promise<Comment> {
  return apiFetch<Comment>({
    method: 'POST',
    path: `/products/${encodeURIComponent(productId)}/comments`,
    json: body,
  })
}
