import type { ProductFormState } from '@/components/product-form-fields'

/** 与契约字段约束一致的发布/编辑前置校验；返回错误文案或 null。 */
export function validateProductForm(form: ProductFormState): string | null {
  if (!form.title.trim()) return '请填写商品标题'
  if (form.title.trim().length > 100) return '标题最长 100 字'
  if (!form.price.trim()) return '请填写价格'
  if (!/^(0|[1-9][0-9]{0,7})(\.[0-9]{1,2})?$/.test(form.price)) {
    return '价格格式不正确，最多两位小数，如 20.00'
  }
  if (!form.category) return '请选择商品分类'
  if (!form.description.trim()) return '请填写商品描述'
  if (form.description.trim().length > 2000) return '描述最长 2000 字'
  return null
}
