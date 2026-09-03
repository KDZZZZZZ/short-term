import { useAuthStore } from '@/stores/auth-store'
import { apiFetch } from '@/lib/http'
import type { AuthData, ChangePasswordRequest, LoginRequest, RegisterRequest, UpdateProfileRequest, UserMe } from '@/lib/types'

export function registerUser(body: RegisterRequest): Promise<AuthData> {
  return apiFetch<AuthData>({ method: 'POST', path: '/auth/register', json: body, public: true })
}

export function loginUser(body: LoginRequest): Promise<AuthData> {
  return apiFetch<AuthData>({ method: 'POST', path: '/auth/login', json: body, public: true })
}

export function logoutUser(): Promise<Record<string, never>> {
  return apiFetch({ method: 'POST', path: '/auth/logout' })
}

export function getCurrentUser(): Promise<UserMe> {
  return apiFetch<UserMe>({ path: '/users/me' })
}

export function updateCurrentUser(body: UpdateProfileRequest): Promise<UserMe> {
  return apiFetch<UserMe>({ method: 'PATCH', path: '/users/me', json: body })
}

export function changeCurrentUserPassword(body: ChangePasswordRequest): Promise<Record<string, never>> {
  return apiFetch({ method: 'PUT', path: '/users/me/password', json: body })
}

/** 注册/登录成功后写入会话。 */
export function persistAuth(data: AuthData) {
  useAuthStore.getState().setAuth(data)
}
