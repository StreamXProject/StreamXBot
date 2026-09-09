import React, { useRef, useState } from 'react'
import { X, GripVertical, Shuffle, Trash2, ListMusic } from 'lucide-react'
import { useQueueStore } from '@/stores/queueStore'
import { usePlayerStore } from '@/stores/playerStore'
import { Artwork } from '@/components/common/Artwork'
import { NowPlayingBars } from '@/components/common/NowPlayingBars'
import { IconButton, Button } from '@/components/md3'
import { EmptyState } from '@/components/common/EmptyState'
import { formatDuration, formatLongDuration } from '@/lib/format'
import { cn } from '@/lib/cn'
import { useSettingsStore } from '@/stores/settingsStore'
import { toast } from '@/stores/uiStore'

/** Shared queue list — used by the side sheet and the full player pane */
export const QueueList: React.FC<{ className?: string; showHeader?: boolean }> = ({ className, showHeader = true }) => {
  const queue = useQueueStore((s) => s.queue)
  const currentIndex = useQueueStore((s) => s.currentIndex)
  const context = useQueueStore((s) => s.context)
  const isShuffle = useQueueStore((s) => s.isShuffle)
  const toggleShuffle = useQueueStore((s) => s.toggleShuffle)
  const clearUpcoming = useQueueStore((s) => s.clearUpcoming)
  const jumpTo = useQueueStore((s) => s.jumpTo)
  const removeFromQueue = useQueueStore((s) => s.removeFromQueue)
  const reorderQueue = useQueueStore((s) => s.reorderQueue)
  const isPlaying = usePlayerStore((s) => s.isPlaying)
  const confirmDestructive = useSettingsStore((s) => s.confirmDestructive)
  const dragFrom = useRef<number | null>(null)
  const [over, setOver] = useState<number | null>(null)

  const current = currentIndex >= 0 ? queue[currentIndex] : null
  const upcoming = queue.slice(currentIndex + 1)
  const remaining = upcoming.reduce((a, t) => a + (t.duration_sec || 0), 0)

  const handleClear = () => {
    if (!confirmDestructive || upcoming.length < 5 || confirm(`Remove ${upcoming.length} upcoming tracks from the queue?`)) {
      clearUpcoming()
      toast('Cleared upcoming tracks')
    }
  }

  if (queue.length === 0) {
    return <EmptyState icon={<ListMusic />} title="Queue is empty" description="Play something and it will show up here. Right-click any track to add it next." compact className={className} />
  }

  return (
    <div className={cn('flex flex-col min-h-0', className)}>
      {showHeader && (
        <div className="flex items-center justify-between px-4 h-12 shrink-0">
          <div className="min-w-0">
            <span className="type-title-sm text-on-surface">Up next</span>
            <span className="type-body-sm text-on-surface-variant ml-2">{upcoming.length} · {formatLongDuration(remaining)}</span>
          </div>
          <div className="flex items-center gap-1">
            <IconButton label="Shuffle" size="sm" selected={isShuffle} onClick={toggleShuffle}><Shuffle /></IconButton>
            <IconButton label="Clear upcoming" size="sm" onClick={handleClear} disabled={upcoming.length === 0}><Trash2 /></IconButton>
          </div>
        </div>
      )}
      <div className="flex-1 min-h-0 overflow-y-auto px-2 pb-4">
        {current && (
          <div className="mb-3">
            <p className="px-2 pb-1 type-label-md text-on-surface-variant">Now playing{context ? ` · ${context.title}` : ''}</p>
            <div className="flex items-center gap-3 p-2 rounded-md bg-secondary-container/50">
              <Artwork src={current.cover_url} alt="" className="size-12 rounded-sm" />
              <div className="min-w-0 flex-1">
                <p className="type-body-md font-semibold text-primary truncate">{current.title}</p>
                <p className="type-body-sm text-on-surface-variant truncate">{current.artist}</p>
              </div>
              <NowPlayingBars playing={isPlaying} className="mr-2" />
            </div>
          </div>
        )}
        {upcoming.length === 0 ? (
          <p className="px-3 py-6 text-center type-body-sm text-on-surface-variant">Nothing queued after this track.</p>
        ) : (
          <ul className="space-y-0.5">
            {upcoming.map((t, i) => {
              const idx = currentIndex + 1 + i
              return (
                <li
                  key={`${t.id}-${idx}`}
                  draggable
                  onDragStart={() => (dragFrom.current = idx)}
                  onDragOver={(e) => {
                    e.preventDefault()
                    setOver(idx)
                  }}
                  onDragLeave={() => setOver(null)}
                  onDrop={() => {
                    if (dragFrom.current !== null && dragFrom.current !== idx) reorderQueue(dragFrom.current, idx)
                    dragFrom.current = null
                    setOver(null)
                  }}
                  onDragEnd={() => setOver(null)}
                  className={cn('group flex items-center gap-2 h-14 pl-1 pr-1 rounded-md state-layer cursor-pointer', over === idx && 'ring-2 ring-primary/60')}
                  onClick={() => void jumpTo(idx)}
                >
                  <span className="cursor-grab text-on-surface-variant/60 opacity-0 group-hover:opacity-100 touch-none"><GripVertical className="size-4" /></span>
                  <Artwork src={t.cover_url} alt="" className="size-10 rounded-sm" />
                  <div className="min-w-0 flex-1">
                    <p className="type-body-md text-on-surface truncate">{t.title}</p>
                    <p className="type-body-sm text-on-surface-variant truncate">{t.artist}</p>
                  </div>
                  <span className="tabular type-body-sm text-on-surface-variant group-hover:hidden">{formatDuration(t.duration_sec)}</span>
                  <IconButton
                    label="Remove from queue"
                    size="sm"
                    className="hidden group-hover:inline-flex"
                    onClick={(e) => {
                      e.stopPropagation()
                      removeFromQueue(idx)
                    }}
                  >
                    <X />
                  </IconButton>
                </li>
              )
            })}
          </ul>
        )}
        {upcoming.length > 0 && (
          <div className="pt-3 px-2">
            <Button variant="text" size="sm" onClick={handleClear}>Clear upcoming</Button>
          </div>
        )}
      </div>
    </div>
  )
}
