import { API_ENDPOINTS } from './endpoints'
import { http } from './client'
import type { UserProfile } from '@/stores/authStore'

export interface LoginResponse {
  ok: boolean
  user_id: number | string
  token: string
  first_name?: string | null
  username?: string | null
  profile_url?: string | null
  photo_url?: string | null
}

function toProfile(data: LoginResponse): UserProfile {
  const pfp = data.profile_url || data.photo_url || null
  return {
    id: String(data.user_id),
    name: data.first_name || data.username || 'User',
    username: data.username || undefined,
    avatarUrl: pfp,
    profile_url: data.profile_url || null,
    photo_url: data.photo_url || null,
  }
}

export async function loginUser(req: { username: string; password: string }): Promise<{ user: UserProfile; token: string }> {
  const data = await http.post<LoginResponse>(API_ENDPOINTS.AUTH_LOGIN, req, { anonymous: true })
  if (!data?.ok || !data.token) throw new Error('Invalid username or password')
  return { user: toProfile(data), token: data.token }
}

export async function registerUser(req: { userid: number; username: string; password: string }) {
  return http.post<{ ok: boolean; message: string; user_id: number }>(API_ENDPOINTS.AUTH_REGISTER, req, { anonymous: true })
}

export async function validateOtp(req: { userid: number; otp: string }): Promise<{ user: UserProfile; token: string }> {
  const data = await http.post<LoginResponse>(API_ENDPOINTS.AUTH_VALIDATE, req, { anonymous: true })
  if (!data?.ok || !data.token) throw new Error('Verification failed')
  return { user: toProfile(data), token: data.token }
}

/** Server / owner password → guest API token (also sets the auth cookie) */
export async function loginWithServerPassword(password: string): Promise<{ token: string }> {
  const data = await http.post<{ ok: boolean; token: string }>(`${API_ENDPOINTS.AUTH_PASSWORD}?set_cookie=true`, { password: password.trim() }, { anonymous: true })
  if (!data?.ok || !data.token) throw new Error('Incorrect server password')
  return { token: data.token }
}

export interface SetupStatus {
  ok: boolean
  configured: boolean
  needs_setup: boolean
  owner_id?: number
}

export async function fetchSetupStatus(baseUrl?: string): Promise<SetupStatus> {
  return http.get<SetupStatus>(API_ENDPOINTS.AUTH_SETUP_STATUS, { anonymous: true, baseUrl, timeoutMs: 8000 })
}

export async function setupOwnerPassword(password: string): Promise<{ token: string }> {
  const data = await http.post<{ ok: boolean; token: string }>(API_ENDPOINTS.AUTH_SETUP, { password }, { anonymous: true })
  if (!data?.ok || !data.token) throw new Error('Setup failed')
  return { token: data.token }
}

export async function changeOwnerPassword(password: string): Promise<void> {
  await http.post(API_ENDPOINTS.AUTH_PASSWORD_CHANGE, { password })
}

export async function setCredentials(username: string, password: string): Promise<void> {
  await http.post(API_ENDPOINTS.AUTH_CREDENTIALS, { username, password })
}

export interface MeResponse {
  ok: boolean
  guest?: boolean
  user_id?: number
  user?: {
    _id?: number
    username?: string
    first_name?: string
    profile_url?: string | null
    photo_url?: string | null
    created_at?: number
  } | null
}

export async function fetchMe(signal?: AbortSignal): Promise<MeResponse> {
  return http.get<MeResponse>(API_ENDPOINTS.AUTH_ME, { signal, timeoutMs: 8000 })
}

export async function logoutServer(): Promise<void> {
  try {
    await http.post(API_ENDPOINTS.AUTH_LOGOUT, undefined, { timeoutMs: 4000 })
  } catch {
    /* best effort */
  }
}
