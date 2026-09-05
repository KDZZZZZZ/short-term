import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Card, toast } from '@heroui/react'
import { LissajousLoader } from '@/components/lissajous-loader'
import { ArrowLeft } from 'lucide-react'
import { getProduct, updateProduct } from '@/lib/api/products'
import { isApiError } from '@/lib/http'
import { ProductFormFields, type ProductFormState } from '@/components/product-form-fields'
import { validateProductForm } from '@/lib/product-validation'
import type { ProductDetail } from '@/lib/types'

/** 表单在商品加载完成后挂载，直接以商品内容初始化，避免用 effect 同步。 */
function EditProductForm({ product }: { product: ProductDetail }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<ProductFormState>({
    title: product.title,
    price: product.price,
    category: product.category,
    description: product.description,
  })
  const [error, setError] = useState<string | null>(null)

  const editable = product.status === 'ON_SALE' || product.status === 'OFF_SHELF'

  const mutation = useMutation({
    mutationFn: (fields: ProductFormState) =>
      updateProduct(product.id, {
        title: fields.title.trim(),
        price: fields.price,
        category: fields.category,
        description: fields.description.trim(),
      }),
    onSuccess: () => {
      toast.success('商品已更新')
      void queryClient.invalidateQueries({ queryKey: ['product', product.id] })
      void queryClient.invalidateQueries({ queryKey: ['my-products'] })
      void queryClient.invalidateQueries({ queryKey: ['products'] })
      navigate(`/products/${product.id}`, { replace: true })
    },
    onError: (err) => {
      setError(isApiError(err) ? err.message : '网络异常，请稍后重试')
    },
  })

  const submit = (event: React.FormEvent) => {
    event.preventDefault()
    if (!editable) {
      setError('商品当前状态不允许修改')
      return
    }
    const invalid = validateProductForm(form)
    if (invalid) {
      setError(invalid)
      return
    }
    setError(null)
    mutation.mutate(form)
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button isIconOnly aria-label="返回" size="sm" variant="tertiary" onPress={() => navigate(-1)}>
          <ArrowLeft className="size-4" />
        </Button>
        <h1 className="text-lg font-bold">编辑商品</h1>
      </div>

      {!editable ? (
        <Card className="p-4 text-sm text-muted">商品处于已预留/已售出状态，不能修改内容。</Card>
      ) : null}

      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Card className="flex flex-col gap-4 p-4 sm:p-6">
          <ProductFormFields form={form} onChange={setForm} />
        </Card>

        {error ? <p className="text-sm text-danger">{error}</p> : null}

        <div className="flex justify-end gap-2">
          <Button variant="tertiary" onPress={() => navigate(-1)}>
            取消
          </Button>
          <Button isDisabled={!editable || mutation.isPending} isPending={mutation.isPending} type="submit">
            保存修改
          </Button>
        </div>
      </form>
    </div>
  )
}

export function EditProductPage() {
  const { productId = '' } = useParams()

  const { data: product, isPending } = useQuery({
    queryKey: ['product', productId],
    queryFn: () => getProduct(productId),
  })

  if (isPending) {
    return (
      <div className="flex h-64 items-center justify-center">
        <LissajousLoader className="size-28 text-foreground" />
      </div>
    )
  }

  if (!product) {
    return (
      <div className="flex h-64 flex-col items-center justify-center gap-3">
        <p className="text-sm text-muted">商品不存在或加载失败</p>
      </div>
    )
  }

  return <EditProductForm product={product} />
}
