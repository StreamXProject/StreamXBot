import React from 'react'
import { cn } from '@/lib/cn'

export interface ListItemProps extends React.HTMLAttributes<HTMLDivElement> {
  headline: React.ReactNode
  supporting?: React.ReactNode
  leading?: React.ReactNode
  trailing?: React.ReactNode
  selected?: boolean
  disabled?: boolean
  as?: 'div' | 'button' | 'a'
}

export const ListItem: React.FC<ListItemProps> = ({
  headline,
  supporting,
  leading,
  trailing,
  selected,
  disabled,
  className,
  onClick,
  ...rest
}) => (
  <div
    role={onClick ? 'button' : undefined}
    tabIndex={onClick && !disabled ? 0 : undefined}
    onClick={disabled ? undefined : onClick}
    onKeyDown={(e) => {
      if (onClick && !disabled && (e.key === 'Enter' || e.key === ' ')) {
        e.preventDefault()
        onClick(e as unknown as React.MouseEvent<HTMLDivElement>)
      }
    }}
    aria-disabled={disabled}
    className={cn(
      'flex items-center gap-4 min-h-14 px-4 py-2 rounded-lg text-left',
      onClick && !disabled && 'state-layer cursor-pointer',
      selected && 'bg-secondary-container text-on-secondary-container',
      disabled && 'opacity-40',
      className
    )}
    {...rest}
  >
    {leading && <div className="shrink-0 flex items-center justify-center text-on-surface-variant [&_svg]:size-6">{leading}</div>}
    <div className="flex-1 min-w-0">
      <div className="type-body-lg text-on-surface truncate">{headline}</div>
      {supporting && <div className="type-body-md text-on-surface-variant truncate mt-0.5">{supporting}</div>}
    </div>
    {trailing && <div className="shrink-0 flex items-center gap-2 text-on-surface-variant">{trailing}</div>}
  </div>
)
