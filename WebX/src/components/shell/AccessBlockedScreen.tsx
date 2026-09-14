import React, { useState } from 'react'
import { Lock, ExternalLink, RefreshCw, LogOut, Users } from 'lucide-react'
import { Button } from '@/components/md3'
import { useAuthStore, type AccessBlock } from '@/stores/authStore'
import { verifyMembership } from '@/api/access'
import { logoutServer } from '@/api/auth'
import { toast } from '@/stores/uiStore'
import { useQueryClient } from '@tanstack/react-query'

/**
 * Full-viewport gate shown when the server refuses the session:
 *  - account_locked       → reason + sign out
 *  - membership_required  → join buttons for each required chat + "I've joined — verify"
 */
export const AccessBlockedScreen: React.FC<{ block: AccessBlock }> = ({ block }) => {
  const setAccessBlock = useAuthStore((s) => s.setAccessBlock)
  const logout = useAuthStore((s) => s.logout)
  const qc = useQueryClient()
  const [checking, setChecking] = useState(false)
  const locked = block.code === 'account_locked'

  const verify = async () => {
    setChecking(true)
    try {
      const res = await verifyMembership()
      if (res.ok) {
        setAccessBlock(null)
        await qc.invalidateQueries()
        toast('Welcome back — access restored')
      } else {
        toast('Still not a member of every required chat', { variant: 'error' })
        setAccessBlock({ ...block, required_chats: res.required_chats.length ? res.required_chats : block.required_chats })
      }
    } catch (e) {
      toast((e as Error).message, { variant: 'error' })
    } finally {
      setChecking(false)
    }
  }

  const signOut = async () => {
    await logoutServer().catch(() => {})
    logout()
    setAccessBlock(null)
  }

  return (
    <div className="fixed inset-0 z-[60] bg-surface text-on-surface flex items-center justify-center p-6 overflow-y-auto">
      <div className="w-full max-w-md rounded-3xl bg-surface-low p-6 sm:p-8 space-y-6">
        <div className="flex items-center gap-4">
          <span className={locked ? 'size-14 rounded-2xl bg-error-container text-on-error-container flex items-center justify-center shrink-0' : 'size-14 rounded-2xl bg-primary-container text-on-primary-container flex items-center justify-center shrink-0'}>
            {locked ? <Lock className="size-7" /> : <Users className="size-7" />}
          </span>
          <div className="min-w-0">
            <h1 className="type-headline-sm text-on-surface">{locked ? 'Account locked' : 'Membership required'}</h1>
            <p className="type-body-md text-on-surface-variant mt-0.5">{block.message}</p>
          </div>
        </div>

        {!locked && (
          <div className="space-y-2">
            {(block.required_chats.length ? block.required_chats : [{ title: 'the required Telegram chat', invite_link: null }]).map((c, i) => (
              <div key={i} className="flex items-center justify-between gap-3 h-14 px-4 rounded-2xl bg-surface-container">
                <span className="type-body-lg text-on-surface truncate">{c.title || 'Required chat'}</span>
                {c.invite_link ? (
                  <a href={c.invite_link} target="_blank" rel="noreferrer" className="state-layer inline-flex items-center gap-1.5 h-9 px-3 rounded-full bg-primary text-on-primary type-label-lg shrink-0">
                    Join <ExternalLink className="size-4" />
                  </a>
                ) : c.is_private ? (
                  <span className="type-label-sm text-on-surface-variant px-3 py-1 rounded-full bg-surface-container-highest shrink-0">
                    Private group · Invite only
                  </span>
                ) : (
                  <span className="type-label-sm text-outline px-3 py-1 rounded-full bg-surface-container-highest shrink-0" title="Bot lacks admin access to export invite link">
                    No invite link · Ask admin
                  </span>
                )}
              </div>
            ))}
          </div>
        )}

        {locked && block.reason && block.reason !== block.message && <p className="type-body-sm text-on-surface-variant text-center">Reason: {block.reason}</p>}

        <div className="flex flex-wrap gap-2 justify-center">
          <Button variant="text" icon={<LogOut />} onClick={() => void signOut()}>Sign out</Button>
          {!locked && <Button icon={<RefreshCw />} loading={checking} onClick={() => void verify()}>I’ve joined — verify</Button>}
          {locked && <Button variant="tonal" icon={<RefreshCw />} loading={checking} onClick={async () => { setChecking(true); try { await qc.invalidateQueries(); setAccessBlock(null) } finally { setChecking(false) } }}>Try again</Button>}
        </div>
        <p className="type-body-sm text-on-surface-variant/70 text-center">{locked ? 'If you think this is a mistake, contact the server administrator on Telegram.' : 'Access is restored automatically as soon as you rejoin.'}</p>
      </div>
    </div>
  )
}
