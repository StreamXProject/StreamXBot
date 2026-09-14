import React, { useState } from 'react'
import { createFileRoute, useNavigate, Link } from '@tanstack/react-router'
import { Lock, User, Eye, EyeOff, ArrowRight, KeyRound, MessageCircle } from 'lucide-react'
import { useAuthStore } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { registerUser, validateOtp } from '@/api/auth'
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

  const numericId = parseInt(telegramId.trim(), 10)

  const register = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!username.trim() || !password.trim()) return setError('Enter a username and password')
    if (!numericId) return setError('Enter your numeric Telegram user ID')
    setBusy(true)
    try {
      const res = await registerUser({ userid: numericId, username: username.trim().toLowerCase(), password })
      if (res.ok) setStep('otp')
      else setError(res.message || 'Registration failed')
    } catch (err) {
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
      {step === 'details' ? (
        <div className="space-y-4">
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
