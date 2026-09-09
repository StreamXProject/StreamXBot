import React from 'react'
import { Music2 } from 'lucide-react'
import { cn } from '@/lib/cn'

export const AuthCard: React.FC<{ title: string; subtitle?: React.ReactNode; children: React.ReactNode; footer?: React.ReactNode; className?: string }> = ({ title, subtitle, children, footer, className }) => (
  <div className={cn('w-full max-w-[420px] page-enter', className)}>
    <div className="flex items-center gap-3 mb-8">
      <span className="size-11 rounded-md bg-primary text-on-primary flex items-center justify-center shadow-md3-1"><Music2 className="size-5" strokeWidth={2.5} /></span>
      <div>
        <p className="type-title-lg text-on-surface leading-none">WebX</p>
        <p className="type-body-sm text-on-surface-variant mt-1">Your music, your server.</p>
      </div>
    </div>
    <div className="rounded-2xl bg-surface-low p-6 sm:p-8">
      <h1 className="type-headline-sm text-on-surface">{title}</h1>
      {subtitle && <p className="type-body-md text-on-surface-variant mt-1.5">{subtitle}</p>}
      <div className="mt-6">{children}</div>
    </div>
    {footer && <div className="mt-5 text-center type-body-sm text-on-surface-variant">{footer}</div>}
  </div>
)

export const ErrorBanner: React.FC<{ message: string | null }> = ({ message }) =>
  message ? (
    <div role="alert" className="mb-4 flex items-start gap-2.5 p-3 rounded-sm bg-error-container text-on-error-container type-body-sm">
      <span className="mt-0.5">⚠</span>
      <span>{message}</span>
    </div>
  ) : null
