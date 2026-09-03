import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { isSessionAlive, useAuthStore } from '@/stores/auth-store'

/** 所有业务接口都需要 Bearer Token，未登录一律重定向到登录页。 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { token, expiresAt } = useAuthStore()
  if (!isSessionAlive({ token, expiresAt })) {
    const from = encodeURIComponent(window.location.pathname + window.location.search)
    return <Navigate to={`/login?from=${from}`} replace />
  }
  return <>{children}</>
}
