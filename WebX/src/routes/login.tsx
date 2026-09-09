import React, { useEffect, useRef, useState } from 'react'
import { createFileRoute, useNavigate, Link } from '@tanstack/react-router'
import { Lock, User, Eye, EyeOff, ArrowRight } from 'lucide-react'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { loginUser, loginWithServerPassword, fetchSetupStatus, setupOwnerPassword } from '@/api/auth'
import { checkHealth } from '@/api/health'
import { AuthCard, ErrorBanner } from '@/components/auth/AuthCard'
import { Button, TextField, SegmentedButton } from '@/components/md3'
import { cn } from '@/lib/cn'

export interface LoginSearch {
  mode?: 'account' | 'guest'
  redirect?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => {
    const out: LoginSearch = {}
    if (search.mode === 'account' || search.mode === 'guest') out.mode = search.mode
    if (typeof search.redirect === 'string') out.redirect = search.redirect
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
  const token = useAuthStore((s) => s.token)
  const user = useAuthStore((s) => s.user)
  const apiBaseUrl = useSettingsStore((s) => s.apiBaseUrl)
  const setSetting = useSettingsStore((s) => s.set)

  const [mode, setMode] = useState<Mode>(search.mode ?? (localStorage.getItem('webx_user') ? 'account' : 'guest'))
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [show, setShow] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [server, setServer] = useState<{ ok: boolean; needsSetup: boolean; latency?: number; checking: boolean }>({ ok: false, needsSetup: false, checking: true })

  // A guest session may intentionally open this page to upgrade to an account —
  // only bounce away once the token actually changes (or if there's nothing to upgrade).
  const initialToken = useRef(token)
  const initialUser = useRef(user)
  useEffect(() => {
    if (!token) return
    const upgradingGuest = search.mode === 'account' && token === initialToken.current && sessionKind(initialToken.current, initialUser.current) === 'guest'
    if (upgradingGuest) return
    navigate({ to: (search.redirect as '/') || '/', replace: true })
  }, [token, navigate, search.redirect, search.mode])

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
      {!server.needsSetup && (
        <SegmentedButton<Mode>
          value={mode}
          onChange={(m) => { setMode(m); setError(null) }}
          showCheck={false}
          className="w-full mb-5"
          options={[{ value: 'guest', label: 'Server password', icon: <Lock /> }, { value: 'account', label: 'Account', icon: <User /> }]}
        />
      )}

      <ErrorBanner message={error} />

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
