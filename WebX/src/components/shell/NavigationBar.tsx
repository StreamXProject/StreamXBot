import React from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Home, Search, Library, Settings } from 'lucide-react'
import { cn } from '@/lib/cn'

const ITEMS = [
  { label: 'Home', to: '/', icon: Home, match: (p: string) => p === '/' || p.startsWith('/mix') },
  { label: 'Search', to: '/search', icon: Search, match: (p: string) => p.startsWith('/search') },
  { label: 'Library', to: '/library', icon: Library, match: (p: string) => p.startsWith('/library') || p.startsWith('/playlist') },
  { label: 'Settings', to: '/settings', icon: Settings, match: (p: string) => p.startsWith('/settings') },
]

/** MD3 navigation bar (compact windows) */
export const NavigationBar: React.FC = () => {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <nav className="md:hidden fixed inset-x-0 bottom-0 z-50 elev-2 bg-surface-container border-t border-outline-variant/40 flex items-stretch justify-around pb-[env(safe-area-inset-bottom,0px)]" aria-label="Primary">
      {ITEMS.map((it) => {
        const active = it.match(pathname)
        const Icon = it.icon
        return (
          <Link key={it.to} to={it.to} aria-current={active ? 'page' : undefined} className="group flex-1 flex flex-col items-center justify-center gap-1 h-16 py-1 outline-none">
            <span className={cn('state-layer flex items-center justify-center w-16 h-8 rounded-full transition-colors duration-200 ease-emphasized', active ? 'bg-primary-container text-on-primary-container' : 'text-on-surface-variant')}>
              <Icon className="size-6" strokeWidth={active ? 2.4 : 1.9} />
            </span>
            <span className={cn('type-label-md transition-colors', active ? 'text-primary font-bold' : 'text-on-surface-variant')}>{it.label}</span>
          </Link>
        )
      })}
    </nav>
  )
}
