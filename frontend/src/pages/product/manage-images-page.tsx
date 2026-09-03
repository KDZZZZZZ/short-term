import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Card, Spinner, toast } from '@heroui/react'
import { ArrowLeft, ImageOff, Trash2 } from 'lucide-react'
import { addProductImages, deleteProductImage, getProduct } from '@/lib/api/products'
import { isApiError } from '@/lib/http'
import { MAX_IMAGES } from '@/lib/format'
import { ImageUploader } from '@/components/image-uploader'

export function ManageImagesPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const { data: product, isPending } = useQuery({
    queryKey: ['product', productId],
    queryFn: () => getProduct(productId),
  })

  const [files, setFiles] = useState<File[]>([])
  const [error, setError] = useState<string | null>(null)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['product', productId] })
    void queryClient.invalidateQueries({ queryKey: ['my-products'] })
    void queryClient.invalidateQueries({ queryKey: ['products'] })
  }

  const addMutation = useMutation({
    mutationFn: () => addProductImages(productId, files),
    onSuccess: () => {
      toast.success('图片已添加')
      setFiles([])
      invalidate()
    },
    onError: (err) => {
      setError(isApiError(err) ? err.message : '网络异常，请稍后重试')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (imageId: string) => deleteProductImage(productId, imageId),
    onSuccess: () => {
      toast.success('图片已删除')
      invalidate()
    },
    onError: (err) => {
      toast.danger(isApiError(err) ? err.message : '网络异常，请稍后重试')
    },
  })

  if (isPending || !product) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    )
  }

  const editable = product.status === 'ON_SALE' || product.status === 'OFF_SHELF'
  const existing = product.images
  const remaining = MAX_IMAGES - existing.length - files.length

  const submit = (event: React.FormEvent) => {
    event.preventDefault()
    if (!editable) {
      setError('商品当前状态不允许修改图片')
      return
    }
    if (files.length === 0) {
      setError('请先选择要添加的图片')
      return
    }
    setError(null)
    addMutation.mutate()
  }

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button isIconOnly aria-label="返回" size="sm" variant="tertiary" onPress={() => navigate(-1)}>
          <ArrowLeft className="size-4" />
        </Button>
        <h1 className="text-lg font-bold">管理商品图片</h1>
      </div>

      {!editable ? (
        <Card className="p-4 text-sm text-muted">商品处于已预留/已售出状态，图片不可修改。</Card>
      ) : null}

      <Card className="flex flex-col gap-3 p-4 sm:p-6">
        <div className="text-sm font-semibold">当前图片（{existing.length}/{MAX_IMAGES}）</div>
        {existing.length === 0 ? (
          <div className="flex items-center gap-2 rounded-xl border border-dashed border-border-secondary p-6 text-muted">
            <ImageOff className="size-5" />
            暂无图片，第一张新图片将成为封面
          </div>
        ) : (
          <div className="flex flex-wrap gap-3">
            {existing.map((image) => (
              <div
                className="group relative h-28 w-28 overflow-hidden rounded-xl border border-border-secondary bg-surface"
                key={image.id}
              >
                <img alt={`图片 ${image.sort_order}`} className="h-full w-full object-cover" src={image.url} />
                {image.sort_order === 1 ? (
                  <span className="absolute inset-x-0 bottom-0 bg-black/55 py-0.5 text-center text-[10px] text-white">
                    封面
                  </span>
                ) : null}
                <Button
                  isIconOnly
                  aria-label="删除图片"
                  className="absolute end-1 top-1"
                  isDisabled={!editable || deleteMutation.isPending}
                  size="sm"
                  variant="danger"
                  onPress={() => deleteMutation.mutate(image.id)}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </Card>

      {editable ? (
        <form className="flex flex-col gap-4" onSubmit={submit}>
          <Card className="flex flex-col gap-4 p-4 sm:p-6">
            <div className="text-sm font-semibold">添加图片</div>
            {remaining <= 0 ? (
              <p className="text-sm text-muted">图片数量已达上限（{MAX_IMAGES} 张），请先删除再添加。</p>
            ) : (
              <ImageUploader
                existingCount={existing.length}
                files={files}
                onChange={setFiles}
              />
            )}
            {error ? <p className="text-sm text-danger">{error}</p> : null}
            <div className="flex justify-end">
              <Button isDisabled={remaining <= 0 || files.length === 0} isPending={addMutation.isPending} type="submit">
                添加 {files.length > 0 ? `（${files.length} 张）` : ''}
              </Button>
            </div>
          </Card>
        </form>
      ) : null}
    </div>
  )
}
