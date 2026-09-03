import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { AlertDialog, Button, Card, Separator, Spinner, toast } from '@heroui/react'
import {
  ChevronRight,
  Heart,
  HeartOff,
  ImageOff,
  Images,
  MessageCircle,
  Package,
  Pencil,
  ShoppingCart,
  Store,
} from 'lucide-react'
import { addFavorite, removeFavorite } from '@/lib/api/favorites'
import { getOrCreateProductConversation } from '@/lib/api/conversations'
import { getProduct, offShelfProduct, relistProduct } from '@/lib/api/products'
import { createTrade } from '@/lib/api/trades'
import { isApiError } from '@/lib/http'
import { categoryLabel, formatDateTime, formatPrice, nicknameInitial } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { ProductStatusChip } from '@/components/status-chip'

export function ProductDetailPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const me = useAuthStore((state) => state.user)

  const [activeImage, setActiveImage] = useState(0)

  const { data: product, isPending, error } = useQuery({
    queryKey: ['product', productId],
    queryFn: () => getProduct(productId),
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['product', productId] })
    void queryClient.invalidateQueries({ queryKey: ['products'] })
  }

  const favoriteMutation = useMutation({
    mutationFn: async (favorite: boolean) => (favorite ? addFavorite(productId) : removeFavorite(productId)),
    onSuccess: (_, favorite) => {
      toast.success(favorite ? '已收藏' : '已取消收藏')
      invalidate()
    },
    onError: (err) => {
      if (isApiError(err)) toast.danger(err.message)
    },
  })

  const chatMutation = useMutation({
    mutationFn: () => getOrCreateProductConversation(productId),
    onSuccess: (conversation) => navigate(`/chats/${conversation.id}`),
    onError: (err) => {
      if (isApiError(err)) {
        if (err.code === 'SELF_ACTION_NOT_ALLOWED') {
          toast.warning('不能和自己发布的商品发起会话')
        } else {
          toast.danger(err.message)
        }
      }
    },
  })

  const tradeMutation = useMutation({
    mutationFn: () => createTrade(productId),
    onSuccess: (trade) => {
      toast.success(
        trade.status === 'PENDING'
          ? '已向卖家发出购买意向，等待卖家处理'
          : '该商品的购买意向已存在（可在我买到的中查看）',
      )
      navigate('/trades', { state: { as: 'buyer' } })
    },
    onError: (err) => {
      if (isApiError(err)) {
        if (err.code === 'SELF_ACTION_NOT_ALLOWED') {
          toast.warning('不能购买自己发布的商品')
        } else if (err.code === 'PRODUCT_NOT_AVAILABLE') {
          toast.warning('商品当前不可交易')
        } else {
          toast.danger(err.message)
        }
      }
    },
  })

  const shelfMutation = useMutation({
    mutationFn: (off: boolean) => (off ? offShelfProduct(productId) : relistProduct(productId)),
    onSuccess: (updated) => {
      toast.success(updated.status === 'OFF_SHELF' ? '商品已下架' : '商品已重新上架')
      invalidate()
    },
    onError: (err) => {
      if (isApiError(err)) toast.danger(err.message)
    },
  })

  const isMine = useMemo(() => me?.id === product?.seller.id, [me, product])

  if (isPending) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error || !product) {
    const notFound = isApiError(error) && error.status === 404
    return (
      <div className="flex h-64 flex-col items-center justify-center gap-3">
        <Package className="size-10 text-muted" />
        <p className="text-base font-medium">{notFound ? '商品不存在' : '加载失败，请稍后重试'}</p>
        <Button variant="outline" onPress={() => queryClient.invalidateQueries({ queryKey: ['product', productId] })}>
          重新加载
        </Button>
      </div>
    )
  }

  const urls = product.images.map((image) => image.url)
  const active = urls[Math.min(activeImage, Math.max(urls.length - 1, 0))]

  return (
    <div className="flex flex-col gap-6">
      {/* 面包屑：市场 > 分类 > 商品名 */}
      <nav aria-label="面包屑" className="flex flex-wrap items-center gap-1 text-sm text-muted">
        <Link className="hover:text-accent hover:underline" to="/">
          市场
        </Link>
        <ChevronRight className="size-3.5" />
        <span>{categoryLabel(product.category)}</span>
        <ChevronRight className="size-3.5" />
        <span className="line-clamp-1 font-medium text-foreground">{product.title}</span>
      </nav>

      <div className="grid gap-8 lg:grid-cols-2">
        {/* 左：竖排缩略图 + 主图 */}
        <div className="flex gap-3">
          {urls.length > 1 ? (
            <div className="flex w-20 shrink-0 flex-col gap-2">
              {urls.map((url, index) => (
                <button
                  className={`h-20 w-20 overflow-hidden rounded-lg border bg-surface-secondary transition-colors ${
                    index === activeImage ? 'border-accent' : 'border-border-secondary hover:border-accent/50'
                  }`}
                  key={url}
                  onClick={() => setActiveImage(index)}
                  type="button"
                >
                  <img alt={`第 ${index + 1} 张`} className="h-full w-full object-cover" src={url} />
                </button>
              ))}
            </div>
          ) : null}

          <div className="relative min-h-72 flex-1 overflow-hidden rounded-xl border border-border-secondary bg-surface-secondary">
            {active ? (
              <img
                alt={product.title}
                className="pointer-events-none absolute inset-0 h-full w-full object-cover"
                src={active}
              />
            ) : (
              <div className="flex h-full w-full items-center justify-center text-muted">
                <ImageOff className="size-10 opacity-40" />
              </div>
            )}
          </div>
        </div>

        {/* 右：标题 / 价格 / 描述 / 卖家 / 操作 */}
        <div className="flex flex-col">
          {product.status !== 'ON_SALE' ? (
            <div className="mb-3 w-fit">
              <ProductStatusChip status={product.status} />
            </div>
          ) : null}

          <h1 className="text-3xl font-bold leading-tight tracking-tight text-foreground">
            {product.title}
          </h1>
          <p className="tabular-nums mt-3 text-3xl font-bold text-accent">{formatPrice(product.price)}</p>
          <p className="mt-2 text-xs text-muted">
            {categoryLabel(product.category)} · 发布于 {formatDateTime(product.created_at)}
          </p>

          <p className="mt-5 whitespace-pre-wrap text-sm leading-6 text-foreground/90">
            {product.description}
          </p>

          <Separator className="my-6" />

          {/* 卖家联系方式 */}
          <div className="flex items-center gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-accent-soft text-sm font-semibold text-accent-soft-foreground">
              {nicknameInitial(product.seller.nickname)}
            </div>
            <div className="min-w-0 flex-1 text-sm">
              <p className="font-medium text-foreground">{product.seller.nickname}</p>
              <p className="text-xs text-muted">
                微信 {product.seller.wechat ?? '未填写'} · QQ {product.seller.qq ?? '未填写'}
              </p>
            </div>
          </div>

          {/* 操作按钮（买家） */}
          {isMine ? (
            <div className="mt-6 flex flex-col gap-2">
              {product.status === 'ON_SALE' ? (
                <>
                  <AlertDialog>
                    <Button className="w-full" size="lg" variant="outline">
                      下架商品
                    </Button>
                    <AlertDialog.Backdrop>
                      <AlertDialog.Container>
                        <AlertDialog.Dialog className="sm:max-w-[400px]">
                          <AlertDialog.CloseTrigger />
                          <AlertDialog.Header>
                            <AlertDialog.Icon status="danger" />
                            <AlertDialog.Heading>确认下架该商品？</AlertDialog.Heading>
                          </AlertDialog.Header>
                          <AlertDialog.Body>
                            <p>
                              下架后商品不再出现在市场列表中，可随时重新上架。若存在待处理的购买意向，需要先处理完毕。
                            </p>
                          </AlertDialog.Body>
                          <AlertDialog.Footer>
                            <Button slot="close" variant="tertiary">
                              取消
                            </Button>
                            <Button
                              slot="close"
                              variant="danger"
                              onPress={() => shelfMutation.mutate(true)}
                            >
                              确认下架
                            </Button>
                          </AlertDialog.Footer>
                        </AlertDialog.Dialog>
                      </AlertDialog.Container>
                    </AlertDialog.Backdrop>
                  </AlertDialog>
                  <Button className="w-full" onPress={() => navigate(`/products/${productId}/edit`)}>
                    <Pencil className="size-4" />
                    编辑信息
                  </Button>
                  <Button
                    className="w-full"
                    variant="outline"
                    onPress={() => navigate(`/products/${productId}/images`)}
                  >
                    <Images className="size-4" />
                    管理图片
                  </Button>
                </>
              ) : product.status === 'OFF_SHELF' ? (
                <>
                  <Button
                    className="w-full"
                    isPending={shelfMutation.isPending}
                    size="lg"
                    onPress={() => shelfMutation.mutate(false)}
                  >
                    <Store className="size-4" />
                    重新上架
                  </Button>
                  <Button
                    className="w-full"
                    variant="outline"
                    onPress={() => navigate(`/products/${productId}/edit`)}
                  >
                    <Pencil className="size-4" />
                    编辑信息
                  </Button>
                  <Button
                    className="w-full"
                    variant="outline"
                    onPress={() => navigate(`/products/${productId}/images`)}
                  >
                    <Images className="size-4" />
                    管理图片
                  </Button>
                </>
              ) : (
                <p className="text-sm text-muted">
                  商品当前为 {product.status === 'RESERVED' ? '已预留' : '已售出'}
                  状态，不能编辑或上下架。
                </p>
              )}
              <Separator className="my-4" />
              <Link
                className="text-center text-sm text-muted underline-offset-4 hover:text-accent hover:underline"
                to="/trades"
              >
                查看我卖出的购买意向 →
              </Link>
            </div>
          ) : (
            <div className="mt-6 flex flex-col gap-2">
              <Button
                className="w-full"
                isDisabled={product.status !== 'ON_SALE'}
                isPending={tradeMutation.isPending}
                size="lg"
                onPress={() => tradeMutation.mutate()}
              >
                <ShoppingCart className="size-4" />
                {product.status === 'ON_SALE' ? '我想要' : '商品已不可交易'}
              </Button>
              <Button
                className="w-full"
                isPending={chatMutation.isPending}
                size="lg"
                variant="outline"
                onPress={() => chatMutation.mutate()}
              >
                <MessageCircle className="size-4" />
                和卖家聊聊
              </Button>
              <Button
                className="w-full"
                variant="outline"
                onPress={() => favoriteMutation.mutate(!product.is_favorited)}
              >
                {product.is_favorited ? (
                  <>
                    <HeartOff className="size-4" />
                    取消收藏
                  </>
                ) : (
                  <>
                    <Heart className="size-4" />
                    收藏
                  </>
                )}
              </Button>
              <p className="mt-2 text-center text-xs text-muted">
                线下面交，请当面验货并确认后再完成交易
              </p>
            </div>
          )}
        </div>
      </div>

      {/* 底部占位，避免右列超高时贴底 */}
      <Card className="p-4 text-xs text-muted">
        商品信息由卖家发布，请通过站内会话沟通确认商品成色、价格与交易地点后再发起购买意向。
      </Card>
    </div>
  )
}
