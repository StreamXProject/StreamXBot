import React from 'react'
import { cn } from '@/lib/cn'
import { isLossless } from '@/lib/format'

export const QualityBadge: React.FC<{ type?: string | null; hz?: number | null; className?: string; verbose?: boolean }> = ({ type, hz, className, verbose }) => {
  if (!type) return null
  const t = type.toLowerCase()
  const lossless = isLossless(t)
  const label = t.includes('flac') ? 'FLAC' : t.includes('alac') ? 'ALAC' : t.includes('mp3') ? 'MP3' : t.includes('aac') || t.includes('m4a') ? 'AAC' : t.includes('opus') ? 'OPUS' : t.includes('wav') ? 'WAV' : t.toUpperCase().slice(0, 4)
  const hiRes = lossless && hz && hz > 48000
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 h-[18px] px-1.5 rounded-xs text-[10px] font-bold tracking-wide tabular',
        lossless ? 'bg-tertiary-container text-on-tertiary-container' : 'bg-surface-highest text-on-surface-variant',
        className
      )}
      title={hz ? `${label} · ${(hz / 1000).toFixed(1)} kHz` : label}
    >
      {hiRes ? 'HI-RES' : label}
      {verbose && hz ? <span className="opacity-70 font-medium">{(hz / 1000).toFixed(1)}k</span> : null}
    </span>
  )
}
