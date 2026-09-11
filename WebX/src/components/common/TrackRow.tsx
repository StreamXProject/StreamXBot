import React, { useCallback } from 'react'
import { useLongPress } from '@/hooks/useLongPress'
import { Play, Heart, MoreHorizontal } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import type { Track } from '@/schemas/track'
import { Artwork } from './Artwork'
import { NowPlayingBars } from './NowPlayingBars'
import { QualityBadge } from './QualityBadge'
import { usePlayerStore } from '@/stores/playerStore'
import { useLibraryStore } from '@/stores/libraryStore'
import { useUiStore } from '@/stores/uiStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { formatDuration } from '@/lib/format'
import { cn } from '@/lib/cn'

interface TrackRowProps {
  track: Track
  index: number
  /** All tracks in this list, used for "play next" context and context menu */
  tracks?: Track[]
  onPlay?: (track: Track, index: number) => void
  showAlbum?: boolean
  showCover?: boolean
  showIndex?: boolean
  dense?: boolean
  /** Extra trailing action (e.g. remove from playlist) */
  trailing?: React.ReactNode
  className?: string
}

export const TrackRow: React.FC<TrackRowProps> = React.memo(
  ({ track, index, tracks, onPlay, showAlbum = true, showCover = true, showIndex = true, dense, trailing, className }) => {
    const isCurrent = usePlayerStore((s) => s.currentTrack?.id === track.id)
    const isPlaying = usePlayerStore((s) => s.isPlaying && s.currentTrack?.id === track.id)
    const isLiked = useLibraryStore((s) => s.likedIds.has(track.id))
    const toggleLike = useLibraryStore((s) => s.toggleLike)
    const openContextMenu = useUiStore((s) => s.openContextMenu)
    const showQuality = useSettingsStore((s) => s.showQualityBadges)
    const compact = useSettingsStore((s) => s.compactRows) || dense

    const play = useCallback(
      (e?: React.SyntheticEvent) => {
        e?.stopPropagation()
        if (onPlay) onPlay(track, index)
        else void usePlayerStore.getState().playTrack(track)
      },
      [onPlay, track, index]
    )

    const openMenu = (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      openContextMenu({ track, tracks, index, anchor: e.currentTarget instanceof HTMLButtonElement ? e.currentTarget : { x: e.clientX, y: e.clientY } })
    }
    const longPress = useLongPress(
      useCallback((pt) => openContextMenu({ track, tracks, index, anchor: { x: pt.x, y: pt.y } }), [openContextMenu, track, tracks, index])
    )

    return (
      <div
        role="button"
        tabIndex={0}
        onClick={(e) => {
          if (!longPress.swallowClick(e)) play(e)
        }}
        onDoubleClick={play}
        onContextMenu={openMenu}
        {...longPress.handlers}
        onKeyDown={(e) => {
          if (e.key === 'Enter') play(e)
        }}
        className={cn(
          'group state-layer grid items-center gap-3 rounded-md cursor-pointer select-none text-on-surface outline-none',
          compact ? 'h-12 px-2' : 'h-14 px-3',
          showAlbum ? 'grid-cols-[auto_minmax(0,1fr)_auto]' : 'grid-cols-[auto_minmax(0,1fr)_auto]',
          isCurrent && 'bg-secondary-container/40',
          className
        )}
        aria-current={isCurrent ? 'true' : undefined}
      >
        {/* index / now playing / hover play */}
        <div className="flex items-center gap-3">
          {showIndex && (
            <div className="w-6 flex items-center justify-center shrink-0 tabular text-on-surface-variant type-body-sm">
              {isCurrent ? (
                <NowPlayingBars playing={isPlaying} />
              ) : (
                <>
                  <span className="group-hover:hidden group-focus-visible:hidden">{index + 1}</span>
                  <Play className="hidden group-hover:block group-focus-visible:block size-4 fill-current text-on-surface" />
                </>
              )}
            </div>
          )}
          {showCover && (
            <Artwork src={track.cover_url} alt="" className={cn('shrink-0 rounded-sm', compact ? 'size-9' : 'size-10')} />
          )}
        </div>

        {/* title / artist / album */}
        <div className={cn('min-w-0 grid gap-x-4 items-center', showAlbum ? 'md:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]' : '')}>
          <div className="min-w-0">
            <div className={cn('truncate type-body-md font-medium', isCurrent ? 'text-primary' : 'text-on-surface')}>{track.title}</div>
            <div className="truncate type-body-sm text-on-surface-variant flex items-center gap-1.5">
              {showQuality && <QualityBadge type={track.type} hz={track.sampling_rate_hz} />}
              {track.artist_id ? (
                <Link to="/artist/$artistId" params={{ artistId: track.artist_id }} onClick={(e) => e.stopPropagation()} className="hover:underline truncate">
                  {track.artist}
                </Link>
              ) : (
                <span className="truncate">{track.artist}</span>
              )}
            </div>
          </div>
          {showAlbum && (
            <div className="hidden md:block truncate type-body-sm text-on-surface-variant">
              {track.album_id ? (
                <Link to="/album/$albumId" params={{ albumId: track.album_id }} onClick={(e) => e.stopPropagation()} className="hover:underline">
                  {track.album}
                </Link>
              ) : (
                track.album || ''
              )}
            </div>
          )}
        </div>

        {/* actions */}
        <div className="flex items-center gap-0.5 justify-end">
          <button
            onClick={(e) => {
              e.stopPropagation()
              void toggleLike(track)
            }}
            aria-label={isLiked ? 'Remove from favourites' : 'Add to favourites'}
            className={cn(
              'size-8 rounded-full inline-flex items-center justify-center transition-[opacity,color]',
              isLiked ? 'text-primary' : 'text-on-surface-variant opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 max-md:opacity-100'
            )}
          >
            <Heart className={cn('size-4', isLiked && 'fill-current')} />
          </button>
          {trailing}
          <span className="w-10 text-right tabular type-body-sm text-on-surface-variant">{formatDuration(track.duration_sec)}</span>
          <button
            onClick={openMenu}
            aria-label="More options"
            className="size-8 rounded-full inline-flex items-center justify-center text-on-surface-variant opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 max-md:opacity-100 hover:text-on-surface"
          >
            <MoreHorizontal className="size-4" />
          </button>
        </div>
      </div>
    )
  }
)
TrackRow.displayName = 'TrackRow'
