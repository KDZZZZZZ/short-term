import { useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { motion } from 'motion/react'
import {
  AlertDialog,
  Avatar,
  Button,
  Card,
  Label,
  ListBox,
  Pagination,
  Select,
  TextArea,
  TextField,
  toast,
} from '@heroui/react'
import { LissajousLoader } from '@/components/lissajous-loader'
import {
  Check,
  CheckCheck,
  MessageCircle,
  Star,
  Store,
  X,
} from 'lucide-react'
import { HandshakeDealIcon } from '@/components/icons/koboyo'
import {
  acceptTrade,
  cancelTrade,
  confirmTrade,
  createTradeReview,
  listTrades,
  rejectTrade,
} from '@/lib/api/trades'
import { getOrCreateProductConversation } from '@/lib/api/conversations'
import { isApiError } from '@/lib/http'
import { formatPrice, nicknameInitial, TRADE_STATUS_OPTIONS } from '@/lib/format'
import type { Trade, TradeReview, TradeRole, TradeStatus } from '@/lib/types'
import { EmptyState } from '@/components/empty-state'
import { TradeStatusChip } from '@/components/status-chip'
import { useAuthStore } from '@/stores/auth-store'
import { TradeTimeline } from '@/components/trade-timeline'

const PAGE_SIZE = 10

type ReasonDialogState =
  | { kind: 'reject'; trade: Trade }
  | { kind: 'cancel'; trade: Trade }
  | null

function TradeActions({ trade, as }: { trade: Trade; as: TradeRole }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [dialog, setDialog] = useState<ReasonDialogState>(null)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['trades'] })
    void queryClient.invalidateQueries({ queryKey: ['product', trade.product.id] })
  }

  const mutation = useMutation({
    mutationFn: async (action: { type: 'accept' | 'confirm' | 'reject' | 'cancel'; reason?: string }) => {
      switch (action.type) {
        case 'accept':
          return acceptTrade(trade.id)
        case 'confirm':
          return confirmTrade(trade.id)
        case 'reject':
          return rejectTrade(trade.id, { reason: action.reason ?? '' })
        case 'cancel':
          return cancelTrade(trade.id, { reason: action.reason ?? '' })
      }
    },
    onSuccess: (_, action) => {
      toast.success(
        action.type === 'accept'
          ? '已接受，商品转为已预留'
          : action.type === 'confirm'
            ? '已确认，等待对方确认'
            : action.type === 'reject'
              ? '已拒绝该购买意向'
              : '已取消交易',
      )
      invalidate()
      setDialog(null)
    },
    onError: (err) => {
      toast.danger(isApiError(err) ? err.message : '操作失败，请稍后重试')
    },
  })

  const chatMutation = useMutation({
    mutationFn: () => getOrCreateProductConversation(trade.product.id),
    onSuccess: (conversation) => navigate(`/chats/${conversation.id}`),
    onError: (err) => {
      toast.danger(isApiError(err) ? err.message : '无法打开会话')
    },
  })

  const isSellerView = as === 'seller'
  const canDecide = isSellerView && trade.status === 'PENDING'
  const canBuyerCancel = !isSellerView && trade.status === 'PENDING'
  const canCollaborate = trade.status === 'ACCEPTED'
  const canConfirm =
    canCollaborate &&
    (isSellerView ? !trade.seller_confirmed : !trade.buyer_confirmed)

  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      {trade.conversation_id ? (
        <Button
          isIconOnly
          aria-label="打开会话"
          isPending={chatMutation.isPending}
          size="sm"
          variant="tertiary"
          onPress={() => navigate(`/chats/${trade.conversation_id}`)}
        >
          <MessageCircle className="size-4" />
        </Button>
      ) : null}

      {canDecide ? (
        <>
          <Button
            isDisabled={mutation.isPending}
            size="sm"
            onPress={() => mutation.mutate({ type: 'accept' })}
          >
            <Check className="size-4" />
            接受
          </Button>
          <Button size="sm" variant="outline" onPress={() => setDialog({ kind: 'reject', trade })}>
            <X className="size-4" />
            拒绝
          </Button>
        </>
      ) : null}

      {canBuyerCancel ? (
        <Button size="sm" variant="outline" onPress={() => setDialog({ kind: 'cancel', trade })}>
          取消意向
        </Button>
      ) : null}

      {canCollaborate ? (
        <Button
          size="sm"
          variant="outline"
          onPress={() => setDialog({ kind: 'cancel', trade })}
        >
          取消交易
        </Button>
      ) : null}

      {canConfirm ? (
        <Button
          isDisabled={mutation.isPending}
          size="sm"
          variant="secondary"
          onPress={() => mutation.mutate({ type: 'confirm' })}
        >
          <CheckCheck className="size-4" />
          确认完成
        </Button>
      ) : null}

      <ReasonDialog
        onClose={() => setDialog(null)}
        state={dialog}
        onSubmit={(reason) => mutation.mutate({ type: dialog?.kind === 'reject' ? 'reject' : 'cancel', reason })}
      />
    </div>
  )
}

function ReasonDialog({
  state,
  onClose,
  onSubmit,
}: {
  state: ReasonDialogState
  onClose: () => void
  onSubmit: (reason: string) => void
}) {
  const [reason, setReason] = useState('')
  const open = state != null
  const isReject = state?.kind === 'reject'

  return (
    <AlertDialog.Backdrop isOpen={open} onOpenChange={(next) => !next && onClose()}>
      <AlertDialog.Container>
        <AlertDialog.Dialog className="sm:max-w-[420px]">
          <AlertDialog.CloseTrigger />
          <AlertDialog.Header>
            <AlertDialog.Icon status={isReject ? 'danger' : 'warning'} />
            <AlertDialog.Heading>
              {isReject ? '拒绝该购买意向？' : '取消该交易？'}
            </AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <p className="mb-3 text-sm text-muted">
              {isReject
                ? '拒绝后该买家对本商品的购买意向将结束（同一买家不会产生第二条意向）。'
                : state?.trade.status === 'ACCEPTED'
                  ? '已接受的交易取消后，预留的商品会恢复为在售。'
                  : '取消后可随时重新发起购买意向。'}
            </p>
            <TextField className="w-full" value={reason} onChange={setReason}>
              <Label>原因（对方可见，必填）</Label>
              <TextArea maxLength={200} placeholder="请填写 1-200 字的原因" rows={3} />
            </TextField>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button
              onPress={() => {
                setReason('')
                onClose()
              }}
              slot="close"
              variant="tertiary"
            >
              返回
            </Button>
            <Button
              isDisabled={reason.trim().length === 0 || reason.trim().length > 200}
              slot="close"
              variant={isReject ? 'danger' : 'primary'}
              onPress={() => {
                onSubmit(reason.trim())
                setReason('')
              }}
            >
              确认{isReject ? '拒绝' : '取消'}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}

/** 买家评价：仅 COMPLETED 交易的买家可见；每笔交易最多一条，重复发布服务端 409。 */
function TradeReviewSection({ trade }: { trade: Trade }) {
  const queryClient = useQueryClient()
  const [composing, setComposing] = useState(false)
  const [score, setScore] = useState<number | null>(null)
  const [content, setContent] = useState('')
  const [review, setReview] = useState<TradeReview | null>(null)

  const mutation = useMutation({
    mutationFn: () =>
      createTradeReview(trade.id, {
        score: score as number,
        // 文字可选：未填写时提交 null，而不是空字符串。
        content: content.trim() ? content.trim() : null,
      }),
    onSuccess: (created) => {
      toast.success('评价已发布')
      setReview(created)
      setComposing(false)
      setContent('')
      void queryClient.invalidateQueries({ queryKey: ['trades'] })
      void queryClient.invalidateQueries({ queryKey: ['product', trade.product.id] })
    },
    onError: (err) => {
      if (isApiError(err)) {
        if (err.code === 'TRADE_REVIEW_ALREADY_EXISTS') {
          toast.warning('该交易已发布过评价')
          setComposing(false)
        } else {
          toast.danger(err.message)
        }
      }
    },
  })

  if (review) {
    return (
      <div className="rounded-xl border border-border-secondary bg-surface-tertiary p-3">
        <p className="flex items-center gap-1.5 text-xs font-medium text-foreground">
          <Star className="size-3.5 text-highlight" />
          我的买家评价 · {review.score} 分
        </p>
        {review.content ? <p className="mt-1 leading-5 text-foreground/90">{review.content}</p> : null}
      </div>
    )
  }

  if (!composing) {
    return (
      <Button className="self-start" size="sm" variant="outline" onPress={() => setComposing(true)}>
        <Star className="size-4" />
        写买家评价
      </Button>
    )
  }

  return (
    <div className="flex flex-col gap-2 rounded-xl border border-border-secondary bg-surface-tertiary p-3">
      <div className="flex items-center gap-1.5">
        <span className="text-xs font-medium text-foreground">评分</span>
        {[1, 2, 3, 4, 5].map((value) => (
          <Button
            key={value}
            aria-label={`${value} 分`}
            className="size-8"
            isIconOnly
            size="sm"
            variant={score === value ? 'primary' : 'outline'}
            onPress={() => setScore(value)}
          >
            <Star className="size-3.5" />
            {value}
          </Button>
        ))}
      </div>
      <TextField className="w-full" value={content} onChange={setContent}>
        <Label>买家评价（公开展示，发布后不可修改删除，文字可选）</Label>
        <TextArea maxLength={500} placeholder="1-500 字，说说这次交易体验（可选）" rows={3} />
      </TextField>
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted">{content.length}/500</span>
        <div className="flex gap-2">
          <Button size="sm" variant="tertiary" onPress={() => setComposing(false)}>
            取消
          </Button>
          <Button
            isDisabled={score == null}
            isPending={mutation.isPending}
            size="sm"
            onPress={() => mutation.mutate()}
          >
            发布评价
          </Button>
        </div>
      </div>
    </div>
  )
}

function TradeCard({ trade, as }: { trade: Trade; as: TradeRole }) {
  const me = useAuthStore((state) => state.user)
  const counterpart = as === 'buyer' ? trade.seller : trade.buyer
  const asLabel = as === 'buyer' ? '卖家' : '买家'

  return (
    <Card className="card-interactive flex flex-col gap-3 p-4">
      <div className="flex flex-wrap items-center gap-3">
        <Link
          className="h-16 w-16 shrink-0 overflow-hidden rounded-xl bg-surface"
          to={`/products/${trade.product.id}`}
        >
          {trade.product.cover_url ? (
            <img
              alt={trade.product.title}
              className="h-full w-full object-cover"
              loading="lazy"
              src={trade.product.cover_url}
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-muted">
              <Store className="size-5 opacity-50" />
            </div>
          )}
        </Link>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <Link
            className="line-clamp-1 text-sm font-medium text-foreground hover:underline"
            to={`/products/${trade.product.id}`}
          >
            {trade.product.title}
          </Link>
          <span className="tabular-nums text-base font-bold text-highlight">
            {formatPrice(trade.price_snapshot)}
            <span className="ms-1 text-xs font-normal text-muted">成交快照价</span>
          </span>
        </div>
        <TradeStatusChip status={trade.status} />
      </div>

      <div className="flex flex-wrap items-center gap-x-6 gap-y-1 text-xs text-muted">
        <span className="flex items-center gap-1.5">
          <Avatar className="size-5">
            <Avatar.Fallback>{nicknameInitial(counterpart.nickname)}</Avatar.Fallback>
          </Avatar>
          {asLabel}：
          <Link
            className="font-medium text-foreground underline-offset-4 hover:text-accent hover:underline"
            to={`/users/${counterpart.id}`}
          >
            {counterpart.nickname}
          </Link>
        </span>
        <span>微信 {counterpart.wechat ?? '未填写'}</span>
        <span>QQ {counterpart.qq ?? '未填写'}</span>
      </div>

      <TradeTimeline trade={trade} />

      {trade.status === 'ACCEPTED' ? (
        <div className="flex flex-wrap gap-2 text-xs">
          <span className={trade.buyer_confirmed ? 'text-success' : 'text-muted'}>
            {trade.buyer.id === me?.id ? '我（买家）' : '买家'}{trade.buyer_confirmed ? '已确认' : '未确认'}
          </span>
          <span className={trade.seller_confirmed ? 'text-success' : 'text-muted'}>
            {trade.seller.id === me?.id ? '我（卖家）' : '卖家'}{trade.seller_confirmed ? '已确认' : '未确认'}
          </span>
        </div>
      ) : null}

      {trade.cancel_reason ? (
        <p className="rounded-xl border border-border-secondary bg-surface-tertiary p-2.5 text-xs text-muted">
          取消原因：{trade.cancel_reason}
        </p>
      ) : null}

      {as === 'buyer' && trade.status === 'COMPLETED' ? <TradeReviewSection trade={trade} /> : null}

      {trade.status === 'PENDING' || trade.status === 'ACCEPTED' ? (
        <TradeActions as={as} trade={trade} />
      ) : null}
    </Card>
  )
}

export function TradesPage() {
  const location = useLocation()
  const initialAs = (location.state as { as?: TradeRole } | null)?.as === 'seller' ? 'seller' : 'buyer'

  const [as, setAs] = useState<TradeRole>(initialAs)
  const [status, setStatus] = useState<TradeStatus | 'ALL'>('ALL')
  const [page, setPage] = useState(1)

  const { data, isPending } = useQuery({
    queryKey: ['trades', { as, status, page }],
    queryFn: () =>
      listTrades({
        as,
        status: status === 'ALL' ? undefined : status,
        page,
        page_size: PAGE_SIZE,
      }),
    placeholderData: keepPreviousData,
    refetchInterval: 8_000,
    // 交易状态由对方操作推进,切回页面必须拉新,不吃缓存
    staleTime: 0,
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <h1 className="text-lg font-bold">交易</h1>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="flex gap-2">
          {(
            [
              { value: 'buyer', label: '我买到的' },
              { value: 'seller', label: '我卖出的' },
            ] as const
          ).map((tab) => (
            <Button
              key={tab.value}
              size="sm"
              variant={as === tab.value ? 'primary' : 'tertiary'}
              onPress={() => {
                setAs(tab.value)
                setPage(1)
              }}
            >
              {tab.label}
            </Button>
          ))}
        </div>

        <Select
          className="sm:w-40"
          value={status}
          onChange={(key) => {
            setStatus(key as TradeStatus | 'ALL')
            setPage(1)
          }}
        >
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              <ListBox.Item id="ALL" textValue="全部">
                全部
                <ListBox.ItemIndicator />
              </ListBox.Item>
              {TRADE_STATUS_OPTIONS.map((option) => (
                <ListBox.Item id={option.value} key={option.value} textValue={option.label}>
                  {option.label}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </div>

      {isPending ? (
        <div className="flex h-40 items-center justify-center">
          <LissajousLoader className="size-28 text-foreground" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          actionLabel="去逛逛市场"
          actionTo="/"
          description="在商品详情页点击“我想买”即可发起购买意向"
          icon={<HandshakeDealIcon className="h-14 w-auto" />}
          title={as === 'buyer' ? '还没有购买记录' : '还没有卖出记录'}
        />
      ) : (
        <>
          <div className="flex flex-col gap-3">
            {data.items.map((trade, index) => (
              <motion.div
                animate={{ opacity: 1, y: 0 }}
                initial={{ opacity: 0, y: 10 }}
                key={trade.id}
                transition={{ delay: Math.min(index * 0.04, 0.24), duration: 0.25 }}
              >
                <TradeCard as={as} trade={trade} />
              </motion.div>
            ))}
          </div>

          {totalPages > 1 ? (
            <Pagination className="justify-center">
              <Pagination.Content>
                <Pagination.Item>
                  <Pagination.Previous isDisabled={page === 1} onPress={() => setPage((p) => p - 1)}>
                    <Pagination.PreviousIcon />
                  </Pagination.Previous>
                </Pagination.Item>
                {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
                  <Pagination.Item key={p}>
                    <Pagination.Link isActive={p === page} onPress={() => setPage(p)}>
                      {p}
                    </Pagination.Link>
                  </Pagination.Item>
                ))}
                <Pagination.Item>
                  <Pagination.Next isDisabled={page === totalPages} onPress={() => setPage((p) => p + 1)}>
                    <Pagination.NextIcon />
                  </Pagination.Next>
                </Pagination.Item>
              </Pagination.Content>
            </Pagination>
          ) : null}
        </>
      )}
    </div>
  )
}
