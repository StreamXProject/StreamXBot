import React from 'react'
import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/cn'

export const SectionHeader: React.FC<{
  title: React.ReactNode
  subtitle?: React.ReactNode
  action?: React.ReactNode
  onMore?: () => void
  className?: string
}> = ({ title, subtitle, action, onMore, className }) => (
  <div className={cn('flex items-end justify-between gap-4 mb-3', className)}>
    <div className="min-w-0">
      <h2 className="type-title-lg text-on-surface truncate">{title}</h2>
      {subtitle && <p className="type-body-sm text-on-surface-variant mt-0.5">{subtitle}</p>}
    </div>
    <div className="flex items-center gap-2 shrink-0">
      {action}
      {onMore && (
        <button onClick={onMore} className="state-layer inline-flex items-center gap-0.5 h-8 pl-3 pr-2 rounded-full type-label-lg text-primary">
          More <ChevronRight className="size-4" />
        </button>
      )}
    </div>
  </div>
)
