import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { AuthData, UserMe } from '@/lib/types'

interface AuthState {
  token: string | null
  /** Unix 毫秒；来自登录响应 expires_in。 */
  expiresAt: number | null
  user: UserMe | null
  setAuth: (data: AuthData) => void
  setUser: (user: UserMe) => void
  clear: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      expiresAt: null,
      user: null,
      setAuth: (data) =>
        set({
          token: data.access_token,
          expiresAt: Date.now() + data.expires_in * 1000,
          user: data.user,
        }),
      setUser: (user) => set({ user }),
      clear: () => set({ token: null, expiresAt: null, user: null }),
    }),
    { name: 'st-auth' },
  ),
)

/** 本地预判会话是否仍然有效（服务端 401 仍是最终裁决）。 */
export function isSessionAlive(state: Pick<AuthState, 'token' | 'expiresAt'>): boolean {
  if (!state.token) return false
  if (state.expiresAt === null) return true
  return Date.now() < state.expiresAt - 30_000
}
