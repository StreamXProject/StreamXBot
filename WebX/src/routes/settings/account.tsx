import React, { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LogOut, KeyRound, UserPlus, ShieldCheck, LogIn, Hash, Eye, EyeOff } from 'lucide-react'
import { SettingsPage, SettingsSection, SettingRow } from '@/components/settings/SettingsPrimitives'
import { Button, Dialog, TextField, IdPill } from '@/components/md3'
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
  const [serverPw, setServerPw] = useState('')
  const [serverPw2, setServerPw2] = useState('')
  const [showServerPw, setShowServerPw] = useState(false)
  const [pwErr, setPwErr] = useState<string | null>(null)

  const [credOpen, setCredOpen] = useState(false)
  const [credUsername, setCredUsername] = useState('')
  const [credPw, setCredPw] = useState('')
  const [showCredPw, setShowCredPw] = useState(false)
  const [credErr, setCredErr] = useState<string | null>(null)

  const [busy, setBusy] = useState(false)

  const serverUser = me?.user
  const displayId =
    user?.id ||
    (user as any)?.user_id ||
    (user as any)?.userid ||
    serverUser?.id ||
    (serverUser as any)?.user_id ||
    (serverUser as any)?.userid ||
    serverUser?._id

  const openPwDialog = () => {
    setPwErr(null)
    setServerPw('')
    setServerPw2('')
    setShowServerPw(false)
    setPwOpen(true)
  }

  const closePwDialog = () => {
    setPwOpen(false)
    setServerPw('')
    setServerPw2('')
    setPwErr(null)
  }

  const openCredDialog = () => {
    setCredErr(null)
    setCredUsername(user?.username || serverUser?.username || '')
    setCredPw('')
    setShowCredPw(false)
    setCredOpen(true)
  }

  const closeCredDialog = () => {
    setCredOpen(false)
    setCredPw('')
    setCredErr(null)
  }

  const submitPw = async () => {
    setPwErr(null)
    if (serverPw.length < 6) return setPwErr('Use at least 6 characters')
    if (serverPw !== serverPw2) return setPwErr('Passwords do not match')
    setBusy(true)
    try {
      await changeOwnerPassword(serverPw)
      toast('Server password updated')
      closePwDialog()
    } catch (e) {
      setPwErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const submitCred = async () => {
    setCredErr(null)
    const u = credUsername.trim()
    if (!u) return setCredErr('Choose a username')
    if (credPw.length < 6) return setCredErr('Use at least 6 characters')
    setBusy(true)
    try {
      await setCredentials(u, credPw)
      useAuthStore.getState().updateUser({ username: u })
      toast('Credentials saved — you can now sign in with them')
      closeCredDialog()
    } catch (e) {
      setCredErr((e as Error).message)
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
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5 mt-1">
            {kind === 'user' && (user?.username || serverUser?.username) && (
              <p className="type-body-md text-on-surface-variant font-medium">@{user?.username || serverUser?.username}</p>
            )}
            {kind === 'user' && displayId && (
              <IdPill id={displayId} size="xs" copyText="Telegram ID copied" />
            )}
            {kind === 'guest' && (
              <p className="type-body-md text-on-surface-variant">Signed in with the server password</p>
            )}
          </div>
          {serverUser?.created_at && <p className="type-body-sm text-on-surface-variant mt-1">Member since {new Date(serverUser.created_at * (serverUser.created_at < 1e12 ? 1000 : 1)).toLocaleDateString()}</p>}
        </div>
        <span className="hidden sm:inline-flex items-center gap-1.5 h-8 px-3 rounded-full bg-secondary-container text-on-secondary-container type-label-md">
          <ShieldCheck className="size-4" /> {kind === 'user' ? 'Account' : kind === 'guest' ? 'Guest' : 'Signed out'}
        </span>
      </div>

      <SettingsSection title="Session">
        {kind === 'user' && displayId && (
          <SettingRow
            icon={<Hash />}
            label="Telegram User ID"
            description="Your linked Telegram account identifier"
            control={<IdPill id={displayId} size="sm" copyText="Telegram ID copied" />}
          />
        )}
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
          <SettingRow icon={<KeyRound />} label="Change server password" description="The shared password used for guest access" onClick={openPwDialog} />
          <SettingRow icon={<UserPlus />} label={user?.username ? 'Update username & password' : 'Set username & password'} description="Sign in with credentials instead of Telegram" onClick={openCredDialog} />
        </SettingsSection>
      )}

      <SettingsSection title="Session token">
        <SettingRow label="Token" description={token ? `${token.slice(0, 14)}… (${token.length} chars)` : '—'} control={token ? <Button variant="text" size="sm" onClick={() => navigator.clipboard.writeText(token).then(() => toast('Token copied'))}>Copy</Button> : undefined} />
        {user && <SettingRow label="Last update" description={relativeTime(Date.now()) || '—'} />}
      </SettingsSection>

      <Dialog
        open={pwOpen}
        onClose={closePwDialog}
        title="Change server password"
        actions={
          <>
            <Button variant="text" onClick={closePwDialog} disabled={busy}>
              Cancel
            </Button>
            <Button onClick={() => void submitPw()} loading={busy} disabled={serverPw.length < 6 || serverPw !== serverPw2}>
              Update
            </Button>
          </>
        }
      >
        <form onSubmit={(e) => { e.preventDefault(); void submitPw(); }} className="space-y-4">
          <TextField
            id="server-password"
            name="new-password"
            label="New password"
            type={showServerPw ? 'text' : 'password'}
            value={serverPw}
            onChange={(e) => setServerPw(e.target.value)}
            autoComplete="new-password"
            disabled={busy}
            trailing={
              <button
                type="button"
                onClick={() => setShowServerPw((s) => !s)}
                aria-label={showServerPw ? 'Hide password' : 'Show password'}
                className="state-layer size-8 rounded-full inline-flex items-center justify-center cursor-pointer text-on-surface-variant hover:text-on-surface"
              >
                {showServerPw ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            }
          />
          <TextField
            id="server-password-repeat"
            name="confirm-password"
            label="Repeat password"
            type={showServerPw ? 'text' : 'password'}
            value={serverPw2}
            onChange={(e) => setServerPw2(e.target.value)}
            autoComplete="new-password"
            error={pwErr}
            disabled={busy}
          />
          <button type="submit" className="hidden" tabIndex={-1} />
        </form>
      </Dialog>

      <Dialog
        open={credOpen}
        onClose={closeCredDialog}
        title="Account credentials"
        actions={
          <>
            <Button variant="text" onClick={closeCredDialog} disabled={busy}>
              Cancel
            </Button>
            <Button onClick={() => void submitCred()} loading={busy} disabled={!credUsername.trim() || credPw.length < 6}>
              Save
            </Button>
          </>
        }
      >
        <form onSubmit={(e) => { e.preventDefault(); void submitCred(); }} className="space-y-4">
          <TextField
            id="cred-username"
            name="username"
            label="Username"
            value={credUsername}
            onChange={(e) => setCredUsername(e.target.value)}
            autoCapitalize="off"
            autoComplete="username"
            disabled={busy}
          />
          <TextField
            id="cred-password"
            name="password"
            label="Password"
            type={showCredPw ? 'text' : 'password'}
            value={credPw}
            onChange={(e) => setCredPw(e.target.value)}
            autoComplete="new-password"
            error={credErr}
            disabled={busy}
            trailing={
              <button
                type="button"
                onClick={() => setShowCredPw((s) => !s)}
                aria-label={showCredPw ? 'Hide password' : 'Show password'}
                className="state-layer size-8 rounded-full inline-flex items-center justify-center cursor-pointer text-on-surface-variant hover:text-on-surface"
              >
                {showCredPw ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            }
          />
          <button type="submit" className="hidden" tabIndex={-1} />
        </form>
      </Dialog>
    </SettingsPage>
  )
}
