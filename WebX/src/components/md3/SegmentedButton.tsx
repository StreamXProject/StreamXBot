import React from 'react'
import { Check } from 'lucide-react'
import { cn } from '@/lib/cn'

export interface SegmentOption<T extends string> {
  value: T
  label: React.ReactNode
  icon?: React.ReactNode
}

export interface SegmentedButtonProps<T extends string> {
  options: SegmentOption<T>[]
  value: T
  onChange: (v: T) => void
  className?: string
  size?: 'sm' | 'md'
  showCheck?: boolean
}

export function SegmentedButton<T extends string>({ options, value, onChange, className, size = 'md', showCheck = true }: SegmentedButtonProps<T>) {
  return (
    <div role="radiogroup" className={cn('inline-flex rounded-full border border-outline overflow-hidden', className)}>
      {options.map((o, i) => {
        const active = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.value)}
            className={cn(
              'state-layer flex-1 inline-flex items-center justify-center gap-1.5 font-semibold whitespace-nowrap transition-colors',
              size === 'sm' ? 'h-8 px-3 text-[12px] [&_svg]:size-4' : 'h-10 px-4 text-[14px] [&_svg]:size-[18px]',
              i > 0 && 'border-l border-outline',
              active ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface'
            )}
          >
            {active && showCheck ? <Check /> : o.icon}
            <span>{o.label}</span>
          </button>
        )
      })}
    </div>
  )
}
