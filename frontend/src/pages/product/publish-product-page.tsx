import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Button, Card, toast } from '@heroui/react'
import { ArrowLeft } from 'lucide-react'
import { createProduct } from '@/lib/api/products'
import { isApiError } from '@/lib/http'
import { ProductFormFields, type ProductFormState } from '@/components/product-form-fields'
import { validateProductForm } from '@/lib/product-validation'
import { ImageUploader } from '@/components/image-uploader'

export function PublishProductPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<ProductFormState>({ title: '', price: '', category: '', description: '' })
  const [files, setFiles] = useState<File[]>([])
  const [error, setError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: () =>
      createProduct(
        {
          title: form.title.trim(),
          price: form.price,
          category: form.category,
          description: form.description.trim(),
        },
        files,
      ),
    onSuccess: (product) => {
      toast.success('商品已发布')
      void queryClient.invalidateQueries({ queryKey: ['products'] })
      void queryClient.invalidateQueries({ queryKey: ['my-products'] })
      navigate(`/products/${product.id}`, { replace: true })
    },
    onError: (err) => {
      if (isApiError(err)) {
        setError(
          err.code === 'CONTACT_REQUIRED'
            ? '发布前请在“个人资料”中至少填写微信或 QQ'
            : err.message,
        )
      } else {
        setError('网络异常，请稍后重试')
      }
    },
  })

  const submit = (event: React.FormEvent) => {
    event.preventDefault()
    const invalid = validateProductForm(form)
    if (invalid) {
      setError(invalid)
      return
    }
    setError(null)
    mutation.mutate()
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button isIconOnly aria-label="返回" size="sm" variant="tertiary" onPress={() => navigate(-1)}>
          <ArrowLeft className="size-4" />
        </Button>
        <h1 className="text-lg font-bold">发布商品</h1>
      </div>

      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Card className="flex flex-col gap-4 p-4 sm:p-6">
          <ProductFormFields form={form} onChange={setForm} />
        </Card>

        <Card className="flex flex-col gap-4 p-4 sm:p-6">
          <div className="text-sm font-semibold">商品图片（选填）</div>
          <ImageUploader files={files} onChange={setFiles} />
        </Card>

        {error ? <p className="text-sm text-danger">{error}</p> : null}

        <div className="flex justify-end gap-2">
          <Button variant="tertiary" onPress={() => navigate(-1)}>
            取消
          </Button>
          <Button isDisabled={mutation.isPending} isPending={mutation.isPending} type="submit">
            发布
          </Button>
        </div>
      </form>
    </div>
  )
}
