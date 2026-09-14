import React, { useEffect, useRef, useState, useCallback } from 'react'
import { haptic } from '@/hooks/useLongPress'
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
  const listRef = useRef<HTMLUListElement>(null)
  const scrollerRef = useRef<HTMLDivElement>(null)
  const [over, setOver] = useState<number | null>(null)
  const [drag, setDrag] = useState<{ from: number; dy: number } | null>(null)
  const dragState = useRef<{ from: number; startY: number } | null>(null)
  const overRef = useRef<number | null>(null)
  const justDropped = useRef(0)
  const holdTimer = useRef<number | null>(null)
  const autoScrollRaf = useRef<number | null>(null)
  const lastClientY = useRef<number | null>(null)

  const clearHold = () => {
    if (holdTimer.current) {
      window.clearTimeout(holdTimer.current)
      holdTimer.current = null
    }
  }

  /** Calculate which queue slot clientY corresponds to, excluding the dragged item */
  const updateOver = useCallback((clientY: number, fromIdx: number): number => {
    const list = listRef.current
    if (!list) return fromIdx
    const otherItems: Array<{ idx: number; mid: number }> = []
    Array.from(list.children).forEach((child) => {
      const el = child as HTMLElement
      const idx = Number(el.dataset.queueIndex)
      if (!Number.isFinite(idx) || idx === fromIdx) return
      const r = el.getBoundingClientRect()
      otherItems.push({ idx, mid: r.top + r.height / 2 })
    })
    otherItems.sort((a, b) => a.idx - b.idx)
    if (otherItems.length === 0) return fromIdx

    if (clientY < otherItems[0].mid) {
      return fromIdx < otherItems[0].idx ? fromIdx : otherItems[0].idx
    }
    if (clientY > otherItems[otherItems.length - 1].mid) {
      return fromIdx > otherItems[otherItems.length - 1].idx
        ? fromIdx
        : otherItems[otherItems.length - 1].idx
    }

    for (let i = 0; i < otherItems.length - 1; i++) {
      const curr = otherItems[i]
      const next = otherItems[i + 1]
      if (clientY >= curr.mid && clientY <= next.mid) {
        if (curr.idx < fromIdx && next.idx > fromIdx) {
          return fromIdx
        }
        if (next.idx <= fromIdx) {
          return next.idx
        }
        return curr.idx
      }
    }
    return fromIdx
  }, [])

  const startDrag = useCallback((idx: number, startY: number) => {
    dragState.current = { from: idx, startY }
    setDrag({ from: idx, dy: 0 })
    overRef.current = idx
    setOver(idx)
    lastClientY.current = startY
    haptic()
  }, [])

  const beginDrag = (e: React.PointerEvent<HTMLElement>, idx: number) => {
    if (e.button !== 0) return
    e.preventDefault()
    e.stopPropagation()
    clearHold()
    startDrag(idx, e.clientY)
  }

  /* Press-and-hold anywhere on row on touch devices */
  const onRowPointerDown = (e: React.PointerEvent<HTMLElement>, idx: number) => {
    if (e.pointerType === 'mouse' || e.button !== 0) return
    const startX = e.clientX
    const startY = e.clientY

    clearHold()
    holdTimer.current = window.setTimeout(() => {
      holdTimer.current = null
      startDrag(idx, startY)
    }, 280)

    const onMoveCheck = (ev: PointerEvent) => {
      if (Math.abs(ev.clientX - startX) > 10 || Math.abs(ev.clientY - startY) > 10) {
        clearHold()
        window.removeEventListener('pointermove', onMoveCheck)
        window.removeEventListener('pointerup', onUpCheck)
      }
    }
    const onUpCheck = () => {
      clearHold()
      window.removeEventListener('pointermove', onMoveCheck)
      window.removeEventListener('pointerup', onUpCheck)
    }
    window.addEventListener('pointermove', onMoveCheck)
    window.addEventListener('pointerup', onUpCheck)
  }

  /* Global window listeners during drag */
  useEffect(() => {
    if (!drag) return

    const scroller = scrollerRef.current

    const onPointerMove = (e: PointerEvent) => {
      const st = dragState.current
      if (!st) return
      e.preventDefault()
      lastClientY.current = e.clientY
      const dy = e.clientY - st.startY
      setDrag({ from: st.from, dy })

      const target = updateOver(e.clientY, st.from)
      if (target !== overRef.current) {
        overRef.current = target
        setOver(target)
        haptic(4)
      }

      // Auto-scroll when near top/bottom edges of the scroller
      if (scroller) {
        const rect = scroller.getBoundingClientRect()
        const zone = 60
        let speed = 0
        if (e.clientY < rect.top + zone) {
          speed = -Math.min(16, (rect.top + zone - e.clientY) * 0.35)
        } else if (e.clientY > rect.bottom - zone) {
          speed = Math.min(16, (e.clientY - (rect.bottom - zone)) * 0.35)
        }

        if (speed !== 0) {
          if (!autoScrollRaf.current) {
            const step = () => {
              if (!dragState.current) return
              scroller.scrollTop += speed
              if (lastClientY.current !== null && dragState.current) {
                const newTarget = updateOver(lastClientY.current, dragState.current.from)
                if (newTarget !== overRef.current) {
                  overRef.current = newTarget
                  setOver(newTarget)
                }
              }
              autoScrollRaf.current = requestAnimationFrame(step)
            }
            autoScrollRaf.current = requestAnimationFrame(step)
          }
        } else if (autoScrollRaf.current) {
          cancelAnimationFrame(autoScrollRaf.current)
          autoScrollRaf.current = null
        }
      }
    }

    const onPointerUp = () => {
      const st = dragState.current
      if (!st) return
      if (autoScrollRaf.current) {
        cancelAnimationFrame(autoScrollRaf.current)
        autoScrollRaf.current = null
      }
      const target = overRef.current
      if (target !== null && target !== st.from) {
        reorderQueue(st.from, target)
        haptic(10)
      }
      dragState.current = null
      justDropped.current = Date.now()
      setDrag(null)
      setOver(null)
      overRef.current = null
    }

    const onTouchMove = (e: TouchEvent) => {
      e.preventDefault()
    }

    window.addEventListener('pointermove', onPointerMove, { passive: false })
    window.addEventListener('pointerup', onPointerUp, { capture: true })
    window.addEventListener('pointercancel', onPointerUp, { capture: true })
    document.addEventListener('touchmove', onTouchMove, { passive: false })

    return () => {
      if (autoScrollRaf.current) {
        cancelAnimationFrame(autoScrollRaf.current)
        autoScrollRaf.current = null
      }
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp, { capture: true })
      window.removeEventListener('pointercancel', onPointerUp, { capture: true })
      document.removeEventListener('touchmove', onTouchMove)
    }
  }, [drag?.from, updateOver, reorderQueue])

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
    return <EmptyState icon={<ListMusic />} title="Queue is empty" description="Play something and it shows up here" compact className={className} />
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
      <div ref={scrollerRef} className="flex-1 min-h-0 overflow-y-auto px-2 pb-4">
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
          <ul ref={listRef} className="space-y-0.5">
            {upcoming.map((t, i) => {
              const idx = currentIndex + 1 + i
              return (
                <li
                  key={`${t.id}-${idx}`}
                  data-queue-index={idx}
                  className={cn(
                    'relative group flex items-center gap-2 h-14 pl-1 pr-1 rounded-md state-layer cursor-pointer select-none',
                    drag?.from === idx
                      ? 'z-30 bg-surface-high shadow-lg scale-[1.02] pointer-events-none'
                      : 'hover:bg-on-surface/5 active:bg-on-surface/8'
                  )}
                  style={drag?.from === idx ? { transform: `translateY(${drag.dy}px) scale(1.02)`, transition: 'none' } : undefined}
                  onClick={() => {
                    if (drag || Date.now() - justDropped.current < 400) return
                    void jumpTo(idx)
                  }}
                  onPointerDown={(e) => onRowPointerDown(e, idx)}
                  onContextMenu={(e) => e.preventDefault()}
                >
                  {drag && drag.from !== idx && over === idx && (
                    <div
                      className={cn(
                        'absolute inset-x-2 h-1 bg-primary rounded-full shadow-md z-20 pointer-events-none',
                        drag.from < idx ? '-bottom-0.5' : '-top-0.5'
                      )}
                    />
                  )}
                  <span
                    role="button"
                    aria-label="Drag to reorder"
                    className="drag-handle touch-none select-none flex items-center justify-center size-10 -ml-1 rounded-full cursor-grab active:cursor-grabbing text-on-surface-variant/70 hover:text-on-surface hover:bg-on-surface/8 active:bg-on-surface/12 transition-colors max-sm:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
                    onPointerDown={(e) => beginDrag(e, idx)}
                    onClick={(e) => e.stopPropagation()}
                  >
                    <GripVertical className="size-4" />
                  </span>
                  <Artwork src={t.cover_url} alt="" className="size-10 rounded-sm" />
                  <div className="min-w-0 flex-1">
                    <p className="type-body-md text-on-surface truncate">{t.title}</p>
                    <p className="type-body-sm text-on-surface-variant truncate">{t.artist}</p>
                  </div>
                  <span className="tabular type-body-sm text-on-surface-variant group-hover:hidden pointer-coarse:hidden">{formatDuration(t.duration_sec)}</span>
                  <IconButton
                    label="Remove from queue"
                    size="sm"
                    className="hidden group-hover:inline-flex pointer-coarse:inline-flex"
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
