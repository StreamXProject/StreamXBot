import React, { useCallback } from 'react'
import { Virtuoso } from 'react-virtuoso'
import type { Track } from '@/schemas/track'
import { TrackRow } from './TrackRow'
import { EmptyState } from './EmptyState'
import { useQueueStore, type QueueContext } from '@/stores/queueStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { Music2 } from 'lucide-react'

interface VirtualTrackListProps {
  tracks: Track[]
  showAlbum?: boolean
  showCover?: boolean
  emptyTitle?: string
  emptyMessage?: string
  emptyAction?: React.ReactNode
  header?: React.ReactNode
  context?: QueueContext
  /** Use the page's scroll container instead of an internal one */
  useWindowScroll?: boolean
  trailing?: (track: Track, index: number) => React.ReactNode
  className?: string
  onEndReached?: () => void
}

export const VirtualTrackList: React.FC<VirtualTrackListProps> = ({
  tracks,
  showAlbum = true,
  showCover = true,
  emptyTitle = 'Nothing here yet',
  emptyMessage = 'No tracks found.',
  emptyAction,
  header,
  context,
  useWindowScroll = true,
  trailing,
  className,
  onEndReached,
}) => {
  const playTrackWithQueue = useQueueStore((s) => s.playTrackWithQueue)
  const compact = useSettingsStore((s) => s.compactRows)
  const safe = Array.isArray(tracks) ? tracks : []

  const onPlay = useCallback((_t: Track, index: number) => void playTrackWithQueue(safe, index, context), [safe, context, playTrackWithQueue])

  if (safe.length === 0) {
    return (
      <>
        {header}
        <EmptyState icon={<Music2 />} title={emptyTitle} description={emptyMessage} action={emptyAction} compact />
      </>
    )
  }

  return (
    <div className={className}>
      <Virtuoso
        useWindowScroll={useWindowScroll}
        customScrollParent={useWindowScroll ? (document.getElementById('app-scroll') ?? undefined) : undefined}
        style={useWindowScroll ? undefined : { height: '100%' }}
        data={safe}
        defaultItemHeight={compact ? 48 : 56}
        increaseViewportBy={400}
        endReached={onEndReached}
        components={{ Header: header ? () => <>{header}</> : undefined }}
        computeItemKey={(i, t) => `${t.id}-${i}`}
        itemContent={(index, track) => (
          <TrackRow track={track} index={index} tracks={safe} showAlbum={showAlbum} showCover={showCover} onPlay={onPlay} trailing={trailing?.(track, index)} />
        )}
      />
    </div>
  )
}

export const TrackListHeader: React.FC<{ showAlbum?: boolean; showCover?: boolean }> = ({ showAlbum = true, showCover = true }) => (
  <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-3 h-9 type-label-md text-on-surface-variant border-b border-outline-variant mb-1">
    <div className="flex items-center gap-3">
      <div className="w-6 text-center">#</div>
      {showCover && <div className="size-10" />}
    </div>
    <div className={showAlbum ? 'grid md:grid-cols-[minmax(0,3fr)_minmax(0,2fr)] gap-x-4' : ''}>
      <div>Title</div>
      {showAlbum && <div className="hidden md:block">Album</div>}
    </div>
    <div className="w-[7.5rem] text-right pr-9">Time</div>
  </div>
)
