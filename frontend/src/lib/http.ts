import type { ErrorResponse } from '@/lib/types'
import { useAuthStore } from '@/stores/auth-store'

export const API_BASE = '/api/v1'

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details: Record<string, unknown> | null

  constructor(status: number, body: Partial<ErrorResponse> | null, fallback: string) {
    super(body?.message?.trim() || fallback)
    this.name = 'ApiError'
    this.status = status
    this.code = body?.code ?? 'INTERNAL_ERROR'
    this.details = body?.details ?? null
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  /** 相对 /api/v1 的路径，如 "/products"。 */
  path: string
  query?: object
  json?: unknown
  formData?: FormData
  idempotencyKey?: string
  /** 401 时不触发全局登出（登录、注册等公开接口使用）。 */
  public?: boolean
}

export function buildQuery(query?: object): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === undefined || value === null || value === '') continue
    if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') continue
    search.set(key, String(value))
  }
  const encoded = search.toString()
  return encoded ? `?${encoded}` : ''
}

/**
 * 401 统一处理：清除本地会话并跳转登录页。
 * 登录/注册本身的 401 属于业务错误（密码错误），不触发跳转。
 */
function handleUnauthorized() {
  useAuthStore.getState().clear()
  const current = window.location.pathname + window.location.search
  const target = current.startsWith('/login') ? '/login' : `/login?expired=1&from=${encodeURIComponent(current)}`
  if (window.location.pathname !== '/login') {
    window.location.assign(target)
  }
}

export async function apiFetch<T>(options: RequestOptions): Promise<T> {
  const { method = 'GET', path, query, json, formData, idempotencyKey, public: isPublic } = options

  const headers = new Headers()
  const token = useAuthStore.getState().token
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (json !== undefined) {
    headers.set('Content-Type', 'application/json')
  }
  if (idempotencyKey) {
    headers.set('Idempotency-Key', idempotencyKey)
  }

  let body: BodyInit | undefined
  if (formData) {
    body = formData // 浏览器自动设置 multipart boundary
  } else if (json !== undefined) {
    body = JSON.stringify(json)
  }

  const response = await fetch(`${API_BASE}${path}${buildQuery(query)}`, {
    method,
    headers,
    body,
  })

  let payload: unknown = null
  const text = await response.text()
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }

  if (!response.ok) {
    if (response.status === 401 && !isPublic) {
      handleUnauthorized()
    }
    throw new ApiError(response.status, payload as Partial<ErrorResponse> | null, '请求失败，请稍后重试')
  }

  const envelope = payload as { code?: string; data?: T } | null
  if (envelope && envelope.code === 'OK') {
    return envelope.data as T
  }
  throw new ApiError(response.status, payload as Partial<ErrorResponse> | null, '响应格式异常')
}

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2)}-${Math.random().toString(36).slice(2)}`
}
