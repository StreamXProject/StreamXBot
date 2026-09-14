import { create } from 'zustand'

export interface UserProfile {
  id: string
  name: string
  username?: string
  avatarUrl?: string | null
  profile_url?: string | null
  photo_url?: string | null
  /** Owner / sudo (from /auth/me) */
  is_admin?: boolean
  status?: 'active' | 'locked' | 'restricted'
}

export interface AccessBlock {
  code: 'account_locked' | 'membership_required'
  message: string
  reason?: string | null
  required_chats: Array<{ title: string | null; invite_link: string | null; is_private?: boolean }>
}

export type SessionKind = 'none' | 'guest' | 'user'

interface AuthStoreState {
  user: UserProfile | null
  token: string | null
  /** Set when the server rejected our token */
  sessionExpired: boolean
  /** Server refused access (locked account / left required chat) */
  accessBlock: AccessBlock | null
  setAccessBlock: (b: AccessBlock | null) => void
  login: (user: UserProfile, token: string) => void
  loginGuest: (token: string) => void
  updateUser: (patch: Partial<UserProfile>) => void
  logout: () => void
  clearExpired: () => void
}

const TOKEN_KEY = 'webx_token'
const USER_KEY = 'webx_user'

export function parseTokenPayload(token: string): Record<string, unknown> | null {
  try {
    const parts = token.split('.')
    const payloadPart = parts.length === 3 ? parts[1] : parts[0]
    const base64 = payloadPart.replace(/-/g, '+').replace(/_/g, '/')
    const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4)
    const json = decodeURIComponent(
      atob(padded)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    )
    return JSON.parse(json)
  } catch {
    return null
  }
}

export function isApiToken(token: string | null): boolean {
  if (!token) return false
  const p = parseTokenPayload(token)
  return p?.uid === '__api__' || p?.user_id === '__api__' || p?.user_id === 0
}

export function sessionKind(token: string | null, user: UserProfile | null): SessionKind {
  if (!token) return 'none'
  if (user) return 'user'
  return isApiToken(token) ? 'guest' : 'user'
}

function profileFromPayload(payload: Record<string, unknown> | null, base?: UserProfile | null): UserProfile | null {
  if (!payload) return base ?? null
  const pfp = (payload.profile_url as string) || (payload.photo_url as string) || null
  const name = (payload.first_name as string) || (payload.name as string) || (payload.username as string) || null
  const id = payload.uid ?? payload.user_id
  if (!base && !(pfp || name || id)) return null
  return {
    id: String(base?.id ?? id ?? 'user'),
    name: base?.name && base.name !== 'User' ? base.name : name || 'User',
    username: base?.username ?? (payload.username as string | undefined),
    avatarUrl: base?.avatarUrl ?? pfp,
    profile_url: base?.profile_url ?? ((payload.profile_url as string) || null),
    photo_url: base?.photo_url ?? ((payload.photo_url as string) || null),
  }
}

function readStored(): { user: UserProfile | null; token: string | null } {
  if (typeof window === 'undefined') return { user: null, token: null }
  try {
    let token = localStorage.getItem(TOKEN_KEY)
    if (!token) {
      // migrate from StreamXWeb
      token = localStorage.getItem('streamx_auth_token') || localStorage.getItem('streamw:auth:token')
      if (token) localStorage.setItem(TOKEN_KEY, token)
    }
    if (!token) return { user: null, token: null }
    if (isApiToken(token)) return { user: null, token }
    const rawUser = localStorage.getItem(USER_KEY) || localStorage.getItem('streamx_user_info') || localStorage.getItem('streamw:auth:userInfo')
    let user: UserProfile | null = rawUser ? JSON.parse(rawUser) : null
    user = profileFromPayload(parseTokenPayload(token), user)
    if (user) localStorage.setItem(USER_KEY, JSON.stringify(user))
    return { user, token }
  } catch {
    return { user: null, token: null }
  }
}

export const useAuthStore = create<AuthStoreState>((set, get) => {
  const initial = readStored()
  return {
    user: initial.user,
    token: initial.token,
    sessionExpired: false,
    accessBlock: null,
    setAccessBlock: (b) => set({ accessBlock: b }),

    login: (user, token) => {
      const finalUser = profileFromPayload(parseTokenPayload(token), user) ?? user
      localStorage.setItem(USER_KEY, JSON.stringify(finalUser))
      localStorage.setItem(TOKEN_KEY, token)
      set({ user: finalUser, token, sessionExpired: false })
    },

    loginGuest: (token) => {
      localStorage.removeItem(USER_KEY)
      localStorage.setItem(TOKEN_KEY, token)
      set({ user: null, token, sessionExpired: false })
    },

    updateUser: (patch) => {
      const cur = get().user
      if (!cur) return
      const next = { ...cur, ...patch }
      localStorage.setItem(USER_KEY, JSON.stringify(next))
      set({ user: next })
    },

    logout: () => {
      localStorage.removeItem(USER_KEY)
      localStorage.removeItem(TOKEN_KEY)
      set({ user: null, token: null })
    },

    clearExpired: () => set({ sessionExpired: false }),
  }
})

// Drop the session when the server says our token is no longer valid.
if (typeof window !== 'undefined') {
  window.addEventListener('webx:access-denied', (e) => {
    const d = (e as CustomEvent<{ detail?: string; message?: string; reason?: string | null; required_chats?: AccessBlock['required_chats'] }>).detail || {}
    if (d.detail !== 'account_locked' && d.detail !== 'membership_required') return
    useAuthStore.setState({
      accessBlock: { code: d.detail, message: d.message || (d.detail === 'account_locked' ? 'Your account has been locked.' : 'Join the required Telegram chat to continue.'), reason: d.reason ?? null, required_chats: d.required_chats ?? [] },
    })
  })
  window.addEventListener('webx:unauthorized', () => {
    const s = useAuthStore.getState()
    if (s.token) {
      localStorage.removeItem(USER_KEY)
      localStorage.removeItem(TOKEN_KEY)
      useAuthStore.setState({ user: null, token: null, sessionExpired: true })
    }
  })
}
