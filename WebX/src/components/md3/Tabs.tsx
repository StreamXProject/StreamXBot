import React from 'react'
import { cn } from '@/lib/cn'

export interface TabItem<T extends string> {
  value: T
  label: React.ReactNode
  icon?: React.ReactNode
  badge?: React.ReactNode
}

export function Tabs<T extends string>({
  items,
  value,
  onChange,
  className,
  variant = 'primary',
}: {
  items: TabItem<T>[]
  value: T
  onChange: (v: T) => void
  className?: string
  variant?: 'primary' | 'secondary'
}) {
  const activeTabRef = React.useRef<HTMLButtonElement>(null)

  React.useEffect(() => {
    if (activeTabRef.current) {
      activeTabRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'nearest',
        inline: 'center',
      })
    }
  }, [value])

  return (
    <div
      role="tablist"
      className={cn(
        'flex border-b border-outline-variant overflow-x-auto scrollbar-none snap-x snap-mandatory',
        className
      )}
    >
      {items.map((it) => {
        const active = it.value === value
        return (
          <button
            key={it.value}
            ref={active ? activeTabRef : undefined}
            role="tab"
            aria-selected={active}
            onClick={() => onChange(it.value)}
            className={cn(
              'state-layer relative shrink-0 sm:flex-1 snap-start inline-flex items-center justify-center gap-2.5 h-12 px-5 type-title-sm whitespace-nowrap transition-colors [&_svg]:size-5',
              active ? 'text-primary' : 'text-on-surface-variant hover:text-on-surface'
            )}
          >
            {it.icon}
            <span className="whitespace-nowrap">{it.label}</span>
            {it.badge}
            <span
              className={cn(
                'absolute bottom-0 h-[3px] rounded-t-full bg-primary transition-[transform,opacity] duration-200 ease-emphasized',
                variant === 'primary' ? 'w-10' : 'inset-x-0 w-auto rounded-none h-0.5',
                active ? 'scale-x-100 opacity-100' : 'scale-x-0 opacity-0'
              )}
            />
          </button>
        )
      })}
    </div>
  )
}
