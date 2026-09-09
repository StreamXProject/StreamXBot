import React from 'react'
import { Check } from 'lucide-react'
import { cn } from '@/lib/cn'

export interface SwitchProps {
  checked: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
  label?: string
  className?: string
}

export const Switch: React.FC<SwitchProps> = ({ checked, onChange, disabled, label, className }) => (
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    aria-label={label}
    disabled={disabled}
    onClick={() => onChange(!checked)}
    className={cn(
      'relative inline-flex h-8 w-[52px] shrink-0 items-center rounded-full border-2 transition-colors duration-200 ease-standard',
      checked ? 'bg-primary border-primary' : 'bg-surface-highest border-outline',
      disabled && 'opacity-40 pointer-events-none',
      className
    )}
  >
    <span
      className={cn(
        'absolute flex items-center justify-center rounded-full transition-[transform,width,height,background-color] duration-200 ease-emphasized',
        checked
          ? 'size-6 translate-x-[22px] bg-on-primary text-primary'
          : 'size-4 translate-x-[6px] bg-outline'
      )}
    >
      {checked && <Check className="size-4" strokeWidth={3} />}
    </span>
  </button>
)
