import { apiFetch } from '@/lib/http'
import type {
  CategoryFilter,
  Identifier,
  ProductDetail,
  ProductImage,
  ProductPage,
} from '@/lib/types'

export interface ListProductsParams {
  keyword?: string
  category?: CategoryFilter
  page?: number
  page_size?: number
}

export function listProducts(params: ListProductsParams): Promise<ProductPage> {
  return apiFetch<ProductPage>({ path: '/products', query: params })
}

export function getProduct(productId: Identifier): Promise<ProductDetail> {
  return apiFetch<ProductDetail>({ path: `/products/${encodeURIComponent(productId)}` })
}

export interface ProductFieldsInput {
  title: string
  price: string
  category: string
  description: string
}

/** 发布商品：multipart，图片按出现顺序成为 1..N，第一张是封面。 */
export function createProduct(fields: ProductFieldsInput, images: File[]): Promise<ProductDetail> {
  const form = new FormData()
  form.set('title', fields.title)
  form.set('price', fields.price)
  form.set('category', fields.category)
  form.set('description', fields.description)
  for (const image of images) {
    form.append('images', image)
  }
  return apiFetch<ProductDetail>({ method: 'POST', path: '/products', formData: form })
}

/** 修改商品内容（不能修改状态）。省略字段表示保持原值，这里总是提交完整字段。 */
export function updateProduct(
  productId: Identifier,
  fields: Partial<ProductFieldsInput>,
): Promise<ProductDetail> {
  return apiFetch<ProductDetail>({
    method: 'PATCH',
    path: `/products/${encodeURIComponent(productId)}`,
    json: fields,
  })
}

/** 补充图片，新增后总数不能超过三张。 */
export function addProductImages(productId: Identifier, images: File[]): Promise<{ images: ProductImage[] }> {
  const form = new FormData()
  for (const image of images) {
    form.append('images', image)
  }
  return apiFetch<{ images: ProductImage[] }>({
    method: 'POST',
    path: `/products/${encodeURIComponent(productId)}/images`,
    formData: form,
  })
}

export function deleteProductImage(productId: Identifier, imageId: Identifier): Promise<Record<string, never>> {
  return apiFetch({
    method: 'DELETE',
    path: `/products/${encodeURIComponent(productId)}/images/${encodeURIComponent(imageId)}`,
  })
}

export function offShelfProduct(productId: Identifier): Promise<ProductDetail> {
  return apiFetch<ProductDetail>({ method: 'POST', path: `/products/${encodeURIComponent(productId)}/off-shelf` })
}

export function relistProduct(productId: Identifier): Promise<ProductDetail> {
  return apiFetch<ProductDetail>({ method: 'POST', path: `/products/${encodeURIComponent(productId)}/relist` })
}
