import React from 'react'
import { cn } from '@/lib/cn'

export const Divider: React.FC<{ className?: string; inset?: boolean; vertical?: boolean }> = ({ className, inset, vertical }) => (
  <div
    role="separator"
    className={cn(
      'bg-outline-variant shrink-0',
      vertical ? 'w-px self-stretch' : 'h-px w-full',
      inset && !vertical && 'ml-4 w-[calc(100%-1rem)]',
      className
    )}
  />
)
