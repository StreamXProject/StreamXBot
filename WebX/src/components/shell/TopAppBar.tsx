import React, { useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { Search, LogOut, Settings, User as UserIcon, ChevronLeft, Music2, Keyboard, Moon } from 'lucide-react'
import { useAuthStore, sessionKind } from '@/stores/authStore'
import { useUiStore } from '@/stores/uiStore'
import { Menu, IconButton } from '@/components/md3'
import { cn } from '@/lib/cn'
import { logoutServer } from '@/api/auth'
import { useIsDesktop } from '@/hooks/useMediaQuery'

export const UserAvatar: React.FC<{ name?: string | null; url?: string | null; className?: string }> = ({ name, url, className }) => {
  const [err, setErr] = useState(false)
  if (!url || err) {
    return (
      <div className={cn('rounded-full bg-primary-container text-on-primary-container flex items-center justify-center type-label-lg select-none', className)}>
        {(name || 'U').charAt(0).toUpperCase()}
      </div>
    )
  }
  return <img src={url} alt={name || 'User'} referrerPolicy="no-referrer" onError={() => setErr(true)} className={cn('rounded-full object-cover bg-surface-highest', className)} />
}

const TITLES: Array<[RegExp, string]> = [
  [/^\/$/, 'Home'],
  [/^\/search/, 'Search'],
  [/^\/library/, 'Library'],
  [/^\/explore\/albums/, 'Albums'],
  [/^\/explore\/artists/, 'Artists'],
  [/^\/settings/, 'Settings'],
]

export const TopAppBar: React.FC = () => {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const canGoBack = useRouterState({ select: (s) => s.location.state?.__TSR_index ? (s.location.state as { __TSR_index?: number }).__TSR_index! > 0 : window.history.length > 1 })
  const user = useAuthStore((s) => s.user)
  const token = useAuthStore((s) => s.token)
  const logout = useAuthStore((s) => s.logout)
  const kind = sessionKind(token, user)
  const setCommandPalette = useUiStore((s) => s.setCommandPalette)
  const setSleepTimerOpen = useUiStore((s) => s.setSleepTimerOpen)
  const setShortcutsOpen = useUiStore((s) => s.setShortcutsOpen)
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null)
  const isDesktop = useIsDesktop()

  const title = TITLES.find(([re]) => re.test(pathname))?.[1]
  const isDetail = !title

  return (
    <header
      className="bg-surface-container border-b border-outline-variant/40 h-[calc(3.5rem+env(safe-area-inset-top,0px))] md:h-[calc(var(--webx-topbar-height)+env(safe-area-inset-top,0px))] pt-[env(safe-area-inset-top,0px)] shrink-0 flex items-center gap-2 px-3 md:px-5 z-10"
      style={{ backdropFilter: 'none', WebkitBackdropFilter: 'none' }}
    >
      {/* Left: back on detail pages, brand on mobile */}
      <div className="flex items-center gap-1 min-w-0 md:w-1/4">
        {isDetail && canGoBack ? (
          <IconButton label="Back" onClick={() => window.history.back()}>
            <ChevronLeft />
          </IconButton>
        ) : (
          <span className="md:hidden size-9 rounded-md bg-primary text-on-primary flex items-center justify-center mr-1">
            <Music2 className="size-4" strokeWidth={2.5} />
          </span>
        )}
        <h1 className="type-title-lg text-on-surface truncate">{title ?? ''}</h1>
      </div>

      {/* Center: search bar → command palette */}
      <div className="flex-1 flex justify-center min-w-0">
        {pathname !== '/search' && (
          <button
            onClick={() => setCommandPalette(true)}
            className="state-layer w-full max-w-[560px] h-11 md:h-12 rounded-full bg-surface-high text-on-surface-variant flex items-center gap-3 px-4 text-left"
            aria-label="Search (Ctrl+K)"
          >
            <Search className="size-5 shrink-0" />
            <span className="type-body-lg truncate flex-1">Search tracks, albums, artists</span>
            {isDesktop && <kbd className="hidden lg:inline-flex h-6 px-1.5 items-center rounded-xs bg-surface-highest type-label-sm">Ctrl K</kbd>}
          </button>
        )}
      </div>

      {/* Right: status + account */}
      <div className="flex items-center justify-end gap-1 md:w-1/4">

        {kind === 'none' ? (
          <button onClick={() => navigate({ to: '/login' })} className="state-layer h-10 px-4 rounded-full bg-secondary-container text-on-secondary-container type-label-lg inline-flex items-center gap-2">
            <UserIcon className="size-4" /> Sign in
          </button>
        ) : (
          <button onClick={(e) => setMenuAnchor((curr) => (curr ? null : e.currentTarget))} aria-label="Account menu" className="state-layer size-10 rounded-full inline-flex items-center justify-center">
            <UserAvatar name={user?.name ?? 'Guest'} url={user?.profile_url || user?.photo_url || user?.avatarUrl} className="size-8" />
          </button>
        )}
      </div>

      <Menu
        open={Boolean(menuAnchor)}
        onClose={() => setMenuAnchor(null)}
        anchor={menuAnchor}
        align="end"
        header={
          <div className="flex items-center gap-3 py-1">
            <UserAvatar name={user?.name ?? 'Guest'} url={user?.profile_url || user?.photo_url || user?.avatarUrl} className="size-10" />
            <div className="min-w-0">
              <div className="type-title-sm truncate">{user?.name ?? 'Guest session'}</div>
              <div className="type-body-sm text-on-surface-variant truncate">{user?.username ? `@${user.username}` : kind === 'guest' ? 'Server password' : ''}</div>
            </div>
          </div>
        }
        items={[
          { id: 'account', label: 'Account', icon: <UserIcon />, onSelect: () => navigate({ to: '/settings/account' }) },
          { id: 'settings', label: 'Settings', icon: <Settings />, onSelect: () => navigate({ to: '/settings' }) },
          { id: 'sleep', label: 'Sleep timer', icon: <Moon />, onSelect: () => setSleepTimerOpen(true) },
          { id: 'keys', label: 'Keyboard shortcuts', icon: <Keyboard />, trailing: '?', onSelect: () => setShortcutsOpen(true) },
          { id: 'd', label: '', divider: true },
          {
            id: 'logout',
            label: 'Sign out',
            icon: <LogOut />,
            destructive: true,
            onSelect: async () => {
              await logoutServer()
              logout()
              navigate({ to: '/login' })
            },
          },
        ]}
      />
    </header>
  )
}
