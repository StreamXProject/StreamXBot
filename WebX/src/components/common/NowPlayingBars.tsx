import React from 'react'
import { cn } from '@/lib/cn'

export const NowPlayingBars: React.FC<{ playing: boolean; className?: string }> = ({ playing, className }) => (
  <div className={cn('flex items-end gap-[2px] h-4 w-4 justify-center', !playing && 'eq-paused', className)} aria-hidden>
    <span className="eq-bar w-[3px] h-full bg-primary rounded-full" />
    <span className="eq-bar w-[3px] h-full bg-primary rounded-full" />
    <span className="eq-bar w-[3px] h-full bg-primary rounded-full" />
  </div>
)
