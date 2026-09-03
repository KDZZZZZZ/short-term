import type { ReactNode } from 'react'
import { Button } from '@heroui/react'
import { useNavigate } from 'react-router-dom'

interface EmptyStateProps {
  icon: ReactNode
  title: string
  description?: string
  actionLabel?: string
  actionTo?: string
}

export function EmptyState({ icon, title, description, actionLabel, actionTo }: EmptyStateProps) {
  const navigate = useNavigate()
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-border-secondary py-16 text-center">
      <div className="text-muted opacity-60">{icon}</div>
      <p className="text-base font-medium text-foreground">{title}</p>
      {description ? <p className="max-w-sm text-sm text-muted">{description}</p> : null}
      {actionLabel && actionTo ? (
        <Button
          className="mt-1"
          size="sm"
          variant="outline"
          onPress={() => navigate(actionTo)}
        >
          {actionLabel}
        </Button>
      ) : null}
    </div>
  )
}
