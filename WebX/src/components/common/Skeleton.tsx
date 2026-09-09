import React from 'react'
import { cn } from '@/lib/cn'

export const Skeleton: React.FC<React.HTMLAttributes<HTMLDivElement> & { variant?: 'rect' | 'circle' | 'text' }> = ({ className, variant = 'rect', ...props }) => (
  <div
    aria-hidden
    className={cn('skeleton', variant === 'circle' ? 'rounded-full' : variant === 'text' ? 'rounded-xs h-3.5' : 'rounded-md', className)}
    {...props}
  />
)

export const TrackRowSkeleton: React.FC<{ count?: number }> = ({ count = 6 }) => (
  <div className="space-y-1">
    {Array.from({ length: count }).map((_, i) => (
      <div key={i} className="flex items-center gap-3 px-3 h-14">
        <Skeleton className="w-6 h-3" />
        <Skeleton className="size-10 rounded-sm" />
        <div className="flex-1 space-y-2">
          <Skeleton variant="text" className="w-1/2" />
          <Skeleton variant="text" className="w-1/3 h-2.5" />
        </div>
        <Skeleton className="w-10 h-3" />
      </div>
    ))}
  </div>
)

export const CardGridSkeleton: React.FC<{ count?: number; circle?: boolean }> = ({ count = 6, circle }) => (
  <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
    {Array.from({ length: count }).map((_, i) => (
      <div key={i} className="space-y-2">
        <Skeleton className={cn('aspect-square', circle && 'rounded-full')} />
        <Skeleton variant="text" className="w-3/4" />
        <Skeleton variant="text" className="w-1/2 h-2.5" />
      </div>
    ))}
  </div>
)
