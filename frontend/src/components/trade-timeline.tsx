/**
 * 交易生命周期时间线（结构借鉴 Coss UI Timeline，用 HeroUI token 轻量实现）。
 * 创建 → 卖家接受 → 双方确认完成；已取消时显示取消节点与原因。
 */

import type { Trade } from '@/lib/types'
import { formatDateTime } from '@/lib/format'

export function TradeTimeline({ trade }: { trade: Trade }) {
  const steps: Array<{ label: string; at: string | null; done: boolean; danger?: boolean }> =
    trade.status === 'CANCELLED'
      ? [
          { label: '创建意向', at: trade.created_at, done: true },
          { label: '已取消', at: trade.cancelled_at, done: true, danger: true },
        ]
      : [
          { label: '创建意向', at: trade.created_at, done: true },
          { label: '卖家接受', at: trade.accepted_at, done: Boolean(trade.accepted_at) },
          { label: '双方确认', at: trade.completed_at, done: Boolean(trade.completed_at) },
        ]

  return (
    <ol className="flex w-full items-start" data-slot="trade-timeline">
      {steps.map((step, index) => (
        <li className="flex min-w-0 flex-1 flex-col items-start gap-1" key={step.label}>
          <div className="flex w-full items-center">
            <span
              className={`size-2.5 shrink-0 rounded-full border-2 ${
                step.danger
                  ? 'border-danger bg-danger'
                  : step.done
                    ? 'border-highlight bg-highlight'
                    : 'border-border bg-background'
              }`}
            />
            {index < steps.length - 1 ? (
              <span
                className={`h-px flex-1 ${steps[index + 1]?.done ? 'bg-highlight' : 'bg-border'}`}
              />
            ) : null}
          </div>
          <span className={`text-xs ${step.done ? 'font-medium text-foreground' : 'text-muted'}`}>
            {step.label}
          </span>
          {step.at ? <span className="text-[10px] text-muted">{formatDateTime(step.at)}</span> : null}
        </li>
      ))}
    </ol>
  )
}
