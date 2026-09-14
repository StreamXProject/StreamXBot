import React, { useEffect, useRef } from 'react'
import { Minus, Plus, RotateCcw, SlidersHorizontal } from 'lucide-react'
import { useSettingsStore } from '@/stores/settingsStore'
import { cn } from '@/lib/cn'

interface LyricsSyncControlProps {
  className?: string
  compact?: boolean
  showLabel?: boolean
}

export const LyricsSyncControl: React.FC<LyricsSyncControlProps> = ({
  className,
  compact = false,
  showLabel = false,
}) => {
  const offset = useSettingsStore((s) => s.lyricsSyncOffsetMs)
  const setSetting = useSettingsStore((s) => s.set)
  const repeatTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const delayTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clamp = (val: number) => Math.max(-5000, Math.min(5000, Math.round(val / 50) * 50))

  const adjust = (delta: number) => {
    setSetting('lyricsSyncOffsetMs', clamp(useSettingsStore.getState().lyricsSyncOffsetMs + delta))
  }

  const reset = (e?: React.MouseEvent) => {
    e?.stopPropagation()
    setSetting('lyricsSyncOffsetMs', 0)
  }

  const stopAutoRepeat = () => {
    if (delayTimer.current) {
      clearTimeout(delayTimer.current)
      delayTimer.current = null
    }
    if (repeatTimer.current) {
      clearInterval(repeatTimer.current)
      repeatTimer.current = null
    }
  }

  const startAutoRepeat = (delta: number) => {
    stopAutoRepeat()
    adjust(delta)
    delayTimer.current = setTimeout(() => {
      repeatTimer.current = setInterval(() => {
        adjust(delta)
      }, 70)
    }, 320)
  }

  useEffect(() => () => stopAutoRepeat(), [])

  const isZero = offset === 0
  const formattedOffset = isZero ? '0 ms' : `${offset > 0 ? '+' : ''}${offset} ms`
  const directionText = isZero ? 'In sync' : offset > 0 ? `${(offset / 1000).toFixed(2)}s earlier` : `${(Math.abs(offset) / 1000).toFixed(2)}s later`

  return (
    <div
      className={cn('inline-flex items-center select-none', className)}
      onPointerDown={(e) => e.stopPropagation()}
    >
      {showLabel && (
        <span className="type-label-sm text-on-surface-variant font-medium mr-2 flex items-center gap-1.5">
          <SlidersHorizontal className="size-3.5 text-primary" />
          Sync offset
        </span>
      )}

      <div
        className={cn(
          'inline-flex items-center rounded-full bg-surface-container/70 border border-outline-variant/30 text-on-surface shadow-xs transition-colors',
          compact ? 'h-7' : 'h-8'
        )}
      >
        <button
          type="button"
          aria-label="Decrease lyrics sync offset (50ms later)"
          title="Decrease offset (50ms later) — hold to repeat"
          onPointerDown={() => startAutoRepeat(-50)}
          onPointerUp={stopAutoRepeat}
          onPointerLeave={stopAutoRepeat}
          onPointerCancel={stopAutoRepeat}
          className={cn(
            'rounded-full flex items-center justify-center text-on-surface-variant hover:text-on-surface hover:bg-on-surface/10 active:scale-95 transition-all outline-none',
            compact ? 'size-7' : 'size-8'
          )}
        >
          <Minus className={compact ? 'size-3' : 'size-3.5'} />
        </button>

        {isZero ? (
          <span
            className={cn(
              'px-2 font-mono font-semibold text-on-surface-variant/70 tracking-tight cursor-default',
              compact ? 'text-[11px]' : 'text-xs'
            )}
            title="Lyrics timing in sync (0 ms)"
          >
            0 ms
          </span>
        ) : (
          <button
            type="button"
            onClick={reset}
            aria-label={`Reset lyrics offset from ${formattedOffset} (${directionText}) to 0 ms`}
            title={`Offset: ${formattedOffset} (${directionText}). Click to reset to 0 ms.`}
            className={cn(
              'px-2 py-0.5 rounded-full flex items-center gap-1 font-mono font-semibold text-primary hover:bg-primary/10 active:scale-95 transition-all outline-none cursor-pointer',
              compact ? 'text-[11px]' : 'text-xs'
            )}
          >
            <span>{formattedOffset}</span>
            <RotateCcw className={compact ? 'size-2.5 opacity-80' : 'size-3 opacity-80'} />
          </button>
        )}

        <button
          type="button"
          aria-label="Increase lyrics sync offset (50ms earlier)"
          title="Increase offset (50ms earlier) — hold to repeat"
          onPointerDown={() => startAutoRepeat(50)}
          onPointerUp={stopAutoRepeat}
          onPointerLeave={stopAutoRepeat}
          onPointerCancel={stopAutoRepeat}
          className={cn(
            'rounded-full flex items-center justify-center text-on-surface-variant hover:text-on-surface hover:bg-on-surface/10 active:scale-95 transition-all outline-none',
            compact ? 'size-7' : 'size-8'
          )}
        >
          <Plus className={compact ? 'size-3' : 'size-3.5'} />
        </button>
      </div>
    </div>
  )
}
