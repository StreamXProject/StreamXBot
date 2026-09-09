import React from 'react'
import { cn } from '@/lib/cn'

export const EmptyState: React.FC<{
  icon?: React.ReactNode
  title: string
  description?: React.ReactNode
  action?: React.ReactNode
  className?: string
  compact?: boolean
}> = ({ icon, title, description, action, className, compact }) => (
  <div className={cn('flex flex-col items-center justify-center text-center', compact ? 'py-8 px-4' : 'py-16 px-6', className)}>
    {icon && (
      <div className="size-14 rounded-2xl bg-surface-high text-on-surface-variant flex items-center justify-center mb-4 [&_svg]:size-7">
        {icon}
      </div>
    )}
    <h3 className="type-title-md text-on-surface">{title}</h3>
    {description && <p className="type-body-md text-on-surface-variant mt-1 max-w-sm">{description}</p>}
    {action && <div className="mt-5 flex items-center gap-2">{action}</div>}
  </div>
)
