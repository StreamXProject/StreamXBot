import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Mic2, ExternalLink } from 'lucide-react'
import { useTrackLyrics } from '@/hooks/useQueries'
import { useProgressStore } from '@/stores/progressStore'
import { audioEngine } from '@/audio/AudioEngine'
import { cn } from '@/lib/cn'
import type { LyricLine } from '@/schemas/track'
import { Skeleton } from '@/components/common/Skeleton'

function indexAt(lines: LyricLine[], t: number): number {
  // binary search: last line with time <= t
  let lo = 0
  let hi = lines.length - 1
  let ans = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (lines[mid].time <= t) {
      ans = mid
      lo = mid + 1
    } else hi = mid - 1
  }
  return ans
}

/** Subscribes to time but only re-renders when the active line index changes */
function useActiveLine(lines: LyricLine[]) {
  return useProgressStore((s) => (lines.length ? indexAt(lines, s.currentTime + 0.25) : -1))
}

interface LyricsViewProps {
  trackId: string
  className?: string
  /** Larger type for the full player */
  size?: 'md' | 'lg'
}

export const LyricsView: React.FC<LyricsViewProps> = ({ trackId, className, size = 'lg' }) => {
  const { data, isLoading, isError } = useTrackLyrics(trackId)
  const lines = useMemo(() => data?.lines ?? [], [data])
  const active = useActiveLine(lines)
  const containerRef = useRef<HTMLDivElement>(null)
  const [userScrolled, setUserScrolled] = useState(false)
  const scrollTimer = useRef<number | null>(null)

  // Auto-scroll active line to the upper third unless the user is scrolling
  useEffect(() => {
    if (active < 0 || userScrolled || !containerRef.current) return
    const el = containerRef.current.querySelector<HTMLElement>(`[data-line="${active}"]`)
    if (!el) return
    const c = containerRef.current
    const target = el.offsetTop - c.clientHeight * 0.38 + el.offsetHeight / 2
    c.scrollTo({ top: target, behavior: 'smooth' })
  }, [active, userScrolled])

  // Reset when the track changes
  useEffect(() => {
    setUserScrolled(false)
    containerRef.current?.scrollTo({ top: 0 })
  }, [trackId])

  const onScroll = () => {
    setUserScrolled(true)
    if (scrollTimer.current) clearTimeout(scrollTimer.current)
    scrollTimer.current = window.setTimeout(() => setUserScrolled(false), 3500)
  }

  if (isLoading) {
    return (
      <div className={cn('flex flex-col gap-5 p-8', className)}>
        {[80, 60, 72, 50, 66].map((w, i) => <Skeleton key={i} variant="text" className="h-6" style={{ width: `${w}%` }} />)}
      </div>
    )
  }

  if (isError || !data || (!data.synced && !data.plain)) {
    return (
      <div className={cn('flex-1 flex flex-col items-center justify-center text-center p-8 text-on-surface-variant', className)}>
        <Mic2 className="size-8 mb-3 opacity-60" />
        <p className="type-title-md text-on-surface">No lyrics for this track</p>
        <p className="type-body-sm mt-1">Lyrics are fetched from LRCLIB when available.</p>
      </div>
    )
  }

  if (!data.synced) {
    return (
      <div ref={containerRef} className={cn('flex-1 overflow-y-auto px-6 py-8', className)}>
        <p className={cn('whitespace-pre-wrap text-on-surface leading-relaxed', size === 'lg' ? 'text-xl sm:text-2xl font-semibold' : 'type-body-lg')}>{data.plain}</p>
        {data.source && (
          <p className="mt-6 type-label-md text-on-surface-variant flex items-center gap-1"><ExternalLink className="size-3" /> {data.source}</p>
        )}
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      onWheel={onScroll}
      onTouchMove={onScroll}
      className={cn('flex-1 overflow-y-auto px-6 sm:px-8 py-[38%] scrollbar-none [mask-image:linear-gradient(to_bottom,transparent,#000_12%,#000_85%,transparent)]', className)}
    >
      <div className="flex flex-col gap-1">
        {lines.map((line, i) => {
          const isActive = i === active
          const isPast = i < active
          return (
            <button
              key={i}
              data-line={i}
              onClick={() => audioEngine.seek(line.time)}
              className={cn(
                'text-left py-2 px-3 -mx-3 rounded-lg transition-[color,opacity,transform,filter] duration-300 ease-emphasized origin-left',
                size === 'lg' ? 'text-2xl sm:text-3xl font-bold tracking-tight leading-snug' : 'text-lg font-semibold leading-snug',
                isActive ? 'text-on-surface opacity-100 scale-[1.02]' : isPast ? 'text-on-surface-variant opacity-45 hover:opacity-80' : 'text-on-surface-variant opacity-55 hover:opacity-90',
                line.text === '' && (isActive ? 'my-2 leading-none' : 'h-6')
              )}
            >
              {line.text || (isActive ? '♪' : '')}
            </button>
          )
        })}
      </div>
    </div>
  )
}
