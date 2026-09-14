import React, { useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { cn } from '@/lib/cn'
import { toast } from '@/stores/uiStore'

export interface IdPillProps {
  id: string | number
  label?: string
  size?: 'xs' | 'sm' | 'md'
  className?: string
  showToast?: boolean
  copyText?: string
}

export const IdPill: React.FC<IdPillProps> = ({
  id,
  label = 'ID',
  size = 'sm',
  className,
  showToast = true,
  copyText,
}) => {
  const [copied, setCopied] = useState(false)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(String(id))
    setCopied(true)
    if (showToast) {
      toast(copyText || `${label} copied to clipboard`)
    }
    setTimeout(() => setCopied(false), 2000)
  }

  const isXs = size === 'xs'
  const isSm = size === 'sm'

  return (
    <div
      role="group"
      aria-label={`${label}: ${id}`}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full',
        'bg-surface-container-high/90 border border-outline-variant/60 shadow-xs',
        'hover:border-outline/40 transition-all select-none',
        isXs && 'pl-2 pr-1 py-0.5',
        isSm && 'pl-2.5 pr-1.5 py-0.5',
        size === 'md' && 'pl-3 pr-2 py-1 gap-2',
        className
      )}
    >
      <span
        className={cn(
          'rounded-full bg-primary/12 text-primary font-bold tracking-wider uppercase select-none',
          isXs && 'px-1.5 py-0 text-[9px]',
          isSm && 'px-1.5 py-0.5 text-[10px]',
          size === 'md' && 'px-2 py-0.5 text-[11px]'
        )}
      >
        {label}
      </span>
      <span
        className={cn(
          'font-mono font-semibold text-on-surface tracking-wide select-all',
          isXs && 'text-xs',
          isSm && 'text-xs',
          size === 'md' && 'text-sm'
        )}
      >
        {id}
      </span>
      <button
        type="button"
        onClick={handleCopy}
        className={cn(
          'rounded-full inline-flex items-center justify-center transition-all cursor-pointer',
          isXs && 'size-4.5',
          isSm && 'size-5',
          size === 'md' && 'size-6',
          copied
            ? 'bg-primary/15 text-primary scale-105'
            : 'text-on-surface-variant hover:text-on-surface hover:bg-on-surface/8 active:scale-95'
        )}
        title={`Copy ${label}`}
        aria-label={`Copy ${label}`}
      >
        {copied ? (
          <Check className={cn('text-primary transition-transform', isXs ? 'size-2.5' : isSm ? 'size-3' : 'size-3.5')} />
        ) : (
          <Copy className={cn('transition-transform', isXs ? 'size-2.5' : isSm ? 'size-3' : 'size-3.5')} />
        )}
      </button>
    </div>
  )
}
