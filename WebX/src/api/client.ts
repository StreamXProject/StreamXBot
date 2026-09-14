/**
 * HTTP client for the StreamX API.
 *
 * - Base URL comes from settings (normalised, no trailing slash)
 * - Auth token is sent as both `Authorization: Bearer` and `X-Auth-Token`
 *   (the backend accepts either; cookies are included for cookie sessions)
 * - Identical in-flight GETs are de-duplicated
 * - 401 on an authenticated request emits `webx:unauthorized` so the auth
 *   store can drop the stale session
 */
import { useSettingsStore } from '@/stores/settingsStore'
import { useAuthStore } from '@/stores/authStore'

export type HttpMethod = 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'

export interface RequestOptions {
  params?: Record<string, string | number | boolean | null | undefined>
  body?: unknown
  headers?: Record<string, string>
  timeoutMs?: number
  signal?: AbortSignal
  /** 'json' (default) | 'text' | 'response' */
  parse?: 'json' | 'text' | 'response'
  /** Skip sending auth headers */
  anonymous?: boolean
  /** Override base URL for this request (used by "test connection") */
  baseUrl?: string
  /** Disable GET de-duplication */
  noDedupe?: boolean
}

export class ApiError extends Error {
  status: number
  body: unknown
  url: string
  constructor(status: number, message: string, url: string, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
    this.url = url
  }
  get isAuth() {
    return this.status === 401 || this.status === 403
  }
  get isNetwork() {
    return this.status === 0
  }
}

export function normalizeBaseUrl(raw: string | null | undefined): string {
  if (!raw) return ''
  let s = raw.trim()
  if (!s) return ''
  if (!/^https?:\/\//i.test(s)) {
    const isLocal = /^(localhost|127\.0\.0\.1|0\.0\.0\.0)(:\d+)?$/i.test(s)
    const scheme = typeof window !== 'undefined' && window.location?.protocol === 'https:' && !isLocal ? 'https' : 'http'
    s = `${scheme}://${s}`
  }
  return s.replace(/\/+$/, '')
}

export function getBaseUrl(): string {
  const configured = normalizeBaseUrl(useSettingsStore.getState().apiBaseUrl)
  if (typeof window !== 'undefined' && window.location?.origin) {
    const origin = window.location.origin
    if (origin.startsWith('https://') && configured.startsWith('http://localhost')) {
      return origin
    }
  }
  return configured || (typeof window !== 'undefined' ? window.location.origin : '')
}

export function getToken(): string | null {
  return useAuthStore.getState().token
}

export function authHeaders(token: string | null = getToken()): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}`, 'X-Auth-Token': token } : {}
}

export function buildUrl(path: string, params?: RequestOptions['params'], baseUrl = getBaseUrl()): string {
  let url = /^https?:\/\//i.test(path) ? path : `${baseUrl}${path.startsWith('/') ? '' : '/'}${path}`
  if (params) {
    const q = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue
      q.set(k, String(v))
    }
    const qs = q.toString()
    if (qs) url += (url.includes('?') ? '&' : '?') + qs
  }
  return url
}

const inflight = new Map<string, Promise<unknown>>()

async function readBody(res: Response, parse: RequestOptions['parse']) {
  if (parse === 'response') return res
  const text = await res.text()
  if (parse === 'text') return text
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function errorMessage(status: number, body: unknown, statusText: string): string {
  if (body && typeof body === 'object') {
    const b = body as Record<string, unknown>
    const d = b.detail ?? b.message ?? b.error
    if (typeof d === 'string') return d
    if (Array.isArray(d) && d[0] && typeof d[0] === 'object' && 'msg' in (d[0] as object)) return String((d[0] as { msg: string }).msg)
  }
  if (typeof body === 'string' && body.length < 200) return body
  switch (status) {
    case 401: return 'Not authenticated'
    case 403: return 'Not allowed'
    case 404: return 'Not found'
    case 429: return 'Too many requests'
    case 500: return 'Server error'
    case 502:
    case 503:
    case 504: return 'Server unavailable'
    default: return statusText || `HTTP ${status}`
  }
}

export async function request<T = unknown>(method: HttpMethod, path: string, opts: RequestOptions = {}): Promise<T> {
  const base = opts.baseUrl ? normalizeBaseUrl(opts.baseUrl) : getBaseUrl()
  const url = buildUrl(path, opts.params, base)
  const token = opts.anonymous ? null : getToken()
  const key = method === 'GET' && !opts.noDedupe && !opts.signal ? `${url}|${token ?? ''}|${opts.parse ?? 'json'}` : null
  if (key) {
    const existing = inflight.get(key)
    if (existing) return existing as Promise<T>
  }

  const controller = new AbortController()
  const timeout = opts.timeoutMs ?? (method === 'GET' ? 12000 : 20000)
  const timer = setTimeout(() => controller.abort(new DOMException('Request timed out', 'TimeoutError')), timeout)
  if (opts.signal) {
    if (opts.signal.aborted) controller.abort(opts.signal.reason)
    else opts.signal.addEventListener('abort', () => controller.abort(opts.signal!.reason), { once: true })
  }

  const exec = (async () => {
    try {
      const res = await fetch(url, {
        method,
        credentials: 'include',
        signal: controller.signal,
        headers: {
          Accept: 'application/json, text/plain;q=0.9, */*;q=0.8',
          ...(opts.body !== undefined ? { 'Content-Type': 'application/json' } : {}),
          ...authHeaders(token),
          ...(opts.headers ?? {}),
        },
        body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      })
      if (!res.ok) {
        const body = await readBody(res, 'json').catch(() => null)
        if (res.status === 401 && token && typeof window !== 'undefined') {
          window.dispatchEvent(new CustomEvent('webx:unauthorized', { detail: { url } }))
        }
        // Account locked / chat membership required → the shell shows a dedicated blocked screen
        if (res.status === 403 && token && typeof window !== 'undefined' && body && typeof body === 'object') {
          const raw = body as { detail?: unknown }
          const inner = (typeof raw.detail === 'object' && raw.detail !== null ? raw.detail : raw) as { detail?: unknown }
          if (inner.detail === 'account_locked' || inner.detail === 'membership_required') {
            window.dispatchEvent(new CustomEvent('webx:access-denied', { detail: inner }))
          }
        }
        throw new ApiError(res.status, errorMessage(res.status, body, res.statusText), url, body)
      }
      return (await readBody(res, opts.parse ?? 'json')) as T
    } catch (err) {
      if (err instanceof ApiError) throw err
      const e = err as Error
      if (e?.name === 'AbortError' || e?.name === 'TimeoutError') {
        throw new ApiError(0, e.name === 'TimeoutError' || controller.signal.reason?.name === 'TimeoutError' ? 'Request timed out' : 'Request cancelled', url)
      }
      if (typeof window !== 'undefined' && window.location?.protocol === 'https:' && url.startsWith('http://')) {
        throw new ApiError(0, 'Mixed content blocked: HTTPS websites cannot connect to HTTP servers directly. Use an HTTPS or Cloudflare tunnel URL.', url)
      }
      throw new ApiError(0, 'Cannot reach server', url)
    } finally {
      clearTimeout(timer)
      if (key) inflight.delete(key)
    }
  })()

  if (key) inflight.set(key, exec)
  return exec
}

export const http = {
  get: <T,>(path: string, opts?: RequestOptions) => request<T>('GET', path, opts),
  post: <T,>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('POST', path, { ...opts, body }),
  patch: <T,>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PATCH', path, { ...opts, body }),
  put: <T,>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PUT', path, { ...opts, body }),
  delete: <T,>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, opts),
}

/** Backwards-compatible facade used by older modules */
export const ApiClient = {
  get: <T,>(path: string, options: { params?: RequestOptions['params']; signal?: AbortSignal; timeoutMs?: number } = {}) =>
    http.get<T>(path, options),
  post: <T,>(path: string, body?: unknown, options: { signal?: AbortSignal; timeoutMs?: number } = {}) =>
    http.post<T>(path, body, options),
}
