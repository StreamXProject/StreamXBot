import React, { useRef, useState, useEffect } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { SectionHeader } from './SectionHeader'
import { cn } from '@/lib/cn'

/** Horizontal, snap-scrolling shelf with edge buttons on pointer devices */
export const Shelf: React.FC<{
  title: React.ReactNode
  subtitle?: React.ReactNode
  onMore?: () => void
  action?: React.ReactNode
  children: React.ReactNode
  className?: string
  itemWidth?: string
}> = ({ title, subtitle, onMore, action, children, className, itemWidth = 'w-[46vw] max-w-[220px] sm:w-44' }) => {
  const ref = useRef<HTMLDivElement>(null)
  const [canL, setCanL] = useState(false)
  const [canR, setCanR] = useState(false)

  const update = () => {
    const el = ref.current
    if (!el) return
    setCanL(el.scrollLeft > 4)
    setCanR(el.scrollLeft + el.clientWidth < el.scrollWidth - 4)
  }
  useEffect(() => {
    update()
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => ro.disconnect()
  }, [children])

  const scrollBy = (dir: 1 | -1) => ref.current?.scrollBy({ left: dir * ref.current.clientWidth * 0.8, behavior: 'smooth' })

  return (
    <section className={cn('relative', className)}>
      <SectionHeader
        title={title}
        subtitle={subtitle}
        onMore={onMore}
        action={
          <>
            {action}
            <div className="hidden md:flex items-center gap-1">
              <button onClick={() => scrollBy(-1)} disabled={!canL} aria-label="Scroll left" className="state-layer size-8 rounded-full inline-flex items-center justify-center text-on-surface-variant disabled:opacity-30"><ChevronLeft className="size-5" /></button>
              <button onClick={() => scrollBy(1)} disabled={!canR} aria-label="Scroll right" className="state-layer size-8 rounded-full inline-flex items-center justify-center text-on-surface-variant disabled:opacity-30"><ChevronRight className="size-5" /></button>
            </div>
          </>
        }
      />
      <div ref={ref} onScroll={update} className="flex gap-4 overflow-x-auto scrollbar-none snap-x snap-mandatory -mx-4 px-4 md:-mx-2 md:px-2 pb-2 pt-2">
        {React.Children.map(children, (c) => (
          <div className={cn('shrink-0 snap-start', itemWidth)}>{c}</div>
        ))}
      </div>
    </section>
  )
}
