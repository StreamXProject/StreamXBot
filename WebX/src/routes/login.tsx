import React, { useEffect, useRef, useState } from 'react'
import { createFileRoute, useNavigate, Link } from '@tanstack/react-router'
import { Lock, User, Eye, EyeOff, ArrowRight, Check, Copy, ShieldCheck } from 'lucide-react'
import { useAuthStore, sessionKind, parseTokenPayload, type UserProfile } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { loginUser, loginWithServerPassword, fetchSetupStatus, setupOwnerPassword, fetchMe, fetchTelegramConfig, type TelegramConfig } from '@/api/auth'
import { checkHealth } from '@/api/health'
import { getBaseUrl, http } from '@/api/client'
import { API_ENDPOINTS } from '@/api/endpoints'
import { AuthCard, ErrorBanner } from '@/components/auth/AuthCard'
import { Button, TextField, SegmentedButton, IdPill } from '@/components/md3'
import { TelegramIcon } from '@/components/common/TelegramIcon'
import { cn } from '@/lib/cn'

export interface LoginSearch {
  mode?: 'account' | 'guest'
  redirect?: string
  token?: string
  error?: string
  error_description?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => {
    const out: LoginSearch = {}
    if (search.mode === 'account' || search.mode === 'guest') out.mode = search.mode
    if (typeof search.redirect === 'string') out.redirect = search.redirect
    if (typeof search.token === 'string') out.token = search.token
    if (typeof search.error === 'string') out.error = search.error
    if (typeof search.error_description === 'string') out.error_description = search.error_description
    return out
  },
  component: LoginPage,
})

type Mode = 'guest' | 'account'

function LoginPage() {
  const search = Route.useSearch()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const loginGuest = useAuthStore((s) => s.loginGuest)
  const updateUser = useAuthStore((s) => s.updateUser)
  const token = useAuthStore((s) => s.token)
  const user = useAuthStore((s) => s.user)
  const apiBaseUrl = useSettingsStore((s) => s.apiBaseUrl)
  const setSetting = useSettingsStore((s) => s.set)

  const searchToken = search.token
  const searchRedirect = search.redirect

  const sessionExpired = useAuthStore((s) => s.sessionExpired)
  const clearExpired = useAuthStore((s) => s.clearExpired)

  const [mode, setMode] = useState<Mode>(search.mode ?? (localStorage.getItem('webx_user') ? 'account' : 'guest'))
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [show, setShow] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [countdown, setCountdown] = useState(5)
  const [server, setServer] = useState<{ ok: boolean; needsSetup: boolean; latency?: number; checking: boolean }>({ ok: false, needsSetup: false, checking: true })
  const [tgConfig, setTgConfig] = useState<TelegramConfig | null>(null)
  const pendingAuthRef = useRef<{ user: UserProfile; token: string } | null>(null)

  useEffect(() => {
    let alive = true
    fetchTelegramConfig()
      .then((cfg) => {
        if (alive && cfg.ok) setTgConfig(cfg)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  // Listen for login completion from Telegram OAuth popup
  useEffect(() => {
    const onMessage = async (event: MessageEvent) => {
      if (typeof window === 'undefined') return
      const isAllowedOrigin =
        event.origin === window.location.origin ||
        event.origin === 'https://oauth.telegram.org' ||
        event.origin === 'https://telegram.org'
      if (!isAllowedOrigin) return

      let data = event.data
      if (typeof data === 'string') {
        try {
          data = JSON.parse(data)
        } catch {
          return
        }
      }
      if (!data || typeof data !== 'object') return

      // Direct Telegram Widget postMessage: { event: 'auth_result', result: user_data | false }
      if (data.event === 'auth_result') {
        if (!data.result) {
          setBusy(false)
          const domain = window.location.hostname
          setError(
            `Telegram rejected the login. The domain "${domain}" must be authorized in @BotFather: send /setdomain to @BotFather and enter "${domain}".`
          )
          return
        }

        // data.result contains { id, first_name, username, photo_url, auth_date, hash }
        try {
          setBusy(true)
          const authRes = await http.post<{
            ok: boolean
            token?: string
            user_id?: string | number
            first_name?: string
            username?: string
            profile_url?: string
            photo_url?: string
            detail?: string
          }>(API_ENDPOINTS.AUTH_TELEGRAM_WIDGET, data.result, { anonymous: true })
          setBusy(false)
          if (authRes.ok && authRes.token) {
            const payload = parseTokenPayload(authRes.token)
            const u: UserProfile = {
              id: String(authRes.user_id || payload?.uid || payload?.userid || payload?.user_id || ''),
              name: authRes.first_name || (payload?.first_name as string) || 'Telegram User',
              username: authRes.username || (payload?.username as string) || undefined,
              avatarUrl: authRes.profile_url || authRes.photo_url || (payload?.profile_url as string) || null,
              profile_url: authRes.profile_url || authRes.photo_url || (payload?.profile_url as string) || null,
              photo_url: authRes.photo_url || authRes.profile_url || (payload?.photo_url as string) || null,
            }
            pendingAuthRef.current = { user: u, token: authRes.token }
            setSuccessUser(u)
            setCountdown(5)
          } else {
            setError(authRes.detail || 'Telegram verification failed.')
          }
        } catch (err) {
          setBusy(false)
          setError((err as Error).message || 'Failed to verify Telegram credentials.')
        }
        return
      }

      // Internal callback postMessage: { type: 'tg_auth_success', ... }
      if (data.type === 'tg_auth_success' && data.token) {
        setBusy(false)
        const payload = parseTokenPayload(data.token)
        const u: UserProfile = {
          id: String(data.user?.user_id || payload?.uid || payload?.userid || payload?.user_id || ''),
          name: data.user?.first_name || (payload?.first_name as string) || 'Telegram User',
          username: data.user?.username || (payload?.username as string) || undefined,
          avatarUrl: data.user?.profile_url || data.user?.photo_url || (payload?.profile_url as string) || null,
          profile_url: data.user?.profile_url || data.user?.photo_url || (payload?.profile_url as string) || null,
          photo_url: data.user?.photo_url || data.user?.profile_url || (payload?.photo_url as string) || null,
        }
        pendingAuthRef.current = { user: u, token: data.token }
        setSuccessUser(u)
        setCountdown(5)
      } else if (data.type === 'tg_auth_error') {
        setBusy(false)
        setError(data.error === 'access_denied' ? 'Telegram login was cancelled.' : (data.error_description || data.error || 'Telegram login failed.'))
      }
    }

    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [login])

  const startTelegramLogin = () => {
    setError(null)
    setBusy(true)

    const redirectDest = search.redirect || '/'
    if (typeof window !== 'undefined') {
      sessionStorage.setItem('tg_auth_redirect', redirectDest)
    }

    const clientId = tgConfig?.client_id || '8860772420'
    const origin = typeof window !== 'undefined' ? window.location.origin : ''
    const callbackUri = `${origin}/auth/telegram/callback`
    const popupUrl = `https://oauth.telegram.org/auth?bot_id=${clientId}&origin=${encodeURIComponent(origin)}&return_to=${encodeURIComponent(callbackUri)}&request_access=write`

    const width = 550
    const height = 650
    const left = Math.max(0, (window.screen.width - width) / 2) + (window.screenLeft || 0)
    const top = Math.max(0, (window.screen.height - height) / 2) + (window.screenTop || 0)

    try {
      const popup = window.open(
        popupUrl,
        'telegram_oauth',
        `width=${width},height=${height},left=${left},top=${top},status=0,location=0,menubar=0,toolbar=0`
      )

      if (!popup || popup.closed || typeof popup.closed === 'undefined') {
        // Popup blocked by browser — fall back directly to full-page redirect
        const base = getBaseUrl() || origin
        const startUrl = `${base.replace(/\/$/, '')}/auth/telegram/start?redirect=${encodeURIComponent(redirectDest)}&frontend_url=${encodeURIComponent(origin)}`
        window.location.href = startUrl
        return
      }

      popup.focus()
      const timer = setInterval(() => {
        if (popup.closed) {
          clearInterval(timer)
          setBusy(false)
        }
      }, 600)
    } catch {
      const base = getBaseUrl() || origin
      const startUrl = `${base.replace(/\/$/, '')}/auth/telegram/start?redirect=${encodeURIComponent(redirectDest)}&frontend_url=${encodeURIComponent(origin)}`
      window.location.href = startUrl
    }
  }

  useEffect(() => {
    if (sessionExpired) {
      setError('Your previous session expired. Please sign in again.')
      clearExpired()
    }
  }, [sessionExpired, clearExpired])

  const [successUser, setSuccessUser] = useState<{ id: string; name: string; username?: string; avatarUrl?: string | null } | null>(() => {
    if (!searchToken) return null
    const payload = parseTokenPayload(searchToken)
    if (!payload) return null
    return {
      id: String(payload.uid ?? payload.user_id ?? ''),
      name: (payload.first_name as string) || (payload.name as string) || 'Telegram User',
      username: (payload.username as string) || undefined,
      avatarUrl: (payload.profile_url as string) || (payload.photo_url as string) || null,
    }
  })

  // When redirected back from Telegram OIDC callback with token
  useEffect(() => {
    if (!searchToken) return
    let active = true
    const payload = parseTokenPayload(searchToken)
    const initial = {
      id: String(payload?.uid ?? payload?.user_id ?? 'user'),
      name: (payload?.first_name as string) || (payload?.name as string) || 'Telegram User',
      username: (payload?.username as string) || undefined,
      avatarUrl: (payload?.profile_url as string) || (payload?.photo_url as string) || null,
    }

    if (typeof window !== 'undefined' && window.opener && window.opener !== window) {
      try {
        window.opener.postMessage(
          {
            type: 'tg_auth_success',
            token: searchToken,
            user: {
              user_id: initial.id,
              first_name: initial.name,
              username: initial.username,
              photo_url: initial.avatarUrl,
            },
          },
          window.location.origin
        )
        window.close()
        return
      } catch {}
    }

    pendingAuthRef.current = {
      user: {
        id: initial.id,
        name: initial.name,
        username: initial.username,
        avatarUrl: initial.avatarUrl,
        profile_url: initial.avatarUrl,
        photo_url: initial.avatarUrl,
      },
      token: searchToken,
    }
    setSuccessUser(initial)
    setCountdown(5)

    ;(async () => {
      try {
        const me = await fetchMe()
        if (active && me?.user) {
          const updated: UserProfile = {
            id: String(me.user._id || me.user.user_id || initial.id),
            name: me.user.first_name || me.user.username || initial.name,
            username: me.user.username,
            avatarUrl: me.user.profile_url || me.user.photo_url || initial.avatarUrl,
            profile_url: me.user.profile_url || me.user.photo_url || initial.avatarUrl,
            photo_url: me.user.profile_url || me.user.photo_url || initial.avatarUrl,
          }
          if (pendingAuthRef.current) {
            pendingAuthRef.current.user = updated
          }
          setSuccessUser(updated)
        }
      } catch {}
    })()
    return () => {
      active = false
    }
  }, [searchToken])

  const continueNow = () => {
    if (pendingAuthRef.current) {
      login(pendingAuthRef.current.user, pendingAuthRef.current.token)
    }
    navigate({ to: (searchRedirect as '/') || '/', replace: true })
  }

  useEffect(() => {
    if (!successUser) return
    const timer = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          clearInterval(timer)
          if (pendingAuthRef.current) {
            login(pendingAuthRef.current.user, pendingAuthRef.current.token)
          }
          navigate({ to: (searchRedirect as '/') || '/', replace: true })
          return 0
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(timer)
  }, [successUser, searchRedirect, navigate, login])

  useEffect(() => {
    if (search.error) {
      if (search.error === 'access_denied') {
        setError('Telegram login was cancelled.')
      } else {
        setError(search.error_description || 'Telegram login could not be completed. Please try again.')
      }
    }
  }, [search.error, search.error_description])

  // A guest session may intentionally open this page to upgrade to an account —
  // only bounce away once the token actually changes (or if there's nothing to upgrade).
  const initialToken = useRef(token)
  const initialUser = useRef(user)
  useEffect(() => {
    if (!token) return
    if (searchToken) return
    if (successUser) return
    const upgradingGuest = search.mode === 'account' && token === initialToken.current && sessionKind(initialToken.current, initialUser.current) === 'guest'
    if (upgradingGuest) return
    navigate({ to: (search.redirect as '/') || '/', replace: true })
  }, [token, navigate, search.redirect, search.mode, searchToken, successUser])

  // Probe the server so the first screen tells the truth about connectivity
  useEffect(() => {
    let cancelled = false
    setServer((s) => ({ ...s, checking: true }))
    ;(async () => {
      const h = await checkHealth(apiBaseUrl, 5000)
      let needsSetup = false
      if (h.ok) {
        try {
          needsSetup = (await fetchSetupStatus(apiBaseUrl)).needs_setup
        } catch {
          needsSetup = false
        }
      }
      if (!cancelled) setServer({ ok: h.ok, needsSetup, latency: h.latencyMs, checking: false })
    })()
    return () => {
      cancelled = true
    }
  }, [apiBaseUrl])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!password.trim()) return setError('Enter your password')
    if (mode === 'account' && !username.trim()) return setError('Enter your username')
    if (server.needsSetup && password !== confirm) return setError('Passwords do not match')
    setBusy(true)
    try {
      setSetting('hasSeenWelcome', true)
      if (server.needsSetup && mode === 'guest') {
        const { token } = await setupOwnerPassword(password)
        loginGuest(token)
      } else if (mode === 'guest') {
        const { token } = await loginWithServerPassword(password)
        loginGuest(token)
      } else {
        const { user, token } = await loginUser({ username: username.trim(), password })
        login(user, token)
      }
      navigate({ to: (search.redirect as '/') || '/' })
    } catch (err) {
      const msg = (err as Error).message || 'Sign-in failed'
      setError(/401|403|password|credential|Incorrect|Invalid/i.test(msg) ? (mode === 'guest' ? 'Incorrect server password' : 'Invalid username or password') : msg)
    } finally {
      setBusy(false)
    }
  }

  if (successUser) {
    return (
      <AuthCard
        title="Signed in successfully!"
        subtitle="Your Telegram account is connected to WebX."
        footer={<span>Logged in with verified Telegram identity.</span>}
      >
        <div className="space-y-5">
          <div className="flex items-center gap-4 p-4 rounded-xl bg-surface-container border border-outline-variant/60">
            <div className="relative shrink-0">
              {successUser.avatarUrl ? (
                <img
                  src={successUser.avatarUrl}
                  alt={successUser.name}
                  className="size-16 rounded-full object-cover ring-2 ring-primary/40"
                  onError={(e) => {
                    ;(e.currentTarget as HTMLElement).style.display = 'none'
                  }}
                />
              ) : (
                <div className="size-16 rounded-full bg-primary-container text-on-primary-container flex items-center justify-center type-title-lg font-bold">
                  {successUser.name.slice(0, 1).toUpperCase()}
                </div>
              )}
              <span className="absolute -bottom-1 -right-1 size-6 rounded-full bg-surface-container flex items-center justify-center shadow-md">
                <TelegramIcon className="size-4 text-[#2AABEE]" />
              </span>
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <p className="type-title-md text-on-surface font-semibold truncate">{successUser.name}</p>
                <ShieldCheck className="size-4 text-primary shrink-0" />
              </div>
              {successUser.username && (
                <p className="type-body-sm text-on-surface-variant truncate">@{successUser.username}</p>
              )}
              <div className="mt-2">
                <IdPill id={successUser.id} size="md" copyText="Telegram ID copied" />
              </div>
            </div>
          </div>

          <div className="space-y-2 text-center">
            <p className="type-body-md text-on-surface-variant">
              Redirecting in <span className="font-semibold text-primary">{countdown}s</span>...
            </p>
            <div className="h-1.5 w-full bg-surface-container rounded-full overflow-hidden">
              <div
                className="h-full bg-primary transition-all duration-1000 ease-linear rounded-full"
                style={{ width: `${Math.max(0, (countdown / 5) * 100)}%` }}
              />
            </div>
          </div>

          <div className="space-y-2 pt-2">
            <Button
              type="button"
              fullWidth
              size="lg"
              onClick={continueNow}
              trailingIcon={<ArrowRight />}
            >
              Continue now
            </Button>
            <button
              type="button"
              onClick={continueNow}
              className="w-full text-center type-label-md text-on-surface-variant hover:text-on-surface py-2 transition-colors cursor-pointer"
            >
              Skip and login directly
            </button>
          </div>
        </div>
      </AuthCard>
    )
  }

  const title = server.needsSetup && mode === 'guest' ? 'Set up your server' : mode === 'guest' ? 'Welcome back' : 'Sign in'
  const subtitle = server.needsSetup && mode === 'guest'
    ? 'This server has no owner password yet. Choose one to finish setup — you\u2019ll use it to unlock the server.'
    : mode === 'guest'
      ? 'Unlock with the shared server password, or use your account.'
      : 'Use the username and password linked to your Telegram account.'

  return (
    <AuthCard
      title={title}
      subtitle={subtitle}
      footer={
        mode === 'account' ? (
          <span>No account yet? <Link to="/signup" className="text-primary font-semibold hover:underline">Create one</Link></span>
        ) : (
          <span>Tip: press <kbd className="font-mono">Ctrl K</kbd> anywhere to search.</span>
        )
      }
    >
      <ErrorBanner message={error} />

      {!server.needsSetup && (
        <div className="space-y-4 mb-5">
          <Button
            type="button"
            variant="tonal"
            fullWidth
            size="lg"
            icon={<TelegramIcon className="size-5 text-[#2AABEE]" />}
            onClick={startTelegramLogin}
          >
            Continue with Telegram
          </Button>
          <div className="relative flex items-center">
            <div className="grow border-t border-outline-variant" />
            <span className="mx-3 shrink-0 text-outline type-label-sm">or continue with password</span>
            <div className="grow border-t border-outline-variant" />
          </div>
        </div>
      )}

      {!server.needsSetup && (
        <SegmentedButton<Mode>
          value={mode}
          onChange={(m) => { setMode(m); setError(null) }}
          showCheck={false}
          className="w-full mb-5"
          options={[{ value: 'guest', label: 'Server password', icon: <Lock /> }, { value: 'account', label: 'Account', icon: <User /> }]}
        />
      )}

      <form onSubmit={submit} className="space-y-4">
        {mode === 'account' && (
          <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} leading={<User />} autoFocus autoComplete="username" autoCapitalize="off" />
        )}
        <TextField
          label={server.needsSetup && mode === 'guest' ? 'Choose a server password' : 'Password'}
          type={show ? 'text' : 'password'}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          leading={<Lock />}
          autoFocus={mode === 'guest'}
          autoComplete={mode === 'guest' ? 'off' : 'current-password'}
          trailing={
            <button type="button" onClick={() => setShow((s) => !s)} aria-label={show ? 'Hide password' : 'Show password'} className="state-layer size-8 rounded-full inline-flex items-center justify-center">
              {show ? <EyeOff /> : <Eye />}
            </button>
          }
        />
        {server.needsSetup && mode === 'guest' && (
          <TextField label="Repeat password" type={show ? 'text' : 'password'} value={confirm} onChange={(e) => setConfirm(e.target.value)} leading={<Lock />} />
        )}
        <Button type="submit" fullWidth size="lg" loading={busy} disabled={!password.trim() || (mode === 'account' && !username.trim())} trailingIcon={<ArrowRight />}>
          {server.needsSetup && mode === 'guest' ? 'Finish setup' : mode === 'guest' ? 'Unlock' : 'Sign in'}
        </Button>
        {server.needsSetup && (
          <button type="button" onClick={() => setMode(mode === 'guest' ? 'account' : 'guest')} className="w-full type-label-lg text-primary h-10">
            {mode === 'guest' ? 'I already have an account' : 'Set up the server instead'}
          </button>
        )}
      </form>
    </AuthCard>
  )
}
