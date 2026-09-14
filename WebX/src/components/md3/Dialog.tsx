import React, { useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/cn'

export interface DialogProps {
  open: boolean
  onClose: () => void
  title?: React.ReactNode
  icon?: React.ReactNode
  children?: React.ReactNode
  actions?: React.ReactNode
  className?: string
  /** Full-screen on small viewports */
  fullscreenOnMobile?: boolean
  size?: 'sm' | 'md' | 'lg' | 'xl'
}

const SIZE = { sm: 'max-w-sm', md: 'max-w-md', lg: 'max-w-lg', xl: 'max-w-3xl' } as const

export const Dialog: React.FC<DialogProps> = ({ open, onClose, title, icon, children, actions, className, fullscreenOnMobile, size = 'md' }) => {
  const ref = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onCloseRef.current()
      }
    }
    window.addEventListener('keydown', onKey, true)
    const prev = document.activeElement as HTMLElement | null
    requestAnimationFrame(() => {
      if (ref.current && !ref.current.contains(document.activeElement)) {
        ref.current.querySelector<HTMLElement>('input,button,[tabindex]')?.focus()
      }
    })
    return () => {
      window.removeEventListener('keydown', onKey, true)
      prev?.focus?.()
    }
  }, [open])

  if (!open || typeof document === 'undefined') return null

  return createPortal(
    <div className="fixed inset-0 z-[90] flex items-center justify-center p-4" role="presentation">
      <div className="absolute inset-0 bg-scrim/50 page-enter" onClick={onClose} />
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        className={cn(
          'relative w-full elev-3 rounded-2xl shadow-md3-3 p-6 text-on-surface page-enter max-h-[calc(100vh-2rem)] flex flex-col',
          SIZE[size],
          fullscreenOnMobile && 'max-sm:fixed max-sm:inset-0 max-sm:max-w-none max-sm:max-h-none max-sm:rounded-none',
          className
        )}
      >
        {icon && <div className="flex justify-center text-secondary mb-4 [&_svg]:size-6">{icon}</div>}
        {title && <h2 className={cn('type-headline-sm text-on-surface mb-4', icon && 'text-center')}>{title}</h2>}
        <div className="type-body-md text-on-surface-variant min-h-0 overflow-y-auto -mx-6 px-6">{children}</div>
        {actions && <div className="flex items-center justify-end gap-2 mt-6">{actions}</div>}
      </div>
    </div>,
    document.body
  )
}
