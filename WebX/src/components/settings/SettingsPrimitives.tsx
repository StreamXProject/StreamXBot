import React from 'react'
import { cn } from '@/lib/cn'

export const SettingsPage: React.FC<{ title: string; description?: string; children: React.ReactNode; actions?: React.ReactNode }> = ({ title, description, children, actions }) => (
  <div className="page-enter max-w-3xl">
    <div className="flex items-start justify-between gap-4 mb-6">
      <div>
        <h1 className="type-headline-md text-on-surface">{title}</h1>
        {description && <p className="type-body-md text-on-surface-variant mt-1 max-w-prose">{description}</p>}
      </div>
      {actions}
    </div>
    <div className="space-y-8">{children}</div>
  </div>
)

export const SettingsSection: React.FC<{ title?: string; description?: string; children: React.ReactNode; className?: string }> = ({ title, description, children, className }) => (
  <section className={className}>
    {title && (
      <div className="mb-2 px-1">
        <h2 className="type-title-sm text-primary">{title}</h2>
        {description && <p className="type-body-sm text-on-surface-variant mt-0.5">{description}</p>}
      </div>
    )}
    <div className="rounded-lg bg-surface-low overflow-hidden divide-y divide-outline-variant/60">{children}</div>
  </section>
)

export const SettingRow: React.FC<{
  label: React.ReactNode
  description?: React.ReactNode
  control?: React.ReactNode
  icon?: React.ReactNode
  onClick?: () => void
  className?: string
  stacked?: boolean
}> = ({ label, description, control, icon, onClick, className, stacked }) => {
  const Comp = onClick ? 'button' : 'div'
  return (
    <Comp
      onClick={onClick}
      className={cn(
        'w-full flex gap-4 px-4 py-3.5 min-h-14 text-left',
        stacked ? 'flex-col' : 'items-center',
        onClick && 'state-layer',
        className
      )}
    >
      <div className={cn('flex items-center gap-4 min-w-0', stacked ? 'w-full' : 'flex-1')}>
        {icon && <span className="shrink-0 text-on-surface-variant [&_svg]:size-5">{icon}</span>}
        <div className="min-w-0 flex-1">
          <div className="type-body-lg text-on-surface">{label}</div>
          {description && <div className="type-body-sm text-on-surface-variant mt-0.5">{description}</div>}
        </div>
      </div>
      {control && <div className={cn('shrink-0 flex items-center', stacked ? 'w-full' : '')}>{control}</div>}
    </Comp>
  )
}
