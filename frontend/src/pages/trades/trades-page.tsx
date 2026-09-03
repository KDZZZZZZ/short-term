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
  Spinner,
  TextArea,
  TextField,
  toast,
} from '@heroui/react'
import {
  Check,
  CheckCheck,
  Handshake,
  MessageCircle,
  Store,
  X,
} from 'lucide-react'
import {
  acceptTrade,
  cancelTrade,
  confirmTrade,
  listTrades,
  rejectTrade,
} from '@/lib/api/trades'
import { getOrCreateProductConversation } from '@/lib/api/conversations'
import { isApiError } from '@/lib/http'
import { formatDateTime, formatPrice, nicknameInitial, TRADE_STATUS_OPTIONS } from '@/lib/format'
import type { Trade, TradeRole, TradeStatus } from '@/lib/types'
import { EmptyState } from '@/components/empty-state'
import { TradeStatusChip } from '@/components/status-chip'
import { useAuthStore } from '@/stores/auth-store'

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

function TradeCard({ trade, as }: { trade: Trade; as: TradeRole }) {
  const me = useAuthStore((state) => state.user)
  const counterpart = as === 'buyer' ? trade.seller : trade.buyer
  const asLabel = as === 'buyer' ? '卖家' : '买家'

  return (
    <Card className="flex flex-col gap-3 p-4">
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
          <span className="tabular-nums text-base font-bold text-accent">
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
          {asLabel}：{counterpart.nickname}
        </span>
        <span>创建于 {formatDateTime(trade.created_at)}</span>
        {trade.accepted_at ? <span>接受于 {formatDateTime(trade.accepted_at)}</span> : null}
        {trade.completed_at ? <span>完成于 {formatDateTime(trade.completed_at)}</span> : null}
        {trade.cancelled_at ? <span>取消于 {formatDateTime(trade.cancelled_at)}</span> : null}
      </div>

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
        <p className="rounded-xl bg-surface p-2.5 text-xs text-muted">取消原因：{trade.cancel_reason}</p>
      ) : null}

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
          placeholder="全部状态"
          value={status === 'ALL' ? null : status}
          onChange={(key) => {
            setStatus((key as TradeStatus | null) ?? 'ALL')
            setPage(1)
          }}
        >
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
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
          <Spinner size="lg" />
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          actionLabel="去逛逛市场"
          actionTo="/"
          description="在商品详情页点击“我想买”即可发起购买意向"
          icon={<Handshake className="size-10" />}
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
