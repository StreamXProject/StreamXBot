import React from 'react'
import { cn } from '@/lib/cn'
import { CircularProgress } from './Progress'

export type ButtonVariant = 'filled' | 'tonal' | 'outlined' | 'text' | 'elevated'
export type ButtonSize = 'sm' | 'md' | 'lg'

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: React.ReactNode
  trailingIcon?: React.ReactNode
  loading?: boolean
  fullWidth?: boolean
}

const VARIANT: Record<ButtonVariant, string> = {
  filled: 'bg-primary text-on-primary hover:shadow-md3-1',
  tonal: 'bg-secondary-container text-on-secondary-container',
  outlined: 'border border-outline text-primary',
  text: 'text-primary',
  elevated: 'elev-1 text-primary shadow-md3-1 hover:shadow-md3-2',
}

const SIZE: Record<ButtonSize, string> = {
  sm: 'h-8 px-3 text-[12px] gap-1.5 [&_svg]:size-4',
  md: 'h-10 px-5 text-[14px] gap-2 [&_svg]:size-[18px]',
  lg: 'h-12 px-6 text-[15px] gap-2.5 [&_svg]:size-5',
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'filled', size = 'md', icon, trailingIcon, loading, fullWidth, className, children, disabled, ...rest }, ref) => (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={cn(
        'state-layer inline-flex items-center justify-center rounded-full font-semibold tracking-[0.1px] select-none whitespace-nowrap',
        'transition-[box-shadow,transform,background-color] duration-150 ease-standard active:scale-[0.98]',
        'disabled:opacity-40 disabled:pointer-events-none',
        VARIANT[variant],
        SIZE[size],
        icon && !children && 'px-0 aspect-square',
        fullWidth && 'w-full',
        className
      )}
      {...rest}
    >
      {loading ? <CircularProgress size={18} /> : icon}
      {children && <span className="truncate">{children}</span>}
      {trailingIcon}
    </button>
  )
)
Button.displayName = 'Button'
