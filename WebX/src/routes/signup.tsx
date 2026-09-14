import React, { useEffect, useState } from 'react'
import { createFileRoute, useNavigate, Link } from '@tanstack/react-router'
import { Lock, User, Eye, EyeOff, ArrowRight, KeyRound, MessageCircle, Ticket, ExternalLink } from 'lucide-react'
import { useAuthStore } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { registerUser, validateOtp } from '@/api/auth'
import { fetchAccessStatus, accessDenied, type AccessStatus, type RequiredChat } from '@/api/access'
import { AuthCard, ErrorBanner } from '@/components/auth/AuthCard'
import { Button, TextField } from '@/components/md3'
import { TelegramIcon } from '@/components/common/TelegramIcon'

export const Route = createFileRoute('/signup')({
  component: SignupPage,
})

function SignupPage() {
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const apiBaseUrl = useSettingsStore((s) => s.apiBaseUrl)
  const [step, setStep] = useState<'details' | 'otp'>('details')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [telegramId, setTelegramId] = useState('')
  const [otp, setOtp] = useState('')
  const [show, setShow] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [inviteCode, setInviteCode] = useState('')
  const [policy, setPolicy] = useState<AccessStatus | null>(null)
  const [missingChats, setMissingChats] = useState<RequiredChat[] | null>(null)

  useEffect(() => {
    let alive = true
    fetchAccessStatus().then((p) => alive && setPolicy(p)).catch(() => alive && setPolicy({ registration_mode: 'open', enforce_membership: false, required_chats: [] }))
    return () => { alive = false }
  }, [])
  const closed = policy?.registration_mode === 'closed'
  const inviteMode = policy?.registration_mode === 'invite'

  const explainDenied = (err: unknown): boolean => {
    const d = accessDenied(err)
    if (!d) return false
    if (d.detail === 'membership_required') {
      setMissingChats(d.required_chats && d.required_chats.length ? d.required_chats : policy?.required_chats ?? [])
      setError(d.message || 'Join the required Telegram chat first.')
      return true
    }
    const map: Record<string, string> = {
      registration_closed: 'Registrations are currently closed.',
      not_allowlisted: 'This Telegram account is not on the allow list. Ask an admin to add you.',
      invite_required: 'An invite code is required.',
      invite_invalid: 'That invite code is not valid.',
      invite_expired: 'That invite code has expired.',
      invite_exhausted: 'That invite code has already been used.',
    }
    setError(map[d.detail] || d.message || 'Registration is not allowed right now.')
    return true
  }

  const numericId = parseInt(telegramId.trim(), 10)

  const register = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!username.trim() || !password.trim()) return setError('Enter a username and password')
    if (!numericId) return setError('Enter your numeric Telegram user ID')
    if (inviteMode && !inviteCode.trim()) return setError('Enter your invite code')
    setBusy(true)
    setMissingChats(null)
    try {
      const res = await registerUser({ userid: numericId, username: username.trim().toLowerCase(), password, invite_code: inviteMode ? inviteCode.trim().toUpperCase() : undefined })
      if (res.ok) setStep('otp')
      else setError(res.message || 'Registration failed')
    } catch (err) {
      if (explainDenied(err)) return
      const m = (err as Error).message
      setError(/409|exists|already/i.test(m) ? 'That username or Telegram ID is already registered' : m)
    } finally {
      setBusy(false)
    }
  }

  const verify = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!otp.trim()) return setError('Enter the code from Telegram')
    setBusy(true)
    try {
      const { user, token } = await validateOtp({ userid: numericId, otp: otp.trim() })
      login(user, token)
      navigate({ to: '/' })
    } catch (err) {
      if (explainDenied(err)) return
      setError((err as Error).message || 'Invalid or expired code')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthCard
      title={step === 'details' ? 'Create your account' : 'Check Telegram'}
      subtitle={step === 'details' ? 'Accounts are linked to a Telegram user so the bot can verify you.' : `We sent a one-time code to Telegram user ${numericId}. Enter it below.`}
      footer={<span>Already registered? <Link to="/login" search={{ mode: 'account' }} className="text-primary font-semibold hover:underline">Sign in</Link></span>}
    >
      <ErrorBanner message={error} />
      {missingChats && missingChats.length > 0 && (
        <div className="mb-4 space-y-2">
          {missingChats.map((c, i) => (
            <div key={i} className="flex items-center justify-between gap-3 h-12 px-3 rounded-2xl bg-surface-container">
              <span className="type-body-md text-on-surface truncate">{c.title || 'Required chat'}</span>
              {c.invite_link && <a href={c.invite_link} target="_blank" rel="noreferrer" className="state-layer inline-flex items-center gap-1.5 h-8 px-3 rounded-full bg-primary text-on-primary type-label-md shrink-0">Join <ExternalLink className="size-3.5" /></a>}
            </div>
          ))}
          <p className="type-body-sm text-on-surface-variant px-1">Join, then submit again.</p>
        </div>
      )}
      {closed ? (
        <div className="rounded-2xl bg-surface-container p-5 flex items-start gap-3">
          <Lock className="size-5 text-on-surface-variant shrink-0 mt-0.5" />
          <div>
            <p className="type-title-sm text-on-surface">Registrations are closed</p>
            <p className="type-body-sm text-on-surface-variant mt-0.5">The administrator has paused new accounts. Existing accounts can still sign in.</p>
          </div>
        </div>
      ) : step === 'details' ? (
        <div className="space-y-4">
          {policy?.enforce_membership && policy.required_chats.length > 0 && !missingChats && (
            <p className="type-body-sm text-on-surface-variant px-1">You need to be a member of {policy.required_chats.map((c) => c.title || 'the required chat').join(', ')} on Telegram.</p>
          )}
          {policy?.registration_mode === 'allowlist' && <p className="type-body-sm text-on-surface-variant px-1">Registration is limited to approved Telegram accounts.</p>}
          <Button
            type="button"
            variant="tonal"
            fullWidth
            size="lg"
            icon={<TelegramIcon className="size-5 text-[#2AABEE]" />}
            onClick={() => {
              if (typeof window !== 'undefined') {
                sessionStorage.setItem('tg_auth_redirect', '/')
              }
              const frontendUrl = typeof window !== 'undefined' ? window.location.origin : ''
              const startUrl = `${apiBaseUrl.replace(/\/$/, '')}/auth/telegram/start?redirect=${encodeURIComponent('/')}&frontend_url=${encodeURIComponent(frontendUrl)}`
              window.location.href = startUrl
            }}
          >
            Continue with Telegram
          </Button>
          <div className="relative flex items-center">
            <div className="grow border-t border-outline-variant" />
            <span className="mx-3 shrink-0 text-outline type-label-sm">or register manually</span>
            <div className="grow border-t border-outline-variant" />
          </div>
          <form onSubmit={register} className="space-y-4">
            <TextField label="Telegram user ID" inputMode="numeric" value={telegramId} onChange={(e) => setTelegramId(e.target.value.replace(/[^0-9]/g, ''))} leading={<MessageCircle />} supporting="Send /id to the bot if you don't know it" autoFocus />
            <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} leading={<User />} autoCapitalize="off" autoComplete="username" />
            {inviteMode && <TextField label="Invite code" value={inviteCode} onChange={(e) => setInviteCode(e.target.value.toUpperCase())} leading={<Ticket />} autoCapitalize="characters" autoComplete="off" spellCheck={false} className="font-mono tracking-widest" />}
            <TextField
              label="Password"
              type={show ? 'text' : 'password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              leading={<Lock />}
              autoComplete="new-password"
              trailing={<button type="button" onClick={() => setShow((s) => !s)} aria-label="Toggle password" className="state-layer size-8 rounded-full inline-flex items-center justify-center">{show ? <EyeOff /> : <Eye />}</button>}
            />
            <Button type="submit" fullWidth size="lg" loading={busy} trailingIcon={<ArrowRight />}>Continue</Button>
          </form>
        </div>
      ) : (
        <form onSubmit={verify} className="space-y-4">
          <TextField label="Verification code" inputMode="numeric" value={otp} onChange={(e) => setOtp(e.target.value)} leading={<KeyRound />} autoFocus className="font-mono tracking-[0.3em]" />
          <Button type="submit" fullWidth size="lg" loading={busy}>Verify & sign in</Button>
          <button type="button" onClick={() => setStep('details')} className="w-full type-label-lg text-primary h-10">Back</button>
        </form>
      )}
    </AuthCard>
  )
}
