import React from 'react'
import { Check, MoreVertical } from 'lucide-react'
import type { ThemeDefinition } from '@/theme/tokens'
import { previewColors } from '@/theme/themeStore'
import { cn } from '@/lib/cn'

export const ThemeCard: React.FC<{
  theme: ThemeDefinition
  mode: 'light' | 'dark'
  active: boolean
  onSelect: () => void
  onMenu?: (anchor: HTMLElement) => void
}> = ({ theme, mode, active, onSelect, onMenu }) => {
  const c = previewColors(theme, mode)
  const radius = `${Math.round(12 * theme.shape.scale)}px`
  return (
    <div
      role="radio"
      aria-checked={active}
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onSelect()}
      className={cn(
        'group relative flex flex-col rounded-lg overflow-hidden cursor-pointer outline-none transition-[box-shadow,transform] duration-200 ease-emphasized',
        'ring-2 ring-offset-2 ring-offset-surface',
        active ? 'ring-primary' : 'ring-transparent hover:ring-outline-variant focus-visible:ring-primary/60'
      )}
    >
      {/* Mini app preview rendered in the theme's own colors */}
      <div className="relative aspect-[4/3] p-3 flex flex-col gap-2" style={{ background: c.surface, fontFamily: theme.typography.body }}>
        <div className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-full" style={{ background: c.primary }} />
          <span className="h-1.5 w-12 rounded-full" style={{ background: c.onSurface, opacity: 0.85 }} />
          <span className="ml-auto h-4 w-10 rounded-full" style={{ background: c.surfaceContainerHigh }} />
        </div>
        <div className="flex gap-2 flex-1 min-h-0">
          <div className="w-1/3 flex flex-col gap-1.5 p-1.5" style={{ background: c.surfaceContainerLow, borderRadius: radius }}>
            <span className="h-1.5 w-full rounded-full" style={{ background: c.secondaryContainer }} />
            <span className="h-1.5 w-3/4 rounded-full" style={{ background: c.outlineVariant }} />
            <span className="h-1.5 w-5/6 rounded-full" style={{ background: c.outlineVariant }} />
          </div>
          <div className="flex-1 flex flex-col gap-1.5">
            <div className="flex gap-1.5">
              <span className="size-8 shrink-0" style={{ background: c.primaryContainer, borderRadius: radius }} />
              <span className="size-8 shrink-0" style={{ background: c.tertiaryContainer, borderRadius: radius }} />
              <span className="size-8 shrink-0" style={{ background: c.surfaceContainerHighest, borderRadius: radius }} />
            </div>
            <span className="h-1.5 w-2/3 rounded-full" style={{ background: c.onSurface, opacity: 0.8 }} />
            <span className="h-1.5 w-1/2 rounded-full" style={{ background: c.onSurfaceVariant, opacity: 0.6 }} />
          </div>
        </div>
        <div className="flex items-center gap-2 p-1.5 mt-auto" style={{ background: c.surfaceContainer, borderRadius: radius }}>
          <span className="size-4 rounded-sm" style={{ background: c.secondaryContainer }} />
          <span className="h-1 flex-1 rounded-full" style={{ background: c.outlineVariant }}>
            <span className="block h-full w-1/3 rounded-full" style={{ background: c.primary }} />
          </span>
          <span className="size-5 rounded-full" style={{ background: c.primary }} />
        </div>
        {active && (
          <span className="absolute top-2 right-2 size-6 rounded-full flex items-center justify-center shadow-md3-1" style={{ background: c.primary, color: c.onPrimary }}>
            <Check className="size-4" strokeWidth={3} />
          </span>
        )}
      </div>
      <div className="flex items-center gap-2 px-3 py-2.5 bg-surface-low">
        <div className="min-w-0 flex-1">
          <p className="type-title-sm text-on-surface truncate">{theme.name}</p>
          <p className="type-body-sm text-on-surface-variant truncate">{theme.description || (theme.builtIn ? 'Built-in' : 'Custom theme')}</p>
        </div>
        <span className="flex gap-0.5" aria-hidden>
          {[c.primary, c.secondary, c.tertiary].map((h, i) => <span key={i} className="size-3 rounded-full ring-1 ring-outline-variant" style={{ background: h }} />)}
        </span>
        {onMenu && (
          <button
            onClick={(e) => {
              e.stopPropagation()
              onMenu(e.currentTarget)
            }}
            aria-label="Theme options"
            className="state-layer size-8 -mr-1 rounded-full inline-flex items-center justify-center text-on-surface-variant"
          >
            <MoreVertical className="size-4" />
          </button>
        )}
      </div>
    </div>
  )
}
