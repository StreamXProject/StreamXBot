import React, { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LogOut, KeyRound, UserPlus, ShieldCheck, LogIn } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Button, Dialog, TextField } from '@/components/md3'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { UserAvatar } from '@/components/shell/TopAppBar'
import { useMe } from '@/hooks/useQueries'
import { changeOwnerPassword, setCredentials, logoutServer } from '@/api/auth'
import { toast } from '@/stores/uiStore'
import { relativeTime } from '@/lib/format'

export const Route = createFileRoute('/settings/account')({
  component: AccountSettings,
})

function AccountSettings() {
  const user = useAuthStore((s) => s.user)
  const token = useAuthStore((s) => s.token)
  const logout = useAuthStore((s) => s.logout)
  const kind = sessionKind(token, user)
  const navigate = useNavigate()
  const { data: me } = useMe()
  const [pwOpen, setPwOpen] = useState(false)
  const [credOpen, setCredOpen] = useState(false)
  const [pw, setPw] = useState('')
  const [pw2, setPw2] = useState('')
  const [username, setUsername] = useState(user?.username ?? '')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const serverUser = me?.user

  const submitPw = async () => {
    setErr(null)
    if (pw.length < 6) return setErr('Use at least 6 characters')
    if (pw !== pw2) return setErr('Passwords do not match')
    setBusy(true)
    try {
      await changeOwnerPassword(pw)
      toast('Server password updated')
      setPwOpen(false)
      setPw('')
      setPw2('')
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const submitCred = async () => {
    setErr(null)
    if (!username.trim()) return setErr('Choose a username')
    if (pw.length < 6) return setErr('Use at least 6 characters')
    setBusy(true)
    try {
      await setCredentials(username.trim(), pw)
      useAuthStore.getState().updateUser({ username: username.trim() })
      toast('Credentials saved — you can now sign in with them')
      setCredOpen(false)
      setPw('')
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <SettingsPage title="Account">
      <div className="flex items-center gap-4 p-5 rounded-lg bg-surface-low">
        <UserAvatar name={user?.name ?? 'Guest'} url={user?.profile_url || user?.photo_url || user?.avatarUrl || serverUser?.profile_url} className="size-16" />
        <div className="min-w-0 flex-1">
          <p className="type-title-lg text-on-surface truncate">{user?.name ?? serverUser?.first_name ?? 'Guest session'}</p>
          <p className="type-body-md text-on-surface-variant truncate">
            {kind === 'user' ? (user?.username ? `@${user.username}` : `User #${user?.id}`) : kind === 'guest' ? 'Signed in with the server password' : 'Not signed in'}
          </p>
          {serverUser?.created_at && <p className="type-body-sm text-on-surface-variant mt-0.5">Member since {new Date(serverUser.created_at * (serverUser.created_at < 1e12 ? 1000 : 1)).toLocaleDateString()}</p>}
        </div>
        <span className="hidden sm:inline-flex items-center gap-1.5 h-8 px-3 rounded-full bg-secondary-container text-on-secondary-container type-label-md">
          <ShieldCheck className="size-4" /> {kind === 'user' ? 'Account' : kind === 'guest' ? 'Guest' : 'Signed out'}
        </span>
      </div>

      <SettingsSection title="Session">
        {kind === 'guest' && (
          <SettingRow
            icon={<LogIn />}
            label="Sign in to an account"
            description="Guest: favourites and playlists stay local"
            onClick={() => navigate({ to: '/login', search: { mode: 'account' } })}
            control={<Button variant="tonal" size="sm">Sign in</Button>}
          />
        )}
        {kind !== 'none' && (
          <SettingRow
            icon={<LogOut />}
            label="Sign out"
            description="Removes the token from this device"
            onClick={async () => {
              await logoutServer()
              logout()
              navigate({ to: '/login' })
            }}
            control={<Button variant="outlined" size="sm">Sign out</Button>}
          />
        )}
        {kind === 'none' && <SettingRow icon={<LogIn />} label="Sign in" onClick={() => navigate({ to: '/login' })} control={<Button size="sm">Sign in</Button>} />}
      </SettingsSection>

      {kind !== 'none' && (
        <SettingsSection title="Security" description="Requires a valid session">
          <SettingRow icon={<KeyRound />} label="Change server password" description="The shared password used for guest access" onClick={() => { setErr(null); setPwOpen(true) }} />
          <SettingRow icon={<UserPlus />} label={user?.username ? 'Update username & password' : 'Set username & password'} description="Sign in with credentials instead of Telegram" onClick={() => { setErr(null); setCredOpen(true) }} />
        </SettingsSection>
      )}

      <SettingsSection title="Session token">
        <SettingRow label="Token" description={token ? `${token.slice(0, 14)}… (${token.length} chars)` : '—'} control={token ? <Button variant="text" size="sm" onClick={() => navigator.clipboard.writeText(token).then(() => toast('Token copied'))}>Copy</Button> : undefined} />
        {user && <SettingRow label="Last update" description={relativeTime(Date.now()) || '—'} />}
      </SettingsSection>

      <Dialog open={pwOpen} onClose={() => setPwOpen(false)} title="Change server password" actions={<><Button variant="text" onClick={() => setPwOpen(false)}>Cancel</Button><Button onClick={() => void submitPw()} loading={busy}>Update</Button></>}>
        <div className="space-y-3">
          <TextField label="New password" type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoFocus />
          <TextField label="Repeat password" type="password" value={pw2} onChange={(e) => setPw2(e.target.value)} error={err} />
        </div>
      </Dialog>
      <Dialog open={credOpen} onClose={() => setCredOpen(false)} title="Account credentials" actions={<><Button variant="text" onClick={() => setCredOpen(false)}>Cancel</Button><Button onClick={() => void submitCred()} loading={busy}>Save</Button></>}>
        <div className="space-y-3">
          <TextField label="Username" value={username} onChange={(e) => setUsername(e.target.value)} autoFocus autoCapitalize="off" />
          <TextField label="Password" type="password" value={pw} onChange={(e) => setPw(e.target.value)} error={err} />
        </div>
      </Dialog>
    </SettingsPage>
  )
}
