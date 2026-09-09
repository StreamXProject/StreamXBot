import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/cn'

export interface MenuItem {
  id: string
  label: React.ReactNode
  icon?: React.ReactNode
  trailing?: React.ReactNode
  onSelect?: () => void
  disabled?: boolean
  destructive?: boolean
  divider?: boolean
}

export interface MenuProps {
  open: boolean
  onClose: () => void
  items: MenuItem[]
  /** Anchor element or viewport coordinates */
  anchor: HTMLElement | { x: number; y: number } | null
  align?: 'start' | 'end'
  className?: string
  header?: React.ReactNode
}

export const Menu: React.FC<MenuProps> = ({ open, onClose, items, anchor, align = 'start', className, header }) => {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    if (!open || !anchor || !ref.current) return
    const menu = ref.current.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    let top: number
    let left: number
    if (anchor instanceof HTMLElement) {
      const r = anchor.getBoundingClientRect()
      top = r.bottom + 4
      left = align === 'end' ? r.right - menu.width : r.left
    } else {
      top = anchor.y
      left = anchor.x
    }
    if (left + menu.width > vw - 8) left = vw - menu.width - 8
    if (left < 8) left = 8
    if (top + menu.height > vh - 8) top = Math.max(8, top - menu.height - (anchor instanceof HTMLElement ? anchor.getBoundingClientRect().height + 8 : 0))
    setPos({ top, left })
  }, [open, anchor, align, items.length])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    const onDown = (e: PointerEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('pointerdown', onDown, true)
    window.addEventListener('resize', onClose)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('pointerdown', onDown, true)
      window.removeEventListener('resize', onClose)
    }
  }, [open, onClose])

  if (!open || typeof document === 'undefined') return null

  return createPortal(
    <div
      ref={ref}
      role="menu"
      style={pos ? { top: pos.top, left: pos.left } : { top: -9999, left: -9999 }}
      className={cn(
        'fixed z-[95] min-w-[220px] max-w-[300px] py-2 rounded-xs elev-2 shadow-md3-2 text-on-surface page-enter',
        className
      )}
    >
      {header && <div className="px-4 pb-2 mb-1 border-b border-outline-variant">{header}</div>}
      {items.map((it) =>
        it.divider ? (
          <div key={it.id} className="my-1 border-t border-outline-variant" />
        ) : (
          <button
            key={it.id}
            role="menuitem"
            disabled={it.disabled}
            onClick={() => {
              it.onSelect?.()
              onClose()
            }}
            className={cn(
              'state-layer w-full flex items-center gap-3 h-12 px-3 text-left type-body-lg rounded-none [&_svg]:size-5',
              it.destructive ? 'text-error' : 'text-on-surface',
              it.disabled && 'opacity-40 pointer-events-none'
            )}
          >
            <span className="shrink-0 text-on-surface-variant w-6 flex justify-center">{it.icon}</span>
            <span className="flex-1 truncate">{it.label}</span>
            {it.trailing && <span className="text-on-surface-variant type-label-md">{it.trailing}</span>}
          </button>
        )
      )}
    </div>,
    document.body
  )
}
