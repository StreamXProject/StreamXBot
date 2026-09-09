import React from 'react'
import { cn } from '@/lib/cn'

export type IconButtonVariant = 'standard' | 'filled' | 'tonal' | 'outlined'
export type IconButtonSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

export interface IconButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: IconButtonVariant
  size?: IconButtonSize
  /** Toggle state; when defined the button is a toggle */
  selected?: boolean
  label: string
}

const SIZE: Record<IconButtonSize, string> = {
  xs: 'size-7 [&_svg]:size-3.5',
  sm: 'size-8 [&_svg]:size-4',
  md: 'size-10 [&_svg]:size-5',
  lg: 'size-12 [&_svg]:size-6',
  xl: 'size-16 [&_svg]:size-8',
}

function variantClasses(v: IconButtonVariant, selected: boolean | undefined) {
  switch (v) {
    case 'filled':
      return selected === false
        ? 'bg-surface-highest text-primary'
        : 'bg-primary text-on-primary'
    case 'tonal':
      return selected === false
        ? 'bg-surface-highest text-on-surface-variant'
        : 'bg-secondary-container text-on-secondary-container'
    case 'outlined':
      return selected
        ? 'bg-inverse-surface text-inverse-on-surface'
        : 'border border-outline text-on-surface-variant'
    default:
      return selected ? 'text-primary' : 'text-on-surface-variant hover:text-on-surface'
  }
}

export const IconButton = React.forwardRef<HTMLButtonElement, IconButtonProps>(
  ({ variant = 'standard', size = 'md', selected, label, className, children, ...rest }, ref) => (
    <button
      ref={ref}
      type="button"
      aria-label={label}
      title={rest.title ?? label}
      aria-pressed={selected}
      data-selected={selected}
      className={cn(
        'state-layer inline-flex items-center justify-center rounded-full shrink-0 select-none',
        'transition-[background-color,color,transform] duration-150 ease-standard active:scale-95',
        'disabled:opacity-40 disabled:pointer-events-none',
        SIZE[size],
        variantClasses(variant, selected),
        className
      )}
      {...rest}
    >
      {children}
    </button>
  )
)
IconButton.displayName = 'IconButton'
