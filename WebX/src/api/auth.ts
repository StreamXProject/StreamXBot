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

export interface DiscordIntegrationData {
  enabled?: boolean
  token?: string
  mode?: 'gateway' | 'daemon'
  client_id?: string
  daemon_url?: string
  show_artwork?: boolean
}

export interface LastfmIntegrationData {
  enabled?: boolean
  api_key?: string
  api_secret?: string
  session_key?: string
  username?: string
  scrobble_at?: number
  now_playing?: boolean
}

export interface UserIntegrations {
  discord?: DiscordIntegrationData
  lastfm?: LastfmIntegrationData
}

export interface MeResponse {
  ok: boolean
  guest?: boolean
  user_id?: number
  user?: {
    _id?: number
    id?: number
    user_id?: number
    username?: string
    first_name?: string
    profile_url?: string | null
    photo_url?: string | null
    integrations?: UserIntegrations
    created_at?: number
  } | null
}

export async function fetchMe(signal?: AbortSignal): Promise<MeResponse> {
  return http.get<MeResponse>(API_ENDPOINTS.AUTH_ME, { signal, timeoutMs: 8000 })
}

export async function updateUserIntegrations(payload: UserIntegrations): Promise<{ ok: boolean; integrations?: UserIntegrations }> {
  return http.post<{ ok: boolean; integrations?: UserIntegrations }>(API_ENDPOINTS.AUTH_INTEGRATIONS, payload, { timeoutMs: 8000 })
}

export async function logoutServer(): Promise<void> {
  try {
    await http.post(API_ENDPOINTS.AUTH_LOGOUT, undefined, { timeoutMs: 4000 })
  } catch {}
}

export interface TelegramConfig {
  ok: boolean
  enabled: boolean
  client_id?: string
  bot_username?: string
}

export async function fetchTelegramConfig(baseUrl?: string): Promise<TelegramConfig> {
  return http.get<TelegramConfig>(API_ENDPOINTS.AUTH_TELEGRAM_CONFIG, { anonymous: true, baseUrl, timeoutMs: 6000 })
}

export async function loginWithTelegramWidget(data: {
  id: number
  first_name?: string
  last_name?: string
  username?: string
  photo_url?: string
  auth_date: number
  hash: string
}): Promise<{ user: UserProfile; token: string }> {
  const res = await http.post<LoginResponse>(API_ENDPOINTS.AUTH_TELEGRAM_WIDGET, data, { anonymous: true })
  if (!res?.ok || !res.token) throw new Error('Telegram login failed')
  return { user: toProfile(res), token: res.token }
}

export async function loginWithTelegramToken(id_token: string): Promise<{ user: UserProfile; token: string }> {
  const res = await http.post<LoginResponse>(API_ENDPOINTS.AUTH_TELEGRAM_VALIDATE_TOKEN, { id_token }, { anonymous: true })
  if (!res?.ok || !res.token) throw new Error('Telegram login failed')
  return { user: toProfile(res), token: res.token }
}

