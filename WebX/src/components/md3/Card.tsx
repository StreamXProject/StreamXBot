import React from 'react'
import { cn } from '@/lib/cn'

export type CardVariant = 'filled' | 'outlined' | 'elevated'

export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: CardVariant
  interactive?: boolean
}

const VARIANT: Record<CardVariant, string> = {
  filled: 'bg-surface-low',
  outlined: 'bg-surface border border-outline-variant',
  elevated: 'elev-1 shadow-md3-1',
}

export const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ variant = 'filled', interactive, className, ...rest }, ref) => (
    <div
      ref={ref}
      className={cn(
        'rounded-md text-on-surface',
        VARIANT[variant],
        interactive && 'state-layer cursor-pointer transition-[box-shadow,transform] duration-200 ease-standard hover:shadow-md3-1',
        className
      )}
      {...rest}
    />
  )
)
Card.displayName = 'Card'
