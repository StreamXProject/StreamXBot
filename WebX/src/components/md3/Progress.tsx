import React from 'react'
import { cn } from '@/lib/cn'

export const CircularProgress: React.FC<{ size?: number; className?: string; strokeWidth?: number }> = ({
  size = 24,
  className,
  strokeWidth = 3,
}) => (
  <svg
    className={cn('circular-progress shrink-0', className)}
    width={size}
    height={size}
    viewBox="0 0 48 48"
    role="progressbar"
    aria-busy="true"
  >
    <circle cx="24" cy="24" r="20" fill="none" stroke="currentColor" strokeWidth={strokeWidth * 2} />
  </svg>
)

export const LinearProgress: React.FC<{ value?: number; className?: string; indeterminate?: boolean }> = ({
  value = 0,
  className,
  indeterminate,
}) => (
  <div className={cn('h-1 w-full rounded-full bg-surface-highest overflow-hidden', className)} role="progressbar" aria-valuenow={indeterminate ? undefined : value}>
    <div
      className={cn('h-full rounded-full bg-primary', indeterminate && 'animate-pulse w-1/3')}
      style={indeterminate ? undefined : { width: `${Math.max(0, Math.min(100, value))}%` }}
    />
  </div>
)
