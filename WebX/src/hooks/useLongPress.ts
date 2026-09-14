import { useCallback, useRef } from 'react'
import { useSettingsStore } from '@/stores/settingsStore'

/** Short haptic tick (Android / Chrome); silently ignored where unsupported. */
export function haptic(pattern: number | number[] = 12): void {
  if (typeof navigator === 'undefined' || !('vibrate' in navigator)) return
  if (!useSettingsStore.getState().haptics) return
  try {
    navigator.vibrate(pattern)
  } catch {}
}

export interface LongPressPoint { x: number; y: number }

/**
 * Touch/pen long-press → callback with the press coordinates. Mouse users keep right-click.
 * Cancels on movement (> 10px), pointer up, or scroll; swallows the click that follows a fired press.
 */
export function useLongPress(onLongPress: (pt: LongPressPoint, target: HTMLElement) => void, delayMs = 450) {
  const timer = useRef<number | null>(null)
  const start = useRef<LongPressPoint | null>(null)
  const fired = useRef(false)

  const clear = useCallback(() => {
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = null
    start.current = null
  }, [])

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      if (e.pointerType === 'mouse' || e.button !== 0) return
      const target = e.currentTarget
      start.current = { x: e.clientX, y: e.clientY }
      fired.current = false
      timer.current = window.setTimeout(() => {
        timer.current = null
        if (!start.current) return
        fired.current = true
        haptic()
        onLongPress(start.current, target)
        start.current = null
      }, delayMs)
    },
    [onLongPress, delayMs]
  )

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      if (!start.current) return
      if (Math.abs(e.clientX - start.current.x) > 10 || Math.abs(e.clientY - start.current.y) > 10) clear()
    },
    [clear]
  )

  /** Attach to onClick: returns true when the click should be ignored (it ended a long-press). */
  const swallowClick = useCallback((e: React.SyntheticEvent) => {
    if (!fired.current) return false
    fired.current = false
    e.preventDefault()
    e.stopPropagation()
    return true
  }, [])

  return {
    handlers: {
      onPointerDown,
      onPointerMove,
      onPointerUp: clear,
      onPointerCancel: clear,
      onPointerLeave: clear,
    },
    swallowClick,
  }
}
