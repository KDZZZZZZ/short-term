import type { ReactElement } from 'react'
import { Navigate } from 'react-router-dom'
import { isSessionAlive, useAuthStore } from '@/stores/auth-store'

/** 已登录用户访问登录/注册页时重定向回市场。 */
export function GuestOnly({ children }: { children: ReactElement }) {
  const { token, expiresAt } = useAuthStore()
  if (isSessionAlive({ token, expiresAt })) {
    return <Navigate to="/" replace />
  }
  return children
}
