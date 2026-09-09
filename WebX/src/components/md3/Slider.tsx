import React, { useCallback, useRef, useState } from 'react'
import { cn } from '@/lib/cn'

export interface SliderProps {
  value: number
  min?: number
  max?: number
  step?: number
  onChange: (value: number) => void
  onCommit?: (value: number) => void
  disabled?: boolean
  className?: string
  'aria-label'?: string
  id?: string
}

/**
 * MD3 slider. Pointer-capture based so tap, drag and touch all behave the
 * same (native <input type=range> was unreliable inside transformed layers).
 */
export const Slider: React.FC<SliderProps> = ({ value, min = 0, max = 100, step = 1, onChange, onCommit, disabled, className, id, ...rest }) => {
  const ref = useRef<HTMLDivElement>(null)
  const [dragging, setDragging] = useState(false)
  const pct = max > min ? ((Math.min(max, Math.max(min, value)) - min) / (max - min)) * 100 : 0

  const valueFromX = useCallback(
    (clientX: number) => {
      const r = ref.current!.getBoundingClientRect()
      const ratio = Math.max(0, Math.min(1, (clientX - r.left) / r.width))
      let v = min + ratio * (max - min)
      if (step > 0) v = Math.round(v / step) * step
      const decimals = (String(step).split('.')[1] || '').length
      return Math.max(min, Math.min(max, Number(v.toFixed(decimals))))
    },
    [min, max, step]
  )

  const onPointerDown = (e: React.PointerEvent) => {
    if (disabled) return
    e.preventDefault()
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    setDragging(true)
    onChange(valueFromX(e.clientX))
  }
  const onPointerMove = (e: React.PointerEvent) => {
    if (!dragging) return
    onChange(valueFromX(e.clientX))
  }
  const onPointerUp = (e: React.PointerEvent) => {
    if (!dragging) return
    setDragging(false)
    ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
    onCommit?.(valueFromX(e.clientX))
  }
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (disabled) return
    const big = (max - min) / 10
    let next: number | null = null
    if (e.key === 'ArrowRight' || e.key === 'ArrowUp') next = value + step
    if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') next = value - step
    if (e.key === 'PageUp') next = value + big
    if (e.key === 'PageDown') next = value - big
    if (e.key === 'Home') next = min
    if (e.key === 'End') next = max
    if (next !== null) {
      e.preventDefault()
      const v = Math.max(min, Math.min(max, next))
      onChange(v)
      onCommit?.(v)
    }
  }

  return (
    <div
      ref={ref}
      id={id}
      role="slider"
      tabIndex={disabled ? -1 : 0}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={value}
      aria-disabled={disabled}
      aria-label={rest['aria-label']}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onKeyDown={onKeyDown}
      className={cn('group relative h-6 min-w-16 flex items-center cursor-pointer touch-none select-none outline-none rounded-full', disabled && 'opacity-40 pointer-events-none', className)}
    >
      <div className="relative w-full h-1 rounded-full bg-on-surface/15 overflow-hidden group-focus-visible:ring-2 ring-primary/60">
        <div className="absolute inset-y-0 left-0 bg-primary rounded-full" style={{ width: `${pct}%` }} />
      </div>
      <div
        className={cn('absolute w-1 h-5 -ml-0.5 rounded-sm bg-primary shadow-[0_0_0_4px_var(--md-sys-color-surface)] transition-[width] duration-150', dragging && 'w-0.5')}
        style={{ left: `${pct}%` }}
      />
    </div>
  )
}
