import React from 'react'
import { X } from 'lucide-react'
import { useUiStore } from '@/stores/uiStore'
import { cn } from '@/lib/cn'

export const ToastHost: React.FC = () => {
  const toasts = useUiStore((s) => s.toasts)
  const dismiss = useUiStore((s) => s.dismissToast)
  if (toasts.length === 0) return null
  return (
    <div className="fixed left-1/2 -translate-x-1/2 z-[100] flex flex-col items-center gap-2 pointer-events-none w-[min(92vw,560px)]" style={{ bottom: 'calc(var(--webx-player-height) + 5rem + env(safe-area-inset-bottom, 0px))' }}>
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          className={cn(
            'pointer-events-auto flex items-center gap-3 min-h-12 pl-4 pr-2 py-2 rounded-xs shadow-md3-3 page-enter w-[min(92vw,560px)]',
            t.variant === 'error' ? 'bg-error-container text-on-error-container' : 'bg-inverse-surface text-inverse-on-surface'
          )}
        >
          <span className="type-body-md flex-1">{t.message}</span>
          {t.action && (
            <button
              onClick={() => {
                t.action!.onClick()
                dismiss(t.id)
              }}
              className={cn('type-label-lg px-2 h-8 rounded-full', t.variant === 'error' ? 'text-on-error-container' : 'text-inverse-primary')}
              style={{ color: t.variant === 'error' ? undefined : 'var(--md-sys-color-inverse-primary)' }}
            >
              {t.action.label}
            </button>
          )}
          <button onClick={() => dismiss(t.id)} aria-label="Dismiss" className="size-8 rounded-full inline-flex items-center justify-center opacity-70 hover:opacity-100">
            <X className="size-4" />
          </button>
        </div>
      ))}
    </div>
  )
}
