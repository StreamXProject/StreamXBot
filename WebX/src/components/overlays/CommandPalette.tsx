import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from '@tanstack/react-router'
import { Search, Home, Library, Settings, Palette, Disc3, Users, Play, ListPlus, Shuffle, Moon, Keyboard, Clock, ArrowRight, CornerDownLeft } from 'lucide-react'
import { useUiStore } from '@/stores/uiStore'
import { useQueueStore } from '@/stores/queueStore'
import { useDebounce } from '@/hooks/useDebounce'
import { useSearch } from '@/hooks/useQueries'
import { Artwork } from '@/components/common/Artwork'
import { fetchShuffle } from '@/api/browse'
import { cn } from '@/lib/cn'
import type { Track } from '@/schemas/track'

interface Cmd {
  id: string
  label: string
  hint?: string
  icon: React.ReactNode
  run: () => void
  kind: 'nav' | 'action' | 'track' | 'album' | 'artist'
  track?: Track
}

const RECENT_KEY = 'webx.search.recent'
const readRecent = (): string[] => {
  try {
    return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  } catch {
    return []
  }
}
export const pushRecentSearch = (q: string) => {
  const list = [q, ...readRecent().filter((x) => x !== q)].slice(0, 8)
  localStorage.setItem(RECENT_KEY, JSON.stringify(list))
}

export const CommandPalette: React.FC = () => {
  const open = useUiStore((s) => s.commandPaletteOpen)
  const setOpen = useUiStore((s) => s.setCommandPalette)
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const debounced = useDebounce(q, 180)
  const { data, isFetching } = useSearch(open ? debounced : '')

  useEffect(() => {
    if (open) {
      setQ('')
      setActive(0)
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  const close = () => setOpen(false)
  const go = (to: string) => {
    close()
    navigate({ to } as never)
  }

  const commands = useMemo<Cmd[]>(() => {
    const ui = useUiStore.getState()
    const queue = useQueueStore.getState()
    const base: Cmd[] = [
      { id: 'home', label: 'Home', icon: <Home />, kind: 'nav', run: () => go('/') },
      { id: 'library', label: 'Your library', icon: <Library />, kind: 'nav', run: () => go('/library') },
      { id: 'albums', label: 'Browse albums', icon: <Disc3 />, kind: 'nav', run: () => go('/explore/albums') },
      { id: 'artists', label: 'Browse artists', icon: <Users />, kind: 'nav', run: () => go('/explore/artists') },
      { id: 'settings', label: 'Settings', icon: <Settings />, kind: 'nav', run: () => go('/settings') },
      { id: 'appearance', label: 'Appearance & themes', icon: <Palette />, kind: 'nav', run: () => go('/settings/appearance') },
      {
        id: 'shuffle',
        label: 'Shuffle my library',
        hint: 'Play 100 random tracks',
        icon: <Shuffle />,
        kind: 'action',
        run: async () => {
          close()
          const t = await fetchShuffle(100)
          if (t.length) void queue.playTrackWithQueue(t, 0, { type: 'radio', title: 'Library shuffle' })
        },
      },
      { id: 'sleep', label: 'Sleep timer', icon: <Moon />, kind: 'action', run: () => { close(); ui.setSleepTimerOpen(true) } },
      { id: 'shortcuts', label: 'Keyboard shortcuts', icon: <Keyboard />, kind: 'action', run: () => { close(); ui.setShortcutsOpen(true) } },
    ]
    const lq = q.trim().toLowerCase()
    const filtered = lq ? base.filter((c) => c.label.toLowerCase().includes(lq) || c.hint?.toLowerCase().includes(lq)) : base

    const results: Cmd[] = []
    if (lq && data) {
      for (const t of data.tracks.slice(0, 6)) {
        results.push({
          id: `t-${t.id}`,
          label: t.title,
          hint: t.artist,
          icon: <Artwork src={t.cover_url} alt="" className="size-8 rounded-xs" />,
          kind: 'track',
          track: t,
          run: () => {
            close()
            pushRecentSearch(q.trim())
            void queue.playTrackWithQueue(data.tracks, data.tracks.indexOf(t), { type: 'search', title: `Search “${q.trim()}”` })
          },
        })
      }
      for (const a of data.albums.slice(0, 3)) {
        results.push({ id: `a-${a.id}`, label: a.title, hint: `Album · ${a.artist}`, icon: <Artwork src={a.cover_url} alt="" kind="album" className="size-8 rounded-xs" />, kind: 'album', run: () => { close(); navigate({ to: '/album/$albumId', params: { albumId: a.id } }) } })
      }
      for (const ar of data.artists.slice(0, 3)) {
        results.push({ id: `ar-${ar.id}`, label: ar.name, hint: 'Artist', icon: <Artwork src={ar.avatar_url} alt="" kind="artist" className="size-8" />, kind: 'artist', run: () => { close(); navigate({ to: '/artist/$artistId', params: { artistId: ar.id } }) } })
      }
      if (data.total > 0) {
        results.push({ id: 'all', label: `See all results for “${q.trim()}”`, hint: `${data.total} tracks`, icon: <ArrowRight />, kind: 'nav', run: () => { close(); pushRecentSearch(q.trim()); navigate({ to: '/search', search: { q: q.trim() } }) } })
      }
    }
    return [...results, ...filtered]
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, data])

  const recent = useMemo(() => (open && !q ? readRecent() : []), [open, q])

  useEffect(() => setActive(0), [commands.length, q])
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(commands.length - 1, a + 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(0, a - 1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const c = commands[active]
      if (c) c.run()
      else if (q.trim()) {
        close()
        pushRecentSearch(q.trim())
        navigate({ to: '/search', search: { q: q.trim() } })
      }
    } else if (e.key === 'Escape') close()
  }

  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [active])

  if (!open || typeof document === 'undefined') return null

  const groups: Array<[string, Cmd[]]> = []
  const tracks = commands.filter((c) => c.kind === 'track')
  const media = commands.filter((c) => c.kind === 'album' || c.kind === 'artist')
  const nav = commands.filter((c) => c.kind === 'nav' && !c.id.startsWith('all'))
  const actions = commands.filter((c) => c.kind === 'action')
  const all = commands.filter((c) => c.id === 'all')
  if (tracks.length) groups.push(['Tracks', tracks])
  if (media.length) groups.push(['Albums & artists', media])
  if (all.length) groups.push(['', all])
  if (nav.length) groups.push(['Go to', nav])
  if (actions.length) groups.push(['Actions', actions])

  return createPortal(
    <div className="fixed inset-0 z-[92] flex items-start justify-center pt-[10vh] px-4" role="presentation">
      <div className="absolute inset-0 bg-scrim/50" onClick={close} />
      <div role="dialog" aria-modal className="relative w-full max-w-xl elev-3 rounded-2xl shadow-md3-3 overflow-hidden page-enter flex flex-col max-h-[70vh]">
        <div className="flex items-center gap-3 px-4 h-14 border-b border-outline-variant">
          {isFetching ? <span className="size-5 border-2 border-primary/30 border-t-primary rounded-full animate-spin" /> : <Search className="size-5 text-on-surface-variant" />}
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onKey}
            placeholder="Search tracks, albums, artists, or type a command…"
            className="flex-1 bg-transparent outline-none type-body-lg text-on-surface placeholder:text-on-surface-variant/70"
            autoComplete="off"
            spellCheck={false}
          />
          <kbd className="hidden sm:inline-flex h-6 px-1.5 items-center rounded-xs bg-surface-highest text-on-surface-variant type-label-sm">Esc</kbd>
        </div>
        <div ref={listRef} className="overflow-y-auto py-2">
          {recent.length > 0 && (
            <div className="px-2 pb-2">
              <p className="px-3 py-1 type-label-md text-on-surface-variant">Recent searches</p>
              <div className="flex flex-wrap gap-1.5 px-3 pt-1">
                {recent.map((r) => (
                  <button key={r} onClick={() => setQ(r)} className="state-layer inline-flex items-center gap-1.5 h-8 px-3 rounded-sm border border-outline-variant type-label-lg text-on-surface-variant">
                    <Clock className="size-3.5" /> {r}
                  </button>
                ))}
              </div>
            </div>
          )}
          {groups.map(([title, items]) => (
            <div key={title || 'all'} className="px-2">
              {title && <p className="px-3 py-1 type-label-md text-on-surface-variant">{title}</p>}
              {items.map((c) => {
                const idx = commands.indexOf(c)
                const isActive = idx === active
                return (
                  <button
                    key={c.id}
                    data-index={idx}
                    onMouseEnter={() => setActive(idx)}
                    onClick={() => c.run()}
                    className={cn('w-full flex items-center gap-3 h-12 px-3 rounded-sm text-left transition-colors [&>svg]:size-5', isActive ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface')}
                  >
                    <span className="shrink-0 w-8 flex items-center justify-center text-on-surface-variant [&_svg]:size-5">{c.icon}</span>
                    <span className="flex-1 min-w-0">
                      <span className="block truncate type-body-lg">{c.label}</span>
                      {c.hint && <span className="block truncate type-body-sm text-on-surface-variant">{c.hint}</span>}
                    </span>
                    {c.kind === 'track' && <Play className="size-4 opacity-60" />}
                    {isActive && c.kind !== 'track' && <CornerDownLeft className="size-4 opacity-60" />}
                  </button>
                )
              })}
            </div>
          ))}
          {q.trim() && !isFetching && commands.length === 0 && (
            <p className="px-5 py-8 text-center type-body-md text-on-surface-variant">No matches. Press Enter to search the full catalog.</p>
          )}
        </div>
        <div className="flex items-center gap-4 px-4 h-9 border-t border-outline-variant type-label-sm text-on-surface-variant">
          <span><kbd className="font-mono">↑↓</kbd> navigate</span>
          <span><kbd className="font-mono">↵</kbd> select</span>
          <span className="ml-auto flex items-center gap-1"><ListPlus className="size-3.5" /> Right-click any track for more</span>
        </div>
      </div>
    </div>,
    document.body
  )
}
