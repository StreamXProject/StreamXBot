import React from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Home, Search, Library, Disc3, Users, Settings, Plus, PanelLeftClose, PanelLeftOpen, ListMusic, Heart, Music2 } from 'lucide-react'
import { RecapIcon } from '@/components/common/RecapIcon'
import { useUiStore } from '@/stores/uiStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { Artwork } from '@/components/common/Artwork'
import { cn } from '@/lib/cn'
import { useState } from 'react'
import { Dialog, Button, TextField } from '@/components/md3'

interface NavItem {
  label: string
  to: string
  icon: React.ComponentType<{ className?: string; strokeWidth?: number }>
  match?: (path: string) => boolean
}

const PRIMARY: NavItem[] = [
  { label: 'Home', to: '/', icon: Home, match: (p) => p === '/' || p.startsWith('/mix') },
  { label: 'Search', to: '/search', icon: Search },
  { label: 'Library', to: '/library', icon: Library, match: (p) => p.startsWith('/library') || p.startsWith('/playlist') },
  { label: 'Recaps', to: '/recaps', icon: RecapIcon, match: (p) => p.startsWith('/recap') },
]
const SECONDARY: NavItem[] = [
  { label: 'Albums', to: '/explore/albums', icon: Disc3, match: (p) => p.startsWith('/explore/albums') || p.startsWith('/album') },
  { label: 'Artists', to: '/explore/artists', icon: Users, match: (p) => p.startsWith('/explore/artists') || p.startsWith('/artist') },
]

const RailItem: React.FC<{ item: NavItem; active: boolean; expanded: boolean }> = ({ item, active, expanded }) => {
  const Icon = item.icon
  return (
    <Link
      to={item.to}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'group flex items-center outline-none',
        expanded ? 'h-14 px-4 gap-3 rounded-full mx-3' : 'flex-col justify-center gap-1 py-1 w-full'
      )}
    >
      <span
        className={cn(
          'state-layer relative flex items-center justify-center rounded-full transition-[background-color,width] duration-200 ease-emphasized',
          expanded ? 'size-6' : 'w-14 h-8',
          active ? (expanded ? 'text-primary' : 'bg-primary-container text-on-primary-container') : 'text-on-surface-variant group-hover:text-on-surface'
        )}
      >
        <Icon className="size-6" strokeWidth={active ? 2.4 : 1.9} />
      </span>
      <span className={cn('type-label-md transition-colors', expanded ? 'type-label-lg' : '', active ? 'text-primary font-bold' : 'text-on-surface-variant')}>
        {item.label}
      </span>
    </Link>
  )
}

export const NavigationRail: React.FC = () => {
  const collapsed = useUiStore((s) => s.sidebarCollapsed)
  const toggle = useUiStore((s) => s.toggleSidebar)
  const playlists = useLibraryStore((s) => s.playlists)
  const likedCount = useLibraryStore((s) => s.likedTracks.length)
  const createPlaylist = useLibraryStore((s) => s.createPlaylist)
  const railLabels = useSettingsStore((s) => s.railLabels)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const expanded = !collapsed
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')

  const isActive = (item: NavItem) => (item.match ? item.match(pathname) : pathname === item.to)

  return (
    <aside
      className={cn(
        'hidden md:flex flex-col shrink-0 h-full bg-surface z-20 transition-[width] duration-300 ease-emphasized border-r border-outline-variant/60',
        expanded ? 'w-[var(--webx-drawer-width)]' : 'w-[var(--webx-rail-width)]'
      )}
    >
      <div className={cn('flex items-center h-[calc(var(--webx-topbar-height)+env(safe-area-inset-top,0px))] pt-[env(safe-area-inset-top,0px)] shrink-0', expanded ? 'px-5 justify-between' : 'justify-center')}>
        {expanded && (
          <Link to="/" className="flex items-center gap-2.5 min-w-0">
            <span className="size-8 rounded-md bg-primary text-on-primary flex items-center justify-center shrink-0">
              <Music2 className="size-4" strokeWidth={2.5} />
            </span>
            <span className="type-title-lg tracking-tight text-on-surface">WebX</span>
          </Link>
        )}
        <button
          onClick={toggle}
          aria-label={expanded ? 'Collapse navigation' : 'Expand navigation'}
          className="state-layer size-10 rounded-full inline-flex items-center justify-center text-on-surface-variant"
        >
          {expanded ? <PanelLeftClose className="size-5" /> : <PanelLeftOpen className="size-5" />}
        </button>
      </div>

      <nav className={cn('flex flex-col gap-1 pt-2', !expanded && 'items-center')}>
        {PRIMARY.map((it) => <RailItem key={it.to} item={it} active={isActive(it)} expanded={expanded} />)}
        <div className={cn('my-2 border-t border-outline-variant', expanded ? 'mx-6' : 'w-8')} />
        {SECONDARY.map((it) => <RailItem key={it.to} item={it} active={isActive(it)} expanded={expanded} />)}
      </nav>

      <div className="flex-1 min-h-0 overflow-y-auto mt-2">
        {expanded ? (
          <div className="px-3 pb-3">
            <div className="flex items-center justify-between px-3 h-10">
              <span className="type-title-sm text-on-surface-variant">Your library</span>
              <button onClick={() => setCreating(true)} aria-label="New playlist" className="state-layer size-8 rounded-full inline-flex items-center justify-center text-on-surface-variant">
                <Plus className="size-4" />
              </button>
            </div>
            <Link
              to="/library"
              search={{ tab: 'liked' }}
              className={cn('state-layer flex items-center gap-3 h-12 px-3 rounded-full', pathname === '/library' && 'bg-secondary-container/60')}
            >
              <span className="size-8 rounded-sm bg-primary-container text-on-primary-container flex items-center justify-center"><Heart className="size-4 fill-current" /></span>
              <span className="min-w-0 flex-1">
                <span className="block type-body-md text-on-surface truncate">Liked songs</span>
                <span className="block type-body-sm text-on-surface-variant">{likedCount} tracks</span>
              </span>
            </Link>
            {playlists.map((pl) => (
              <Link key={pl.id} to="/playlist/$playlistId" params={{ playlistId: pl.id }} className="state-layer flex items-center gap-3 h-12 px-3 rounded-full">
                <Artwork src={pl.cover_url} collage={pl.thumbnails} alt="" kind="playlist" className="size-8 rounded-sm" />
                <span className="min-w-0 flex-1">
                  <span className="block type-body-md text-on-surface truncate">{pl.name}</span>
                  <span className="block type-body-sm text-on-surface-variant">Playlist</span>
                </span>
              </Link>
            ))}
            {playlists.length === 0 && (
              <p className="px-3 py-2 type-body-sm text-on-surface-variant">Playlists you create will appear here.</p>
            )}
          </div>
        ) : (
          <div className="flex flex-col items-center gap-1 pt-1">
            <Link to="/library" aria-label="Liked songs" className="state-layer size-12 rounded-full inline-flex items-center justify-center text-on-surface-variant">
              <Heart className="size-5" />
            </Link>
            {playlists.slice(0, 6).map((pl) => (
              <Link key={pl.id} to="/playlist/$playlistId" params={{ playlistId: pl.id }} title={pl.name} className="state-layer size-12 rounded-full inline-flex items-center justify-center">
                {pl.cover_url || pl.thumbnails.length ? <Artwork src={pl.cover_url} collage={pl.thumbnails} alt="" className="size-8 rounded-sm" /> : <ListMusic className="size-5 text-on-surface-variant" />}
              </Link>
            ))}
          </div>
        )}
      </div>

      <div className={cn('shrink-0 pb-3 flex flex-col', !expanded && 'items-center')}>
        <RailItem item={{ label: 'Settings', to: '/settings', icon: Settings, match: (p) => p.startsWith('/settings') }} active={pathname.startsWith('/settings')} expanded={expanded} />
        {!railLabels && null}
      </div>

      <Dialog
        open={creating}
        onClose={() => setCreating(false)}
        title="New playlist"
        actions={
          <>
            <Button variant="text" onClick={() => setCreating(false)}>Cancel</Button>
            <Button
              disabled={!name.trim()}
              onClick={async () => {
                await createPlaylist(name.trim())
                setName('')
                setCreating(false)
              }}
            >
              Create
            </Button>
          </>
        }
      >
        <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Dialog>
    </aside>
  )
}
