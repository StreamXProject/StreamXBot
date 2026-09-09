import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, Link } from '@tanstack/react-router'
import { ChevronDown, Heart, Mic2, ListMusic, Info, MoreHorizontal, Moon, Gauge, Download, Disc3, User, ListPlus, Share2, Radio } from 'lucide-react'
import { usePlayerStore } from '@/stores/playerStore'
import { useQueueStore } from '@/stores/queueStore'
import { useUiStore, type FullPlayerPane } from '@/stores/uiStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { useThemeStore } from '@/theme/themeStore'
import { Scrubber } from './Scrubber'
import { PlaybackControls } from './PlaybackControls'
import { VolumeControl } from './VolumeControl'
import { LyricsView } from './LyricsView'
import { QueueList } from './QueueList'
import { AmbientBackdrop } from './AmbientBackdrop'
import { Artwork } from '@/components/common/Artwork'
import { QualityBadge } from '@/components/common/QualityBadge'
import { IconButton, SegmentedButton, Menu, type MenuItem } from '@/components/md3'
import { useIsDesktop } from '@/hooks/useMediaQuery'
import { getDownloadUrl } from '@/api/stream'
import { formatDuration, formatQuality } from '@/lib/format'
import { cn } from '@/lib/cn'
import type { Track } from '@/schemas/track'

const PANES: Array<{ value: FullPlayerPane; label: string; icon: React.ReactNode }> = [
  { value: 'lyrics', label: 'Lyrics', icon: <Mic2 /> },
  { value: 'queue', label: 'Up next', icon: <ListMusic /> },
  { value: 'info', label: 'Details', icon: <Info /> },
]

const DetailsPane: React.FC<{ track: Track }> = ({ track }) => {
  const rows: Array<[string, React.ReactNode]> = [
    ['Title', track.title],
    ['Artist', track.artist],
    ['Album', track.album || '—'],
    ['Year', track.year || '—'],
    ['Duration', formatDuration(track.duration_sec)],
    ['Format', formatQuality(track.type, track.sampling_rate_hz) || '—'],
    ['Bit depth', track.bit_depth ? `${track.bit_depth}-bit` : '—'],
    ['Bitrate', track.bitrate_kbps ? `${track.bitrate_kbps} kbps` : '—'],
    ['Source', track.topic_name || (track.source_chat_id ? `Channel ${track.source_chat_id}` : '—')],
  ]
  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-3">
        {rows.map(([k, v]) => (
          <React.Fragment key={k}>
            <dt className="type-label-lg text-on-surface-variant">{k}</dt>
            <dd className="type-body-lg text-on-surface break-words">{v}</dd>
          </React.Fragment>
        ))}
      </dl>
      {track.spotify_url && (
        <a href={track.spotify_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-2 mt-6 h-10 px-4 rounded-full border border-outline text-primary type-label-lg state-layer">
          Open on Spotify
        </a>
      )}
    </div>
  )
}

/**
 * Full-screen "Now Playing".
 *
 * Perf design:
 *  - Always mounted after the first track; open/close is a pure CSS transform
 *    on a `contain: paint` layer (no React mount/unmount storm).
 *  - The ambient backdrop blurs a 128px image, not the viewport.
 *  - Time-based UI (scrubber, lyrics) subscribes to the engine directly and
 *    re-renders only when the visible value changes.
 *  - While open, the app shell behind is made `inert` and hidden after the
 *    transition so it stops painting.
 */
export const FullScreenPlayer: React.FC = () => {
  const open = useUiStore((s) => s.fullPlayerOpen)
  const close = useUiStore((s) => s.closeFullPlayer)
  const pane = useUiStore((s) => s.fullPlayerPane)
  const paneExplicit = useUiStore((s) => s.fullPlayerPaneExplicit)
  const setPane = useUiStore((s) => s.setFullPlayerPane)
  const openAddToPlaylist = useUiStore((s) => s.openAddToPlaylist)
  const setSleepTimerOpen = useUiStore((s) => s.setSleepTimerOpen)
  const track = usePlayerStore((s) => s.currentTrack)
  const isPlaying = usePlayerStore((s) => s.isPlaying)
  const playbackRate = usePlayerStore((s) => s.playbackRate)
  const setPlaybackRate = usePlayerStore((s) => s.setPlaybackRate)
  const sleepAt = usePlayerStore((s) => s.sleepAt)
  const context = useQueueStore((s) => s.context)
  const upNextCount = useQueueStore((s) => Math.max(0, s.queue.length - s.currentIndex - 1))
  const isLiked = useLibraryStore((s) => (track ? s.likedIds.has(track.id) : false))
  const toggleLike = useLibraryStore((s) => s.toggleLike)
  const ambient = useThemeStore((s) => s.activeTheme.effects.ambientBackdrop)
  const dynamicColor = useThemeStore((s) => s.dynamicColor)
  const setDynamicSeedFromImage = useThemeStore((s) => s.setDynamicSeedFromImage)
  const isDesktop = useIsDesktop()
  const navigate = useNavigate()
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null)
  const [mobilePane, setMobilePane] = useState<'player' | FullPlayerPane>('player')
  const rootRef = useRef<HTMLDivElement>(null)
  const drag = useRef<{ startY: number; dy: number; active: boolean }>({ startY: 0, dy: 0, active: false })
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    if (track && !mounted) setMounted(true)
  }, [track, mounted])

  // Dynamic colour from artwork ("Material You")
  useEffect(() => {
    if (!dynamicColor) return
    void setDynamicSeedFromImage(track?.cover_url ?? null)
  }, [dynamicColor, track?.cover_url, setDynamicSeedFromImage])

  // On compact windows start on the artwork unless a pane was explicitly requested (lyrics / queue buttons)
  useEffect(() => {
    if (open) setMobilePane(paneExplicit && pane !== 'info' ? pane : 'player')
  }, [open, pane, paneExplicit])

  /* Drag-to-dismiss (touch / pen) */
  const onPointerDown = (e: React.PointerEvent) => {
    if (e.pointerType === 'mouse') return
    drag.current = { startY: e.clientY, dy: 0, active: true }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    if (rootRef.current) rootRef.current.style.transition = 'none'
  }
  const onPointerMove = (e: React.PointerEvent) => {
    if (!drag.current.active || !rootRef.current) return
    const dy = Math.max(0, e.clientY - drag.current.startY)
    drag.current.dy = dy
    rootRef.current.style.transform = `translate3d(0, ${dy}px, 0)`
    rootRef.current.style.opacity = String(1 - Math.min(0.5, dy / 800))
  }
  const endDrag = useCallback(() => {
    if (!drag.current.active || !rootRef.current) return
    drag.current.active = false
    const el = rootRef.current
    el.style.transition = ''
    el.style.transform = ''
    el.style.opacity = ''
    if (drag.current.dy > 140) close()
  }, [close])

  if (!mounted || !track) return null

  const menuItems: MenuItem[] = [
    { id: 'playlist', label: 'Add to playlist…', icon: <ListPlus />, onSelect: () => openAddToPlaylist([track]) },
    { id: 'sleep', label: sleepAt ? 'Sleep timer (on)' : 'Sleep timer', icon: <Moon />, onSelect: () => setSleepTimerOpen(true) },
    {
      id: 'rate',
      label: `Speed · ${playbackRate.toFixed(2).replace(/\.?0+$/, '')}×`,
      icon: <Gauge />,
      onSelect: () => {
        const rates = [0.75, 1, 1.25, 1.5, 2]
        setPlaybackRate(rates[(rates.indexOf(playbackRate) + 1) % rates.length] ?? 1)
      },
    },
    { id: 'd1', label: '', divider: true },
    ...(track.album_id ? [{ id: 'album', label: 'Go to album', icon: <Disc3 />, onSelect: () => { close(); navigate({ to: '/album/$albumId', params: { albumId: track.album_id! } }) } }] : []),
    ...(track.artist_id ? [{ id: 'artist', label: 'Go to artist', icon: <User />, onSelect: () => { close(); navigate({ to: '/artist/$artistId', params: { artistId: track.artist_id! } }) } }] : []),
    { id: 'download', label: 'Download', icon: <Download />, onSelect: () => window.open(getDownloadUrl(track.id), '_blank', 'noopener') },
    ...('share' in navigator ? [{ id: 'share', label: 'Share…', icon: <Share2 />, onSelect: () => navigator.share({ title: track.title, text: `${track.title} — ${track.artist}` }).catch(() => {}) }] : []),
  ]

  const showPlayer = isDesktop || mobilePane === 'player'
  const activePane: FullPlayerPane = isDesktop ? pane : mobilePane === 'player' ? 'lyrics' : mobilePane
  const paneOptions = PANES.map((p) => (p.value === 'queue' && upNextCount > 0 ? { ...p, label: `Up next · ${upNextCount}` } : p))

  const TitleBlock = (
    <div className="min-w-0 flex-1">
      <h1 className={cn('text-on-surface truncate', isDesktop ? 'type-headline-md' : 'type-headline-sm')}>{track.title}</h1>
      <p className="type-body-lg text-on-surface-variant truncate mt-0.5">
        {track.artist_id ? (
          <Link to="/artist/$artistId" params={{ artistId: track.artist_id }} onClick={close} className="hover:underline hover:text-on-surface">{track.artist}</Link>
        ) : (
          track.artist
        )}
      </p>
      <p className="mt-1.5 flex items-center gap-2 min-w-0 type-body-sm text-on-surface-variant/80">
        <QualityBadge type={track.type} hz={track.sampling_rate_hz} verbose />
        {(track.album || track.year) && (
          <span className="truncate">
            {track.album_id && track.album ? (
              <Link to="/album/$albumId" params={{ albumId: track.album_id }} onClick={close} className="hover:underline">{track.album}</Link>
            ) : (
              track.album
            )}
            {track.album && track.year ? ' · ' : ''}
            {track.year || ''}
          </span>
        )}
      </p>
    </div>
  )

  const LikeButton = (
    <IconButton label={isLiked ? 'Remove from favourites' : 'Add to favourites'} size="lg" variant="tonal" selected={isLiked} onClick={() => void toggleLike(track)} className={cn(isLiked && 'text-primary')}>
      <Heart className={cn(isLiked && 'fill-current')} />
    </IconButton>
  )

  return (
    <div
      ref={rootRef}
      className="fullplayer fixed inset-0 z-50 bg-surface text-on-surface flex flex-col select-none"
      data-open={open}
      role="dialog"
      aria-modal={open}
      aria-label="Now playing"
      aria-hidden={!open}
      // @ts-expect-error React 19 supports inert
      inert={open ? undefined : ''}
    >
      <AmbientBackdrop src={track.cover_url} enabled={ambient && open} />

      {/* Top bar / drag handle */}
      <header
        className="relative z-10 flex items-center justify-between gap-2 px-3 sm:px-5 h-16 shrink-0 touch-none"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
      >
        <IconButton label="Minimize" size="lg" onClick={close}>
          <ChevronDown />
        </IconButton>
        <div className="flex-1 min-w-0 flex justify-center px-2">
          <div className="inline-flex items-center gap-2 h-9 px-3.5 rounded-full glass border border-outline-variant/40 max-w-full">
            <Radio className="size-4 text-primary shrink-0" />
            <span className="type-label-md text-on-surface-variant shrink-0">Playing from</span>
            <span className="type-label-lg text-on-surface truncate">{context?.title ?? track.album ?? 'Your library'}</span>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <IconButton label="Add to playlist" size="lg" onClick={() => openAddToPlaylist([track])} className="hidden sm:inline-flex">
            <ListPlus />
          </IconButton>
          <IconButton label="More" size="lg" onClick={(e) => setMenuAnchor(e.currentTarget)}>
            <MoreHorizontal />
          </IconButton>
        </div>
        <div className="md:hidden absolute left-1/2 -translate-x-1/2 top-1.5 w-9 h-1 rounded-full bg-on-surface/25" />
      </header>

      {/* Compact / tablet: pane switcher lives right under the header */}
      {!isDesktop && (
        <div className="relative z-10 flex justify-center px-4 pb-2 shrink-0">
          <SegmentedButton
            size="sm"
            showCheck={false}
            value={mobilePane}
            onChange={(v) => setMobilePane(v)}
            options={[
              { value: 'player' as const, label: 'Playing' },
              { value: 'lyrics' as const, label: 'Lyrics', icon: <Mic2 /> },
              { value: 'queue' as const, label: upNextCount > 0 ? `Queue · ${upNextCount}` : 'Queue', icon: <ListMusic /> },
            ]}
          />
        </div>
      )}

      {/* Body */}
      <main
        className={cn(
          'relative z-10 flex-1 min-h-0 w-full max-w-[1440px] mx-auto px-5 sm:px-8',
          isDesktop ? 'grid grid-cols-[minmax(360px,42%)_minmax(0,1fr)] gap-10 xl:gap-16 items-center pb-8' : 'flex flex-col'
        )}
        style={isDesktop ? undefined : { paddingBottom: 'max(env(safe-area-inset-bottom, 0px), 20px)' }}
      >
        {/* Left: artwork + controls */}
        <section className={cn('flex flex-col justify-center min-h-0 w-full', isDesktop ? 'max-w-[540px] mx-auto' : 'flex-1', !showPlayer && 'hidden')}>
          <div className={cn('mx-auto w-full min-h-0', isDesktop ? 'max-w-[460px]' : 'flex-1 flex items-center justify-center py-2')}>
            <div
              className={cn('np-art relative', isDesktop ? 'w-full' : 'max-h-full max-w-full')}
              data-playing={isPlaying}
              style={isDesktop ? undefined : { width: 'min(78vw, 50vh, 480px)' }}
            >
              <Artwork src={track.cover_url} alt={track.title} priority className="aspect-square w-full rounded-xl" />
            </div>
          </div>

          <div className="mt-6 sm:mt-8 flex items-center gap-3 shrink-0">
            {TitleBlock}
            {LikeButton}
          </div>

          <Scrubber size="lg" className="mt-5 shrink-0" />
          <PlaybackControls size="lg" className="mt-3 shrink-0" />

          {/* Secondary row: volume + session chips */}
          <div className="mt-4 sm:mt-5 shrink-0 hidden sm:flex items-center justify-between gap-4">
            <VolumeControl sliderClassName="w-36 lg:w-44" />
            <div className="flex items-center gap-2">
              {sleepAt && (
                <button onClick={() => setSleepTimerOpen(true)} className="state-layer inline-flex items-center gap-1.5 h-8 px-3 rounded-full bg-secondary-container text-on-secondary-container type-label-md">
                  <Moon className="size-3.5" /> {Math.max(1, Math.round((sleepAt - Date.now()) / 60000))}m
                </button>
              )}
              {playbackRate !== 1 && (
                <span className="inline-flex items-center gap-1.5 h-8 px-3 rounded-full bg-secondary-container text-on-secondary-container type-label-md">
                  <Gauge className="size-3.5" /> {playbackRate}×
                </span>
              )}
              {!isDesktop && (
                <IconButton label="Add to playlist" size="md" onClick={() => openAddToPlaylist([track])}>
                  <ListPlus />
                </IconButton>
              )}
            </div>
          </div>
        </section>

        {/* Right: panes */}
        <section className={cn('min-h-0 flex flex-col', isDesktop ? 'h-[min(78vh,860px)] self-center' : 'flex-1', showPlayer && !isDesktop && 'hidden')}>
          <div className="fullplayer-panel flex-1 min-h-0 flex flex-col rounded-2xl glass border border-outline-variant/40 overflow-hidden">
            {isDesktop && (
              <div className="flex items-center justify-between gap-3 px-4 pt-4 pb-3 shrink-0 border-b border-outline-variant/30">
                <SegmentedButton value={pane} onChange={setPane} options={paneOptions} showCheck={false} size="sm" />
              </div>
            )}
            {activePane === 'lyrics' && <LyricsView trackId={track.id} className="flex-1" />}
            {activePane === 'queue' && <QueueList className="flex-1 pt-2" />}
            {activePane === 'info' && <DetailsPane track={track} />}
          </div>
          {!isDesktop && (
            <div className="shrink-0 pt-3">
              <Scrubber size="md" />
              <PlaybackControls size="sm" className="mt-1" />
            </div>
          )}
        </section>
      </main>

      <Menu open={Boolean(menuAnchor)} onClose={() => setMenuAnchor(null)} anchor={menuAnchor} align="end" items={menuItems} />
    </div>
  )
}
