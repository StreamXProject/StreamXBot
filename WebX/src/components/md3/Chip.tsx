import React from 'react'
import { Check, X } from 'lucide-react'
import { cn } from '@/lib/cn'

export interface ChipProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  label: React.ReactNode
  icon?: React.ReactNode
  selected?: boolean
  variant?: 'assist' | 'filter' | 'input' | 'suggestion'
  onRemove?: () => void
  elevated?: boolean
}

export const Chip: React.FC<ChipProps> = ({ label, icon, selected, variant = 'assist', onRemove, elevated, className, ...rest }) => (
  <button
    type="button"
    aria-pressed={variant === 'filter' ? selected : undefined}
    className={cn(
      'state-layer inline-flex items-center gap-2 h-8 px-3 rounded-sm text-[14px] font-medium whitespace-nowrap transition-colors select-none [&_svg]:size-[18px]',
      selected ? 'bg-secondary-container text-on-secondary-container' : elevated ? 'elev-1 shadow-md3-1 text-on-surface-variant' : 'border border-outline-variant text-on-surface-variant',
      className
    )}
    {...rest}
  >
    {selected && variant === 'filter' ? <Check /> : icon}
    <span>{label}</span>
    {onRemove && (
      <span
        role="button"
        aria-label="Remove"
        onClick={(e) => {
          e.stopPropagation()
          onRemove()
        }}
        className="-mr-1 rounded-full p-0.5 hover:bg-on-surface/10"
      >
        <X className="size-4" />
      </span>
    )}
  </button>
)
