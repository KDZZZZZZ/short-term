import { Button } from '@heroui/react'
import { Compass } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

export function NotFoundPage() {
  const navigate = useNavigate()
  return (
    <div className="flex h-64 flex-col items-center justify-center gap-3">
      <Compass className="size-10 text-muted" />
      <p className="text-lg font-semibold">页面不存在</p>
      <p className="text-sm text-muted">你访问的页面可能已被移除或链接有误</p>
      <Button variant="outline" onPress={() => navigate('/')}>
        回到市场
      </Button>
    </div>
  )
}
