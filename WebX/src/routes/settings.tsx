import React from 'react'
import { createFileRoute, Link, Outlet, useRouterState } from '@tanstack/react-router'
import { Palette, SlidersHorizontal, Server, User, Database, Keyboard, Info, ChevronRight, AudioLines, Plug, Smartphone } from 'lucide-react'
import { cn } from '@/lib/cn'
import { useIsDesktop } from '@/hooks/useMediaQuery'
import { useUiStore } from '@/stores/uiStore'
import { useAuthStore } from '@/stores/authStore'

export const Route = createFileRoute('/settings')({
  component: SettingsLayout,
})

export const SETTINGS_SECTIONS = [
  { to: '/settings/appearance', label: 'Appearance', description: 'Themes, colors, motion', icon: Palette },
  { to: '/settings/playback', label: 'Playback', description: 'Format, prefetch, speed', icon: SlidersHorizontal },
  { to: '/settings/equalizer', label: 'Equalizer', description: 'Bands and presets', icon: AudioLines },
  { to: '/settings/integrations', label: 'Integrations', description: 'Last.fm, Discord', icon: Plug },
  { to: '/settings/app', label: 'App & offline', description: 'Install, updates, storage', icon: Smartphone },
  { to: '/settings/server', label: 'Server & endpoints', description: 'Address, connection, demo', icon: Server },
  { to: '/settings/account', label: 'Account', description: 'Profile, session, password', icon: User },
  { to: '/settings/library', label: 'Library & data', description: 'Sync, cache, export', icon: Database },
  { to: '/settings/shortcuts', label: 'Keyboard shortcuts', description: 'Key bindings', icon: Keyboard },
  { to: '/settings/about', label: 'About', description: 'Version, credits', icon: Info },
] as const

function SettingsLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const isDesktop = useIsDesktop()
  const serverStatus = useUiStore((s) => s.serverStatus)
  const user = useAuthStore((s) => s.user)
  const atIndex = pathname === '/settings' || pathname === '/settings/'

  const list = (
    <nav aria-label="Settings sections" className={cn('shrink-0', isDesktop ? 'w-72 xl:w-80 sticky top-0 self-start' : 'w-full')}>
      {!isDesktop && <h1 className="type-headline-md text-on-surface mb-4 px-1">Settings</h1>}
      <ul className="space-y-1">
        {SETTINGS_SECTIONS.map((s) => {
          const active = pathname.startsWith(s.to)
          const Icon = s.icon
          const badge = s.to === '/settings/server' ? serverStatus : s.to === '/settings/account' ? (user ? 'user' : null) : null
          return (
            <li key={s.to}>
              <Link
                to={s.to}
                aria-current={active ? 'page' : undefined}
                className={cn(
                  'state-layer flex items-center gap-4 rounded-full px-4 h-14 outline-none transition-colors',
                  active ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface'
                )}
              >
                <Icon className={cn('size-6 shrink-0', active ? '' : 'text-on-surface-variant')} strokeWidth={active ? 2.2 : 1.8} />
                <span className="min-w-0 flex-1">
                  <span className="block type-title-sm truncate">{s.label}</span>
                  <span className={cn('block type-body-sm truncate', active ? 'text-on-secondary-container/80' : 'text-on-surface-variant')}>{s.description}</span>
                </span>
                {badge === 'online' && <span className="size-2 rounded-full bg-tertiary" title="Server online" />}
                {badge === 'offline' && <span className="size-2 rounded-full bg-error" title="Server offline" />}
                {!isDesktop && <ChevronRight className="size-5 text-on-surface-variant" />}
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )

  if (!isDesktop) {
    return <div className="p-4 pb-8">{atIndex ? list : <Outlet />}</div>
  }

  return (
    <div className="flex gap-8 xl:gap-12 p-6 lg:p-8 max-w-7xl mx-auto items-start">
      {list}
      <div className="flex-1 min-w-0">
        <Outlet />
      </div>
    </div>
  )
}
