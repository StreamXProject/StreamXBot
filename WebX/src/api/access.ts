/** Access control — public policy for signup/login, membership re-verification, and admin management. */
import { API_ENDPOINTS } from './endpoints'
import { http, ApiError } from './client'

export type RegistrationMode = 'open' | 'invite' | 'allowlist' | 'closed'
export type UserAccessStatus = 'active' | 'locked' | 'restricted'
export interface RequiredChat { chat_id?: number; title: string | null; invite_link: string | null; is_private?: boolean }

export interface AccessStatus { registration_mode: RegistrationMode; enforce_membership: boolean; required_chats: RequiredChat[] }

export interface AccessDeniedBody {
  ok: false
  detail: 'account_locked' | 'membership_required' | 'session_revoked' | 'registration_closed' | 'not_allowlisted' | 'invite_required' | 'invite_invalid' | 'invite_expired' | 'invite_exhausted' | string
  message?: string
  reason?: string | null
  required_chats?: RequiredChat[]
}

/** Extract a structured access-denied body from an ApiError (or null). */
export function accessDenied(err: unknown): AccessDeniedBody | null {
  if (!(err instanceof ApiError)) return null
  const b = err.body as Partial<AccessDeniedBody> | { detail?: Partial<AccessDeniedBody> | string } | null
  if (!b || typeof b !== 'object') return null
  // FastAPI wraps HTTPException(detail=dict) as { detail: {...} }; the middleware returns the dict directly
  const inner = typeof (b as { detail?: unknown }).detail === 'object' && (b as { detail?: unknown }).detail !== null ? ((b as { detail: Partial<AccessDeniedBody> }).detail) : (b as Partial<AccessDeniedBody>)
  if (!inner || typeof inner.detail !== 'string') return null
  return { ok: false, detail: inner.detail, message: inner.message, reason: inner.reason ?? null, required_chats: inner.required_chats ?? [] }
}

export const ACCESS_CODES = new Set(['account_locked', 'membership_required', 'session_revoked', 'registration_closed', 'not_allowlisted', 'invite_required', 'invite_invalid', 'invite_expired', 'invite_exhausted'])

export async function fetchAccessStatus(signal?: AbortSignal): Promise<AccessStatus> {
  const d = await http.get<AccessStatus & { ok: boolean }>(API_ENDPOINTS.AUTH_ACCESS_STATUS, { anonymous: true, signal, timeoutMs: 8000 })
  return { registration_mode: d.registration_mode ?? 'open', enforce_membership: Boolean(d.enforce_membership), required_chats: d.required_chats ?? [] }
}

export async function verifyMembership(): Promise<{ ok: boolean; status: UserAccessStatus; required_chats: RequiredChat[] }> {
  return http.post(API_ENDPOINTS.AUTH_VERIFY_MEMBERSHIP, {}, { timeoutMs: 15000, noDedupe: true })
}

/* ---------------- admin ---------------- */

export interface AccessPolicy { registration_mode: RegistrationMode; enforce_membership: boolean; required_chats: Array<{ chat_id: number; title: string | null; invite_link: string | null; is_private?: boolean }>; lock_message: string; updated_at: number }
export interface InviteCode { code: string; created_by: number; created_at: number; max_uses: number; uses: number; used_by: number[]; expires_at: number | null; revoked_at: number | null; note: string | null }
export interface ManagedUser { user_id: number; username: string | null; first_name: string | null; profile_url: string | null; telegram_username: string | null; status: UserAccessStatus; lock_reason: string | null; locked_at: number | null; token_version: number; created_at: number | null }

export const adminAccess = {
  policy: async () => (await http.get<{ policy: AccessPolicy }>(API_ENDPOINTS.ADMIN_ACCESS_POLICY, { noDedupe: true })).policy,
  patchPolicy: async (patch: Partial<Pick<AccessPolicy, 'registration_mode' | 'enforce_membership' | 'lock_message'>>) => (await http.patch<{ policy: AccessPolicy }>(API_ENDPOINTS.ADMIN_ACCESS_POLICY, patch)).policy,
  addRequiredChat: async (chat_id: number, title?: string, invite_link?: string, is_private?: boolean) => (await http.post<{ policy: AccessPolicy }>(API_ENDPOINTS.ADMIN_ACCESS_REQUIRED_CHATS, { chat_id, title, invite_link, is_private })).policy,
  removeRequiredChat: async (chat_id: number) => (await http.delete<{ policy: AccessPolicy }>(API_ENDPOINTS.ADMIN_ACCESS_REQUIRED_CHAT(chat_id))).policy,
  invites: async (includeDead = false) => (await http.get<{ items: InviteCode[] }>(API_ENDPOINTS.ADMIN_ACCESS_INVITES, { params: { include_dead: includeDead }, noDedupe: true })).items,
  createInvite: async (body: { max_uses: number; ttl_days: number | null; note?: string }) => (await http.post<{ invite: InviteCode }>(API_ENDPOINTS.ADMIN_ACCESS_INVITES, body)).invite,
  revokeInvite: (code: string) => http.delete(API_ENDPOINTS.ADMIN_ACCESS_INVITE(code)),
  allowlist: async () => (await http.get<{ items: Array<{ user_id: number; note: string | null; added_at: number }> }>(API_ENDPOINTS.ADMIN_ACCESS_ALLOWLIST, { noDedupe: true })).items,
  allowlistAdd: (user_id: number, note?: string) => http.post(API_ENDPOINTS.ADMIN_ACCESS_ALLOWLIST, { user_id, note }),
  allowlistRemove: (user_id: number) => http.delete(API_ENDPOINTS.ADMIN_ACCESS_ALLOWLIST_USER(user_id)),
  users: (opts: { q?: string; status?: UserAccessStatus; limit?: number; skip?: number } = {}) => http.get<{ total: number; items: ManagedUser[] }>(API_ENDPOINTS.ADMIN_ACCESS_USERS, { params: { q: opts.q || undefined, status: opts.status, limit: opts.limit ?? 50, skip: opts.skip ?? 0 }, noDedupe: true }),
  lock: (user_id: number, reason?: string) => http.post(API_ENDPOINTS.ADMIN_ACCESS_USER_LOCK(user_id), { reason, revoke_sessions: true }),
  unlock: (user_id: number) => http.post(API_ENDPOINTS.ADMIN_ACCESS_USER_UNLOCK(user_id), {}),
  revokeSessions: (user_id: number) => http.post(API_ENDPOINTS.ADMIN_ACCESS_USER_REVOKE(user_id), {}),
  reverify: () => http.post<{ checked: number; restricted: number; restored: number }>(API_ENDPOINTS.ADMIN_ACCESS_REVERIFY, {}, { timeoutMs: 120000 }),
}
