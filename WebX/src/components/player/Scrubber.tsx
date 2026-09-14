import React, { useEffect, useRef, useState, useCallback } from 'react'
import { audioEngine } from '@/audio/AudioEngine'
import { formatDuration } from '@/lib/format'
import { cn } from '@/lib/cn'

interface ScrubberProps {
  showTimes?: boolean
  size?: 'sm' | 'md' | 'lg'
  className?: string
}

/**
 * Progress scrubber driven directly from the audio engine
 */
export const Scrubber: React.FC<ScrubberProps> = React.memo(({ showTimes = true, size = 'md', className }) => {
  const fillRef = useRef<HTMLDivElement>(null)
  const bufferRef = useRef<HTMLDivElement>(null)
  const thumbRef = useRef<HTMLDivElement>(null)
  const curRef = useRef<HTMLSpanElement>(null)
  const durRef = useRef<HTMLSpanElement>(null)
  const barRef = useRef<HTMLDivElement>(null)
  const dragging = useRef(false)
  const durationRef = useRef(0)
  const [hover, setHover] = useState<number | null>(null)
  const [isDragging, setIsDragging] = useState(false)

  const paint = useCallback((pct: number, cur: number) => {
    const p = Math.max(0, Math.min(100, pct))
    if (fillRef.current) fillRef.current.style.transform = `scaleX(${p / 100})`
    if (thumbRef.current) thumbRef.current.style.left = `${p}%`
    if (curRef.current) curRef.current.textContent = formatDuration(cur)
  }, [])

  useEffect(() => {
    return audioEngine.subscribeProgress((p) => {
      durationRef.current = p.duration
      if (durRef.current) durRef.current.textContent = formatDuration(p.duration)
      if (bufferRef.current) bufferRef.current.style.transform = `scaleX(${p.duration ? Math.min(1, p.buffered / p.duration) : 0})`
      if (!dragging.current) paint(p.progressPercent, p.currentTime)
    })
  }, [paint])

  const pctFromEvent = (clientX: number) => {
    const r = barRef.current!.getBoundingClientRect()
    return Math.max(0, Math.min(100, ((clientX - r.left) / r.width) * 100))
  }

  const onPointerDown = (e: React.PointerEvent) => {
    if (!durationRef.current) return
    e.preventDefault()
    dragging.current = true
    setIsDragging(true)
      ; (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    const p = pctFromEvent(e.clientX)
    paint(p, (p / 100) * durationRef.current)
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const p = pctFromEvent(e.clientX)
    if (dragging.current) paint(p, (p / 100) * durationRef.current)
    else setHover(p)
  }
  const onPointerUp = (e: React.PointerEvent) => {
    if (!dragging.current) return
    dragging.current = false
    setIsDragging(false)
      ; (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
    audioEngine.seekPercent(pctFromEvent(e.clientX))
  }

  const h = size === 'lg' ? 'h-2' : size === 'sm' ? 'h-1' : 'h-1.5'

  return (
    <div className={cn('flex items-center gap-3 w-full select-none', className)}>
      {showTimes && <span ref={curRef} className="tabular type-label-md text-on-surface-variant w-10 text-right shrink-0">0:00</span>}
      <div
        ref={barRef}
        role="slider"
        aria-label="Seek"
        tabIndex={0}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onPointerLeave={() => setHover(null)}
        onKeyDown={(e) => {
          if (e.key === 'ArrowRight') audioEngine.seekBy(5)
          if (e.key === 'ArrowLeft') audioEngine.seekBy(-5)
        }}
        className="group relative flex-1 h-6 flex items-center cursor-pointer touch-none outline-none"
      >
        <div className={cn('relative w-full rounded-full bg-on-surface/15 overflow-hidden transition-[height] duration-150', h, 'group-hover:h-2', isDragging && 'h-2')}>
          <div ref={bufferRef} className="absolute inset-0 origin-left bg-on-surface/15" style={{ transform: 'scaleX(0)' }} />
          <div
            ref={fillRef}
            className={cn('absolute inset-0 origin-left bg-primary', !isDragging && 'transition-transform duration-[250ms] ease-linear')}
            style={{ transform: 'scaleX(0)' }}
          />
        </div>
        <div
          ref={thumbRef}
          className={cn(
            'absolute size-3 -ml-1.5 rounded-full bg-primary shadow-md3-1 pointer-events-none transition-[transform,opacity] duration-150',
            isDragging ? 'scale-125 opacity-100' : 'scale-0 opacity-0 group-hover:scale-100 group-hover:opacity-100',
            !isDragging && 'transition-[left] duration-[250ms] ease-linear'
          )}
          style={{ left: 0 }}
        />
        {hover !== null && !isDragging && durationRef.current > 0 && (
          <div className="absolute -top-7 -translate-x-1/2 px-2 h-6 flex items-center rounded-xs bg-inverse-surface text-inverse-on-surface type-label-md tabular pointer-events-none" style={{ left: `${hover}%` }}>
            {formatDuration((hover / 100) * durationRef.current)}
          </div>
        )}
      </div>
      {showTimes && <span ref={durRef} className="tabular type-label-md text-on-surface-variant w-10 shrink-0">0:00</span>}
    </div>
  )
})
Scrubber.displayName = 'Scrubber'

export const IsolatedProgressBar = Scrubber
